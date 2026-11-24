<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    /** Stable key: the data-sec attribute and the key of the open state the view keeps. */
    name: string;
    title: string;
    count?: number;
    open?: boolean;
    /** Controls on the right of the header (the Edit button of a text field). Nothing is rendered when absent. */
    actions?: Snippet;
    children: Snippet;
  }

  let { name, title, count, open = $bindable(true), actions, children }: Props = $props();
</script>

<details class="sec" data-sec={name} bind:open>
  <summary>
    <svg class="chev" viewBox="0 0 12 12" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 2l4 4-4 4" /></svg>
    <span>{title}</span>
    {#if count !== undefined}<span class="cnt">{count}</span>{/if}
    {#if actions}<span class="sumact">{@render actions()}</span>{/if}
  </summary>
  <div class="body">{@render children()}</div>
</details>

<style>
  .sec {
    border-top: 1px solid var(--border);
  }
  summary {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 36px;
    list-style: none;
    cursor: pointer;
    font-size: var(--text-base);
    font-weight: 600;
    user-select: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .chev {
    flex: none;
    color: var(--text-faint);
    transition: transform 0.12s;
  }
  .sec[open] > summary .chev {
    transform: rotate(90deg);
  }
  .cnt {
    font-size: var(--text-xs);
    font-weight: 500;
    background: var(--bg-hover);
    border-radius: 10px;
    padding: 0 6px;
    line-height: 18px;
    color: var(--text-muted);
  }
  .sumact {
    margin-left: auto;
    display: flex;
    gap: var(--space-2);
    align-items: center;
    font-weight: 400;
  }
  .body {
    padding: var(--space-1) 0 var(--space-4) 20px;
    min-width: 0;
  }
  @media (max-width: 479px) {
    .body {
      padding-left: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .chev {
      transition: none;
    }
  }
</style>
