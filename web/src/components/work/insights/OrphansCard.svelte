<script lang="ts">
  import type { InsightsResponse } from "../../../lib/api/types";
  import { orphanRows } from "../../../lib/work/insights";

  interface Props {
    r: InsightsResponse;
    /** "Load more" is available (the list is capped and the limit can still grow). */
    more: boolean;
    onpeek: (id: string) => void;
    onmore: () => void;
  }

  let { r, more, onpeek, onmore }: Props = $props();
  const rows = $derived(orphanRows(r));
</script>

<section class="card" id="orphans-card" aria-label="Orphans">
  <div class="mh">
    <div class="tt">
      <h3>Orphans</h3>
      <div class="sub">Issues with no dependency links</div>
    </div>
    <span class="cnt">{r.orphan_count} total</span>
  </div>
  {#if rows.length === 0}
    <div class="none">None found</div>
  {:else}
    {#each rows as row, i (row.id)}
      <div class="mrow" data-row data-id={row.id}>
        <span class="rk">{i + 1}</span>
        <span class="mid" title={row.id}>{row.id}</span>
        <button type="button" class="t-btn mtt" title={row.title || row.id} onclick={() => onpeek(row.id)}>{row.title || row.id}</button>
      </div>
    {/each}
    <p class="note">Showing {rows.length} of {r.orphan_count} orphans (no dependencies in or out).</p>
    {#if more}<button type="button" class="btn" onclick={onmore}>Load more</button>{/if}
  {/if}
</section>

<style>
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    margin-top: 16px;
    min-width: 0;
  }
  .mh {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    margin-bottom: 8px;
  }
  .tt {
    flex: 1;
    min-width: 0;
  }
  h3 {
    margin: 0;
    font-size: var(--text-base);
    font-weight: 600;
  }
  .sub,
  .cnt {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .none {
    font-size: var(--text-sm);
    color: var(--text-muted);
    padding: 6px 0;
  }
  .mrow {
    display: grid;
    grid-template-columns: 18px 88px minmax(0, 1fr);
    column-gap: 8px;
    align-items: center;
    min-height: 28px;
    padding: 3px 4px;
    margin: 0 -4px;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .mrow:hover {
    background: var(--bg-hover);
  }
  .rk {
    font-size: var(--text-xs);
    color: var(--text-faint);
    text-align: right;
  }
  .mid {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mtt {
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm);
  }
  .note {
    font-size: var(--text-xs);
    color: var(--text-faint);
    margin: 6px 0 0;
  }
  .btn {
    margin-top: 8px;
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
