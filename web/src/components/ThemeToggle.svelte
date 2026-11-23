<script lang="ts">
  import { setThemePref, theme, type ThemePref } from "../lib/theme.svelte";

  const order: ThemePref[] = ["system", "light", "dark"];
  const label = $derived(`Theme: ${theme.pref[0]?.toUpperCase()}${theme.pref.slice(1)}`);

  function cycle(): void {
    const next = order[(order.indexOf(theme.pref) + 1) % order.length] ?? "system";
    setThemePref(next);
  }
</script>

<button type="button" class="toggle" aria-label={label} title={label} onclick={cycle}>
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
    {#if theme.pref === "light"}
      <circle cx="8" cy="8" r="3" />
      <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.4 1.4M11.6 11.6L13 13M3 13l1.4-1.4M11.6 4.4L13 3" />
    {:else if theme.pref === "dark"}
      <path d="M13.5 9.5A6 6 0 0 1 6.5 2.5a6 6 0 1 0 7 7z" />
    {:else}
      <rect x="2" y="3" width="12" height="8" rx="1" />
      <path d="M6 14h4M8 11v3" />
    {/if}
  </svg>
</button>

<style>
  .toggle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .toggle:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
</style>