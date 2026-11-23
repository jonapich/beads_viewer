<script lang="ts">
  import type { ListItem } from "../../lib/api/types";
  import { ancestors, crumbLabel } from "../../lib/issues/breadcrumb";
  import { highlight } from "../../lib/issues/highlight";
  import { awaitsSignoff, displayStatus, needsAnswer, type IssueIndex } from "../../lib/issues/model";
  import { ago, triageText } from "../../lib/issues/present";
  import AssigneeDot from "./AssigneeDot.svelte";
  import LabelChips from "./LabelChips.svelte";
  import PriorityPill from "./PriorityPill.svelte";
  import StatusGlyph from "./StatusGlyph.svelte";
  import TitleLine from "./TitleLine.svelte";

  interface Props {
    item: ListItem;
    index: IssueIndex;
    /** The search text, marked in the title and id. */
    query: string;
    now: number;
    selected: boolean;
    onpeek: (id: string) => void;
    /** "Show in tree" and the breadcrumb segments: reveal this id in the tree. */
    onreveal: (id: string) => void;
  }

  let { item, index, query, now, selected, onpeek, onreveal }: Props = $props();

  const status = $derived(displayStatus(item, now));
  const crumbs = $derived(ancestors(item.id, index.byId));
  const idSegments = $derived(highlight(item.id, query));
  const attention = $derived(awaitsSignoff(item) ? "Sign off" : needsAnswer(item) ? "Answer" : null);
</script>

<div class="row" class:closed={item.status === "closed"} class:sel={selected} data-row data-id={item.id}>
  <div class="rl1">
    <StatusGlyph {status} />
    <TitleLine {item} {index} muted={item.status === "closed"} {query} {onpeek} below={crumb} />
  </div>
  <div class="rl2">
    <span class="c-id" title={item.id}>{#each idSegments as s, i (i)}{#if s.m}<mark>{s.t}</mark>{:else}{s.t}{/if}{/each}</span>
    {#if attention}
      <button type="button" class="att" data-kind={attention === "Sign off" ? "sign" : "answer"} onclick={() => onpeek(item.id)}>{attention}</button>
    {/if}
    <LabelChips labels={item.labels} />
    <AssigneeDot assignee={item.assignee} />
    <PriorityPill priority={item.priority} />
    <span class="c-tri" class:dash={item.triage_score === undefined} title="Triage score">{triageText(item.triage_score)}</span>
    <span class="c-upd" title="Updated">{ago(item.updated_at, now)}</span>
    <button type="button" class="showtree" onclick={() => onreveal(item.id)}>Show in tree</button>
  </div>
</div>

{#snippet crumb()}
  <span class="crumb">
    {#if crumbs.length === 0}
      <span>Top level</span>
    {:else}
      {#each crumbs as a, i (a.id)}
        {#if i > 0}<span class="sep" aria-hidden="true">›</span>{/if}
        <button type="button" title={a.id} onclick={() => onreveal(a.id)}>{crumbLabel(a.title)}</button>
      {/each}
    {/if}
  </span>
{/snippet}

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--row-h);
    padding: 0 8px;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
    position: relative;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .row.sel {
    background: var(--bg-selected);
    box-shadow: inset 2px 0 0 var(--accent);
  }
  .rl1,
  .rl2 {
    display: contents;
  }
  .crumb {
    display: flex;
    align-items: center;
    min-width: 0;
    font-size: var(--text-xs);
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
  }
  .crumb button {
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
    color: var(--text-faint);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 200px;
  }
  .crumb button:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .sep {
    padding: 0 4px;
  }
  .c-id {
    order: 4;
    width: 104px;
    flex: none;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  mark {
    background: var(--warning-subtle);
    color: inherit;
    border-radius: 2px;
  }
  .att {
    order: 6;
    flex: none;
    height: 20px;
    padding: 0 8px;
    border-radius: 10px;
    font-size: var(--text-xs);
    font-weight: 600;
    border: 1px solid transparent;
    cursor: pointer;
  }
  .att[data-kind="sign"] {
    background: var(--accent-subtle);
    color: var(--accent);
    border-color: var(--accent);
  }
  .att[data-kind="answer"] {
    background: var(--warning-subtle);
    color: var(--warning);
    border-color: var(--warning);
  }
  .c-tri {
    order: 10;
    width: 64px;
    flex: none;
    text-align: right;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .c-tri.dash {
    color: var(--text-faint);
  }
  .c-upd {
    order: 11;
    display: none;
    width: 56px;
    flex: none;
    font-size: var(--text-xs);
    color: var(--text-muted);
    text-align: right;
  }
  .showtree {
    order: 12;
    flex: none;
    height: 22px;
    padding: 0 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    color: var(--text-muted);
    background: var(--bg-raised);
    cursor: pointer;
  }
  .showtree:hover {
    color: var(--accent);
    border-color: var(--accent);
  }
  @container list (min-width: 1040px) {
    .c-upd {
      display: block;
    }
  }
  /* Stopgap for narrow lists, as in TreeRow: the two-line layout comes with the narrow slice. */
  @container list (max-width: 559px) {
    .c-tri,
    .c-id,
    .att,
    .showtree {
      display: none;
    }
  }
</style>
