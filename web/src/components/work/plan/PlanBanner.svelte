<script lang="ts">
  interface Props {
    id: string;
    title: string;
    /** How many issues finishing it unblocks. */
    unblocks: number;
    onpeek: (id: string) => void;
  }

  let { id, title, unblocks, onpeek }: Props = $props();
</script>

<div class="rbanner" role="status">
  <span class="t"><b>Start with</b> <span class="mono">{id}</span> {title} — unblocks {unblocks}</span>
  <button type="button" class="btn" onclick={() => onpeek(id)}>Open</button>
</div>

<style>
  .rbanner {
    display: flex;
    align-items: center;
    gap: 8px 12px;
    flex-wrap: wrap;
    margin: 8px 0 16px;
    padding: 8px 12px;
    background: var(--accent-subtle);
    border: 1px solid var(--accent);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
  }
  .t {
    flex: 1 1 240px;
    min-width: 0;
  }
  .mono {
    font-family: var(--font-mono);
  }
  .btn {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .btn:hover {
    background: var(--bg-hover);
  }
</style>
