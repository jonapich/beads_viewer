<script lang="ts">
  import type { ListItem } from "../../lib/api/types";
  import { awaitsSignoff, displayStatus, needsAnswer, type IssueIndex } from "../../lib/issues/model";
  import { ago, triageText } from "../../lib/issues/present";
  import type { IssueRow } from "../../lib/issues/tree";
  import AssigneeDot from "./AssigneeDot.svelte";
  import LabelChips from "./LabelChips.svelte";
  import PriorityPill from "./PriorityPill.svelte";
  import StatusGlyph from "./StatusGlyph.svelte";
  import TitleLine from "./TitleLine.svelte";

  interface Props {
    row: IssueRow;
    item: ListItem;
    index: IssueIndex;
    /** The parent row's issue, for the "Blocked by" hint on a blocked placement. */
    via: ListItem | undefined;
    now: number;
    selected: boolean;
    flash: boolean;
    ontoggle: (id: string) => void;
    onpeek: (id: string) => void;
  }

  let { row, item, index, via, now, selected, flash, ontoggle, onpeek }: Props = $props();

  const status = $derived(displayStatus(item, now));
  const edge = $derived(row.edges[row.edges.length - 1]);
  const attention = $derived(awaitsSignoff(item) ? "Sign off" : needsAnswer(item) ? "Answer" : null);
</script>

<div class="row" class:closed={item.status === "closed"} class:ctx={row.context} class:sel={selected} class:flash data-row data-id={item.id}>
  <div class="rl1">
    <span class="indent">
      {#each row.edges as e, k (k)}<span class="g" class:gb={e === "block"}></span>{/each}
    </span>
    {#if edge === "block"}
      <span class="conn" title={via ? `Blocked by ${via.id} ${via.title}` : "Blocked"}>⤷</span>
    {/if}
    {#if row.hasKids}
      <button
        type="button"
        class="caret"
        aria-expanded={row.expanded}
        aria-label="Expand {item.id}"
        onclick={() => ontoggle(item.id)}>{row.expanded ? "▾" : "▸"}</button
      >
    {:else}
      <span class="caret-sp"></span>
    {/if}
    <StatusGlyph {status} dim={row.context} />
    <TitleLine {item} {index} {row} muted={item.status === "closed" || row.context} {onpeek} />
  </div>
  <div class="rl2">
    <span class="c-id" class:dim={row.context} title={item.id}>{item.id}</span>
    {#if attention}
      <button type="button" class="att" data-kind={attention === "Sign off" ? "sign" : "answer"} onclick={() => onpeek(item.id)}>{attention}</button>
    {/if}
    <LabelChips labels={item.labels} />
    <AssigneeDot assignee={item.assignee} />
    <PriorityPill priority={item.priority} />
    <span class="c-tri" class:dash={item.triage_score === undefined} title="Triage score">{triageText(item.triage_score)}</span>
    <span class="c-upd" title="Updated">{ago(item.updated_at, now)}</span>
  </div>
</div>

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
  .row.flash {
    animation: flash 1.2s ease-out;
  }
  @keyframes flash {
    0%,
    70% {
      background: var(--bg-selected);
      box-shadow: inset 2px 0 0 var(--accent);
    }
    100% {
      background: transparent;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .row.flash {
      animation: none;
    }
  }
  /* The two wrappers exist for the narrow layout; wide rows lay out as one flex line. */
  .rl1,
  .rl2 {
    display: contents;
  }
  .indent {
    order: 1;
    display: flex;
    flex: none;
    align-self: stretch;
  }
  .g {
    display: block;
    width: 16px;
    border-left: 1px solid var(--border);
  }
  .g.gb {
    border-left: 1px dashed color-mix(in srgb, var(--danger) 50%, transparent);
  }
  .conn {
    order: 1;
    color: var(--danger);
    width: 14px;
    flex: none;
    text-align: center;
    font-size: var(--text-sm);
  }
  .caret,
  .caret-sp {
    order: 2;
    width: 24px;
    height: 24px;
    flex: none;
    text-align: center;
    line-height: 24px;
    color: var(--text-muted);
    font-size: 12px;
    border-radius: var(--radius-sm);
  }
  .caret {
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
  }
  .caret:hover {
    background: var(--bg-hover);
    color: var(--text);
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
  .c-id.dim {
    opacity: 0.7;
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
  @container list (min-width: 1040px) {
    .c-upd {
      display: block;
    }
  }
  /* Stopgap for narrow lists: the two-line row layout (rl1/rl2) comes with the narrow-layout
     slice. Until then drop the columns that would overflow the single line. */
  @container list (max-width: 559px) {
    .c-tri,
    .c-id,
    .att {
      display: none;
    }
  }
</style>
