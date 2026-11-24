<script lang="ts">
  import type { Snippet } from "svelte";
  import type { DisplayStatus } from "../../lib/issues/model";
  import { triageText } from "../../lib/issues/present";
  import AssigneeDot from "../issues/AssigneeDot.svelte";
  import PriorityPill from "../issues/PriorityPill.svelte";
  import StatusGlyph from "../issues/StatusGlyph.svelte";
  import Tag from "./Tag.svelte";

  interface Props {
    id: string;
    title: string;
    issue_type: string;
    status: DisplayStatus;
    priority: number;
    assignee?: string | undefined;
    triage_score?: number | undefined;
    /** The row's issue is open in the drawer. */
    selected?: boolean;
    /** The tracker takes writes (meta.capabilities.writes). */
    writes: boolean;
    /** A write on this issue is in flight. */
    busy: boolean;
    /** The priority popover is open from this row's pill. */
    prioOpen: boolean;
    onpeek: (id: string) => void;
    /** The priority pill was pressed: open the popover at it. */
    onprio: (id: string, anchor: HTMLElement) => void;
    /** Before the glyph: a rank, a caret. */
    lead?: Snippet;
    /** After the title and type: tags and a muted action text. */
    tags?: Snippet;
    /** Under the title line. */
    below?: Snippet;
  }

  let { id, title, issue_type, status, priority, assignee, triage_score, selected = false, writes, busy, prioOpen, onpeek, onprio, lead, tags, below }: Props = $props();
</script>

<div class="row" class:sel={selected} class:busy data-row data-id={id}>
  {#if lead}{@render lead()}{/if}
  <StatusGlyph {status} />
  <span class="c-id" title={id}>{id}</span>
  <span class="c-title">
    <span class="t-line">
      <button type="button" class="t-btn" class:epic={issue_type === "epic"} {title} onclick={() => onpeek(id)}>{title}</button>
      {#if issue_type !== "task"}<Tag tone={issue_type === "bug" ? "bug" : "type"}>{issue_type}</Tag>{/if}
      {#if tags}{@render tags()}{/if}
    </span>
    {#if below}{@render below()}{/if}
  </span>
  <AssigneeDot {assignee} />
  <PriorityPill {priority} {busy} expanded={prioOpen} disabled={!writes} onpick={(el) => onprio(id, el)} />
  <span class="c-tri" class:dash={triage_score === undefined} title="Triage score">{triageText(triage_score)}</span>
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--row-h, 32px);
    padding: 0 8px;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
    position: relative;
    cursor: pointer;
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
  .c-title {
    order: 5;
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    justify-content: center;
  }
  .t-line {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .t-btn {
    min-width: 0;
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
  .t-btn.epic {
    font-weight: 600;
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
</style>
