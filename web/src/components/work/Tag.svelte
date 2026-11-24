<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    tone: "ready" | "blk" | "unb" | "muted" | "start" | "type" | "bug";
    title?: string;
    children: Snippet;
  }

  let { tone, title, children }: Props = $props();
</script>

<span class="tag {tone}" {title}>{@render children()}</span>

<style>
  .tag {
    flex: none;
    font-size: var(--text-xs);
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    white-space: nowrap;
  }
  .ready,
  .unb {
    background: var(--success-subtle);
    color: var(--success);
  }
  .blk {
    background: var(--danger-subtle);
    color: var(--danger);
  }
  .muted {
    background: var(--bg-subtle);
    color: var(--text-muted);
    border: 1px solid var(--border);
  }
  .start {
    background: var(--accent-subtle);
    color: var(--accent);
    font-weight: 600;
  }
  .type {
    padding: 0;
    color: var(--text-muted);
  }
  .bug {
    padding: 0;
    color: var(--danger);
  }
</style>
