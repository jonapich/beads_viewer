<script lang="ts">
  import StatusPill from "./StatusPill.svelte";
  import ThemeToggle from "./ThemeToggle.svelte";
</script>

<header>
  <a class="brand" href="/">bv</a>
  <div class="right">
    <StatusPill />
    <ThemeToggle />
  </div>
</header>

<style>
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: var(--header-h);
    padding: 0 var(--space-4);
    background: var(--bg-subtle);
    border-bottom: 1px solid var(--border);
  }
  .brand {
    font-size: var(--text-lg);
    font-weight: 600;
    color: var(--text);
    text-decoration: none;
  }
  .right {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
</style>