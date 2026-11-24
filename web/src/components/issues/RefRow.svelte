<script lang="ts">
  import type { DepRef, IssueRef } from "../../lib/api/types";
  import { refStatus } from "../../lib/issues/peek";
  import PriorityPill from "./PriorityPill.svelte";
  import StatusGlyph from "./StatusGlyph.svelte";

  interface Props {
    ref: IssueRef | DepRef;
    /** Settled or no longer blocking: drawn dimmer. */
    muted?: boolean;
    /** Show the priority at the end of the row (the page does; the compact drawer does not). */
    showPriority?: boolean;
    /** A small label after the title (the kind of a Related link). */
    tag?: string;
    /** Opens the issue in place (the peek drawer): a button. Without it the row is a link to the issue's page. */
    onopen?: (id: string) => void;
  }

  let { ref, muted = false, showPriority = false, tag, onopen }: Props = $props();
</script>

<!-- One row for another issue, shared by the peek drawer and the detail page so they cannot drift.
     The status glyph colours come from StatusGlyph (the list's mapping): to give these rows another
     palette, change it here. -->
{#if ref.missing}
  <span class="lnk gone"><span class="i">{ref.id}</span><span class="t">not in the tracker</span></span>
{:else}
  {#snippet content()}
    <StatusGlyph status={refStatus(ref.status)} />
    <span class="i">{ref.id}</span>
    <span class="t">{ref.title ?? ""}</span>
    {#if tag}<span class="tag">{tag}</span>{/if}
    {#if showPriority && ref.priority !== undefined}<PriorityPill priority={ref.priority} />{/if}
  {/snippet}
  {#if onopen}
    <button type="button" class="lnk" class:muted onclick={() => onopen(ref.id)}>{@render content()}</button>
  {:else}
    <a class="lnk page" class:muted href="/issues/{encodeURIComponent(ref.id)}">{@render content()}</a>
  {/if}
{/if}

<style>
  .lnk {
    display: flex;
    gap: 8px;
    align-items: center;
    width: 100%;
    text-align: left;
    padding: 4px 6px;
    margin: 0 -6px;
    border: 0;
    background: none;
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
    color: inherit;
    text-decoration: none;
    cursor: pointer;
  }
  .lnk.page {
    min-height: 32px;
    font-size: var(--text-base);
  }
  .lnk:hover {
    background: var(--bg-hover);
  }
  .lnk.muted {
    color: var(--text-muted);
  }
  .lnk.gone {
    cursor: default;
    color: var(--text-faint);
  }
  .lnk.gone:hover {
    background: none;
  }
  /* The shared cells are laid out by order inside a list row; here they sit in a plain flex line. */
  .lnk :global(.sg) {
    order: 0;
  }
  .t {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .i {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
    flex: none;
  }
  .tag {
    flex: none;
    font-size: var(--text-xs);
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    color: var(--text-muted);
  }
</style>
