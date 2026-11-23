import { defaultFilters, isStateFilter, type ListFilters, type Mode } from "./filter";
import { isSortKey, type SortKey } from "./sort";

/** What the Issues view keeps in the query string (reload and Back from /issues/:id restore it). */
export interface UrlState {
  query: string;
  filters: ListFilters;
  sortPick: SortKey | null;
  modePick: Mode | null;
  peek: string | null;
}

/**
 * Serialises to the query string, omitting defaults. Repeated params rather
 * than comma lists, because labels may contain commas. `assignee=` (empty
 * value) means unassigned.
 */
export function toQuery(s: UrlState): URLSearchParams {
  const p = new URLSearchParams();
  const q = s.query.trim();
  if (q !== "") p.set("q", s.query);
  if (s.filters.state !== "active") p.set("state", s.filters.state);
  for (const n of [...s.filters.priority].sort((a, b) => a - b)) p.append("p", String(n));
  for (const t of s.filters.type) p.append("type", t);
  for (const l of s.filters.label) p.append("label", l);
  for (const a of s.filters.assignee) p.append("assignee", a);
  if (s.sortPick !== null) p.set("sort", s.sortPick);
  if (s.modePick !== null) p.set("mode", s.modePick);
  if (s.peek) p.set("peek", s.peek);
  return p;
}

/** Parses the query string; unknown or invalid values are dropped silently. */
export function fromQuery(p: URLSearchParams): UrlState {
  const filters: ListFilters = defaultFilters();
  const state = p.get("state");
  if (isStateFilter(state)) filters.state = state;
  filters.priority = [
    ...new Set(
      p
        .getAll("p")
        .filter((v) => /^[0-4]$/.test(v))
        .map(Number),
    ),
  ].sort((a, b) => a - b);
  filters.type = [...new Set(p.getAll("type").filter((v) => v !== ""))];
  filters.label = [...new Set(p.getAll("label").filter((v) => v !== ""))];
  filters.assignee = [...new Set(p.getAll("assignee"))];

  const sort = p.get("sort");
  const mode = p.get("mode");
  return {
    query: p.get("q") ?? "",
    filters,
    sortPick: isSortKey(sort) ? sort : null,
    modePick: mode === "tree" || mode === "flat" ? mode : null,
    peek: p.get("peek") || null,
  };
}

/**
 * True when the two query strings differ in the text query `q` only. The view
 * debounces the write-back of text typing and writes every other change at once.
 */
export function onlyQueryDiffers(a: URLSearchParams, b: URLSearchParams): boolean {
  const strip = (p: URLSearchParams): string => {
    const c = new URLSearchParams(p);
    c.delete("q");
    return c.toString();
  };
  return strip(a) === strip(b);
}
