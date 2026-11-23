import type { MetaResponse } from "../api/types";
import { isSortKey, type SortKey } from "./sort";

/**
 * Remembered per tracker: expansion and sort. Filters, State, search and mode
 * are deliberately not stored; they live in the URL only (beads-c2o.33), so a
 * fresh visit to /issues opens the default tree.
 */
export const MAX_EXPANDED = 10000;

export interface Prefs {
  expanded: string[];
  sort?: SortKey;
}

const PREFIX = "bv.issues.v1:";

/** The storage key for one tracker data source. */
export function prefsKey(meta: MetaResponse | null): string {
  return PREFIX + (meta?.source.project_dir || meta?.source.path || "");
}

/** Parses a stored value; null when absent, not JSON or not the expected shape. */
export function parsePrefs(raw: string | null): Prefs | null {
  if (raw === null) return null;
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof v !== "object" || v === null) return null;
  const o = v as Record<string, unknown>;
  if (!Array.isArray(o.expanded)) return null;
  const prefs: Prefs = {
    expanded: o.expanded.filter((x): x is string => typeof x === "string").slice(0, MAX_EXPANDED),
  };
  if (isSortKey(o.sort)) prefs.sort = o.sort;
  return prefs;
}

export function serializePrefs(p: Prefs): string {
  const out: Prefs = { expanded: p.expanded.slice(0, MAX_EXPANDED) };
  if (p.sort) out.sort = p.sort;
  return JSON.stringify(out);
}

/** Storage can be missing or throw (private windows, blocked site data): fall back to defaults. */
export function loadPrefs(key: string): Prefs | null {
  try {
    return parsePrefs(localStorage.getItem(key));
  } catch {
    return null;
  }
}

export function savePrefs(key: string, prefs: Prefs): void {
  try {
    localStorage.setItem(key, serializePrefs(prefs));
  } catch {
    // not persisted; still applies for this session
  }
}
