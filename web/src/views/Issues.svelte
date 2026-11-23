<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import FilterChips from "../components/issues/FilterChips.svelte";
  import FlatRowView from "../components/issues/FlatRow.svelte";
  import ListEmpty from "../components/issues/ListEmpty.svelte";
  import ListSkeleton from "../components/issues/ListSkeleton.svelte";
  import ListSummary from "../components/issues/ListSummary.svelte";
  import CycleRowView from "../components/issues/CycleRow.svelte";
  import Toolbar from "../components/issues/Toolbar.svelte";
  import TreeRowView from "../components/issues/TreeRow.svelte";
  import VirtualList from "../components/VirtualList.svelte";
  import { isDefaultFilters, passesFilters } from "../lib/issues/filter";
  import { IssuesFeed } from "../lib/issues/feed.svelte";
  import { IssueListState } from "../lib/issues/list-state.svelte";
  import { buildIndex } from "../lib/issues/model";
  import { loadPrefs, prefsKey, savePrefs, serializePrefs, type Prefs } from "../lib/issues/persist";
  import { ROW_H } from "../lib/issues/rows";
  import { SearchRunner } from "../lib/issues/search.svelte";
  import { comparator, effectiveSort } from "../lib/issues/sort";
  import { summarize } from "../lib/issues/summary";
  import {
    findPath,
    flattenTree,
    idsWithKids,
    makeTreeOrder,
    subtreeInfo,
    type TreeRow,
  } from "../lib/issues/tree";
  import { fromQuery, onlyQueryDiffers, toQuery } from "../lib/issues/url";
  import { replaceQuery, route } from "../lib/router/router.svelte";

  /** A row of the list: a tree placement (or cycle marker) or a flat search/filter hit. */
  type Row = TreeRow | { kind: "flat"; key: string; id: string };

  /** How long the revealed row keeps its highlight (matches the .flash animation). */
  const FLASH_MS = 1300;

  const feed = new IssuesFeed();
  const list = new IssueListState();
  const runner = new SearchRunner((rank) => (list.rank = rank));

  let flashTimer: ReturnType<typeof setTimeout> | undefined;
  onMount(() => {
    const stopFeed = feed.start();
    return () => {
      stopFeed();
      runner.stop();
      clearTimeout(flashTimer);
    };
  });

  const index = $derived(feed.data ? buildIndex(feed.data) : null);
  // Relative ages are computed once per data refresh, not per row render.
  const now = $derived.by(() => {
    void feed.data;
    return Date.now();
  });

  // ---- hydrate once the first data and meta are in: URL, then remembered prefs, then defaults.
  let lastSaved = "";
  // The query string this view last wrote or adopted, so it can tell its own replaceQuery
  // echo from a navigation (the nav link, Back/Forward) that changed route.query.
  let lastWritten = "";
  const currentPrefs = (): Prefs => ({
    expanded: [...list.expanded],
    ...(list.sortPick ? { sort: list.sortPick } : {}),
  });

  const parseUrl = (p: URLSearchParams) => fromQuery(p);
  $effect(() => {
    const ix = index;
    const meta = feed.meta;
    if (list.hydrated || !ix || !meta) return;
    untrack(() => {
      lastWritten = route.query.toString();
      list.hydrate({
        tree: ix.tree,
        byId: ix.byId,
        url: parseUrl(route.query),
        prefs: loadPrefs(prefsKey(meta)),
      });
      lastSaved = serializePrefs(currentPrefs());
    });
  });

  // A same-route navigation (the nav link to a bare /issues, Back/Forward between /issues
  // entries) changes route.query without remounting the view: adopt it.
  $effect(() => {
    const qs = route.query.toString();
    if (!list.hydrated || route.path !== "/issues" || qs === lastWritten) return;
    untrack(() => {
      lastWritten = qs;
      list.applyUrl(parseUrl(route.query), loadPrefs(prefsKey(feed.meta))?.sort ?? null);
    });
  });

  // Reflect the view state in the URL (replace only: Back leaves /issues rather than undoing a
  // filter step). Only text typing is debounced; every other change is written at once, because
  // a pending write cannot be flushed on unmount (location has already moved on by then).
  $effect(() => {
    if (!list.hydrated) return;
    const params = toQuery({
      query: list.query,
      filters: list.filters,
      sortPick: list.sortPick,
      modePick: list.modePick,
      peek: list.peek,
    });
    const qs = params.toString();
    if (qs === untrack(() => lastWritten)) return;
    const write = () => {
      if (location.pathname !== "/issues") return;
      lastWritten = qs;
      replaceQuery(params);
    };
    if (untrack(() => onlyQueryDiffers(params, new URLSearchParams(lastWritten)))) {
      const t = setTimeout(write, 300);
      return () => clearTimeout(t);
    }
    write();
  });

  // Remember expansion and sort per tracker (filters are URL-only).
  $effect(() => {
    const meta = feed.meta;
    if (!list.hydrated || !meta) return;
    const s = serializePrefs(currentPrefs());
    if (s === lastSaved) return;
    lastSaved = s;
    savePrefs(prefsKey(meta), currentPrefs());
  });
  // ---- search: the debounced query goes to /api/v1/search; the previous ranking stays until the new one lands.
  $effect(() => {
    if (!list.hydrated) return;
    const q = list.query;
    untrack(() => runner.run(q));
  });

  // ---- derived chain: index -> matches -> rows
  const hasQuery = $derived(list.query.trim() !== "");
  const compare = $derived(comparator(effectiveSort(list.sortPick, list.mode, hasQuery), list.rank));
  const matches = $derived.by(() => {
    const out = new Set<string>();
    if (!index) return out;
    // A query with no ranking yet (first request in flight, too long, failed) matches nothing.
    if (hasQuery && list.rank === null) return out;
    for (const it of index.items) {
      if (passesFilters(it, list.filters) && (!hasQuery || list.rank?.has(it.id))) out.add(it.id);
    }
    return out;
  });
  const order = $derived(index ? makeTreeOrder(index.tree, index.byId, compare) : null);
  const info = $derived(index ? subtreeInfo(index.tree.children, index.byId, matches) : null);

  const flat = $derived.by((): { rows: Row[]; truncated: boolean } => {
    if (!index || !order || !info || !list.hydrated) return { rows: [], truncated: false };
    if (list.mode === "tree") {
      return flattenTree({
        roots: index.tree.roots,
        children: index.tree.children,
        byId: index.byId,
        matches,
        isExpanded: (id) => list.isExpanded(id),
        compare,
        order,
        info,
      });
    }
    const rows = index.items
      .filter((i) => matches.has(i.id))
      .sort(compare)
      .map((i): Row => ({ kind: "flat", key: i.id, id: i.id }));
    return { rows, truncated: false };
  });

  const stats = $derived.by(() => {
    if (!index) return summarize([]);
    return summarize([...matches].flatMap((id) => index.byId.get(id) ?? []));
  });

  const ready = $derived(list.hydrated && index !== null);
  const showError = $derived(feed.error !== null && !(feed.data && feed.meta));
  const emptyKind = $derived(
    !index || index.items.length === 0 ? "empty" : hasQuery ? "query" : list.triggered ? "filters" : "active",
  );
  const canIncludeClosed = $derived(["active", "open", "ready"].includes(list.filters.state));

  type ListApi = {
    getScrollTop: () => number;
    setScrollTop: (px: number) => void;
    scrollToIndex: (i: number, align?: "center" | "nearest") => void;
  };
  let vl = $state<ListApi>();

  // A different list (mode, text or filters) starts at the top, except while revealing or going back.
  let keepScroll = false;
  $effect(() => {
    void list.mode;
    void list.query;
    void list.filters;
    untrack(() => {
      if (!keepScroll) vl?.setScrollTop(0);
    });
  });

  function flash(id: string): void {
    clearTimeout(flashTimer);
    list.flashId = id;
    flashTimer = setTimeout(() => (list.flashId = null), FLASH_MS);
  }

  /**
   * "Show in tree" and the breadcrumb segments: switch to the tree, open the
   * ancestors of the first placement, scroll it to the centre and flash it. The
   * path is looked up after the mode switch has settled, because the sort (and so the
   * display order) differs between flat-with-query and tree.
   */
  async function reveal(id: string): Promise<void> {
    if (!index) return;
    keepScroll = true;
    try {
      list.beginReveal(vl?.getScrollTop() ?? 0);
      await tick();
      const path = order && info ? findPath(order, info, id) : null;
      if (path) {
        list.expandPath(path);
        await tick();
      }
      const i = flat.rows.findIndex((r) => r.kind === "issue" && r.id === id);
      if (i >= 0) vl?.scrollToIndex(i, "center");
    } finally {
      keepScroll = false;
    }
    flash(id);
  }

  async function goBack(): Promise<void> {
    const top = list.back();
    if (top === null) return;
    keepScroll = true;
    try {
      await tick();
      vl?.setScrollTop(top);
    } finally {
      keepScroll = false;
    }
  }

  // One delegated click listener (VirtualList adds it natively): a row click opens the peek,
  // unless it landed on a button or link. The title button is the keyboard path.
  function onRowClick(e: MouseEvent): void {
    const target = e.target as Element | null;
    if (!target || target.closest("button, a")) return;
    const id = target.closest("[data-row]")?.getAttribute("data-id");
    if (id) list.openPeek(id);
  }

  const key = (r: Row) => r.key;
</script>

<section class="issues" aria-label="Issues">
  <h1 class="sr">Issues</h1>
  <div class="pane">
    {#if showError && feed.error}
      <div class="state" role="alert">
        <h3>Can't load issues</h3>
        <p>{feed.error.message}</p>
        <button type="button" class="btn" onclick={() => feed.reload()}>Retry</button>
      </div>
    {:else if !ready || !index}
      <ListSkeleton />
    {:else}
      {#if index.items.length > 0}
        <div class="bars">
          <Toolbar
            {list}
            onexpandall={() => list.expandAll(idsWithKids(index.tree))}
            oncollapseall={() => list.collapseAll(idsWithKids(index.tree))}
          />
          <FilterChips
            filters={list.filters}
            facets={index.facets}
            onchange={(f) => list.setFilters(f)}
            onclear={() => list.clearFilters()}
          />
          <ListSummary
            {stats}
            mode={list.mode}
            query={list.query}
            computing={feed.phase2Pending}
            openOnly={feed.meta?.loaded_open_only ?? false}
            searching={runner.pending}
            searchFailed={runner.error !== null}
            queryTooLong={runner.tooLong}
            canGoBack={list.backTo !== null && list.mode === "tree"}
            onback={() => void goBack()}
          />
        </div>
      {/if}
      {#if flat.rows.length === 0 && hasQuery && list.rank === null && runner.pending}
        <div class="body"><ListSkeleton /></div>
      {:else if flat.rows.length === 0}
        <div class="body">
          <ListEmpty
            kind={emptyKind}
            query={list.query}
            {canIncludeClosed}
            canClearFilters={!isDefaultFilters(list.filters)}
            onclearsearch={() => list.setQuery("")}
            onclearfilters={() => list.clearFilters()}
            onincludeclosed={() => list.setFilters({ ...list.filters, state: "all" })}
          />
        </div>
      {:else}
        {#if flat.truncated}
          <p class="notice">Too many rows to show expanded. Collapse some branches.</p>
        {/if}
        <div class="list">
          <VirtualList
            bind:this={vl}
            items={flat.rows}
            rowHeight={list.mode === "flat" ? ROW_H.flat : ROW_H.tree}
            {key}
            row={renderRow}
            label="Issues"
            onclick={onRowClick}
          />
        </div>
      {/if}
    {/if}
  </div>
</section>

{#snippet renderRow(r: Row)}
  {#if index}
    {@const item = index.byId.get(r.id)}
    {#if r.kind === "cycle"}
      <CycleRowView row={r} {item} />
    {:else if item && r.kind === "flat"}
      <FlatRowView
        {item}
        {index}
        query={list.query}
        {now}
        selected={list.peek === r.id}
        onpeek={(id) => list.openPeek(id)}
        onreveal={(id) => void reveal(id)}
      />
    {:else if item && r.kind === "issue"}
      <TreeRowView
        row={r}
        {item}
        {index}
        via={r.viaId ? index.byId.get(r.viaId) : undefined}
        {now}
        selected={list.peek === r.id}
        flash={list.flashId === r.id}
        ontoggle={(id) => list.toggle(id)}
        onpeek={(id) => list.openPeek(id)}
      />
    {/if}
  {/if}
{/snippet}

<style>
  .issues {
    flex: 1;
    min-height: 0;
    display: flex;
  }
  .pane {
    flex: 1;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .bars {
    flex: none;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .list {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--border);
  }
  .state {
    padding: 40px 16px;
    text-align: center;
    color: var(--text-muted);
  }
  .state h3 {
    margin: 0 0 4px;
    font-size: var(--text-lg);
    color: var(--text);
  }
  .state p {
    margin: 4px 0 12px;
  }
  .btn {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .btn:hover {
    background: var(--bg-hover);
  }
  .notice {
    margin: 0 0 var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--warning);
    border-radius: var(--radius-md);
    background: var(--warning-subtle);
    color: var(--warning);
    font-size: var(--text-sm);
  }
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>