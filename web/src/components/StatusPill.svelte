<script lang="ts">
  import { session } from "../lib/session.svelte";

  const state = $derived(
    session.status === "ok"
      ? { kind: "ok", label: session.health?.version || "dev" }
      : session.status === "offline"
        ? { kind: "offline", label: "disconnected" }
        : { kind: "checking", label: "connecting" },
  );
</script>

<span class="pill" title="bv serve status">
  <span class="dot {state.kind}"></span>
  <span class="label">{state.label}</span>
</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--warning);
  }
  .dot.ok {
    background: var(--success);
  }
  .dot.offline {
    background: var(--danger);
  }
</style>