<script lang="ts">
  import type { InsightsResponse } from "../../../lib/api/types";
  import { parseInline } from "../../../lib/markdown";
  import { canLoadMore, cardState, cycleChains, listLen, metricRows } from "../../../lib/work/insights";
  import { CARD_ROWS, ID_LISTS, type MetricDef } from "../../../lib/work/metrics";
  import { formatMetric, plural } from "../../../lib/work/present";
  import Inlines from "../../detail/Inlines.svelte";
  import Notice from "../Notice.svelte";
  import Tag from "../Tag.svelte";
  import ValueBar from "../ValueBar.svelte";
  import CycleChain from "./CycleChain.svelte";

  interface Props {
    def: MetricDef;
    r: InsightsResponse;
    /** The explanation is showing. */
    info: boolean;
    /** The card spans the grid and lists everything loaded. */
    wide: boolean;
    /** The limit the lists were fetched at. */
    limit: number;
    onpeek: (id: string) => void;
    oninfo: () => void;
    onall: () => void;
    onmore: () => void;
  }

  let { def, r, info, wide, limit, onpeek, oninfo, onall, onmore }: Props = $props();

  const st = $derived(cardState(r, def.key));
  const n = $derived(listLen(r, def.key));
  const isCycles = $derived(def.key === "cycles");
  const hasValue = $derived(!ID_LISTS.has(def.key));
  const shown = $derived(wide ? n : Math.min(n, CARD_ROWS));
  const rows = $derived(metricRows(r, def.key).slice(0, shown));
  const chains = $derived(cycleChains(r).slice(0, shown));
  const hasList = $derived(st.state === "list" || st.state === "computing");
  const cyclic = $derived(isCycles && hasList && r.cycle_count > 0);
  const titleOf = (id: string): string | undefined => r.titles[id];

  const explain = $derived([
    { h: "What", t: parseInline(def.whatIs) },
    { h: "Why useful", t: parseInline(def.whyUseful) },
    { h: "How to use", t: parseInline(def.howToUse) },
    { h: "Formula", t: parseInline(def.formula) },
  ]);
</script>

<section class="card mcard" class:wide class:danger={cyclic} aria-label={def.title} data-metric={def.key}>
  <div class="mh">
    <div class="tt">
      <h3>{def.title}</h3>
      <div class="sub">{def.short}</div>
    </div>
    <div class="chips">
      {#if st.state === "computing"}<span class="chip"><span class="spin" aria-hidden="true"></span>Computing…</span>{/if}
      {#if cyclic}<Tag tone="blk">{plural(r.cycle_count, "cycle", "cycles")}</Tag>{/if}
      <button type="button" class="ibtn" aria-expanded={info} aria-label="About {def.title}" title="About this metric" onclick={oninfo}>i</button>
    </div>
  </div>

  {#if info}
    <div class="info">
      {#each explain as e (e.h)}<p><strong>{e.h}:</strong> <Inlines nodes={e.t} /></p>{/each}
    </div>
  {/if}

  <div class="mbody">
    {#if st.state === "skeleton"}
      {#each [0, 1, 2] as k (k)}<div class="sk" aria-hidden="true"><i class={k % 2 ? "w50" : "w70"}></i></div>{/each}
      <span class="sr" role="status">Computing {def.title}…</span>
    {:else if st.state === "not-computed"}
      <div class="none">Not computed (graph analysis failed)</div>
    {:else if st.state === "skipped"}
      <Notice tone="skipped" text="Skipped: {st.reason}" />
    {:else if st.state === "none"}
      <div class="none" class:ok={isCycles}>{isCycles ? "No cycles" : "None found"}</div>
    {:else if isCycles}
      {#each chains as c, i (i)}<CycleChain chain={c} {titleOf} {onpeek} />{/each}
    {:else}
      {#each rows as row, i (row.id)}
        <div class="mrow" class:nov={!hasValue} data-row data-id={row.id}>
          <span class="rk">{i + 1}</span>
          <span class="mid" title={row.id}>{row.id}</span>
          <span class="mt">
            <button type="button" class="t-btn mtt" title={row.title || row.id} onclick={() => onpeek(row.id)}>{row.title || row.id}</button>
            {#if hasValue}<ValueBar pct={row.pct} />{/if}
          </span>
          {#if hasValue}
            <span class="mv">
              {#if def.key === "slack" && row.value === 0}<Tag tone="start">critical path</Tag>{/if}
              {formatMetric(row.value, def.format)}
            </span>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  {#if hasList && n > CARD_ROWS}
    <div class="mfoot">
      <button type="button" class="link" aria-expanded={wide} onclick={onall}>{wide ? `Show top ${CARD_ROWS}` : `Show all (${n})`}</button>
      {#if wide && canLoadMore(r, def.key, limit)}
        <button type="button" class="btn" onclick={onmore}>Load more</button>
      {/if}
    </div>
  {/if}
</section>

<style>
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    min-width: 0;
  }
  .mcard {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .mcard.wide {
    grid-column: 1 / -1;
  }
  .mcard.danger {
    border-color: var(--danger);
  }
  .mh {
    display: flex;
    gap: 8px;
    align-items: flex-start;
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
  .sub {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .chips {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: var(--text-xs);
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text-muted);
    border: 1px solid var(--border);
    white-space: nowrap;
  }
  .spin {
    width: 8px;
    height: 8px;
    border: 2px solid var(--border-strong);
    border-top-color: var(--accent);
    border-radius: 50%;
    display: inline-block;
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .ibtn {
    width: 24px;
    height: 24px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: var(--text-sm);
    font-style: italic;
    font-family: Georgia, serif;
    flex: none;
    padding: 0;
    cursor: pointer;
  }
  .ibtn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .ibtn[aria-expanded="true"] {
    border-color: var(--accent);
    color: var(--accent);
  }
  .info {
    font-size: var(--text-sm);
    color: var(--text-muted);
    border-left: 3px solid var(--border-strong);
    padding: 2px 0 2px 10px;
  }
  .info p {
    margin: 3px 0;
  }
  .info strong {
    color: var(--text);
  }
  .none {
    font-size: var(--text-sm);
    color: var(--text-muted);
    padding: 6px 0;
  }
  .none.ok {
    color: var(--success);
  }
  .mrow {
    display: grid;
    grid-template-columns: 18px 88px minmax(0, 1fr) auto;
    column-gap: 8px;
    align-items: center;
    min-height: 28px;
    padding: 3px 4px;
    margin: 0 -4px;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .mrow.nov {
    grid-template-columns: 18px 88px minmax(0, 1fr);
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
  .mt {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
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
  .mv {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    display: flex;
    gap: 6px;
    align-items: center;
    justify-content: flex-end;
  }
  .mfoot {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
    margin-top: 2px;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .link:hover {
    text-decoration: underline;
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
  .sk {
    height: 28px;
    display: flex;
    align-items: center;
  }
  .sk i {
    display: block;
    height: 10px;
    border-radius: 5px;
    background: linear-gradient(90deg, var(--bg-subtle), var(--bg-hover), var(--bg-subtle));
    background-size: 200% 100%;
    animation: shim 1.4s linear infinite;
  }
  .w50 {
    width: 50%;
  }
  .w70 {
    width: 70%;
  }
  @keyframes shim {
    from {
      background-position: 200% 0;
    }
    to {
      background-position: -200% 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sk i,
    .spin {
      animation: none;
    }
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
