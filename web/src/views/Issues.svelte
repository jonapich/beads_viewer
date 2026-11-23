<script lang="ts">
  import { onMount, untrack } from "svelte";
  import ListEmpty from "../components/issues/ListEmpty.svelte";
  import ListSkeleton from "../components/issues/ListSkeleton.svelte";
  import CycleRowView from "../components/issues/CycleRow.svelte";
  import TreeRowView from "../components/issues/TreeRow.svelte";
  import VirtualList from "../components/VirtualList.svelte";
  import { passesFilters } from "../lib/issues/filter";
  import { IssuesFeed } from "../lib/issues/feed.svelte";
  import { IssueListState } from "../lib/issues/list-state.svelte";
  import { buildIndex } from "../lib/issues/model";
  import { loadPrefs, prefsKey, savePrefs, serializePrefs, type Prefs } from "../lib/issues/persist";
  import { ROW_H } from "../lib/issues/rows";
  import { comparator, effectiveSort } from "../lib/issues/sort";
  import { flattenTree, makeTreeOrder, subtreeInfo, type TreeRow } from "../lib/issues/tree";
  import { fromQuery, onlyQueryDiffers, toQuery } from "../lib/issues/url";
  import { replaceQuery, route } from "../lib/router/router.svelte";

  const feed = new IssuesFeed();
  const list = new IssueListState();

  onMount(() => feed.start());

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

  // Text search lands with the toolbar; until then a q param is ignored (and dropped from the URL).
  const parseUrl = (p: URLSearchParams) => ({ ...fromQuery(p), query: "" });

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
  // ---- derived chain: index -> matches -> rows
  const compare = $derived(comparator(effectiveSort(list.sortPick, list.mode, list.query.trim() !== ""), list.rank));
  const matches = $derived.by(() => {
    const out = new Set<string>();
    if (!index) return out;
    for (const it of index.items) {
      if (passesFilters(it, list.filters) && (list.rank === null || list.rank.has(it.id))) out.add(it.id);
    }
    return out;
  });
  const order = $derived(index ? makeTreeOrder(index.tree, index.byId, compare) : null);
  const info = $derived(index ? subtreeInfo(index.tree.children, index.byId, matches) : null);

  const flat = $derived.by((): { rows: TreeRow[]; truncated: boolean } => {
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
    // Interim flat results: the matches in sort order as plain depth-0 rows. The flat row
    // layout (breadcrumbs, highlighting, "Show in tree") arrives with the toolbar.
    const rows = index.items
      .filter((i) => matches.has(i.id))
      .sort(compare)
      .map((i): TreeRow => ({
        kind: "issue",
        key: i.id,
        id: i.id,
        depth: 0,
        edges: [],
        viaId: null,
        hasKids: false,
        expanded: false,
        context: false,
        descCount: 0,
        attention: null,
      }));
    return { rows, truncated: false };
  });

  const ready = $derived(list.hydrated && index !== null);
  const showError = $derived(feed.error !== null && !(feed.data && feed.meta));
  const emptyKind = $derived(!index || index.items.length === 0 ? "empty" : list.triggered ? "filters" : "active");
  const canIncludeClosed = $derived(["active", "open", "ready"].includes(list.filters.state));

  let vl = $state<{ setScrollTop: (px: number) => void }>();

  // A different mode is a different list: start at the top.
  $effect(() => {
    void list.mode;
    untrack(() => vl?.setScrollTop(0));
  });

  // One delegated click listener (VirtualList adds it natively): a row click opens the peek,
  // unless it landed on a button or link. The title button is the keyboard path.
  function onRowClick(e: MouseEvent): void {
    const target = e.target as Element | null;
    if (!target || target.closest("button, a")) return;
    const id = target.closest("[data-row]")?.getAttribute("data-id");
    if (id) list.openPeek(id);
  }

  const key = (r: TreeRow) => r.key;
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
    {:else if !ready}
      <ListSkeleton />
    {:else if flat.rows.length === 0}
      <ListEmpty
        kind={emptyKind}
        {canIncludeClosed}
        onclearfilters={() => list.clearFilters()}
        onincludeclosed={() => list.setFilters({ ...list.filters, state: "all" })}
      />
    {:else}
      {#if flat.truncated}
        <p class="notice">Too many rows to show expanded. Collapse some branches.</p>
      {/if}
      <div class="list">
        <VirtualList
          bind:this={vl}
          items={flat.rows}
          rowHeight={ROW_H.tree}
          {key}
          row={renderRow}
          label="Issues"
          onclick={onRowClick}
        />
      </div>
    {/if}
  </div>
</section>

{#snippet renderRow(r: TreeRow)}
  {#if index}
    {@const item = index.byId.get(r.id)}
    {#if r.kind === "cycle"}
      <CycleRowView row={r} {item} />
    {:else if item}
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
