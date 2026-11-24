<script lang="ts">
  interface Props {
    /** This editor's save is in flight. */
    saving: boolean;
    /** Save is not allowed (nothing changed, a problem with the draft, or another write is running). */
    disabled?: boolean;
    /** Show the Save button; the type select saves when the value changes, so it has only Cancel. */
    save?: boolean;
    /** Another write on this issue is in flight: Cancel waits too. */
    busy?: boolean;
    oncancel: () => void;
  }

  let { saving, disabled = false, save = true, busy = false, oncancel }: Props = $props();
</script>

<div class="btnrow">
  {#if save}
    <!-- aria-disabled, not disabled, so the button a keyboard user pressed keeps its focus while saving. -->
    <button type="submit" class="btn sm primary" aria-disabled={disabled || saving ? "true" : undefined} aria-busy={saving ? "true" : undefined} onclick={(e) => {
      if (disabled || saving) e.preventDefault();
    }}>
      {#if saving}<span class="spin" aria-hidden="true"></span>Saving…{:else}Save{/if}
    </button>
  {/if}
  <button type="button" class="btn sm ghost" disabled={saving || busy} onclick={oncancel}>Cancel</button>
</div>

<style>
  .btnrow {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 8px;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 10px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    line-height: 1;
    cursor: pointer;
  }
  .btn:hover:not(:disabled):not([aria-disabled="true"]) {
    background: var(--bg-hover);
  }
  .btn:disabled,
  .btn[aria-disabled="true"] {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .btn[aria-busy="true"] {
    cursor: progress;
  }
  .primary {
    background: var(--accent-strong);
    border-color: var(--accent-strong);
    color: var(--on-accent);
  }
  .primary:hover:not(:disabled):not([aria-disabled="true"]) {
    background: var(--accent-strong);
    filter: brightness(1.1);
  }
  .ghost {
    border-color: transparent;
    background: transparent;
    color: var(--text-muted);
  }
  .spin {
    width: 10px;
    height: 10px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin {
      animation-duration: 2s;
    }
  }
</style>
