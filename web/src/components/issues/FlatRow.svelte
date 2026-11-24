<script lang="ts">
  import type { ListItem } from "../../lib/api/types";
  import { ancestors, crumbLabel } from "../../lib/issues/breadcrumb";
  import { highlight } from "../../lib/issues/highlight";
  import { awaitsSignoff, canAnswer, displayStatus, type IssueIndex } from "../../lib/issues/model";
  import type { PeekSection } from "../../lib/issues/peek";
  import { ago, triageText } from "../../lib/issues/present";
  import AssigneeDot from "./AssigneeDot.svelte";
  import LabelChips from "./LabelChips.svelte";
  import PriorityPill from "./PriorityPill.svelte";
  import RowMenuButton from "./RowMenuButton.svelte";
  import StatusGlyph from "./StatusGlyph.svelte";
  import TitleLine from "./TitleLine.svelte";

  interface Props {
    item: ListItem;
    index: IssueIndex;
    /** The search text, marked in the title and id. */
    query: string;
    now: number;
    selected: boolean;
    /** The list is narrower than the breakpoint: two-line rows. */
    narrow: boolean;
    onpeek: (id: string, section?: PeekSection) => void;
    /** The tracker takes writes (meta.capabilities.writes). */
    writes: boolean;
    /** A write on this issue is in flight. */
    busy: boolean;
    /** The priority pill or the "..." button was pressed: open the row actions popover at it. */
    onprio: (id: string, anchor: HTMLElement) => void;
    onmenu: (id: string, anchor: HTMLElement) => void;
    /** Which of this row's buttons has its popover open. */
    popKind: "prio" | "menu" | null;
    /** "Show in tree" and the breadcrumb segments: reveal this id in the tree. */
    onreveal: (id: string) => void;
  }

  let { item, index, query, now, selected, narrow, onpeek, onreveal, writes, busy, onprio, onmenu, popKind }: Props = $props();

  const status = $derived(displayStatus(item, now));
  const crumbs = $derived(ancestors(item.id, index.byId));
  const idSegments = $derived(highlight(item.id, query));
  const attention = $derived(awaitsSignoff(item) ? "Sign off" : canAnswer(item) ? "Answer" : null);
</script>

<div class="row" class:closed={item.status === "closed"} class:sel={selected} class:narrow class:busy data-row data-id={item.id}>
  <div class="rl1">
    <StatusGlyph {status} />
    <TitleLine {item} {index} muted={item.status === "closed"} {query} {onpeek} below={crumb} />
  </div>
  <div class="rl2">
    <span class="c-id" title={item.id}>{#each idSegments as s, i (i)}{#if s.m}<mark>{s.t}</mark>{:else}{s.t}{/if}{/each}</span>
    {#if attention}
      <button type="button" class="att" data-kind={attention === "Sign off" ? "sign" : "answer"} onclick={() => onpeek(item.id, attention === "Sign off" ? "signoff" : "answer")}>{attention}</button>
    {/if}
    <LabelChips labels={item.labels} />
    <AssigneeDot assignee={item.assignee} />
    <PriorityPill priority={item.priority} {busy} expanded={popKind === "prio"} disabled={!writes} onpick={(el) => onprio(item.id, el)} />
    <span class="c-tri" class:dash={item.triage_score === undefined} title="Triage score">{triageText(item.triage_score)}</span>
    <span class="c-upd" title="Updated">{ago(item.updated_at, now)}</span>
    <button type="button" class="showtree" onclick={() => onreveal(item.id)}>Show in tree</button>
    <RowMenuButton id={item.id} {busy} expanded={popKind === "menu"} disabled={!writes} onmenu={(el) => onmenu(item.id, el)} />
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
  .row.busy {
    cursor: progress;
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
  @container list (max-width: 559px) {
    .c-tri {
      display: none;
    }
  }

  /* Narrow lists: a fixed-height two-line row. Line 1: glyph, title and breadcrumb. Line 2: id,
     attention chip, priority, Show in tree. */
  .row.narrow {
    display: block;
    padding: 4px 8px 0;
  }
  .row.narrow .rl1 {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    height: 38px;
  }
  .row.narrow .rl2 {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 20px;
    padding-left: 22px;
  }
  .row.narrow .c-id {
    width: auto;
  }
  .row.narrow .c-upd {
    display: none;
  }
  .row.narrow .showtree {
    margin-left: auto;
  }
  .row.narrow :global(.menu) {
    height: 20px;
  }</style>
