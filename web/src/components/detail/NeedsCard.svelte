<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    /** The accent of the left border (and the bell). */
    tone: "warning" | "accent" | "success" | "danger";
    /** Accessible name of the card. */
    label: string;
    /** The one-line note that replaces a card once its action is done: tighter, header only. */
    compact?: boolean;
    /** A bell before the title. */
    bell?: boolean;
    title: Snippet;
    /** Right of the title (the sign-off pill). */
    aside?: Snippet;
    /** A small line under the header. */
    sub?: string;
    children?: Snippet;
  }

  let { tone, label, compact = false, bell = true, title, aside, sub, children }: Props = $props();
</script>

<section class="needs" class:compact data-needs={label} data-tone={tone} aria-label={label}>
  <div class="nh">
    {#if bell}
      <svg class="bell" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 11V7a4 4 0 118 0v4l1 1.5H3z" /><path d="M6.5 14h3" /></svg>
    {/if}
    <span class="ttl">{@render title()}</span>
    {#if aside}<span class="aside">{@render aside()}</span>{/if}
  </div>
  {#if sub}<div class="nsub">{sub}</div>{/if}
  {@render children?.()}
</section>

<style>
  .needs {
    --tone: var(--warning);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-left: 3px solid var(--tone);
    border-radius: var(--radius-md);
    padding: var(--space-4);
    margin-bottom: var(--space-4);
  }
  .needs[data-tone="accent"] {
    --tone: var(--accent);
  }
  .needs[data-tone="success"] {
    --tone: var(--success);
  }
  .needs[data-tone="danger"] {
    --tone: var(--danger);
  }
  .needs.compact {
    padding: 10px var(--space-4);
  }
  .needs.compact .nh {
    color: var(--tone);
  }
  .nh {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-base);
    font-weight: 600;
  }
  .bell {
    flex: none;
    color: var(--tone);
  }
  .ttl {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .aside {
    margin-left: auto;
  }
  .nsub {
    font-size: var(--text-sm);
    color: var(--text-muted);
    margin-top: 2px;
  }
</style>
