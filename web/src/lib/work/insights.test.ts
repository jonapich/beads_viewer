import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import insightsFixture from "../api/__fixtures__/insights.json";
import triageFixture from "../api/__fixtures__/triage.json";
import type { InsightsResponse, TriageResponse } from "../api/types";
import { parseInline } from "../markdown";
import {
  canLoadMore,
  cardState,
  cycleChains,
  healthState,
  healthTiles,
  insightsView,
  LIMITS,
  listLen,
  metricRows,
  nextLimit,
  orphanRows,
  sparkBars,
  sparkWeeks,
} from "./insights";
import { METRICS } from "./metrics";
import type { ResourceState } from "./triage";

const fx = insightsFixture as unknown as InsightsResponse;
const tri = triageFixture as unknown as TriageResponse;
const mk = (over: Partial<InsightsResponse> = {}): InsightsResponse => ({ ...fx, ...over });
const res = <T>(data: T | null, error: ApiError | null = null): ResourceState<T> => ({ data, error, pending: false });
const err = (m = "boom") => new ApiError(0, "network", m);

describe("METRICS", () => {
  it("has the nine metrics in the TUI order", () => {
    expect(METRICS.map((m) => m.key)).toEqual([
      "bottlenecks",
      "keystones",
      "influencers",
      "hubs",
      "authorities",
      "cores",
      "articulation",
      "slack",
      "cycles",
    ]);
    expect(METRICS.find((m) => m.key === "articulation")?.title).toBe("Cut points");
  });
  it("every explanation is filled and uses only **strong** and `code` (no stray asterisks)", () => {
    for (const m of METRICS) {
      for (const text of [m.title, m.short, m.whatIs, m.whyUseful, m.howToUse, m.formula]) {
        expect(text.trim()).not.toBe("");
        expect(text.replaceAll("**", "")).not.toContain("*");
      }
      expect(parseInline(m.whatIs).some((n) => n.t === "text")).toBe(true);
    }
    expect(parseInline(METRICS[0]!.formula).some((n) => n.t === "code")).toBe(true);
  });
  it("every key is a list of the insights fixture", () => {
    for (const m of METRICS) expect(Array.isArray(fx[m.key])).toBe(true);
  });
});

describe("limits", () => {
  it("steps 25 -> 100 -> 500 and stops", () => {
    expect(LIMITS).toEqual([25, 100, 500]);
    expect(nextLimit(25)).toBe(100);
    expect(nextLimit(100)).toBe(500);
    expect(nextLimit(500)).toBeNull();
  });
});

describe("insightsView", () => {
  it("no data and no error is loading", () => {
    expect(insightsView(res<InsightsResponse>(null)).state).toBe("loading");
  });
  it("no data and an error is failed", () => {
    expect(insightsView(res<InsightsResponse>(null, err("bv serve is not reachable")))).toMatchObject({ state: "failed", message: "bv serve is not reachable" });
  });
  it("the fixture is data, not provisional", () => {
    expect(insightsView(res(fx))).toMatchObject({ state: "data", provisional: false, phase2Error: "", stale: false });
  });
  it("phase 2 pending is provisional", () => {
    expect(insightsView(res(mk({ phase2_ready: false }))).provisional).toBe(true);
  });
  it("phase 2 failed carries the message and is not provisional", () => {
    expect(insightsView(res(mk({ phase2_ready: false, phase2_error: "deadline" })))).toMatchObject({ state: "data", phase2Error: "deadline", provisional: false });
  });
  it("phase 2 skipped outranks everything", () => {
    expect(insightsView(res(mk({ phase2_skipped: true, phase2_ready: false })))).toMatchObject({ state: "skipped", provisional: false });
  });
  it("an error with data keeps the data and flags it stale", () => {
    expect(insightsView(res(fx, err("down")))).toMatchObject({ state: "data", stale: true, staleTitle: "down" });
  });
});

describe("cardState", () => {
  it("a skipped metric names its reason (the fixture skips keystones and slack)", () => {
    expect(fx.skipped["keystones"]).toBeTruthy();
    expect(cardState(fx, "keystones")).toEqual({ state: "skipped", reason: fx.skipped["keystones"] });
    expect(cardState(fx, "slack").state).toBe("skipped");
  });
  it("a skipped reason wins over an empty list", () => {
    expect(cardState(mk({ keystones: [] }), "keystones").state).toBe("skipped");
  });
  it("before phase 2 an empty list is a skeleton", () => {
    expect(cardState(mk({ phase2_ready: false, skipped: {}, hubs: [] }), "hubs").state).toBe("skeleton");
  });
  it("before phase 2 a partial list is computing", () => {
    expect(cardState(mk({ phase2_ready: false, skipped: {} }), "cores").state).toBe("computing");
  });
  it("after a failed phase 2 an empty list is not computed, never none", () => {
    const r = mk({ phase2_ready: false, phase2_error: "x", skipped: {}, hubs: [] });
    expect(cardState(r, "hubs").state).toBe("not-computed");
    expect(cardState(r, "cycles").state).toBe("list"); // data from phase 1 stays a list
  });
  it("an empty list after phase 2 is none; a full one is a list", () => {
    expect(cardState(mk({ hubs: [] }), "hubs").state).toBe("none");
    expect(cardState(mk({ cycles: [], cycle_count: 0 }), "cycles").state).toBe("none");
    expect(fx.authorities.length).toBeGreaterThan(0);
    expect(cardState(fx, "authorities").state).toBe("list");
  });
  it("takes the list length as an argument", () => {
    expect(cardState(fx, "authorities", 0).state).toBe("none");
  });
});

describe("canLoadMore", () => {
  it("is offered when the list is full at a limit below the cap", () => {
    expect(canLoadMore(mk({ hubs: Array.from({ length: 25 }, (_, i) => ({ id: `h${i}`, value: 1 })) }), "hubs", 25)).toBe(true);
  });
  it("not when the list is shorter than the limit, or at the cap", () => {
    expect(canLoadMore(fx, "bottlenecks", 25)).toBe(false);
    expect(canLoadMore(mk({ hubs: Array.from({ length: 500 }, (_, i) => ({ id: `h${i}`, value: 1 })) }), "hubs", 500)).toBe(false);
  });
  it("cycles use their total", () => {
    expect(canLoadMore(mk({ cycles: [["a", "b"]], cycle_count: 4 }), "cycles", 25)).toBe(true);
    expect(canLoadMore(mk({ cycles: [["a", "b"]], cycle_count: 1 }), "cycles", 25)).toBe(false);
  });
  it("articulation uses its length like the value lists", () => {
    expect(canLoadMore(mk({ articulation: Array.from({ length: 25 }, (_, i) => `a${i}`) }), "articulation", 25)).toBe(true);
  });
});

describe("metricRows", () => {
  it("takes titles from the entry, else the titles map, and bars against the largest value", () => {
    const r = mk({ bottlenecks: [{ id: "a", title: "A", value: 4 }, { id: "b", value: 1 }, { id: "c", value: 0 }], titles: { b: "From map" } });
    expect(metricRows(r, "bottlenecks")).toEqual([
      { id: "a", title: "A", value: 4, pct: 100 },
      { id: "b", title: "From map", value: 1, pct: 25 },
      { id: "c", title: "", value: 0, pct: 0 },
    ]);
  });
  it("all-zero values give empty bars", () => {
    expect(metricRows(mk({ slack: [{ id: "a", value: 0 }] }), "slack")[0]?.pct).toBe(0);
  });
  it("cut points are ids with titles from the map and no value", () => {
    const r = mk({ articulation: ["a", "z"], titles: { a: "Title A" } });
    expect(metricRows(r, "articulation")).toEqual([
      { id: "a", title: "Title A", value: 0, pct: 0 },
      { id: "z", title: "", value: 0, pct: 0 },
    ]);
  });
  it("cycles have no rows", () => {
    expect(metricRows(fx, "cycles")).toEqual([]);
  });
});

describe("cycleChains and orphanRows", () => {
  it("a cycle is a chain with a caption of titles, falling back to the id", () => {
    const r = mk({ cycles: [["a", "b", "c"]], titles: { a: "A", b: "B" } });
    expect(cycleChains(r)).toEqual([{ ids: ["a", "b", "c"], caption: "A → B → c" }]);
  });
  it("the fixture's cycle is titled", () => {
    const c = cycleChains(fx);
    expect(c.length).toBe(fx.cycles.length);
    expect(c[0]?.caption).toContain("→");
  });
  it("orphans carry titles", () => {
    expect(orphanRows(mk({ orphans: ["a", "q"], titles: { a: "A" } }))).toEqual([
      { id: "a", title: "A" },
      { id: "q", title: "" },
    ]);
  });
  it("listLen counts entries, ids and cycles", () => {
    expect(listLen(mk({ cycles: [["a", "b"], ["c", "d"]] }), "cycles")).toBe(2);
    expect(listLen(mk({ articulation: ["a"] }), "articulation")).toBe(1);
  });
});

describe("sparkline", () => {
  const week = (closed: number, i: number) => ({ week_start: `2026-01-0${i + 1}T00:00:00Z`, closed });
  it("reverses newest-first weeks and keeps the last 8", () => {
    const weekly = Array.from({ length: 10 }, (_, i) => week(i, i)); // newest first: closed 0 is the newest
    expect(sparkWeeks(weekly)).toEqual([7, 6, 5, 4, 3, 2, 1, 0]);
  });
  it("takes what there is, and nothing for no velocity", () => {
    expect(sparkWeeks([week(1, 0), week(2, 1)])).toEqual([2, 1]);
    expect(sparkWeeks(undefined)).toEqual([]);
  });
  it("draws 6px bars at an 8px pitch, 2..22 tall, dim when empty", () => {
    expect(sparkBars([0, 5, 10])).toEqual([
      { x: 1, y: 22, h: 2, dim: true },
      { x: 9, y: 13, h: 11, dim: false },
      { x: 17, y: 2, h: 22, dim: false },
    ]);
  });
  it("all zero does not divide by zero", () => {
    expect(sparkBars([0, 0]).every((b) => b.h === 2)).toBe(true);
    expect(sparkBars([])).toEqual([]);
  });
});

describe("health strip", () => {
  it("pending or first fetch with no data is skeleton tiles", () => {
    expect(healthState({ data: null, error: null, pending: true })).toEqual({ state: "skeleton", message: "" });
    expect(healthState(res<TriageResponse>(null)).state).toBe("skeleton");
  });
  it("an error with no data is the unavailable line", () => {
    expect(healthState(res<TriageResponse>(null, err("analysis failed")))).toEqual({ state: "unavailable", message: "analysis failed" });
  });
  it("data is tiles, even with an error (the old copy stays)", () => {
    expect(healthState(res(tri, err())).state).toBe("tiles");
  });
  it("builds the tiles from the triage, with density and orphans from the insights", () => {
    const h = healthTiles(tri, fx);
    expect(h.ready).toBe(tri.project_health.counts.actionable);
    expect(h.blocked).toBe(tri.project_health.counts.dependency_blocked);
    expect(h.inProgress).toBe(tri.project_health.counts.by_status["in_progress"] ?? 0);
    expect(h.graph.nodes).toBe(tri.project_health.graph.node_count);
    expect(h.graph.density).toBe(fx.cluster_density.toFixed(3));
    expect(h.graph.orphans).toBe(fx.orphan_count);
  });
  it("without insights the graph tile has no density or orphans", () => {
    expect(healthTiles(tri, null).graph).toMatchObject({ density: null, orphans: null });
  });
  it("velocity is optional", () => {
    const t = { ...tri, project_health: { ...tri.project_health } } as TriageResponse;
    delete t.project_health.velocity;
    expect(healthTiles(t, null).closed).toBeNull();
    expect(healthTiles(tri, null).closed?.weeks.length).toBeLessThanOrEqual(8);
  });
  it("the average days to close reads only when something closed", () => {
    const withVelocity = (closed: number): TriageResponse =>
      ({
        ...tri,
        project_health: {
          ...tri.project_health,
          velocity: { closed_last_7_days: 0, closed_last_30_days: closed, avg_days_to_close: 2.345, weekly: [] },
        },
      }) as TriageResponse;
    expect(healthTiles(withVelocity(4), null).closed).toEqual({ last30: 4, avgDays: "2.3", weeks: [] });
    expect(healthTiles(withVelocity(0), null).closed).toEqual({ last30: 0, avgDays: null, weeks: [] });
  });
  it("missing in_progress is zero", () => {
    const t = { ...tri, project_health: { ...tri.project_health, counts: { ...tri.project_health.counts, by_status: {} } } } as TriageResponse;
    expect(healthTiles(t, null).inProgress).toBe(0);
  });
});
