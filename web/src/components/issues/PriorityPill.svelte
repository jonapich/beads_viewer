<script lang="ts">
  // A <span> for now: priority is read-only until the write actions (beads-c2o.19).
  let { priority }: { priority: number } = $props();
</script>

<span class="pp" data-p={priority} title="Priority P{priority}">P{priority}</span>

<style>
  .pp {
    order: 9;
    width: 26px;
    height: 18px;
    flex: none;
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 18px;
    text-align: center;
    background: transparent;
    color: var(--text-faint);
  }
  .pp[data-p="0"] {
    background: var(--danger);
    color: var(--on-accent);
  }
  .pp[data-p="1"] {
    background: var(--warning-subtle);
    color: var(--warning);
  }
  .pp[data-p="2"] {
    background: var(--bg-subtle);
    color: var(--text-muted);
  }
</style>
