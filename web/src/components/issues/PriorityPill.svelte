<script lang="ts">
  import { READ_ONLY_TITLE } from "../../lib/issues/writes";

  interface Props {
    priority: number;
    /**
     * Makes the pill a button that asks for the priority popover (it gets the button, to anchor
     * the popover). Without it the pill is a plain label.
     */
    onpick?: (anchor: HTMLElement) => void;
    /** A write on this issue is in flight. */
    busy?: boolean;
    /** The tracker takes no writes. */
    disabled?: boolean;
    /** Its popover is open. */
    expanded?: boolean;
  }

  let { priority, onpick, busy = false, disabled = false, expanded = false }: Props = $props();
</script>

{#if onpick}
  <button
    type="button"
    class="pp btn"
    class:busy
    data-p={priority}
    data-pop-anchor="prio"
    aria-label="Priority P{priority}, change"
    title={disabled ? READ_ONLY_TITLE : "Change priority"}
    {disabled}
    aria-haspopup="dialog"
    aria-expanded={expanded}
    aria-controls={expanded ? "row-actions-pop" : undefined}
    aria-busy={busy ? "true" : undefined}
    aria-disabled={busy ? "true" : undefined}
    onclick={(e) => {
      // Busy is not `disabled`, which would drop the focus a closed popover hands back.
      if (!busy) onpick(e.currentTarget);
    }}>{busy ? "…" : `P${priority}`}</button
  >
{:else}
  <span class="pp" data-p={priority} title="Priority P{priority}">P{priority}</span>
{/if}

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
  .btn {
    padding: 0;
    border: 0;
    cursor: pointer;
  }
  .btn:hover:not(:disabled) {
    outline: 1px solid var(--border-strong);
  }
  .btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .btn.busy {
    opacity: 0.5;
    pointer-events: none;
  }
</style>
