// Shared builders for the issue-list unit tests. Imported by *.test.ts only,
// so it never ships.
import type { IssuesResponse, ListItem, ListTree, TreeEdge } from "../api/types";

export function mkItem(id: string, over: Partial<ListItem> = {}): ListItem {
  return {
    id,
    title: `Title of ${id}`,
    status: "open",
    priority: 2,
    issue_type: "task",
    labels: [],
    created_at: "2026-03-01T10:00:00Z",
    updated_at: "2026-03-02T10:00:00Z",
    child_count: 0,
    blocked_by: [],
    dependent_count: 0,
    ready: true,
    blocked: false,
    ...over,
  };
}

/** A response whose tree is `children` (id -> [childId, kind?]) and `roots`. */
export function mkResponse(
  items: ListItem[],
  roots: string[],
  children: Record<string, Array<string | [string, "child" | "block"]>> = {},
): IssuesResponse {
  const tree: ListTree = { roots, children: {} };
  for (const [p, kids] of Object.entries(children)) {
    tree.children[p] = kids.map((k): TreeEdge => (typeof k === "string" ? { id: k, kind: "child" } : { id: k[0], kind: k[1] }));
  }
  return { phase2_ready: true, total: items.length, items, tree };
}
