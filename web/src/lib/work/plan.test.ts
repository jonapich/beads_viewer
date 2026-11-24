import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import planFixture from "../api/__fixtures__/plan.json";
import type { PlanItem, PlanResponse, PlanTrack } from "../api/types";
import { capItems, foldSingles, itemById, planView, showBanner, startId, summaryLine, summaryText, trackLetter } from "./plan";
import type { ResourceState } from "./triage";

const fixture = planFixture as unknown as PlanResponse;

const mkItem = (id: string): PlanItem => ({ id, title: `T ${id}`, issue_type: "task", status: "open", priority: 2, unblocks_ids: [] });
const mkTrack = (letter: string, ...ids: string[]): PlanTrack => ({ track_id: `track-${letter}`, reason: "r", items: ids.map(mkItem) });
const mk = (over: Partial<PlanResponse> = {}): PlanResponse => ({ ...fixture, ...over });
const res = (data: PlanResponse | null, error: ApiError | null = null): ResourceState<PlanResponse> => ({ data, error, pending: false });
const empty = mk({ tracks: [], total_actionable: 0, summary: { highest_impact: "", impact_reason: "", unblocks_count: 0 } });

describe("planView", () => {
  it("no data and no error is loading", () => {
    expect(planView(res(null), null).state).toBe("loading");
  });
  it("no data and an error is failed with its message", () => {
    expect(planView(res(null, new ApiError(500, "internal", "boom")), 4)).toMatchObject({ state: "failed", message: "boom" });
    expect(planView(res(null, new ApiError(0, "network", "bv serve is not reachable")), 4).state).toBe("failed");
  });
  it("tracks are data", () => {
    expect(fixture.tracks.length).toBeGreaterThan(0);
    expect(planView(res(fixture), 12)).toEqual({ state: "data", message: "", stale: false, staleTitle: "" });
  });
  it("no tracks and no issues at all is the empty tracker", () => {
    expect(planView(res(empty), 0).state).toBe("empty");
  });
  it("no tracks but issues exist is none (all blocked or done)", () => {
    expect(planView(res(empty), 5).state).toBe("none");
  });
  it("no tracks while the issue count is not known yet is none, not empty", () => {
    expect(planView(res(empty), null).state).toBe("none");
  });
  it("an error with data keeps the data and flags it stale", () => {
    expect(planView(res(fixture, new ApiError(500, "x", "refresh failed")), 12)).toMatchObject({
      state: "data",
      stale: true,
      staleTitle: "refresh failed",
    });
  });
});

describe("trackLetter", () => {
  it.each([
    ["track-A", "A"],
    ["track-AB", "AB"],
    ["other", "other"],
  ])("%s -> %s", (id, want) => {
    expect(trackLetter(id)).toBe(want);
  });
});

describe("foldSingles", () => {
  const singles = (n: number) => Array.from({ length: n }, (_, i) => mkTrack(`S${i}`, `s${i}`));
  it("shows everything up to six singles", () => {
    const t = [mkTrack("A", "a1", "a2"), ...singles(6)];
    const r = foldSingles(t);
    expect(r.shown).toEqual(t);
    expect(r.folded).toEqual([]);
  });
  it("puts multi-item tracks first, then singles, each group in server order", () => {
    const multi1 = mkTrack("A", "a1", "a2");
    const s = singles(3);
    const multi2 = mkTrack("Z", "z1", "z2");
    const t = [s[0]!, s[1]!, multi1, s[2]!, multi2];
    expect(foldSingles(t).shown.map((x) => x.track_id)).toEqual(["track-A", "track-Z", "track-S0", "track-S1", "track-S2"]);
  });
  it("folds the singles past six (server order), never the multi-item tracks", () => {
    const multi1 = mkTrack("A", "a1", "a2");
    const s = singles(9);
    const multi2 = mkTrack("Z", "z1", "z2");
    const t = [s[0]!, s[1]!, multi1, s[2]!, s[3]!, s[4]!, s[5]!, s[6]!, multi2, s[7]!, s[8]!];
    const r = foldSingles(t);
    expect(r.shown.map((x) => x.track_id)).toEqual(["track-A", "track-Z", "track-S0", "track-S1", "track-S2", "track-S3", "track-S4", "track-S5"]);
    expect(r.folded.map((x) => x.track_id)).toEqual(["track-S6", "track-S7", "track-S8"]);
  });  it("counts singles on the item count, not the reason", () => {
    const odd: PlanTrack = { track_id: "track-X", reason: "Single actionable item", items: [mkItem("a"), mkItem("b")] };
    expect(foldSingles([...singles(7), odd]).shown).toContain(odd);
  });
  it("keep is a parameter", () => {
    expect(foldSingles(singles(4), 2).folded).toHaveLength(2);
    expect(foldSingles([], 6)).toEqual({ shown: [], folded: [] });
  });
});

describe("capItems", () => {
  const items = Array.from({ length: 15 }, (_, i) => mkItem(`i${i}`));
  it("caps at twelve by default, keeping order", () => {
    const r = capItems(items);
    expect(r.shown.map((i) => i.id)).toEqual(items.slice(0, 12).map((i) => i.id));
    expect(r.hidden.map((i) => i.id)).toEqual(["i12", "i13", "i14"]);
  });
  it("hides nothing at or under the cap", () => {
    expect(capItems(items.slice(0, 12)).hidden).toEqual([]);
    expect(capItems([], 12)).toEqual({ shown: [], hidden: [] });
  });
});

describe("showBanner and startId", () => {
  const sum = (highest_impact: string, unblocks_count: number) => mk({ summary: { highest_impact, impact_reason: "r", unblocks_count } });
  it("needs an item that unblocks something", () => {
    expect(showBanner(sum("a", 2))).toBe(true);
    expect(startId(sum("a", 2))).toBe("a");
  });
  it("a highest-impact item that unblocks nothing is not a banner and not tagged", () => {
    expect(fixture.summary.highest_impact).not.toBe("");
    expect(showBanner(fixture)).toBe(fixture.summary.unblocks_count > 0);
    expect(showBanner(sum("a", 0))).toBe(false);
    expect(startId(sum("a", 0))).toBe("");
  });
  it("no highest-impact id is no banner", () => {
    expect(showBanner(sum("", 3))).toBe(false);
    expect(startId(empty)).toBe("");
  });
});

describe("itemById", () => {
  it("finds an item in any track", () => {
    const p = mk({ tracks: [mkTrack("A", "a1"), mkTrack("B", "b1", "b2")] });
    expect(itemById(p, "b2")?.id).toBe("b2");
    expect(itemById(p, "nope")).toBeUndefined();
  });
});

describe("summaryLine", () => {
  it("keeps in progress apart from the disjoint counts", () => {
    const p = mk({ tracks: [mkTrack("A", "a"), mkTrack("B", "b"), mkTrack("C", "c")], total_actionable: 7, total_blocked: 3, total_deferred_or_parked: 1, total_in_progress: 1 });
    const s = summaryLine(p);
    expect(summaryText(s)).toBe("7 actionable in 3 tracks · 3 blocked · 1 deferred or parked");
    expect(s.inProgress).toBe(1);
  });
  it("says track for exactly one and reports zero in progress", () => {
    const s = summaryLine(mk({ tracks: [mkTrack("A", "a")], total_in_progress: 0 }));
    expect(s.parts[0]?.text).toBe("actionable in 1 track");
    expect(s.inProgress).toBe(0);
  });
  it("is all zeros for an empty plan", () => {
    expect(summaryText(summaryLine(empty))).toBe("0 actionable in 0 tracks · " + `${empty.total_blocked} blocked · ${empty.total_deferred_or_parked} deferred or parked`);
  });
});
