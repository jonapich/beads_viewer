<script lang="ts">
  import type { SliceMetricsResponse } from "../../lib/api/types";
  import { metricsCells } from "../../lib/desk/desk";

  let { report }: { report: SliceMetricsResponse } = $props();

  const total = $derived(metricsCells(report.total));
  const label = $derived(`All · ${report.epics.length} ${report.epics.length === 1 ? "epic" : "epics"}`);
  const cols = ["Slices", "✓/✗", "Rev/slice", "1st-pass %", "Tests", "Red", "Min/slice", "Tokens k (cod/rev/oth)", "Esc"];
</script>

<table>
  <thead>
    <tr>
      <th class="epic">Epic</th>
      <th>Title</th>
      {#each cols as c (c)}<th class="num">{c}</th>{/each}
    </tr>
  </thead>
  <tbody>
    {#each report.epics as m (m.epic)}
      {@const c = metricsCells(m)}
      <tr data-metrics-row={m.epic}>
        <td class="epic" title={c.epic}>{c.epic}</td>
        <td class="title" title={c.title}>{c.title}</td>
        <td class="num">{c.slices}</td>
        <td class="num">{c.outcome}</td>
        <td class="num">{c.revPerSlice}</td>
        <td class="num">{c.firstPass}</td>
        <td class="num">{c.tests}</td>
        <td class="num">{c.red}</td>
        <td class="num">{c.minPerSlice}</td>
        <td class="num" title={m.slices_with_unknown_tokens > 0 ? `${m.slices_with_unknown_tokens} slice(s) logged no token count` : ""}>{c.tokens}</td>
        <td class="num">{c.esc}</td>
      </tr>
    {/each}
  </tbody>
  <tfoot>
    <tr class="total" data-metrics-total>
      <td class="epic" colspan="2">{label}</td>
      <td class="num">{total.slices}</td>
      <td class="num">{total.outcome}</td>
      <td class="num">{total.revPerSlice}</td>
      <td class="num">{total.firstPass}</td>
      <td class="num">{total.tests}</td>
      <td class="num">{total.red}</td>
      <td class="num">{total.minPerSlice}</td>
      <td class="num">{total.tokens}</td>
      <td class="num">{total.esc}</td>
    </tr>
  </tfoot>
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
  th.num,
  td.num {
    text-align: right;
  }
  td {
    height: 36px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
    white-space: nowrap;
  }
  td.num {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
  }
  .epic {
    width: 140px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  td.title {
    max-width: 0;
    width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* The total stays in view at the foot of the scrolling card. */
  tfoot td {
    position: sticky;
    bottom: 0;
    z-index: 1;
    background: var(--bg-subtle);
    border-top: 1px solid var(--border-strong);
    border-bottom: 0;
    font-weight: 600;
  }
  tfoot td.epic {
    color: var(--text);
    font-family: inherit;
    font-size: var(--text-base);
  }
</style>
