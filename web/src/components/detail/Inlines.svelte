<script lang="ts">
  import type { Inline } from "../../lib/markdown";

  /** Already-parsed inline runs (lib/markdown.ts). Text is interpolated, so it is escaped; hrefs are http(s) only. */
  let { nodes }: { nodes: readonly Inline[] } = $props();
</script>

{#each nodes as n, i (i)}{#if n.t === "text"}{n.v}{:else if n.t === "code"}<code>{n.v}</code>{:else if n.t === "strong"}<strong>{n.v}</strong>{:else if n.t === "link"}<a href={n.href} target="_blank" rel="noopener noreferrer">{n.v}</a>{:else}<br />{/if}{/each}

<style>
  code {
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0 4px;
  }
</style>
