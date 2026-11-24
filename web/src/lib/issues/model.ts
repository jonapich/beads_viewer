import type { IssuesResponse, ListItem, ListTree } from "../api/types";

export interface Facets {
  types: Map<string, number>;
  labels: Map<string, number>;
  /** "" counts the unassigned issues. */
  assignees: Map<string, number>;
}

export interface IssueIndex {
  items: readonly ListItem[];
  byId: Map<string, ListItem>;
  /** Placement for the tree view; a flat roots-only tree when the response has none. */
  tree: ListTree;
  /** parent_id -> child ids (for breadcrumbs and the epic "3/12"). Not the tree placement. */
  childrenByParent: Map<string, string[]>;
  /** parent_id -> number of its children with status closed. */
  closedChildren: Map<string, number>;
  facets: Facets;
}

function bump(m: Map<string, number>, k: string): void {
  m.set(k, (m.get(k) ?? 0) + 1);
}

/** Builds the lookup structures once per response. */
export function buildIndex(resp: IssuesResponse): IssueIndex {
  const byId = new Map<string, ListItem>();
  const childrenByParent = new Map<string, string[]>();
  const closedChildren = new Map<string, number>();
  const facets: Facets = { types: new Map(), labels: new Map(), assignees: new Map() };

  for (const it of resp.items) byId.set(it.id, it);
  for (const it of resp.items) {
    if (it.parent_id && byId.has(it.parent_id)) {
      const kids = childrenByParent.get(it.parent_id);
      if (kids) kids.push(it.id);
      else childrenByParent.set(it.parent_id, [it.id]);
      if (it.status === "closed") bump(closedChildren, it.parent_id);
    }
    bump(facets.types, it.issue_type);
    for (const l of it.labels) bump(facets.labels, l);
    bump(facets.assignees, it.assignee ?? "");
  }

  const tree: ListTree = resp.tree ?? { roots: resp.items.map((i) => i.id), children: {} };
  return { items: resp.items, byId, tree, childrenByParent, closedChildren, facets };
}

/** Shown status: bd's glyph set (what `bd list` prints). */
export type DisplayStatus = "open" | "in_progress" | "blocked" | "closed" | "deferred";

export function deferredNow(item: Pick<ListItem, "defer_until">, now: number): boolean {
  if (!item.defer_until) return false;
  const t = Date.parse(item.defer_until);
  return Number.isFinite(t) && t > now;
}

export function displayStatus(
  item: Pick<ListItem, "status" | "blocked" | "defer_until">,
  now: number = Date.now(),
): DisplayStatus {
  if (item.status === "closed") return "closed";
  if (item.status === "deferred" || deferredNow(item, now)) return "deferred";
  if (item.status === "in_progress") return "in_progress";
  if (item.status === "blocked" || item.blocked) return "blocked";
  return "open";
}

/** Not closed, and blocked by status or by an active blocker. */
export function isBlockedAny(item: ListItem): boolean {
  return item.status !== "closed" && (item.blocked || item.status === "blocked");
}

/** Labelled needs-decision or needs-design: waiting for the user. */
export function needsAnswer(item: ListItem): boolean {
  return item.labels.includes("needs-decision") || item.labels.includes("needs-design");
}

/** A closed issue whose sign-off is still pending. */
export function awaitsSignoff(item: ListItem): boolean {
  return item.status === "closed" && item.signoff === "pending";
}

/**
 * An open issue labelled needs-decision or needs-design, so the Answer action
 * applies. The server sends no decision for a closed issue, and an answer would be
 * refused (409), so a closed issue that still carries the label is not answerable.
 */
export function canAnswer(item: ListItem): boolean {
  return item.status !== "closed" && needsAnswer(item);
}
