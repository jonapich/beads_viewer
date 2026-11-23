<script lang="ts">
  import type { Mode } from "../../lib/issues/filter";
  import { summaryText, type Stats } from "../../lib/issues/summary";

  interface Props {
    stats: Stats;
    mode: Mode;
    query: string;
    /** Phase 2 (triage scores) is still being computed. */
    computing: boolean;
    /** The tracker was loaded open-issues-only, so State Closed/All are otherwise silently empty. */
    openOnly: boolean;
    searching: boolean;
    searchFailed: boolean;
    queryTooLong: boolean;
    /** Show the "Back to results" pill. */
    canGoBack: boolean;
    onback: () => void;
  }

  let { stats, mode, query, computing, openOnly, searching, searchFailed, queryTooLong, canGoBack, onback }: Props = $props();
</script>

<div class="summary" aria-live="polite">
  <span>
    {summaryText(stats, mode, query)}
    {#if queryTooLong}· Query too long{/if}
    {#if searchFailed}· Search failed{/if}
    {#if searching}· searching…{/if}
    {#if computing}· computing triage…{/if}
    {#if openOnly}· closed issues not loaded (large tracker){/if}
  </span>
  {#if canGoBack}
    <button type="button" class="backpill" onclick={onback}>← Back to results</button>
  {/if}
</div>

<style>
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
  .backpill {
    border: 1px solid var(--accent);
    color: var(--accent);
    border-radius: 12px;
    padding: 0 10px;
    height: 22px;
    font-size: var(--text-sm);
    background: var(--accent-subtle);
    cursor: pointer;
  }
</style>
