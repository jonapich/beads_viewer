<script lang="ts">
  import { onMount } from "svelte";
  import { getPlanIfChanged } from "../lib/api/client";
  import type { PlanResponse } from "../lib/api/types";
  import { Resource, ViewFeed } from "../lib/feeds/view-feed.svelte";
  import { session } from "../lib/session.svelte";
  import { foldSingles, itemById, planView, showBanner, startId, summaryLine } from "../lib/work/plan";
  import { PeekHost } from "../lib/work/peek-host.svelte";
  import { plural } from "../lib/work/present";
  import { rowClicks } from "../lib/work/row-click";
  import Notice from "../components/work/Notice.svelte";
  import SkeletonRows from "../components/work/SkeletonRows.svelte";
  import WorkPeek from "../components/work/WorkPeek.svelte";
  import PlanBanner from "../components/work/plan/PlanBanner.svelte";
  import TrackCard from "../components/work/plan/TrackCard.svelte";

  const plan = new Resource<PlanResponse>("/api/v1/plan", getPlanIfChanged);
  const feed = new ViewFeed([plan]);
  const host = new PeekHost({
    writable: () => feed.writes,
    refresh: () => feed.refresh(),
    lookup: (id) => {
      const it = plan.data ? itemById(plan.data, id) : undefined;
      return it ? { id, priority: it.priority, status: it.status, labels: [] } : undefined;
    },
  });
  onMount(() => feed.start());

  const view = $derived(planView(plan, feed.meta?.issue_count ?? null));
  const p = $derived(plan.data);
  // Relative dates are computed once per data refresh, not per row render.
  const now = $derived.by(() => {
    void plan.data;
    return Date.now();
  });

  const summary = $derived(p ? summaryLine(p) : null);
  const start = $derived(p ? startId(p) : "");
  const startItem = $derived(p && start ? itemById(p, start) : undefined);
  const folds = $derived(p ? foldSingles(p.tracks) : { shown: [], folded: [] });
  let foldOpen = $state(false);

  // The drawer refetches when the snapshot changes (/meta moves with every view), not only when this
  // view's data does. Same shape in Triage, Plan and Insights.
  const reloadKey = $derived([feed.meta, plan.data] as const);

  const openPeek = (id: string): void => host.open(id);
</script>

<section class="view" class:offline={session.status === "offline"} aria-label="Plan">
  <div class="scroll" data-work-scroll tabindex="-1" use:rowClicks={openPeek}>
    <h1>Plan</h1>

    {#if view.state === "failed"}
      <Notice tone="danger" text="Can't load the plan: {view.message}" action={{ label: "Retry", run: () => feed.reload() }} />
    {:else if view.state === "loading"}
      <div class="summary"></div>
      <div class="tracks">
        <div class="card"><SkeletonRows n={4} label="Loading the plan…" /></div>
        <div class="card"><SkeletonRows n={3} label="" /></div>
      </div>
    {:else if p && summary}
      <div class="summary" aria-live="polite">
        {#each summary.parts as part (part.text)}
          <span><b>{part.n}</b> {part.text}</span>
        {/each}
        {#if summary.inProgress > 0}
          <span class="chip" title="Also counted above as actionable or blocked">{summary.inProgress} in progress</span>
        {/if}
        {#if view.stale}
          <span class="chip" title={view.staleTitle}>Showing the last loaded plan</span>
        {/if}
      </div>

      {#if host.missing}
        <Notice tone="warning" text={host.missing} action={{ label: "Dismiss", run: () => (host.missing = null) }} />
      {/if}

      {#if view.state === "empty"}
        <div class="empty">
          <h3>Nothing to plan</h3>
          <p>This tracker has no open issues.</p>
          <div class="acts"><code>bd create "…"</code></div>
        </div>
      {:else if view.state === "none"}
        <div class="empty">
          <p>No actionable items. All tasks are either blocked or completed.</p>
          <div class="acts"><a class="btn" href="/triage">Open Triage: blockers to clear</a></div>
        </div>
      {:else}
        {#if showBanner(p) && startItem}
          <PlanBanner id={startItem.id} title={startItem.title} unblocks={p.summary.unblocks_count} onpeek={openPeek} />
        {/if}
        <div class="tracks">
          {#each folds.shown as t (t.track_id)}
            <TrackCard track={t} startId={start} {now} writes={feed.writes} {host} />
          {/each}
          {#if folds.folded.length > 0}
            <button type="button" class="btn fold" aria-expanded={foldOpen} onclick={() => (foldOpen = !foldOpen)}>
              {foldOpen ? "▾" : "▸"} {plural(folds.folded.length, "more single-item track", "more single-item tracks")}
            </button>
            {#if foldOpen}
              {#each folds.folded as t (t.track_id)}
                <TrackCard track={t} startId={start} {now} writes={feed.writes} {host} />
              {/each}
            {/if}
          {/if}
        </div>
      {/if}
    {/if}
  </div>
  <WorkPeek {host} writes={feed.writes} {now} {reloadKey} />
</section>

<style>
  .view {
    --row-h: 32px;
    flex: 1;
    min-height: 0;
    display: flex;
  }
  .view.offline .scroll {
    opacity: 0.6;
  }
  .scroll {
    flex: 1;
    min-width: 0;
    overflow: auto;
    container: view / inline-size;
    padding-bottom: var(--space-5);
  }
  .scroll:focus {
    outline: none;
  }
  h1 {
    margin: 0;
  }
  .summary {
    font-size: var(--text-sm);
    color: var(--text-muted);
    margin: 4px 0 8px;
    min-height: 18px;
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
    align-items: center;
  }
  .summary b {
    font-weight: 600;
    color: var(--text);
  }
  .chip {
    padding: 0 8px;
    line-height: 18px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--bg-subtle);
  }
  .tracks {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    min-width: 0;
  }
  .btn {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    display: inline-flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
    cursor: pointer;
    color: var(--text);
    text-decoration: none;
  }
  .btn:hover {
    background: var(--bg-hover);
  }
  .fold {
    align-self: flex-start;
  }
  .empty {
    padding: 40px 16px;
    text-align: center;
    color: var(--text-muted);
  }
  .empty h3 {
    margin: 0 0 4px;
    font-size: var(--text-lg);
    color: var(--text);
    font-weight: 600;
  }
  .empty p {
    margin: 4px 0;
  }
  .acts {
    display: flex;
    gap: 8px;
    justify-content: center;
    margin-top: 12px;
  }
  .acts code {
    font-family: var(--font-mono);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 2px 8px;
  }
</style>
