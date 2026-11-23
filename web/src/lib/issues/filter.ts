import type { ListItem } from "../api/types";

/** "Active" is the default: not closed, plus closed issues still awaiting sign-off. */
export type StateFilter = "active" | "open" | "ready" | "closed" | "all";

export const STATE_FILTERS: readonly StateFilter[] = ["active", "open", "ready", "closed", "all"];

export interface ListFilters {
  state: StateFilter;
  /** 0..4; empty = any. */
  priority: number[];
  type: string[];
  label: string[];
  /** "" stands for unassigned. */
  assignee: string[];
}

export type Mode = "tree" | "flat";

export function defaultFilters(): ListFilters {
  return { state: "active", priority: [], type: [], label: [], assignee: [] };
}

export function isDefaultFilters(f: ListFilters): boolean {
  return (
    f.state === "active" && f.priority.length === 0 && f.type.length === 0 && f.label.length === 0 && f.assignee.length === 0
  );
}

export function isStateFilter(v: unknown): v is StateFilter {
  return typeof v === "string" && (STATE_FILTERS as readonly string[]).includes(v);
}

export function stateMatches(item: ListItem, s: StateFilter): boolean {
  switch (s) {
    case "active":
      // "disputed" stays in: it still needs attention.
      return item.status !== "closed" || item.signoff === "pending" || item.signoff === "disputed";
    case "open":
      return item.status !== "closed";
    case "ready":
      return item.ready;
    case "closed":
      return item.status === "closed";
    default:
      return true;
  }
}

/** Facets AND across chips; within one chip the values are OR (a label chip with two labels matches either). */
export function passesFilters(item: ListItem, f: ListFilters): boolean {
  if (!stateMatches(item, f.state)) return false;
  if (f.priority.length > 0 && !f.priority.includes(item.priority)) return false;
  if (f.type.length > 0 && !f.type.includes(item.issue_type)) return false;
  if (f.label.length > 0 && !item.labels.some((l) => f.label.includes(l))) return false;
  if (f.assignee.length > 0 && !f.assignee.includes(item.assignee ?? "")) return false;
  return true;
}

/** True when a query or any non-default filter is set: the list then shows flat results. */
export function isTriggered(query: string, f: ListFilters): boolean {
  return query.trim() !== "" || !isDefaultFilters(f);
}

/** The mode in effect: the explicit pick, else flat while triggered, else tree. */
export function resolveMode(pick: Mode | null, triggered: boolean): Mode {
  return pick ?? (triggered ? "flat" : "tree");
}

/**
 * Keeps the explicit Tree/Flat pick in step with the trigger. A forced "tree"
 * only means something while a query or filter would otherwise switch to flat,
 * so it resets when the trigger clears. An explicit "flat" survives clearing.
 */
export function nextModePick(pick: Mode | null, triggered: boolean): Mode | null {
  return !triggered && pick === "tree" ? null : pick;
}
