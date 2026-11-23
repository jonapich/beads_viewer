<script lang="ts">
  import { onMount } from "svelte";
  import Header from "./components/Header.svelte";
  import Nav from "./components/Nav.svelte";
  import SignedOut from "./components/SignedOut.svelte";
  import { refreshHealth } from "./lib/health";
  import { route, startRouter } from "./lib/router/router.svelte";
  import { session } from "./lib/session.svelte";
  import { startTheme } from "./lib/theme.svelte";
  import { routes } from "./routes";

  // One-time setup: onMount runs untracked, so reading theme.pref inside
  // startTheme() does not re-run this on every toggle.
  onMount(() => {
    const stopTheme = startTheme();
    const stopRouter = startRouter(routes);
    // A restarted server means a new token, so re-check when the tab returns.
    const onVisible = () => {
      if (document.visibilityState === "visible") void refreshHealth();
    };
    document.addEventListener("visibilitychange", onVisible);
    void refreshHealth();
    return () => {
      stopTheme();
      stopRouter();
      document.removeEventListener("visibilitychange", onVisible);
    };
  });
</script>

{#if session.status === "signed-out"}
  <SignedOut />
{:else}
  <div class="shell">
    <Header />
    <div class="body">
      <Nav />
      <main>
        {#if session.status === "offline"}
          <div class="banner" role="alert">
            <span>Can't reach bv serve. It may have stopped.</span>
            <button type="button" onclick={() => void refreshHealth()}>Retry</button>
          </div>
        {/if}
        {#if route.def}
          {@const View = route.def.component}
          <View {...route.def.props} />
        {/if}
      </main>
    </div>
  </div>
{/if}

<style>
  .shell {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .body {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  main {
    flex: 1;
    min-width: 0;
    overflow: auto;
    padding: var(--space-5);
  }
  .banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--danger);
    border-radius: var(--radius-md);
    color: var(--danger);
    background: var(--bg-raised);
  }
  .banner button {
    padding: var(--space-1) var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    cursor: pointer;
  }
  .banner button:hover {
    background: var(--bg-hover);
  }
</style>