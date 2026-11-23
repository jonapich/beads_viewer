<script lang="ts">
  import { route } from "../lib/router/router.svelte";

  const items = [
    { href: "/", label: "Overview" },
    { href: "/issues", label: "Issues" },
    { href: "/triage", label: "Triage" },
    { href: "/insights", label: "Insights" },
  ];

  const active = (href: string) =>
    href === "/" ? route.path === "/" : route.path === href || route.path.startsWith(href + "/");
</script>

<nav aria-label="Main">
  {#each items as item (item.href)}
    <a href={item.href} class:active={active(item.href)} aria-current={active(item.href) ? "page" : undefined}>
      {item.label}
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