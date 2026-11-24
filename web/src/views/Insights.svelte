<script lang="ts">
  import { onMount } from "svelte";
  import { getInsightsIfChanged, getTriageIfChanged } from "../lib/api/client";
  import type { InsightsResponse, TriageResponse } from "../lib/api/types";
  import { Resource, ViewFeed } from "../lib/feeds/view-feed.svelte";
  import { session } from "../lib/session.svelte";
  import { healthState, healthTiles, insightsView, nextLimit } from "../lib/work/insights";
  import { METRICS, type MetricKey } from "../lib/work/metrics";
  import { PeekHost } from "../lib/work/peek-host.svelte";
  import { plural } from "../lib/work/present";
  import { rowClicks } from "../lib/work/row-click";
  import Notice from "../components/work/Notice.svelte";
  import SkeletonRows from "../components/work/SkeletonRows.svelte";
  import WorkPeek from "../components/work/WorkPeek.svelte";
  import HealthStrip from "../components/work/insights/HealthStrip.svelte";
  import MetricCard from "../components/work/insights/MetricCard.svelte";
  import OrphansCard from "../components/work/insights/OrphansCard.svelte";

  const path = (limit: number) => `/api/v1/insights?limit=${limit}` as const;
  const fetcher = (limit: number) => (etag: string | null, signal: AbortSignal) => getInsightsIfChanged(limit, etag, signal);

  let limit = $state(25);
  const insights = new Resource<InsightsResponse>(path(25), fetcher(25));
  const triage = new Resource<TriageResponse>("/api/v1/triage", getTriageIfChanged);
  const feed = new ViewFeed([insights, triage]);
  // The rows here have no priority pill, so the popover only ever opens from the drawer.
  const host = new PeekHost({ writable: () => feed.writes, refresh: () => feed.refresh(), lookup: () => undefined });
  onMount(() => feed.start());

  const view = $derived(insightsView(insights));
  const r = $derived(insights.data);
  const health = $derived(healthState(triage));
  const tiles = $derived(triage.data ? healthTiles(triage.data, r) : null);
  const emptyTracker = $derived(feed.meta?.issue_count === 0);
  // Relative dates are computed once per data refresh, not per row render.
  const now = $derived.by(() => {
    void insights.data;
    return Date.now();
  });

  let infoOpen = $state.raw<ReadonlySet<MetricKey>>(new Set());
  let showAll = $state.raw<ReadonlySet<MetricKey>>(new Set());
  let orphansOpen = $state(false);
  const flip = (s: ReadonlySet<MetricKey>, k: MetricKey): Set<MetricKey> => {
    const next = new Set(s);
    if (!next.delete(k)) next.add(k);
    return next;
  };

  /** Every list grows together: one request, the next limit (the variant ETag is per limit). */
  function loadMore(): void {
    const next = nextLimit(limit);
    if (next === null) return;
    limit = next;
    insights.setSource(path(next), fetcher(next));
    feed.kick();
  }
  const moreOrphans = $derived(r !== null && r.orphan_count > r.orphans.length && nextLimit(limit) !== null);

  // The drawer refetches when the snapshot changes (/meta moves with every view), not only when this
  // view's data does. Same shape in Triage, Plan and Insights.
  const reloadKey = $derived([feed.meta, insights.data] as const);

  const openPeek = (id: string): void => host.open(id);
</script>

<section class="view" class:offline={session.status === "offline"} aria-label="Insights">
  <div class="scroll" data-work-scroll tabindex="-1" use:rowClicks={openPeek}>
    <h1>Insights</h1>

    <div class="summary" aria-live="polite">
      {#if emptyTracker}
        <span>0 issues</span>
      {:else if view.state === "data"}
        <span>{METRICS.length} graph metrics</span>
        {#if tiles}<span>{plural(tiles.graph.nodes, "node", "nodes")}</span>{/if}
        {#if view.provisional}<span title="Graph analysis (phase 2) has not finished">provisional · computing graph metrics…</span>{/if}
        {#if view.stale}<span class="chip" title={view.staleTitle}>Showing the last loaded insights</span>{/if}
      {/if}
    </div>

    {#if host.missing}
      <Notice tone="warning" text={host.missing} action={{ label: "Dismiss", run: () => (host.missing = null) }} />
    {/if}

    {#if emptyTracker}
      <div class="empty">
        <h3>No insights yet</h3>
        <p>This tracker has no open issues.</p>
        <div class="acts"><code>bd create "…"</code></div>
      </div>
    {:else}
      {#if view.state === "failed"}
        <Notice tone="danger" text="Can't load insights: {view.message}" action={{ label: "Retry", run: () => feed.reload() }} />
      {/if}
      {#if view.phase2Error}
        <Notice tone="warning" text="Graph analysis failed: {view.phase2Error}. Showing phase-1 results." action={{ label: "Retry", run: () => feed.reload() }} />
      {/if}

      <HealthStrip
        state={health.state}
        message={health.message}
        {tiles}
        {orphansOpen}
        onorphans={() => (orphansOpen = !orphansOpen)}
      />

      {#if view.state === "skipped"}
        <div class="gap">
          <Notice tone="warning" text="Graph metrics are off for trackers this large (phase 2 is skipped). Triage and Plan still work." />
        </div>
      {:else if view.state === "loading"}
        <div class="mgrid">
          {#each METRICS as m (m.key)}
            <section class="card skc" aria-label={m.title}>
              <h3>{m.title}</h3>
              <div class="sub">{m.short}</div>
              <SkeletonRows n={3} label="" />
            </section>
          {/each}
        </div>
      {:else if r}
        <div class="mgrid">
          {#each METRICS as m (m.key)}
            <MetricCard
              def={m}
              {r}
              {limit}
              info={infoOpen.has(m.key)}
              wide={showAll.has(m.key)}
              onpeek={openPeek}
              oninfo={() => (infoOpen = flip(infoOpen, m.key))}
              onall={() => (showAll = flip(showAll, m.key))}
              onmore={loadMore}
            />
          {/each}
        </div>
        {#if orphansOpen}
          <OrphansCard {r} more={moreOrphans} onpeek={openPeek} onmore={loadMore} />
        {/if}
      {/if}
    {/if}
  </div>
  <WorkPeek {host} writes={feed.writes} {now} {reloadKey} />
</section>

<style>
  .view {
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
  .chip {
    padding: 0 8px;
    line-height: 18px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--bg-subtle);
  }
  .gap {
    margin-top: 16px;
  }
  .mgrid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 16px;
    margin-top: 16px;
  }
  @container view (max-width: 899px) {
    .mgrid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @container view (max-width: 559px) {
    .mgrid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    min-width: 0;
  }
  .card h3 {
    margin: 0;
    font-size: var(--text-base);
    font-weight: 600;
  }
  .sub {
    font-size: var(--text-xs);
    color: var(--text-faint);
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
