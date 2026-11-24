import type { PlanItem, PlanResponse, PlanTrack } from "../api/types";
import type { ResourceState } from "./triage";

export type PlanState = "loading" | "failed" | "empty" | "none" | "data";

export interface PlanView {
  state: PlanState;
  /** failed: the error to show. */
  message: string;
  /** data: the last refetch failed, so the plan on show may be out of date. */
  stale: boolean;
  staleTitle: string;
}

/**
 * Which of the view's states the plan resource is in. The plan is structure only: the
 * server never answers 503, so there is no "computing" state, and before phase 2 only
 * the triage score cells differ (they read "—").
 *
 * - empty: no tracks and the tracker holds no issues at all.
 * - none: no tracks, but issues exist (all blocked, deferred or closed).
 */
export function planView(res: ResourceState<PlanResponse>, issueCount: number | null): PlanView {
  const p = res.data;
  const base: PlanView = { state: "loading", message: "", stale: false, staleTitle: "" };
  if (!p) return res.error ? { ...base, state: "failed", message: res.error.message } : base;
  const stale = res.error !== null;
  const common = { ...base, stale, staleTitle: res.error?.message ?? "" };
  if (p.tracks.length > 0) return { ...common, state: "data" };
  return { ...common, state: issueCount === 0 ? "empty" : "none" };
}

/** "track-A" -> "A". */
export function trackLetter(trackId: string): string {
  return trackId.replace(/^track-/, "");
}

/**
 * Multi-item tracks first, then single-item tracks (a stable partition: server order within
 * each group), so the tracks with real structure, and the one the banner points at, are not
 * pushed below a column of single cards. Past `keep` single-item tracks the rest go to
 * `folded` (one closing disclosure). A track is single on its item count, never on its
 * reason text.
 */
export function foldSingles(tracks: readonly PlanTrack[], keep = 6): { shown: PlanTrack[]; folded: PlanTrack[] } {
  const multi = tracks.filter((t) => t.items.length !== 1);
  const singles = tracks.filter((t) => t.items.length === 1);
  return { shown: [...multi, ...singles.slice(0, keep)], folded: singles.slice(keep) };
}

/** The first `max` items of a track, and the rest behind "Show {k} more". */
export function capItems(items: readonly PlanItem[], max = 12): { shown: PlanItem[]; hidden: PlanItem[] } {
  return { shown: items.slice(0, max), hidden: items.slice(max) };
}

/**
 * The banner and the "Start here" tag share one rule: the plan names a highest-impact item
 * and it unblocks something. With nothing unblocked the item is an arbitrary lowest-id pick.
 */
export function showBanner(p: PlanResponse): boolean {
  return p.summary.unblocks_count > 0 && p.summary.highest_impact !== "";
}

/** The id to tag "Start here", or "" when there is none. */
export function startId(p: PlanResponse): string {
  return showBanner(p) ? p.summary.highest_impact : "";
}

/** An item of any track by id. */
export function itemById(p: PlanResponse, id: string): PlanItem | undefined {
  for (const t of p.tracks) {
    const it = t.items.find((x) => x.id === id);
    if (it) return it;
  }
  return undefined;
}

export interface SummaryLine {
  /** The counts that are disjoint: actionable (over the tracks), blocked, deferred or parked. */
  parts: Array<{ n: number; text: string }>;
  /** In progress overlaps actionable and blocked, so it is shown apart and never added in. 0 = leave it out. */
  inProgress: number;
}

export function summaryLine(p: PlanResponse): SummaryLine {
  const tracks = p.tracks.length;
  return {
    parts: [
      { n: p.total_actionable, text: `actionable in ${tracks} ${tracks === 1 ? "track" : "tracks"}` },
      { n: p.total_blocked, text: "blocked" },
      { n: p.total_deferred_or_parked, text: "deferred or parked" },
    ],
    inProgress: p.total_in_progress,
  };
}

/** The summary line as text. */
export function summaryText(s: SummaryLine): string {
  return s.parts.map((x) => `${x.n} ${x.text}`).join(" · ");
}
