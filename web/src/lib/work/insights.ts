import type { InsightEntry, InsightsResponse, TriageResponse, VelocityWeek } from "../api/types";
import type { MetricKey } from "./metrics";
import { barPct } from "./present";
import type { ResourceState } from "./triage";

/** The list sizes "Load more" steps through (the server's cap is the last). */
export const LIMITS: readonly number[] = [25, 100, 500];

/** The next limit after `limit`, or null at the cap. */
export function nextLimit(limit: number): number | null {
  return LIMITS.find((l) => l > limit) ?? null;
}

export type InsightsState = "loading" | "failed" | "skipped" | "data";

export interface InsightsView {
  state: InsightsState;
  /** failed: the error to show (insights never 503s or 500s, so this is a network failure or the like). */
  message: string;
  /** Phase 2 failed; the phase 1 lists are on show. */
  phase2Error: string;
  /** Phase 2 is still running: the lists may be empty or partial. */
  provisional: boolean;
  /** The last refetch failed, so what is on show may be out of date. */
  stale: boolean;
  staleTitle: string;
}

/**
 * Which state the insights resource is in. phase2_skipped (the huge tier) outranks everything:
 * the whole grid becomes one notice.
 */
export function insightsView(res: ResourceState<InsightsResponse>): InsightsView {
  const r = res.data;
  const base: InsightsView = { state: "loading", message: "", phase2Error: "", provisional: false, stale: false, staleTitle: "" };
  if (!r) return res.error ? { ...base, state: "failed", message: res.error.message } : base;
  const phase2Error = r.phase2_error ?? "";
  return {
    ...base,
    state: r.phase2_skipped ? "skipped" : "data",
    phase2Error,
    provisional: !r.phase2_skipped && !r.phase2_ready && phase2Error === "",
    stale: res.error !== null,
    staleTitle: res.error?.message ?? "",
  };
}

/** How many entries a metric's list holds now. */
export function listLen(r: InsightsResponse, key: MetricKey): number {
  return r[key].length;
}

export type CardKind = "skeleton" | "list" | "computing" | "skipped" | "none" | "not-computed";

export interface CardState {
  state: CardKind;
  /** skipped: why. */
  reason: string;
}

/**
 * What one metric card shows. A skipped or timed-out metric names its reason; before phase 2 an
 * empty list is a skeleton and a partial one carries a "Computing…" chip; after a failed phase 2
 * an empty list is "not computed" (never "none found": `skipped` stays empty then, so an empty
 * list does not mean none exist).
 */
export function cardState(r: InsightsResponse, key: MetricKey, len: number = listLen(r, key)): CardState {
  const reason = r.skipped[key];
  if (reason) return { state: "skipped", reason };
  const phase2Error = (r.phase2_error ?? "") !== "";
  if (!r.phase2_ready && !phase2Error) return { state: len === 0 ? "skeleton" : "computing", reason: "" };
  if (phase2Error && len === 0) return { state: "not-computed", reason: "" };
  return { state: len === 0 ? "none" : "list", reason: "" };
}

/**
 * "Load more" is offered on a widened card when the list is full at the current limit and the
 * server would send more. The cycles list is capped too, but its total is known.
 */
export function canLoadMore(r: InsightsResponse, key: MetricKey, limit: number): boolean {
  if (limit >= (LIMITS[LIMITS.length - 1] ?? limit)) return false;
  if (key === "cycles") return r.cycle_count > r.cycles.length;
  return listLen(r, key) >= limit;
}

export interface MetricRow {
  id: string;
  title: string;
  value: number;
  /** The value as a share of the list's largest, for the bar. */
  pct: number;
}

/** The rows of a value list (or id list), with titles filled from the titles map, and bar shares. */
export function metricRows(r: InsightsResponse, key: MetricKey): MetricRow[] {
  if (key === "cycles") return [];
  if (key === "articulation") {
    return r.articulation.map((id) => ({ id, title: r.titles[id] ?? "", value: 0, pct: 0 }));
  }
  const entries: readonly InsightEntry[] = r[key];
  const max = entries.reduce((m, e) => Math.max(m, e.value), 0);
  return entries.map((e) => ({ id: e.id, title: e.title ?? r.titles[e.id] ?? "", value: e.value, pct: barPct(e.value, max) }));
}

export interface Chain {
  ids: string[];
  /** The titles joined with arrows, for the caption. */
  caption: string;
}

/** Each cycle as a chain of ids. */
export function cycleChains(r: InsightsResponse): Chain[] {
  return r.cycles.map((ids) => ({ ids, caption: ids.map((id) => r.titles[id] ?? id).join(" → ") }));
}

/** The orphans with their titles. */
export function orphanRows(r: InsightsResponse): Array<{ id: string; title: string }> {
  return r.orphans.map((id) => ({ id, title: r.titles[id] ?? "" }));
}

/** Weekly closes, newest first as the server sends them -> the last 8 weeks, oldest first. */
export function sparkWeeks(weekly: readonly VelocityWeek[] | undefined): number[] {
  return [...(weekly ?? [])].reverse().slice(-8).map((w) => w.closed);
}

export interface SparkBar {
  x: number;
  y: number;
  h: number;
  /** Weeks with no closes are drawn dim. */
  dim: boolean;
}

/** Bars for a 64x24 sparkline: 6 wide at an 8px pitch, 2..22 tall. */
export function sparkBars(counts: readonly number[]): SparkBar[] {
  const max = Math.max(1, ...counts);
  return counts.map((c, i) => {
    const h = Math.max(2, Math.round((c / max) * 22));
    return { x: i * 8 + 1, y: 24 - h, h, dim: c === 0 };
  });
}

export type HealthState = "skeleton" | "unavailable" | "tiles";

/** The health strip's state, from the triage resource. */
export function healthState(res: ResourceState<TriageResponse>): { state: HealthState; message: string } {
  if (res.data) return { state: "tiles", message: "" };
  if (res.error) return { state: "unavailable", message: res.error.message };
  return { state: "skeleton", message: "" };
}

export interface HealthTiles {
  ready: number;
  blocked: number;
  inProgress: number;
  /** null when the response carries no velocity. */
  /** avgDays is null when nothing closed in 30 days (an average of nothing reads "0.0d"). */
  closed: { last30: number; avgDays: string | null; weeks: number[] } | null;
  graph: { nodes: number; edges: number; density: string | null; orphans: number | null };
}

/** The five tiles. The graph tile's density and orphans come from the insights, once they are in. */
export function healthTiles(t: TriageResponse, insights: InsightsResponse | null): HealthTiles {
  const h = t.project_health;
  const v = h.velocity;
  return {
    ready: h.counts.actionable,
    blocked: h.counts.dependency_blocked,
    inProgress: h.counts.by_status["in_progress"] ?? 0,
    closed: v
      ? {
          last30: v.closed_last_30_days,
          avgDays: v.closed_last_30_days > 0 ? v.avg_days_to_close.toFixed(1) : null,
          weeks: sparkWeeks(v.weekly),
        }
      : null,
    graph: {
      nodes: h.graph.node_count,
      edges: h.graph.edge_count,
      density: insights ? insights.cluster_density.toFixed(3) : null,
      orphans: insights ? insights.orphan_count : null,
    },
  };
}
