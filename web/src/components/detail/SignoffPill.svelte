<script lang="ts">
  import { signoffPill } from "../../lib/issues/detail";

  /** The sign-off state (pending, approved, disputed, waived) as a coloured pill, by [data-state]. */
  let { state }: { state: string } = $props();

  const pill = $derived(signoffPill(state));
</script>

<span class="pill" data-state={state}>
  {#if pill.glyph}<span aria-hidden="true">{pill.glyph}</span>{/if}{pill.label}
</span>

<style>
  .pill {
    --c: var(--text-muted);
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 22px;
    padding: 0 var(--space-2);
    border-radius: 11px;
    font-size: var(--text-sm);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 14%, transparent);
    border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
  }
  .pill[data-state="pending"] {
    --c: var(--warning);
  }
  .pill[data-state="approved"] {
    --c: var(--success);
  }
  .pill[data-state="disputed"] {
    --c: var(--danger);
  }
  .pill[data-state="waived"] {
    --c: var(--status-closed);
  }
</style>
