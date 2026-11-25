<script lang="ts">
  import { onDestroy } from "svelte";
  import { COPIED_MS, copyIssueId } from "../lib/copy";
  import { toasts } from "../lib/toasts.svelte";

  /** The issue id to copy: the plain id, nothing around it. */
  let { id }: { id: string } = $props();

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy(e: MouseEvent): Promise<void> {
    // The button can sit inside a row or a link: its click is its own.
    e.stopPropagation();
    e.preventDefault();
    if (!(await copyIssueId(id, { clipboard: navigator.clipboard, toasts }))) return;
    copied = true;
    clearTimeout(timer);
    timer = setTimeout(() => (copied = false), COPIED_MS);
  }

  onDestroy(() => clearTimeout(timer));
</script>

<button type="button" class="copy" class:done={copied} aria-label="Copy issue id" title="Copy issue id" onclick={(e) => void copy(e)}>
  {#if copied}
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 8.5l3.2 3.2L13 4.8" /></svg>
  {:else}
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="5" width="8" height="8" rx="1.5" /><path d="M3 10V4a1 1 0 011-1h6" /></svg>
  {/if}
</button>

<style>
  .copy {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .copy:hover {
    background: var(--bg-hover);
    border-color: var(--border);
  }
  .copy.done {
    color: var(--success);
  }
</style>