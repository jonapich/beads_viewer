<script lang="ts">
  import { badgeCount, badgeTitle } from "../lib/desk/desk";
  import { navCounts } from "../lib/desk/nav-counts.svelte";
  import { route } from "../lib/router/router.svelte";

  const items = [
    { href: "/", label: "Overview" },
    { href: "/issues", label: "Issues" },
    { href: "/desk", label: "Agent desk" },
    { href: "/triage", label: "Triage" },
    { href: "/plan", label: "Plan" },
    { href: "/insights", label: "Insights" },
  ];

  const active = (href: string) =>
    href === "/" ? route.path === "/" : route.path === href || route.path.startsWith(href + "/");

  // The Agent desk badge: questions plus sign-offs awaiting the user, hidden at 0.
  const deskBadge = $derived(badgeCount(navCounts));
  const deskTitle = $derived(badgeTitle(navCounts));
</script>

<nav aria-label="Main">
  {#each items as item (item.href)}
    <a href={item.href} class:active={active(item.href)} aria-current={active(item.href) ? "page" : undefined}>
      {item.label}
      {#if item.href === "/desk" && deskBadge > 0}
        <span class="badge" title={deskTitle} aria-label={deskTitle}>{deskBadge}</span>
      {/if}
    </a>
  {/each}
</nav>

<style>
  nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: var(--sidebar-w);
    flex-shrink: 0;
    padding: var(--space-2);
    background: var(--bg-subtle);
    border-right: 1px solid var(--border);
  }
  a {
    display: flex;
    align-items: center;
    height: 32px;
    padding: 0 var(--space-3);
    border-radius: var(--radius-md);
    color: var(--text);
    text-decoration: none;
  }
  .badge {
    margin-left: auto;
    min-width: 18px;
    height: 18px;
    padding: 0 6px;
    border-radius: 9px;
    background: var(--accent-subtle);
    color: var(--accent);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 18px;
    text-align: center;
  }
  @media (max-width: 639px) {
    nav {
      width: auto;
      flex-direction: row;
      overflow-x: auto;
      border-right: 0;
      border-bottom: 1px solid var(--border);
      padding: 4px var(--space-2);
    }
    a {
      white-space: nowrap;
      flex: none;
    }
  }
  a:hover {
    background: var(--bg-hover);
  }
  a.active {
    background: var(--bg-selected);
    color: var(--accent);
  }
</style>
