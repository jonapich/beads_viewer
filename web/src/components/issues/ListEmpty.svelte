<script lang="ts">
  interface Props {
    /**
     * "empty": the tracker has no issues. "active": the default view has nothing.
     * "filters": the filters match nothing. "query": the search matches nothing.
     */
    kind: "empty" | "active" | "filters" | "query";
    /** The text search, for the "query" message. */
    query?: string;
    /** State excludes closed issues, so offering them can help. */
    canIncludeClosed: boolean;
    /** Some filter is non-default. */
    canClearFilters: boolean;
    onclearsearch: () => void;
    onclearfilters: () => void;
    onincludeclosed: () => void;
  }

  let { kind, query = "", canIncludeClosed, canClearFilters, onclearsearch, onclearfilters, onincludeclosed }: Props = $props();
</script>

<div class="empty">
  {#if kind === "empty"}
    <h3>No issues yet</h3>
    <p>Create one with</p>
    <p><code>bd create "My first issue"</code></p>
  {:else}
    <h3>
      {#if kind === "query"}No issues match “{query.trim()}”{:else if kind === "filters"}Nothing matches these filters{:else}No active issues{/if}
    </h3>
    <div class="acts">
      {#if kind === "query"}
        <button type="button" class="btn" onclick={onclearsearch}>Clear search</button>
      {/if}
      {#if canIncludeClosed}
        <button type="button" class="btn" onclick={onincludeclosed}>Include closed</button>
      {/if}
      {#if canClearFilters}
        <button type="button" class="btn" onclick={onclearfilters}>Clear filters</button>
      {/if}
    </div>
  {/if}
</div>
<style>
  .empty {
    padding: 40px 16px;
    text-align: center;
    color: var(--text-muted);
  }
  h3 {
    margin: 0 0 4px;
    font-size: var(--text-lg);
    color: var(--text);
    font-weight: 600;
  }
  p {
    margin: 4px 0;
  }
  code {
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 2px 8px;
  }
  .acts {
    display: flex;
    gap: 8px;
    justify-content: center;
    margin-top: 12px;
    flex-wrap: wrap;
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
</style>
