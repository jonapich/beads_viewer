<script lang="ts">
  import { ApiError, postReveal } from "../../lib/api/client";
  import { appCaps } from "../../lib/desk/nav-counts.svelte";
  import { toasts } from "../../lib/toasts.svelte";

  interface Props {
    /** The absolute path of the file to show. */
    abs: string;
    /** Draw it as a text button (the lightbox footer) instead of an icon. */
    text?: boolean;
  }
  let { abs, text = false }: Props = $props();

  let busy = $state(false);

  async function reveal(e: MouseEvent): Promise<void> {
    e.stopPropagation();
    e.preventDefault();
    if (busy) return;
    busy = true;
    try {
      await postReveal(abs);
    } catch (err) {
      toasts.push("danger", err instanceof ApiError || err instanceof Error ? err.message : "Could not show the file.");
    } finally {
      busy = false;
    }
  }
</script>

<!-- Only where the server can do it (Windows, a writable tracker): reveal only selects the file in Explorer, it never opens it. -->
{#if appCaps.reveal && abs}
  <button type="button" class="rv" class:txt={text} aria-label="Show in Explorer" title="Show in Explorer" disabled={busy} onclick={(e) => void reveal(e)}>
    {#if text}Show in Explorer{:else}
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true"><path d="M1.5 4.5a1 1 0 011-1h3.2l1.4 1.5h6.4a1 1 0 011 1v6a1 1 0 01-1 1h-11a1 1 0 01-1-1z" /></svg>
    {/if}
  </button>
{/if}

<style>
  .rv {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    margin-left: 2px;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-faint);
    vertical-align: baseline;
    cursor: pointer;
  }
  .rv:hover:not(:disabled) {
    color: var(--text);
    background: var(--bg-hover);
    border-color: var(--border);
  }
  .rv:disabled {
    opacity: 0.5;
    cursor: progress;
  }
  .rv.txt {
    width: auto;
    height: 28px;
    margin: 0;
    padding: 0 var(--space-2);
    border-color: var(--border-strong);
    background: var(--bg-raised);
    color: var(--text);
    font-size: var(--text-sm);
    white-space: nowrap;
  }
</style>