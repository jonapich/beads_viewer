<script lang="ts">
  import { session } from "../lib/session.svelte";

  const origin = location.origin;
</script>

<h1>Overview</h1>

<section class="card" aria-label="Server health">
  <h2>bv serve</h2>
  <dl>
    <dt>Status</dt>
    <dd>{session.status === "offline" ? "unreachable" : (session.health?.status ?? "checking")}</dd>
    <dt>Version</dt>
    <dd>{session.health?.version || "dev"}</dd>
    <dt>Origin</dt>
    <dd><code>{origin}</code></dd>
  </dl>
</section>

<style>
  .card {
    max-width: 420px;
    padding: var(--space-4);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
  h2 {
    font-size: var(--text-lg);
  }
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: var(--space-1) var(--space-4);
    margin: 0;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
  }
</style>