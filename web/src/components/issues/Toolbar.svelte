<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import type { IssueListState } from "../../lib/issues/list-state.svelte";
  import { effectiveSort, SORT_KEYS, SORT_LABELS, type SortKey } from "../../lib/issues/sort";

  interface Props {
    list: IssueListState;
    onexpandall: () => void;
    oncollapseall: () => void;
  }

  /** The input lags the list state by this much while typing. */
  const DEBOUNCE_MS = 120;

  let { list, onexpandall, oncollapseall }: Props = $props();

  // The input keeps its own draft; the list state (and so the search request) gets the
  // debounced value. A change that did not come from here (Clear search, a new URL) is adopted.
  let draft = $state(untrack(() => list.query));
  let lastPushed = untrack(() => list.query);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function push(v: string): void {
    clearTimeout(timer);
    lastPushed = v;
    list.setQuery(v);
  }

  $effect(() => {
    const q = list.query;
    if (q !== lastPushed) {
      lastPushed = q;
      draft = q;
      clearTimeout(timer);
    }
  });

  onDestroy(() => clearTimeout(timer));

  const hasQuery = $derived(list.query.trim() !== "");
  const relevance = $derived(list.mode === "flat" && hasQuery);
  const sort = $derived(effectiveSort(list.sortPick, list.mode, hasQuery));
  const sortKeys = $derived(SORT_KEYS.filter((k) => k !== "relevance"));

  function onSort(e: Event): void {
    const v = (e.currentTarget as HTMLSelectElement).value as SortKey;
    list.setSort(v === "relevance" ? null : v);
  }
</script>

<div class="tb1">
  <div class="search">
    <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" class="mag">
      <circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" stroke-width="1.5" />
      <path d="M10.5 10.5 14 14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
    </svg>
    <input
      type="text"
      placeholder="Search issues"
      aria-label="Search issues"
      autocomplete="off"
      spellcheck="false"
      bind:value={draft}
      oninput={() => {
        clearTimeout(timer);
        timer = setTimeout(() => push(draft), DEBOUNCE_MS);
      }}
    />
    {#if draft !== ""}
      <button
        type="button"
        class="x"
        aria-label="Clear search"
        onclick={() => {
          draft = "";
          push("");
        }}>✕</button
      >
    {/if}
  </div>

  <div class="seg" role="group" aria-label="View mode">
    <button type="button" aria-pressed={list.mode === "tree"} onclick={() => list.setMode("tree")}>Tree</button>
    <button type="button" aria-pressed={list.mode === "flat"} onclick={() => list.setMode("flat")}>Flat</button>
  </div>

  {#if list.mode === "tree"}
    <div class="tools">
      <button type="button" class="ibtn" title="Expand all" aria-label="Expand all" onclick={onexpandall}>
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M4 6l4 4 4-4M4 2l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" transform="translate(0 2)" />
        </svg>
      </button>
      <button type="button" class="ibtn" title="Collapse all" aria-label="Collapse all" onclick={oncollapseall}>
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M4 5l4-4 4 4M4 15l4-4 4 4" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" transform="translate(0 -1)" />
        </svg>
      </button>
    </div>
  {/if}

  <select class="sortsel" aria-label="Sort" value={sort} onchange={onSort}>
    {#if relevance}<option value="relevance">{SORT_LABELS.relevance}</option>{/if}
    {#each sortKeys as k (k)}
      <option value={k}>{SORT_LABELS[k]}</option>
    {/each}
  </select>
</div>

<style>
  .tb1 {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
    padding-top: 4px;
  }
  .search {
    flex: 1 1 220px;
    max-width: 520px;
    position: relative;
    height: 32px;
  }
  @media (max-width: 639px) {
    .search {
      flex-basis: 100%;
      max-width: none;
    }
  }
  .mag {
    position: absolute;
    left: 9px;
    top: 9px;
    color: var(--text-faint);
    pointer-events: none;
  }
  input {
    width: 100%;
    height: 32px;
    padding: 0 30px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font: inherit;
    color: inherit;
  }
  input::placeholder {
    color: var(--text-faint);
  }
  .x {
    position: absolute;
    right: 4px;
    top: 4px;
    width: 24px;
    height: 24px;
    border: 0;
    background: none;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    cursor: pointer;
  }
  .x:hover {
    background: var(--bg-hover);
  }
  .seg {
    display: inline-flex;
    height: 32px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--bg-raised);
  }
  .seg button {
    padding: 0 12px;
    border: 0;
    background: none;
    font-size: var(--text-sm);
    color: var(--text-muted);
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
  .tools {
    display: inline-flex;
    gap: 6px;
  }
  .ibtn {
    width: 32px;
    height: 32px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }
  .ibtn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .sortsel {
    height: 32px;
    padding: 0 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    color: inherit;
  }
</style>
