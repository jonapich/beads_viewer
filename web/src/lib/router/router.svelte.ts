import type { Component } from "svelte";
import { match } from "./match";

export interface RouteDef {
  pattern: string;
  title: string;
  component: Component<any>;
  props?: Record<string, unknown>;
}

export const route = $state({
  path: "/",
  params: {} as Record<string, string>,
  query: new URLSearchParams(),
  def: null as RouteDef | null,
});

let table: RouteDef[] = [];

function resolve(url: URL): void {
  route.path = url.pathname;
  route.query = url.searchParams;
  route.def = null;
  route.params = {};
  for (const def of table) {
    const params = match(def.pattern, url.pathname);
    if (params) {
      route.def = def;
      route.params = params;
      document.title = `${def.title} - bv`;
      return;
    }
  }
}

/** Navigates to a same-origin path (with optional query) in history mode. */
export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  const url = new URL(to, location.origin);
  if (opts.replace) history.replaceState(null, "", url);
  else history.pushState(null, "", url);
  resolve(url);
  // <main> is the scroll container; the window itself never scrolls.
  document.querySelector("main")?.scrollTo(0, 0);
}

function onClick(e: MouseEvent): void {
  if (e.defaultPrevented || e.button !== 0) return;
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const a = (e.target as Element | null)?.closest("a[href]") as HTMLAnchorElement | null;
  if (!a || a.hasAttribute("download")) return;
  const target = a.getAttribute("target");
  if (target && target !== "_self") return;
  if (a.origin !== location.origin) return;
  if (a.pathname.startsWith("/api") || a.pathname.startsWith("/auth")) return;
  e.preventDefault();
  navigate(a.pathname + a.search + a.hash);
}

function onPopState(): void {
  resolve(new URL(location.href));
}

/**
 * Starts history-mode routing over `routes` (first match wins). Plain
 * <a href="/issues/x"> links navigate client-side. Returns the cleanup.
 */
export function startRouter(routes: RouteDef[]): () => void {
  table = routes;
  resolve(new URL(location.href));
  window.addEventListener("popstate", onPopState);
  document.addEventListener("click", onClick);
  return () => {
    window.removeEventListener("popstate", onPopState);
    document.removeEventListener("click", onClick);
  };
}