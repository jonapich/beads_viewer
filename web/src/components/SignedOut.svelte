<script lang="ts">
  import { refreshHealth } from "../lib/health";
</script>

<div class="wrap">
  <section class="card">
    <h1>Open bv from your terminal</h1>
    <p>
      This browser has no session for this bv server. Open the link <code>bv serve</code> printed in your terminal; it
      looks like <code>http://127.0.0.1:PORT/auth?token=...</code> Each run of <code>bv serve</code> prints a new link.
    </p>
    <button type="button" onclick={() => void refreshHealth()}>Try again</button>
  </section>
</div>

<style>
  .wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100%;
    padding: var(--space-5);
  }
  .card {
    width: 100%;
    max-width: 480px;
    padding: var(--space-5);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
  }
  p {
    color: var(--text-muted);
  }
  button {
    padding: var(--space-2) var(--space-4);
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--accent-strong);
    color: var(--on-accent);
    cursor: pointer;
  }
</style>