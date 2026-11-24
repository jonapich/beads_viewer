import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import type { ScoreBreakdown, TriageRecommendation, TriageResponse } from "../api/types";
import triageFixture from "../api/__fixtures__/triage.json";
import {
  alertIds,
  alertsOf,
  blockerRows,
  CARD_FACTORS,
  FACTORS,
  factorNote,
  factorValue,
  filterRecs,
  joinPicks,
  recStatus,
  summaryParts,
  summaryText,
  triageView,
  winRows,
  type ResourceState,
} from "./triage";

const fixture = triageFixture as unknown as TriageResponse;

const mkRec = (id: string, over: Partial<TriageRecommendation> = {}): TriageRecommendation => ({
  id,
  title: `Title of ${id}`,
  type: "task",
  status: "open",
  priority: 2,
  labels: [],
  score: 0.5,
  breakdown: {} as ScoreBreakdown,
  action: "Work on it",
  reasons: [],
  unblocks_ids: [],
  claimable: true,
  ...over,
});

const mk = (over: Partial<TriageResponse> = {}): TriageResponse => ({ ...fixture, ...over });
const res = (data: TriageResponse | null, over: Partial<ResourceState<TriageResponse>> = {}): ResourceState<TriageResponse> => ({
  data,
  error: null,
  pending: false,
  ...over,
});
const apiError = (status: number, code: string, message: string) => new ApiError(status, code, message);

describe("triageView: no data", () => {
  it("first fetch in flight is loading", () => {
    expect(triageView(res(null))).toMatchObject({ state: "loading", computing: false });
  });
  it("503 analysis_pending is loading with the computing summary", () => {
    expect(triageView(res(null, { pending: true }))).toMatchObject({ state: "loading", computing: true });
  });
  it("an error is failed with its message", () => {
    const v = triageView(res(null, { error: apiError(500, "analysis_failed", "triage analysis failed: boom") }));
    expect(v).toMatchObject({ state: "failed", message: "triage analysis failed: boom" });
  });
  it("a network failure with no data is failed too", () => {
    expect(triageView(res(null, { error: apiError(0, "network", "bv serve is not reachable") })).state).toBe("failed");
  });
  it("pending outranks an old error", () => {
    expect(triageView(res(null, { pending: true, error: apiError(500, "x", "y") })).state).toBe("loading");
  });
});

describe("triageView: data", () => {
  it("the fixture is data, phase 2 done, nothing flagged", () => {
    expect(fixture.recommendations.length).toBeGreaterThan(0);
    expect(triageView(res(fixture))).toEqual({
      state: "data",
      computing: false,
      message: "",
      noIssues: false,
      provisional: false,
      phase2Error: "",
      recomputing: false,
      stale: false,
      staleTitle: "",
      allBlocked: false,
    });
  });
  it("phase 2 still running is provisional", () => {
    expect(triageView(res(mk({ phase2_ready: false }))).provisional).toBe(true);
  });
  it("phase 2 failed is not provisional but carries the message", () => {
    const v = triageView(res(mk({ phase2_ready: false, phase2_error: "context deadline exceeded" })));
    expect(v).toMatchObject({ provisional: false, phase2Error: "context deadline exceeded", state: "data" });
  });
  it("pending with data is recomputing", () => {
    expect(triageView(res(fixture, { pending: true }))).toMatchObject({ state: "data", recomputing: true, stale: false });
  });
  it("an error with data is stale, not recomputing, and the title says why", () => {
    const v = triageView(res(fixture, { error: apiError(500, "analysis_failed", "boom") }));
    expect(v).toMatchObject({ state: "data", recomputing: false, stale: true, staleTitle: "boom" });
    // offline reads the same as in Plan and Insights
    expect(triageView(res(fixture, { error: apiError(0, "network", "bv serve is not reachable") }))).toMatchObject({ recomputing: false, stale: true });
  });
  it("no recommendations is empty: no issues at all", () => {
    const v = triageView(res(mk({ issue_count: 0, recommendations: [], quick_ref: { ...fixture.quick_ref, top_picks: [] } })));
    expect(v).toMatchObject({ state: "empty", noIssues: true, allBlocked: false });
  });
  it("no recommendations is empty: issues exist but none to recommend", () => {
    const v = triageView(res(mk({ issue_count: 4, recommendations: [], quick_ref: { ...fixture.quick_ref, top_picks: [] } })));
    expect(v).toMatchObject({ state: "empty", noIssues: false });
  });
  it("recommendations but no top picks is all blocked", () => {
    const v = triageView(res(mk({ quick_ref: { ...fixture.quick_ref, top_picks: [] } })));
    expect(v).toMatchObject({ state: "data", allBlocked: true });
  });
});

describe("summaryParts", () => {
  it("lists the counts and links only ready and the issue total", () => {
    const parts = summaryParts(fixture.quick_ref, fixture.issue_count);
    expect(parts.map((p) => p.label)).toEqual(["ready", "in progress", "blocked", "deferred", "issues"]);
    expect(parts.filter((p) => p.href).map((p) => p.href)).toEqual(["/issues?state=ready", "/issues?state=all"]);
    expect(parts[0]?.n).toBe(fixture.quick_ref.actionable_count);
    expect(parts[2]?.n).toBe(fixture.quick_ref.readiness_blocked_count);
    expect(parts[3]?.n).toBe(fixture.quick_ref.deferred_or_parked_count);
  });
  it("says issue for exactly one", () => {
    expect(summaryParts(fixture.quick_ref, 1).at(-1)?.label).toBe("issue");
    expect(summaryParts(fixture.quick_ref, 0).at(-1)?.label).toBe("issues");
  });
  it("reads as a line", () => {
    const q = { ...fixture.quick_ref, actionable_count: 7, in_progress_count: 1, readiness_blocked_count: 3, deferred_or_parked_count: 1 };
    expect(summaryText(summaryParts(q, 12))).toBe("7 ready · 1 in progress · 3 blocked · 1 deferred · 12 issues");
  });
});

describe("joinPicks", () => {
  it("joins the top picks to their recommendations by id, in order", () => {
    const picks = joinPicks(fixture);
    expect(picks.map((p) => p.pick.id)).toEqual(fixture.quick_ref.top_picks.map((p) => p.id));
    expect(picks.map((p) => p.rank)).toEqual(picks.map((_, i) => i + 1));
    for (const p of picks) expect(p.rec?.id).toBe(p.pick.id);
  });
  it("leaves rec undefined when the pick has no recommendation", () => {
    const r = mk({ recommendations: [], quick_ref: { ...fixture.quick_ref, top_picks: [{ id: "x", title: "X", score: 1, reasons: [], unblocks: 0 }] } });
    expect(joinPicks(r)).toEqual([{ rank: 1, pick: r.quick_ref.top_picks[0], rec: undefined }]);
  });
  it("is empty without top picks", () => {
    expect(joinPicks(mk({ quick_ref: { ...fixture.quick_ref, top_picks: [] } }))).toEqual([]);
  });
});

describe("filterRecs", () => {
  const recs = [
    mkRec("a"),
    mkRec("b", { claimable: false, blocked_by: ["a"] }),
    mkRec("c", { claimable: false, type: "epic" }),
    mkRec("d", { blocked_by: [] }),
  ];
  it("all keeps every row with its rank", () => {
    expect(filterRecs(recs, "all").map((x) => [x.rank, x.rec.id])).toEqual([[1, "a"], [2, "b"], [3, "c"], [4, "d"]]);
  });
  it("claimable keeps the claimable ones, ranks unchanged", () => {
    expect(filterRecs(recs, "claimable").map((x) => [x.rank, x.rec.id])).toEqual([[1, "a"], [4, "d"]]);
  });
  it("blocked keeps those with blockers", () => {
    expect(filterRecs(recs, "blocked").map((x) => [x.rank, x.rec.id])).toEqual([[2, "b"]]);
  });
  it("can come out empty", () => {
    expect(filterRecs([mkRec("a")], "blocked")).toEqual([]);
  });
});

describe("recStatus", () => {
  const now = Date.parse("2026-03-10T12:00:00Z");
  it.each([
    [mkRec("a"), "open"],
    [mkRec("a", { status: "in_progress" }), "in_progress"],
    [mkRec("a", { blocked_by: ["x"] }), "blocked"],
    [mkRec("a", { defer_until: "2026-04-01T00:00:00Z" }), "deferred"],
    [mkRec("a", { status: "blocked" }), "blocked"],
  ] as const)("%#", (rec, want) => {
    expect(recStatus(rec, now)).toBe(want);
  });
});

describe("factors", () => {
  const b = { pagerank_norm: 0.5, time_to_impact_norm: 0.25, time_to_impact_explanation: "soon", risk_norm: undefined } as unknown as ScoreBreakdown;
  it("has the eight factors in the TUI order", () => {
    expect(FACTORS.map((f) => f.label)).toEqual([
      "Impact",
      "Bottleneck",
      "Blocker ratio",
      "Priority",
      "Staleness",
      "Time to impact",
      "Urgency",
      "Risk",
    ]);
    expect(CARD_FACTORS.map((f) => f.label)).toEqual(["Impact", "Bottleneck", "Time to impact"]);
  });
  it("reads values, 0 when absent, and explanations", () => {
    const f = (label: string) => FACTORS.find((x) => x.label === label)!;
    expect(factorValue(b, f("Impact"))).toBe(0.5);
    expect(factorValue(b, f("Risk"))).toBe(0);
    expect(factorValue(b, f("Staleness"))).toBe(0);
    expect(factorNote(b, f("Time to impact"))).toBe("soon");
    expect(factorNote(b, f("Urgency"))).toBe("");
    expect(factorNote(b, f("Impact"))).toBe("");
  });
});

describe("rail rows", () => {
  it("blockers say what they unblock and whether they can be worked on now", () => {
    const rows = blockerRows([
      { id: "a", title: "A", unblocks_count: 3, unblocks_ids: ["x", "y", "z"], actionable: true },
      { id: "b", title: "B", unblocks_count: 1, unblocks_ids: ["x"], actionable: false, blocked_by: ["a", "c"] },
      { id: "c", title: "C", unblocks_count: 1, unblocks_ids: ["x"], actionable: false },
    ]);
    expect(rows).toEqual([
      { id: "a", title: "A", figure: "unblocks 3", tag: { tone: "ready", text: "ready" } },
      { id: "b", title: "B", figure: "unblocks 1", tag: { tone: "blk", text: "blocked by a, c" } },
      { id: "c", title: "C", figure: "unblocks 1", tag: { tone: "blk", text: "blocked" } },
    ]);
  });
  it("quick wins show the score and the reason", () => {
    expect(winRows([{ id: "q", title: "Q", status: "open", score: 0.4167, reason: "small and unblocks one" }])).toEqual([
      { id: "q", title: "Q", figure: "0.42", note: "small and unblocks one" },
    ]);
  });
  it("both are empty for no items", () => {
    expect(blockerRows([])).toEqual([]);
    expect(winRows([])).toEqual([]);
  });
  it("the fixture maps one row per item", () => {
    expect(blockerRows(fixture.blockers_to_clear)).toHaveLength(fixture.blockers_to_clear.length);
    expect(winRows(fixture.quick_wins)).toHaveLength(fixture.quick_wins.length);
  });
});

describe("alerts", () => {
  it("collects the ids of both fields without repeats", () => {
    expect(alertIds({ type: "x", severity: "info", message: "m", issue_id: "a", issue_ids: ["a", "b"] })).toEqual(["a", "b"]);
    expect(alertIds({ type: "x", severity: "info", message: "m", issue_ids: ["c"] })).toEqual(["c"]);
    expect(alertIds({ type: "x", severity: "info", message: "m" })).toEqual([]);
  });
  it("treats an omitted list as empty", () => {
    const { alerts: _alerts, ...rest } = fixture;
    expect(alertsOf(rest as TriageResponse)).toEqual([]);
    expect(alertsOf(fixture)).toEqual(fixture.alerts ?? []);
  });
});
