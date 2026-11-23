import type { ListItem } from "../api/types";

/**
 * The parent_id chain of `id`, outermost first, not including the item itself.
 * This is the structural ancestry, not the tree placement (a blocked issue sits
 * under its blockers, but its crumb shows its parent). A visited set stops
 * parent cycles; a parent that is not in the list ends the chain.
 */
export function ancestors(id: string, byId: ReadonlyMap<string, ListItem>): ListItem[] {
  const chain: ListItem[] = [];
  const seen = new Set<string>([id]);
  let cur = byId.get(id)?.parent_id;
  while (cur && !seen.has(cur)) {
    const p = byId.get(cur);
    if (!p) break;
    seen.add(cur);
    chain.unshift(p);
    cur = p.parent_id;
  }
  return chain;
}

export function trunc(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}

/** A crumb segment's label: the parent's title cut to 28 characters. */
export function crumbLabel(title: string): string {
  return trunc(title, 28);
}
