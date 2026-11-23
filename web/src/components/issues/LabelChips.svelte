<script lang="ts">
  let { labels }: { labels: readonly string[] } = $props();

  const shown = $derived(labels.slice(0, 2));
  const rest = $derived(labels.slice(2));

  function kind(l: string): "need" | "sign" | "cc" | "" {
    if (l === "needs-decision" || l === "needs-design") return "need";
    if (l === "needs-signoff") return "sign";
    if (l === "close-candidate") return "cc";
    return "";
  }
</script>

<span class="c-labels" title={labels.join(", ")}>
  {#each shown as l (l)}
    <span class="lbl" data-kind={kind(l)}>{kind(l) === "need" ? "? " : ""}{l}</span>
  {/each}
  {#if rest.length > 0}
    <span class="lbl" title={rest.join(", ")}>+{rest.length}</span>
  {/if}
</span>

<style>
  .c-labels {
    order: 7;
    display: flex;
    gap: 4px;
    flex: none;
    width: 204px;
    overflow: hidden;
  }
  .lbl {
    flex: none;
    max-width: 100px;
    overflow: hidden;
    text-overflow: ellipsis;
    height: 18px;
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    white-space: nowrap;
    color: var(--text-muted);
  }
  .lbl[data-kind="need"] {
    background: var(--warning-subtle);
    color: var(--warning);
    border-color: transparent;
  }
  .lbl[data-kind="sign"] {
    background: var(--accent-subtle);
    color: var(--accent);
    border-color: transparent;
  }
  .lbl[data-kind="cc"] {
    border: 1px dashed var(--border-strong);
    background: transparent;
    color: var(--text-faint);
  }
  @container list (max-width: 779px) {
    .c-labels {
      display: none;
    }
  }
</style>
