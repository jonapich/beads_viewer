<script lang="ts">
  import { initials } from "../../lib/issues/present";

  let { assignee }: { assignee: string | undefined } = $props();
</script>

{#if assignee}
  <span class="c-asg" title={assignee}>{initials(assignee)}</span>
{:else}
  <span class="c-asg none" title="Unassigned"></span>
{/if}

<style>
  .c-asg {
    order: 8;
    width: 20px;
    height: 20px;
    flex: none;
    border-radius: 50%;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    font-size: 9px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    font-weight: 600;
  }
  .c-asg.none {
    background: transparent;
    border: 1px dashed var(--border-strong);
  }
  @container list (max-width: 559px) {
    .c-asg {
      display: none;
    }
  }
</style>
