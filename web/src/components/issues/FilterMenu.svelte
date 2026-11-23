<script lang="ts">
  import { facetOptions, FILTER_NAMES, STATE_LABELS, toggleValue, type FilterKey } from "../../lib/issues/chips";
  import { STATE_FILTERS, type ListFilters, type StateFilter } from "../../lib/issues/filter";
  import type { Facets } from "../../lib/issues/model";

  interface Props {
    filterKey: FilterKey;
    filters: ListFilters;
    facets: Facets;
    onchange: (f: ListFilters) => void;
  }

  let { filterKey, filters, facets, onchange }: Props = $props();

  let search = $state("");

  const searchable = $derived(filterKey === "label" || filterKey === "assignee");
  const options = $derived(
    filterKey === "type"
      ? facetOptions(facets.types, filters.type, "")
      : filterKey === "label"
        ? facetOptions(facets.labels, filters.label, search)
        : filterKey === "assignee"
          ? facetOptions(facets.assignees, filters.assignee, search)
          : [],
  );

  function toggle(value: string): void {
    if (filterKey === "type") onchange({ ...filters, type: toggleValue(filters.type, value) });
    else if (filterKey === "label") onchange({ ...filters, label: toggleValue(filters.label, value) });
    else if (filterKey === "assignee") onchange({ ...filters, assignee: toggleValue(filters.assignee, value) });
  }
</script>

<div class="pop" role="group" aria-label="{FILTER_NAMES[filterKey]} filter">
  {#if filterKey === "state"}
    {#each STATE_FILTERS as s (s)}
      <label class="opt">
        <input
          type="radio"
          name="state-filter"
          value={s}
          checked={filters.state === s}
          onchange={() => onchange({ ...filters, state: s as StateFilter })}
        />
        {STATE_LABELS[s]}
      </label>
    {/each}
  {:else if filterKey === "priority"}
    {#each [0, 1, 2, 3, 4] as p (p)}
      <label class="opt">
        <input
          type="checkbox"
          checked={filters.priority.includes(p)}
          onchange={() => onchange({ ...filters, priority: toggleValue(filters.priority, p).sort((a, b) => a - b) })}
        />
        P{p}
      </label>
    {/each}
  {:else}
    {#if searchable}
      <input
        class="find"
        type="text"
        placeholder="Filter {FILTER_NAMES[filterKey].toLowerCase()}s"
        aria-label="Filter {FILTER_NAMES[filterKey].toLowerCase()} list"
        autocomplete="off"
        bind:value={search}
      />
    {/if}
    {#each options as o (o.value)}
      <label class="opt">
        <input type="checkbox" checked={o.selected} onchange={() => toggle(o.value)} />
        <span class="name">{o.label}</span>
        <span class="cnt">{o.count}</span>
      </label>
    {:else}
      <p class="none">Nothing to pick</p>
    {/each}
  {/if}
</div>

<style>
  .pop {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 50;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    padding: 6px;
    min-width: 190px;
    max-width: min(320px, calc(100vw - 16px));
    max-height: 340px;
    overflow: auto;
  }
  .opt {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 28px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .opt:hover {
    background: var(--bg-hover);
  }
  .opt input {
    margin: 0;
  }
  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cnt {
    margin-left: auto;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .find {
    width: 100%;
    height: 28px;
    margin-bottom: 4px;
    padding: 0 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg);
    font: inherit;
    color: inherit;
  }
  .none {
    margin: 4px 8px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
