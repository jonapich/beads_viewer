<script lang="ts">
  import { chipText, FILTER_KEYS, showClearFilters, type FilterKey } from "../../lib/issues/chips";
  import type { ListFilters } from "../../lib/issues/filter";
  import type { Facets } from "../../lib/issues/model";
  import FilterMenu from "./FilterMenu.svelte";

  interface Props {
    filters: ListFilters;
    facets: Facets;
    onchange: (f: ListFilters) => void;
    onclear: () => void;
  }

  let { filters, facets, onchange, onclear }: Props = $props();

  let open = $state<FilterKey | null>(null);
  let root = $state<HTMLDivElement>();

  // While a menu is open: an outside click or Esc closes it (Esc returns focus to its chip).
  $effect(() => {
    const current = open;
    if (current === null) return;
    const onPointer = (e: PointerEvent) => {
      if (root && e.target instanceof Node && !root.contains(e.target)) open = null;
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault(); // so the drawer's Esc handler leaves this press alone
      open = null;
      root?.querySelector<HTMLButtonElement>(`[data-chip="${current}"]`)?.focus();
    };
    document.addEventListener("pointerdown", onPointer, true);
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("pointerdown", onPointer, true);
      document.removeEventListener("keydown", onKey, true);
    };
  });
</script>

<div class="tb2" bind:this={root}>
  {#each FILTER_KEYS as key (key)}
    {@const c = chipText(key, filters)}
    <div class="wrap">
      <button
        type="button"
        class="chip"
        class:on={c.on}
        data-chip={key}
        aria-haspopup="true"
        aria-expanded={open === key}
        onclick={() => (open = open === key ? null : key)}
      >
        {c.name}{c.value !== "" ? `: ${c.value}` : ""} <span class="car" aria-hidden="true">▾</span>
      </button>
      {#if open === key}
        <FilterMenu filterKey={key} {filters} {facets} {onchange} />
      {/if}
    </div>
  {/each}
  {#if showClearFilters(filters)}
    <button type="button" class="link" onclick={onclear}>Clear filters</button>
  {/if}
</div>

<style>
  .tb2 {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-wrap: wrap;
    padding-top: 8px;
  }
  .wrap {
    position: relative;
  }
  .chip {
    height: 28px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: 14px;
    font-size: var(--text-sm);
    background: var(--bg-raised);
    display: inline-flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
    cursor: pointer;
  }
  .chip:hover {
    background: var(--bg-hover);
  }
  .chip.on {
    border-color: var(--accent);
    color: var(--accent);
  }
  .car {
    font-size: 9px;
    color: var(--text-faint);
  }
  .link {
    color: var(--accent);
    font-size: var(--text-sm);
    padding: 0 6px;
    height: 28px;
    border: 0;
    background: none;
    cursor: pointer;
  }
  .link:hover {
    text-decoration: underline;
  }
</style>
