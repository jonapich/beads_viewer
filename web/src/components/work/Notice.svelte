<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    tone: "info" | "warning" | "danger" | "skipped";
    text?: string;
    /** A button at the right (Retry). */
    action?: { label: string; run: () => void };
    children?: Snippet;
  }

  let { tone, text = "", action, children }: Props = $props();
</script>

<div class="notice {tone}" role={tone === "danger" ? "alert" : "status"}>
  <span class="nt">{#if children}{@render children()}{:else}{text}{/if}</span>
  {#if action}<button type="button" class="btn" onclick={action.run}>{action.label}</button>{/if}
</div>

<style>
  .notice {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    padding: 8px 12px;
    margin: 0 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .nt {
    flex: 1 1 240px;
    min-width: 0;
  }
  .info {
    border-color: var(--accent);
    background: var(--accent-subtle);
    color: var(--text);
  }
  .warning {
    border-color: var(--warning);
    background: var(--warning-subtle);
    color: var(--warning);
  }
  .danger {
    border-color: var(--danger);
    background: var(--danger-subtle);
    color: var(--danger);
  }
  .skipped {
    border-style: dashed;
    margin: 4px 0;
    padding: 12px 16px;
  }
  .btn {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
    white-space: nowrap;
  }
  .danger .btn {
    border-color: var(--danger);
    color: var(--danger);
  }
  .btn:hover {
    background: var(--bg-hover);
  }
</style>
