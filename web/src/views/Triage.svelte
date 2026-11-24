<script lang="ts">
  import { onMount } from "svelte";
  import { getTriageIfChanged } from "../lib/api/client";
  import type { TriageResponse } from "../lib/api/types";
  import { Resource, ViewFeed } from "../lib/feeds/view-feed.svelte";
  import { session } from "../lib/session.svelte";
  import { PeekHost } from "../lib/work/peek-host.svelte";
  import { rowClicks } from "../lib/work/row-click";
  import {
    alertsOf,
    blockerRows,
    filterRecs,
    joinPicks,
    REC_FILTERS,
    summaryParts,
    triageView,
    winRows,
    type RecFilter,
  } from "../lib/work/triage";
  import Notice from "../components/work/Notice.svelte";
  import SkeletonRows from "../components/work/SkeletonRows.svelte";
  import WorkPeek from "../components/work/WorkPeek.svelte";
  import AlertList from "../components/work/triage/AlertList.svelte";
  import PickCard from "../components/work/triage/PickCard.svelte";
  import RailList from "../components/work/triage/RailList.svelte";
  import RecRow from "../components/work/triage/RecRow.svelte";

  const triage = new Resource<TriageResponse>("/api/v1/triage", getTriageIfChanged);
  const feed = new ViewFeed([triage]);
  const host = new PeekHost({
    writable: () => feed.writes,
    refresh: () => feed.refresh(),
    lookup: (id) => {
      const r = triage.data?.recommendations.find((x) => x.id === id);
      return r ? { id, priority: r.priority, status: r.status, labels: r.labels } : undefined;
    },
  });
  onMount(() => feed.start());

  const view = $derived(triageView(triage));
  const t = $derived(triage.data);
  // Relative dates are computed once per data refresh, not per row render.
  const now = $derived.by(() => {
    void triage.data;
    return Date.now();
  });

  let filter = $state<RecFilter>("all");
  let expanded = $state.raw<ReadonlySet<string>>(new Set());
  function toggle(id: string): void {
    const next = new Set(expanded);
    if (!next.delete(id)) next.add(id);
    expanded = next;
  }

  const picks = $derived(t ? joinPicks(t) : []);
  const recs = $derived(t ? filterRecs(t.recommendations, filter) : []);
  const alerts = $derived(t ? alertsOf(t) : []);
  const parts = $derived(t ? summaryParts(t.quick_ref, t.issue_count) : []);
  const blockers = $derived(t ? blockerRows(t.blockers_to_clear) : []);
  const wins = $derived(t ? winRows(t.quick_wins) : []);
  const titles = $derived.by(() => {
    const m = new Map<string, string>();
    for (const r of [...blockers, ...wins]) m.set(r.id, r.title);
    for (const r of t?.recommendations ?? []) m.set(r.id, r.title);
    return m;
  });

  let rail = $state<HTMLElement>();
  const openPeek = (id: string): void => host.open(id);
  const prio = (id: string, el: HTMLElement): void => host.togglePop(id, el, "row");
</script>

<section class="view" class:offline={session.status === "offline"} aria-label="Triage">
  <div class="scroll" data-work-scroll tabindex="-1" use:rowClicks={openPeek}>
    <h1>Triage</h1>

    {#if view.state === "failed"}
      <Notice tone="danger" text="Can't load triage: {view.message}" action={{ label: "Retry", run: () => feed.reload() }} />
    {:else}
      <div class="summary" aria-live="polite">
        {#if view.state === "loading"}
          {#if view.computing}<span class="spin" aria-hidden="true"></span><span>Computing triage…</span>{/if}
        {:else}
          {#each parts as p (p.label)}
            {#if p.href}
              <a href={p.href}><b>{p.n}</b> {p.label}</a>
            {:else}
              <span><b>{p.n}</b> {p.label}</span>
            {/if}
          {/each}
          {#if view.provisional}
            <span title="Graph analysis (phase 2) has not finished">provisional · computing graph metrics… <span class="spin" aria-hidden="true"></span></span>
          {/if}
          {#if view.recomputing}
            <span class="chip" title={view.recomputingTitle || "Recomputing after a change"}><span class="spin" aria-hidden="true"></span>Recomputing…</span>
          {/if}
        {/if}
      </div>

      {#if host.missing}
        <Notice tone="warning" text={host.missing} action={{ label: "Dismiss", run: () => (host.missing = null) }} />
      {/if}
      {#if view.phase2Error}
        <Notice tone="warning" text="Graph analysis failed: {view.phase2Error}. Showing phase-1 results." />
      {/if}

      {#if view.state === "empty"}
        <div class="empty">
          {#if view.noIssues}
            <h3>Nothing to triage</h3>
            <p>This tracker has no open issues.</p>
            <div class="acts"><code>bd create "…"</code></div>
          {:else}
            <h3>Nothing to triage: no open work.</h3>
          {/if}
        </div>
      {:else}
        {#if t && alerts.length > 0}
          <AlertList {alerts} titleOf={(id) => titles.get(id)} onpeek={openPeek} />
        {/if}

        <div class="tgrid">
          <div class="main-col">
            <h2 class="sh">Start here</h2>
            {#if view.state === "loading"}
              <div class="picks">
                {#each [0, 1, 2] as n (n)}<div class="card skc"><SkeletonRows n={3} label="Loading picks…" /></div>{/each}
              </div>
            {:else if view.allBlocked}
              <div class="dashed" role="status">
                No claimable work: every open issue is blocked, deferred or claimed.
                <button type="button" class="link" onclick={() => rail?.scrollIntoView({ block: "nearest" })}>See Blockers to clear</button>
              </div>
            {:else}
              <div class="picks">
                {#each picks as p (p.pick.id)}
                  <PickCard
                    view={p}
                    writes={feed.writes}
                    busy={host.actions.isBusy(p.pick.id)}
                    prioOpen={host.rowPopOpen(p.pick.id)}
                    selected={host.peek === p.pick.id}
                    onpeek={openPeek}
                    onprio={prio}
                  />
                {/each}
              </div>
            {/if}

            <h2 class="sh">
              Recommendations
              {#if t}<span class="cnt">({filter === "all" ? t.recommendations.length : `${recs.length} of ${t.recommendations.length}`})</span>{/if}
            </h2>
            {#if t}<p class="cap">Top {t.recommendations.length} by triage score</p>{/if}
            <div class="seg" role="group" aria-label="Filter recommendations">
              {#each REC_FILTERS as f (f.key)}
                <button type="button" aria-pressed={filter === f.key} disabled={view.state === "loading"} onclick={() => (filter = f.key)}>{f.label}</button>
              {/each}
            </div>
            <div class="list" role="list" aria-label="Recommendations">
              {#if view.state === "loading"}
                <SkeletonRows n={8} label="Loading recommendations…" />
              {:else if recs.length === 0}
                <div class="empty compact"><p>No recommendations match this filter.</p></div>
              {:else}
                {#each recs as r (r.rec.id)}
                  <RecRow
                    ranked={r}
                    {now}
                    writes={feed.writes}
                    busy={host.actions.isBusy(r.rec.id)}
                    prioOpen={host.rowPopOpen(r.rec.id)}
                    selected={host.peek === r.rec.id}
                    open={expanded.has(r.rec.id)}
                    ontoggle={toggle}
                    onpeek={openPeek}
                    onprio={prio}
                  />
                {/each}
              {/if}
            </div>
          </div>

          <aside class="rail" bind:this={rail} aria-label="Blockers and quick wins">
            <RailList
              title="Blockers to clear"
              sub="Open issues that block the most other work"
              rows={blockers}
              loading={view.state === "loading"}
              selected={host.peek}
              onpeek={openPeek}
            />
            <RailList
              title="Quick wins"
              sub="Low-effort items worth picking off"
              rows={wins}
              loading={view.state === "loading"}
              selected={host.peek}
              onpeek={openPeek}
            />
          </aside>
        </div>
      {/if}
    {/if}
  </div>
  <WorkPeek {host} writes={feed.writes} {now} reloadKey={triage.data} />
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
  .summary a {
    color: var(--accent);
    text-decoration: none;
  }
  .summary a b {
    color: inherit;
  }
  .summary a:hover {
    text-decoration: underline;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0 8px;
    line-height: 18px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--bg-subtle);
  }
  .spin {
    width: 10px;
    height: 10px;
    border: 2px solid var(--border-strong);
    border-top-color: var(--accent);
    border-radius: 50%;
    display: inline-block;
    animation: spin 0.9s linear infinite;
    flex: none;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin {
      animation: none;
    }
  }
  .tgrid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 280px;
    gap: 20px;
    align-items: start;
  }
  @container view (max-width: 899px) {
    .tgrid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .main-col {
    min-width: 0;
  }
  h2.sh {
    margin: 16px 0 8px;
    font-size: var(--text-lg);
    font-weight: 600;
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }
  .cnt {
    font-size: var(--text-xs);
    color: var(--text-faint);
    font-weight: 400;
  }
  .cap {
    margin: -4px 0 8px;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .picks {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 16px;
  }
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    min-width: 0;
  }
  .skc {
    height: 150px;
  }
  .dashed {
    padding: 12px 16px;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    font: inherit;
    cursor: pointer;
  }
  .link:hover {
    text-decoration: underline;
  }
  .seg {
    display: inline-flex;
    height: 32px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--bg-raised);
    max-width: 100%;
  }
  .seg button {
    padding: 0 12px;
    border: 0;
    background: none;
    font-size: var(--text-sm);
    color: var(--text-muted);
    white-space: nowrap;
    cursor: pointer;
  }
  .seg button + button {
    border-left: 1px solid var(--border);
  }
  .seg button[aria-pressed="true"] {
    background: var(--bg-selected);
    color: var(--accent);
    font-weight: 600;
  }
  .seg button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }
  .list {
    margin-top: 8px;
    border-top: 1px solid var(--border);
  }
  .rail {
    display: flex;
    flex-direction: column;
    gap: 16px;
    min-width: 0;
  }
  .empty {
    padding: 40px 16px;
    text-align: center;
    color: var(--text-muted);
  }
  .empty.compact {
    padding: 24px 16px;
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
