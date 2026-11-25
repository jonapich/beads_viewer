<script lang="ts">
  import { live, type LiveStatus } from "../lib/live.svelte";
  import { session } from "../lib/session.svelte";

  const liveLabel: Record<LiveStatus, string> = { live: "live", connecting: "reconnecting", down: "polling", off: "off" };
  // What the retired Overview page showed: where bv serve is, and how updates reach the page.
  const title = $derived(`bv serve at ${location.origin} · live updates: ${liveLabel[live.status]}`);

  const state = $derived(
    session.status === "ok"
      ? { kind: "ok", label: session.health?.version || "dev" }
      : session.status === "offline"
        ? { kind: "offline", label: "disconnected" }
        : { kind: "checking", label: "connecting" },
  );
</script>

<span class="pill" {title}>
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