import type { ListItem } from "../api/types";
import type { Mode } from "./filter";
import { isBlockedAny } from "./model";

export interface Stats {
  total: number;
  open: number;
  ready: number;
  blocked: number;
}

/** Counts for the summary line over the items that pass the filters. */
export function summarize(items: Iterable<ListItem>): Stats {
  const s: Stats = { total: 0, open: 0, ready: 0, blocked: 0 };
  for (const i of items) {
    s.total++;
    if (i.status !== "closed") s.open++;
    if (i.ready) s.ready++;
    if (isBlockedAny(i)) s.blocked++;
  }
  return s;
}

/** "29 issues · 18 open · 6 ready · 7 blocked", or `12 results for “sign”` in flat mode with a query. */
export function summaryText(s: Stats, mode: Mode, query: string): string {
  const q = query.trim();
  if (mode === "flat" && q !== "") return `${s.total} result${s.total === 1 ? "" : "s"} for “${q}”`;
  return `${s.total} issue${s.total === 1 ? "" : "s"} · ${s.open} open · ${s.ready} ready · ${s.blocked} blocked`;
}
