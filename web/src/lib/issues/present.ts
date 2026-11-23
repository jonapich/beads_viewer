import type { DisplayStatus } from "./model";

export const GLYPH: Record<DisplayStatus, string> = {
  open: "○",
  in_progress: "◐",
  blocked: "●",
  closed: "✓",
  deferred: "❄",
};

export const STATUS_NAME: Record<DisplayStatus, string> = {
  open: "Open",
  in_progress: "In progress",
  blocked: "Blocked",
  closed: "Closed",
  deferred: "Deferred",
};

/** Two characters for the assignee circle. */
export function initials(name: string): string {
  const parts = name.split(/[\s._-]+/).filter(Boolean);
  const letters =
    parts.length >= 2 ? (parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "") : name.replace(/[\s._-]+/g, "").slice(0, 2);
  return letters.toUpperCase();
}

/** Compact relative age: 5m, 3h, 2d. Future timestamps read as 1m. */
export function ago(ts: string, now: number): string {
  const t = Date.parse(ts);
  if (!Number.isFinite(t)) return "";
  const m = Math.max(0, Math.round((now - t) / 60000));
  if (m < 60) return `${Math.max(1, m)}m`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h}h`;
  return `${Math.round(h / 24)}d`;
}

/** Whole percent for the .pct-N bar classes. Below 100 only when something is still open. */
export function percent(done: number, total: number): number {
  if (total <= 0) return 0;
  const p = Math.round((done / total) * 100);
  return done < total ? Math.min(p, 99) : 100;
}

/** The triage cell: two decimals, or an em dash while the score is absent. */
export function triageText(score: number | undefined): string {
  return score === undefined ? "—" : score.toFixed(2);
}
