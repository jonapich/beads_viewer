<script lang="ts">
  import { toasts } from "../lib/toasts.svelte";

  const icon = { success: "✓", danger: "✕", warning: "!" } as const;
</script>

<!-- The container is always mounted, so its live region exists before the first toast arrives:
     screen readers announce what is added to one that was already there. role="log" (implicitly
     polite) so that the aria-label counts: a name on a role-less div is ignored. -->
<div class="toasts" role="log" aria-live="polite" aria-label="Notifications">
  {#each toasts.items as t (t.id)}
    <div class="toast" data-kind={t.kind} role={t.kind === "danger" ? "alert" : undefined}>
      <span class="ic" aria-hidden="true">{icon[t.kind]}</span>
      <span class="tx">{t.text}</span>
      {#if t.kind === "danger"}
        <button type="button" class="x" aria-label="Dismiss" onclick={() => toasts.dismiss(t.id)}>✕</button>
      {/if}
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 16px;
    bottom: 16px;
    z-index: 70;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-width: min(360px, calc(100vw - 32px));
    pointer-events: none;
  }
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 10px 14px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    font-size: var(--text-sm);
    pointer-events: auto;
  }
  .toast[data-kind="danger"] {
    border-color: var(--danger);
  }
  .toast[data-kind="warning"] {
    border-color: var(--warning);
  }
  .ic {
    flex: none;
    font-weight: 700;
    color: var(--success);
  }
  .toast[data-kind="danger"] .ic {
    color: var(--danger);
  }
  .toast[data-kind="warning"] .ic {
    color: var(--warning);
  }
  .tx {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .x {
    flex: none;
    padding: 0 2px;
    border: 0;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
  }
</style>
