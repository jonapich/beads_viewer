import type { ListItem } from "../api/types";
import type { Mode } from "./filter";

export type SortKey = "default" | "triage" | "priority" | "created_desc" | "created_asc" | "updated" | "relevance";

export const SORT_KEYS: readonly SortKey[] = [
  "default",
  "triage",
  "priority",
  "created_desc",
  "created_asc",
  "updated",
  "relevance",
];

export const SORT_LABELS: Record<SortKey, string> = {
  default: "Default",
  triage: "Triage score",
  priority: "Priority",
  created_desc: "Created (newest)",
  created_asc: "Created (oldest)",
  updated: "Updated",
  relevance: "Relevance",
};

export function isSortKey(v: unknown): v is SortKey {
  return typeof v === "string" && (SORT_KEYS as readonly string[]).includes(v);
}

const cmpStr = (a: string, b: string): number => (a < b ? -1 : a > b ? 1 : 0);

/**
 * The key actually used. "relevance" only applies in flat mode with a query;
 * everywhere else it (and the automatic pick) falls back to "default".
 */
export function effectiveSort(sortPick: SortKey | null, mode: Mode, hasQuery: boolean): SortKey {
  if (sortPick !== null && sortPick !== "relevance") return sortPick;
  return mode === "flat" && hasQuery ? "relevance" : "default";
}

/**
 * Comparator for one sort key, tie-breaks as in the approved mock. `rank` maps
 * an id to its position in the search hits (lower is better); ids it lacks sort
 * after every hit.
 */
export function comparator(key: SortKey, rank?: ReadonlyMap<string, number> | null): (a: ListItem, b: ListItem) => number {
  const tri = (i: ListItem): number => i.triage_score ?? -1;
  const missing = rank ? rank.size : 0;
  switch (key) {
    case "relevance":
      return (a, b) =>
        (rank?.get(a.id) ?? missing) - (rank?.get(b.id) ?? missing) ||
        a.priority - b.priority ||
        cmpStr(b.created_at, a.created_at) ||
        cmpStr(a.id, b.id);
    case "triage":
      return (a, b) => tri(b) - tri(a) || a.priority - b.priority || cmpStr(a.id, b.id);
    case "priority":
      return (a, b) => a.priority - b.priority || cmpStr(a.id, b.id);
    case "created_desc":
      return (a, b) => cmpStr(b.created_at, a.created_at) || cmpStr(a.id, b.id);
    case "created_asc":
      return (a, b) => cmpStr(a.created_at, b.created_at) || cmpStr(a.id, b.id);
    case "updated":
      return (a, b) => cmpStr(b.updated_at, a.updated_at) || cmpStr(a.id, b.id);
    default:
      return (a, b) => a.priority - b.priority || cmpStr(b.created_at, a.created_at) || cmpStr(a.id, b.id);
  }
}
