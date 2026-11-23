<script lang="ts">
  import type { Snippet } from "svelte";
  import type { ListItem } from "../../lib/api/types";
  import { highlight } from "../../lib/issues/highlight";
  import type { IssueIndex } from "../../lib/issues/model";
  import { percent } from "../../lib/issues/present";
  import type { EdgeKind, IssueRow } from "../../lib/issues/tree";

  interface Props {
    item: ListItem;
    index: IssueIndex;
    /** Tree rows only: the placement, for the collapsed count and the "Blocked by" tag. */
    row?: IssueRow;
    muted?: boolean;
    /** Flat results: the search text to mark in the title. */
    query?: string;
    /** Flat results: a second line under the title (the breadcrumb). */
    below?: Snippet;
    onpeek: (id: string) => void;
  }

  let { item, index, row, muted = false, query = "", below, onpeek }: Props = $props();

  const segments = $derived(highlight(item.title, query));

  const epic = $derived(item.issue_type === "epic");
  const kids = $derived(epic ? (index.childrenByParent.get(item.id)?.length ?? 0) : 0);
  const done = $derived(index.closedChildren.get(item.id) ?? 0);
  const pct = $derived(percent(done, kids));
  const ready = $derived(item.ready && item.status === "open" && !epic);
  // Under its blocker the placement already says "blocked", so the tag is for the other rows.
  const lastEdge = $derived<EdgeKind>(row?.edges[row.edges.length - 1] ?? "child");
  const showBlocked = $derived(item.blocked && item.blocked_by.length > 0 && lastEdge === "child");
  const collapsed = $derived(!!row && row.hasKids && !row.expanded);
</script>

<span class="c-title">
  <span class="t-line">
    {#if ready}<span class="tag ready">Ready</span>{/if}
    <button type="button" class="t-btn" class:muted class:epic onclick={() => onpeek(item.id)}>{#each segments as s, i (i)}{#if s.m}<mark>{s.t}</mark>{:else}{s.t}{/if}{/each}</button>
    {#if item.issue_type !== "task"}
      <span class="tag type" class:bug={item.issue_type === "bug"}>{item.issue_type}</span>
    {/if}
    {#if epic && kids > 0}
      <span class="pn">{done}/{kids}</span>
      <span class="prog" title="{done} of {kids} children closed"><i class="pct-{pct}"></i></span>
    {/if}
    {#if showBlocked}
      <span class="tag blk" title="Blocked by {item.blocked_by.join(', ')}">Blocked by {item.blocked_by.length}</span>
    {/if}
    {#if row && collapsed}
      {#if row.descCount > 0}<span class="cnt">({row.descCount})</span>{/if}
      {#if row.attention}
        <span
          class="dot"
          data-attn={row.attention}
          title={row.attention === "danger" ? "Contains blocked issues" : "Contains issues waiting on a decision"}
        ></span>
      {/if}
    {/if}
  </span>
  {#if below}{@render below()}{/if}
</span>

<style>
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
  .t-line > *:not(.t-btn) {
    flex: none;
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
  .t-btn.muted {
    color: var(--text-muted);
  }
  .t-btn.epic {
    font-weight: 600;
  }
  .tag {
    font-size: var(--text-xs);
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    white-space: nowrap;
  }
  .tag.ready {
    background: var(--success-subtle);
    color: var(--success);
  }
  .tag.blk {
    background: var(--danger-subtle);
    color: var(--danger);
  }
  .tag.type {
    padding: 0;
    color: var(--text-muted);
  }
  .tag.type.bug {
    color: var(--danger);
  }
  .cnt {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    display: inline-block;
  }
  .dot[data-attn="danger"] {
    background: var(--danger);
  }
  .dot[data-attn="warning"] {
    background: var(--warning);
  }
  .pn {
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .prog {
    width: 48px;
    height: 4px;
    border-radius: 2px;
    background: var(--border);
    overflow: hidden;
    display: inline-block;
  }
  .prog i {
    display: block;
    height: 100%;
    background: var(--success);
  }
  mark {
    background: var(--warning-subtle);
    color: inherit;
    border-radius: 2px;
  }
</style>
