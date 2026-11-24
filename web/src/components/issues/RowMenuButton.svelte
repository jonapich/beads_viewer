<script lang="ts">
  import { READ_ONLY_TITLE } from "../../lib/issues/writes";

  interface Props {
    id: string;
    /** Asks for the row actions popover, anchored to this button. */
    onmenu: (anchor: HTMLElement) => void;
    busy?: boolean;
    /** The tracker takes no writes. */
    disabled?: boolean;
    /** Its popover is open. */
    expanded?: boolean;
  }

  let { id, onmenu, busy = false, disabled = false, expanded = false }: Props = $props();
</script>

<button
  type="button"
  class="menu"
  class:busy
  data-pop-anchor="menu"
  aria-label="Actions for {id}"
  aria-haspopup="dialog"
  aria-expanded={expanded}
  aria-controls={expanded ? "row-actions-pop" : undefined}
  aria-busy={busy ? "true" : undefined}
  aria-disabled={busy ? "true" : undefined}
  title={disabled ? READ_ONLY_TITLE : "Actions"}
  {disabled}
  onclick={(e) => {
    if (!busy) onmenu(e.currentTarget);
  }}>…</button
>

<style>
  .menu {
    order: 13;
    flex: none;
    width: 28px;
    height: 28px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--text-faint);
    font-size: 16px;
    line-height: 1;
    cursor: pointer;
  }
  :global(.row:hover) .menu,
  .menu:hover:not(:disabled) {
    color: var(--text);
  }
  .menu:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .menu:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .menu.busy {
    opacity: 0.5;
    pointer-events: none;
  }
</style>
