<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { absTime, PILL_GLYPH, PILL_NAME, pillStatus, relTime, shortDate } from "../../lib/issues/detail";
  import { toasts } from "../../lib/toasts.svelte";
  import PriorityPill from "../issues/PriorityPill.svelte";
  import TypeIcon from "../issues/TypeIcon.svelte";

  interface Props {
    detail: IssueDetail;
    /** Wall-clock time for the relative "Updated ..." text; changes with every refetch. */
    now: number;
  }

  let { detail, now }: Props = $props();

  const status = $derived(pillStatus(detail, now));
  const parent = $derived(detail.parent);

  async function copyId(): Promise<void> {
    try {
      await navigator.clipboard.writeText(detail.id);
      toasts.push("success", `Copied ${detail.id}`);
    } catch {
      toasts.push("danger", "Couldn't copy the id.");
    }
  }
</script>

<header class="head">
  {#if parent}
    <nav class="crumb" aria-label="Parent">
      {#if parent.missing}
        <span class="mono">{parent.id}</span>
      {:else}
        <a href="/issues/{encodeURIComponent(parent.id)}" title={parent.title ?? parent.id}>{parent.title ?? parent.id}</a>
        <span class="mono">{parent.id}</span>
      {/if}
    </nav>
  {/if}

  <div class="titlerow">
    <span class="ticon"><TypeIcon type={detail.issue_type} /></span>
    <h1 class="title">{detail.title}</h1>
    <span class="idg">
      {detail.id}
      <button type="button" class="iconbtn" aria-label="Copy id" title="Copy id" onclick={() => void copyId()}>
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="5" width="8" height="8" rx="1.5" /><path d="M3 10V4a1 1 0 011-1h6" /></svg>
      </button>
    </span>
  </div>

  <div class="pillrow">
    <!-- The status pill is read-only here; its menu comes with the inline editors (beads-c2o.14). -->
    <span class="pill st" data-status={status}>
      {#if PILL_GLYPH[status]}<span aria-hidden="true">{PILL_GLYPH[status]}</span>{/if}{PILL_NAME[status]}
    </span>
    <PriorityPill priority={detail.priority} />
    <span class="dt" title={absTime(detail.updated_at)}>Updated {relTime(detail.updated_at, now)}</span>
    {#if detail.status === "closed" && detail.closed_at}
      <span class="dt" title={absTime(detail.closed_at)}>· Closed {shortDate(detail.closed_at)}</span>
    {/if}
  </div>
</header>

<style>
  .crumb {
    font-size: var(--text-sm);
    color: var(--text-muted);
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
    margin-bottom: var(--space-2);
  }
  .crumb .mono {
    flex: 0 0 auto;
    white-space: nowrap;
    font-family: var(--font-mono);
  }
  .crumb a {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 60ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-block;
    color: var(--text-muted);
    text-decoration: none;
    vertical-align: bottom;
  }
  .crumb a:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .titlerow {
    display: flex;
    align-items: flex-start;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .ticon {
    flex: none;
    margin-top: 6px;
    display: inline-flex;
  }
  .title {
    flex: 1 1 0;
    min-width: 0;
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
    line-height: 1.3;
    overflow-wrap: anywhere;
  }
  .idg {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    margin-top: 5px;
    margin-left: auto;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .iconbtn {
    border: 0;
    background: transparent;
    width: 24px;
    height: 24px;
    border-radius: var(--radius-sm);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    cursor: pointer;
  }
  .iconbtn:hover {
    background: var(--bg-hover);
  }
  @media (max-width: 479px) {
    .idg {
      flex-basis: 100%;
      margin: 0 0 0 24px;
    }
  }
  .pillrow {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }
  .dt {
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .pill {
    --c: var(--text-muted);
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 22px;
    padding: 0 var(--space-2);
    border-radius: 11px;
    font-size: var(--text-sm);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 14%, transparent);
    border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
  }
  .pill[data-status="open"] {
    --c: var(--status-open);
  }
  .pill[data-status="in_progress"] {
    --c: var(--status-progress);
  }
  .pill[data-status="blocked"] {
    --c: var(--status-blocked);
  }
  .pill[data-status="review"] {
    --c: var(--status-review);
  }
  .pill[data-status="closed"] {
    --c: var(--status-closed);
  }
  .pill[data-status="deferred"] {
    --c: var(--status-deferred);
  }
  .pill[data-status="pinned"] {
    --c: var(--status-pinned);
  }
</style>
