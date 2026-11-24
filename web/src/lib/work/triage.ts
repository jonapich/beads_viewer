import type { ApiError } from "../api/client";
import type { Alert, BlockerItem, QuickRef, QuickWin, ScoreBreakdown, TopPick, TriageRecommendation, TriageResponse } from "../api/types";
import { displayStatus, type DisplayStatus } from "../issues/model";

/** The parts of a Resource the state functions read. */
export interface ResourceState<T> {
  data: T | null;
  error: ApiError | null;
  pending: boolean;
}

export type TriageState = "loading" | "failed" | "empty" | "data";

export interface TriageView {
  state: TriageState;
  /** loading: the server answered 503 analysis_pending ("Computing triage…"). */
  computing: boolean;
  /** failed: the error to show. */
  message: string;
  /** empty: the tracker has no open issues at all (else: nothing to recommend). */
  noIssues: boolean;
  /** Phase 2 is still running: the scores are provisional. */
  provisional: boolean;
  /** Phase 2 failed; the phase 1 result is on show. */
  phase2Error: string;
  /** A refetch is pending or failed while the old copy stays on show. */
  recomputing: boolean;
  /** The recomputing chip's tooltip: the failure, if that is why. */
  recomputingTitle: string;
  /** Recommendations exist but none is claimable, so "Start here" has nothing to show. */
  allBlocked: boolean;
}

const NONE: TriageView = {
  state: "loading",
  computing: false,
  message: "",
  noIssues: false,
  provisional: false,
  phase2Error: "",
  recomputing: false,
  recomputingTitle: "",
  allBlocked: false,
};

/** Which of the view's states the triage resource is in (every branch of the mock's state switcher). */
export function triageView(res: ResourceState<TriageResponse>): TriageView {
  const t = res.data;
  if (!t) {
    if (res.pending) return { ...NONE, state: "loading", computing: true };
    if (res.error) return { ...NONE, state: "failed", message: res.error.message };
    return NONE;
  }
  const recs = t.recommendations.length;
  const phase2Error = t.phase2_error ?? "";
  return {
    ...NONE,
    state: recs === 0 ? "empty" : "data",
    noIssues: t.issue_count === 0,
    provisional: !t.phase2_ready && phase2Error === "",
    phase2Error,
    recomputing: res.pending || res.error !== null,
    recomputingTitle: res.error?.message ?? "",
    // Top picks are gated on claimable.
    allBlocked: t.quick_ref.top_picks.length === 0 && recs > 0,
  };
}

/** One entry of the summary line; `href` makes it a link (only the counts the Issues filters know). */
export interface SummaryPart {
  n: number;
  label: string;
  href?: string;
}

/** The summary line under the title: counts from quick_ref, and the issue total. */
export function summaryParts(q: QuickRef, issueCount: number): SummaryPart[] {
  return [
    { n: q.actionable_count, label: "ready", href: "/issues?state=ready" },
    { n: q.in_progress_count, label: "in progress" },
    { n: q.readiness_blocked_count, label: "blocked" },
    { n: q.deferred_or_parked_count, label: "deferred" },
    { n: issueCount, label: issueCount === 1 ? "issue" : "issues", href: "/issues?state=all" },
  ];
}

/** The summary line as text (for the tests and the page's accessible name). */
export function summaryText(parts: readonly SummaryPart[]): string {
  return parts.map((p) => `${p.n} ${p.label}`).join(" · ");
}

export interface PickView {
  /** 1-based, the pick's place among the top picks. */
  rank: number;
  pick: TopPick;
  /** The recommendation behind it: the type, priority, status and action. */
  rec: TriageRecommendation | undefined;
}

/** The top picks joined to their recommendations by id. */
export function joinPicks(resp: TriageResponse): PickView[] {
  const byId = new Map(resp.recommendations.map((r) => [r.id, r]));
  return resp.quick_ref.top_picks.map((pick, i) => ({ rank: i + 1, pick, rec: byId.get(pick.id) }));
}

export type RecFilter = "all" | "claimable" | "blocked";

export const REC_FILTERS: ReadonlyArray<{ key: RecFilter; label: string }> = [
  { key: "all", label: "All" },
  { key: "claimable", label: "Claimable" },
  { key: "blocked", label: "Blocked" },
];

export interface RankedRec {
  /** 1-based place in the full list, kept when a filter hides rows. */
  rank: number;
  rec: TriageRecommendation;
}

/** The recommendations a filter shows, each with its rank in the unfiltered list. */
export function filterRecs(recs: readonly TriageRecommendation[], filter: RecFilter): RankedRec[] {
  const ranked = recs.map((rec, i): RankedRec => ({ rank: i + 1, rec }));
  switch (filter) {
    case "claimable":
      return ranked.filter((x) => x.rec.claimable);
    case "blocked":
      return ranked.filter((x) => (x.rec.blocked_by?.length ?? 0) > 0);
    default:
      return ranked;
  }
}

/** The status glyph of a recommendation: blocked and deferred outrank its own status. */
export function recStatus(rec: TriageRecommendation, now: number): DisplayStatus {
  return displayStatus(
    { status: rec.status, blocked: (rec.blocked_by?.length ?? 0) > 0, ...(rec.defer_until ? { defer_until: rec.defer_until } : {}) },
    now,
  );
}

type NormKey = {
  [K in keyof ScoreBreakdown]-?: K extends `${string}_norm` ? K : never;
}[keyof ScoreBreakdown];
type ExplanationKey = Extract<keyof ScoreBreakdown, `${string}_explanation`>;

export interface Factor {
  label: string;
  norm: NormKey;
  explanation?: ExplanationKey;
}

/** The eight score factors, in the TUI's order. */
export const FACTORS: readonly Factor[] = [
  { label: "Impact", norm: "pagerank_norm" },
  { label: "Bottleneck", norm: "betweenness_norm" },
  { label: "Blocker ratio", norm: "blocker_ratio_norm" },
  { label: "Priority", norm: "priority_boost_norm" },
  { label: "Staleness", norm: "staleness_norm" },
  { label: "Time to impact", norm: "time_to_impact_norm", explanation: "time_to_impact_explanation" },
  { label: "Urgency", norm: "urgency_norm", explanation: "urgency_explanation" },
  { label: "Risk", norm: "risk_norm", explanation: "risk_explanation" },
];

/** The three factors a pick card draws. */
export const CARD_FACTORS: readonly Factor[] = [FACTORS[0]!, FACTORS[1]!, FACTORS[5]!];

/** A factor's normalised value (0..1), 0 when absent. */
export function factorValue(b: ScoreBreakdown, f: Factor): number {
  return b[f.norm] || 0;
}

/** A factor's explanation text, "" when it has none. */
export function factorNote(b: ScoreBreakdown, f: Factor): string {
  return f.explanation ? (b[f.explanation] ?? "") : "";
}

/** The ids an alert names, without repeats. */
export function alertIds(a: Alert): string[] {
  return [...new Set([...(a.issue_id ? [a.issue_id] : []), ...(a.issue_ids ?? [])])];
}

/** The alerts, treating the omitted list as empty. */
export function alertsOf(t: TriageResponse): readonly Alert[] {
  return t.alerts ?? [];
}

/** One row of the Blockers to clear and Quick wins lists. */
export interface RailRow {
  id: string;
  title: string;
  /** The figure at the right: "unblocks 3", "0.42". */
  figure: string;
  /** A tag under the title: ready, or blocked by ... */
  tag?: { tone: "ready" | "blk"; text: string };
  /** Plain text after the id (a quick win's reason). */
  note?: string;
}

/** Blockers to clear: what each unblocks, and whether it can be worked on now. */
export function blockerRows(items: readonly BlockerItem[]): RailRow[] {
  return items.map((b): RailRow => {
    const blockedBy = b.blocked_by ?? [];
    return {
      id: b.id,
      title: b.title,
      figure: `unblocks ${b.unblocks_count}`,
      tag: b.actionable ? { tone: "ready", text: "ready" } : { tone: "blk", text: blockedBy.length > 0 ? `blocked by ${blockedBy.join(", ")}` : "blocked" },
    };
  });
}

/** Quick wins: the score and why it is a win. */
export function winRows(items: readonly QuickWin[]): RailRow[] {
  return items.map((q): RailRow => ({ id: q.id, title: q.title, figure: q.score.toFixed(2), note: q.reason }));
}
