<script lang="ts">
  import type { PipelineResponse } from "../../lib/api/types";
  import { holderText, pipelineFlag } from "../../lib/desk/desk";

  interface Props {
    pipeline: PipelineResponse;
    /** The issue open in the drawer. */
    selectedId: string | null;
    now: number;
    onpeek: (id: string) => void;
  }

  let { pipeline, selectedId, now, onpeek }: Props = $props();
</script>

<table>
  <thead>
    <tr>
      <th class="stage">Stage</th>
      <th class="id">ID</th>
      <th>Title</th>
      <th class="holder">Session / holder</th>
      <th class="age">In stage</th>
      <th class="flag">Flag</th>
    </tr>
  </thead>
  <tbody>
    {#each pipeline.rows as r (r.id)}
      {@const flag = pipelineFlag(r, now)}
      <tr data-row data-id={r.id} class:stale={r.stale} class:sel={r.id === selectedId}>
        <td class="stage">
          {#if r.stage}<span class="tag">{r.stage}</span>{:else}<span class="none" title="No stage set">—</span>{/if}
        </td>
        <td class="id" title={r.id}>{r.id}</td>
        <td class="title"><button type="button" class="t-btn" title={r.title} onclick={() => onpeek(r.id)}>{r.title}</button></td>
        <td class="holder" title={holderText(r)}>{holderText(r) || "—"}</td>
        <td class="age">{r.stage_age ?? "—"}</td>
        <td class="flag">
          {#if flag.tone === "stale"}<span class="warn">{flag.text}</span>{:else if flag.tone === "lease"}<span class="lease">{flag.text}</span>{/if}
        </td>
      </tr>
    {/each}
  </tbody>
</table>

<style>
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-base);
  }
  th {
    position: sticky;
    top: 0;
    z-index: 1;
    height: 28px;
    padding: 0 var(--space-3);
    background: var(--bg-subtle);
    border-bottom: 1px solid var(--border);
    font-size: var(--text-xs);
    font-weight: 400;
    color: var(--text-muted);
    text-align: left;
    white-space: nowrap;
  }
  td {
    height: 36px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
    white-space: nowrap;
  }
  tbody tr {
    cursor: pointer;
  }
  tbody tr:hover {
    background: var(--bg-hover);
  }
  tr.sel {
    background: var(--bg-selected);
  }
  /* The row's left edge: accent when open in the drawer, warning when stale (stale wins). */
  tr.sel td:first-child {
    box-shadow: inset 2px 0 0 var(--accent);
  }
  tr.stale td:first-child {
    box-shadow: inset 2px 0 0 var(--warning);
  }
  .stage {
    width: 96px;
  }
  .id {
    width: 120px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .holder {
    width: 200px;
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .age {
    width: 90px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-align: right;
  }
  th.age {
    text-align: right;
  }
  .flag {
    width: 260px;
  }
  .title {
    max-width: 0;
    width: 100%;
  }
  .t-btn {
    max-width: 100%;
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
    font-size: var(--text-base);
  }
  .tag {
    padding: 0 6px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg-subtle);
    font-size: var(--text-xs);
    line-height: 16px;
    color: var(--text-muted);
  }
  .none {
    color: var(--text-faint);
  }
  .warn {
    padding: 0 6px;
    border-radius: var(--radius-sm);
    background: var(--warning-subtle);
    color: var(--warning);
    font-size: var(--text-xs);
    line-height: 16px;
  }
  .lease {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
</style>
