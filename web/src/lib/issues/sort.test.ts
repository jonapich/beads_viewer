import { describe, expect, it } from "vitest";
import { comparator, effectiveSort, isSortKey } from "./sort";
import { mkItem } from "./test-helpers";

const ids = (keyItems: ReturnType<typeof mkItem>[], cmp: ReturnType<typeof comparator>) =>
  [...keyItems].sort(cmp).map((i) => i.id);

describe("comparator", () => {
  const a = mkItem("a", { priority: 2, created_at: "2026-03-01T00:00:00Z", updated_at: "2026-03-05T00:00:00Z", triage_score: 0.2 });
  const b = mkItem("b", { priority: 1, created_at: "2026-03-02T00:00:00Z", updated_at: "2026-03-04T00:00:00Z", triage_score: 0.9 });
  const c = mkItem("c", { priority: 2, created_at: "2026-03-03T00:00:00Z", updated_at: "2026-03-06T00:00:00Z" });
  const d = mkItem("d", { priority: 2, created_at: "2026-03-03T00:00:00Z", updated_at: "2026-03-06T00:00:00Z", triage_score: 0.2 });
  const all = [a, b, c, d];

  it("default: priority, then newest, then id", () => {
    expect(ids(all, comparator("default"))).toEqual(["b", "c", "d", "a"]);
  });

  it("priority: priority then id", () => {
    expect(ids(all, comparator("priority"))).toEqual(["b", "a", "c", "d"]);
  });

  it("triage: score descending, missing as -1, then priority, then id", () => {
    expect(ids(all, comparator("triage"))).toEqual(["b", "a", "d", "c"]);
  });

  it("created and updated sorts break ties by id", () => {
    expect(ids(all, comparator("created_desc"))).toEqual(["c", "d", "b", "a"]);
    expect(ids(all, comparator("created_asc"))).toEqual(["a", "b", "c", "d"]);
    expect(ids(all, comparator("updated"))).toEqual(["c", "d", "a", "b"]);
  });

  it("relevance orders by search rank, unranked last, then the default tie-breaks", () => {
    const rank = new Map([
      ["d", 0],
      ["a", 1],
    ]);
    expect(ids(all, comparator("relevance", rank))).toEqual(["d", "a", "b", "c"]);
    // no rank at all: everything ties on rank and falls to priority / newest / id
    expect(ids(all, comparator("relevance"))).toEqual(["b", "c", "d", "a"]);
  });
});

describe("effectiveSort", () => {
  it("uses relevance only in flat mode with a query", () => {
    expect(effectiveSort(null, "flat", true)).toBe("relevance");
    expect(effectiveSort(null, "flat", false)).toBe("default");
    expect(effectiveSort(null, "tree", true)).toBe("default");
    expect(effectiveSort("relevance", "flat", true)).toBe("relevance");
    expect(effectiveSort("relevance", "tree", true)).toBe("default");
    expect(effectiveSort("relevance", "flat", false)).toBe("default");
  });

  it("honours an explicit sort everywhere", () => {
    expect(effectiveSort("triage", "flat", true)).toBe("triage");
    expect(effectiveSort("default", "flat", true)).toBe("default");
    expect(effectiveSort("updated", "tree", false)).toBe("updated");
  });

  it("validates keys", () => {
    expect(isSortKey("priority")).toBe(true);
    expect(isSortKey("nope")).toBe(false);
    expect(isSortKey(null)).toBe(false);
  });
});
