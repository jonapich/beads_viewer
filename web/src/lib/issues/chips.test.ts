import { describe, expect, it } from "vitest";
import { chipText, facetOptions, showClearFilters, toggleValue } from "./chips";
import { defaultFilters, type ListFilters } from "./filter";

const f = (over: Partial<ListFilters>): ListFilters => ({ ...defaultFilters(), ...over });

describe("chipText", () => {
  it("always shows State, non-default when not Active", () => {
    expect(chipText("state", defaultFilters())).toEqual({ name: "State", value: "Active", on: false });
    expect(chipText("state", f({ state: "closed" }))).toEqual({ name: "State", value: "Closed", on: true });
  });

  it("shows empty chips as off", () => {
    for (const k of ["priority", "type", "label", "assignee"] as const) {
      expect(chipText(k, defaultFilters())).toEqual({ name: expect.any(String), value: "", on: false });
    }
  });

  it("lists priorities sorted, types joined, labels as first +n, unassigned by name", () => {
    expect(chipText("priority", f({ priority: [1, 0] })).value).toBe("P0, P1");
    expect(chipText("type", f({ type: ["bug", "epic"] })).value).toBe("bug, epic");
    expect(chipText("label", f({ label: ["a"] })).value).toBe("a");
    expect(chipText("label", f({ label: ["a", "b", "c"] })).value).toBe("a +2");
    expect(chipText("assignee", f({ assignee: ["", "ada"] })).value).toBe("Unassigned, ada");
  });
});

describe("filters helpers", () => {
  it("shows Clear filters for any non-default filter", () => {
    expect(showClearFilters(defaultFilters())).toBe(false);
    expect(showClearFilters(f({ state: "all" }))).toBe(true);
    expect(showClearFilters(f({ assignee: [""] }))).toBe(true);
  });

  it("toggles a value immutably", () => {
    const a = [1, 2];
    expect(toggleValue(a, 3)).toEqual([1, 2, 3]);
    expect(toggleValue(a, 1)).toEqual([2]);
    expect(a).toEqual([1, 2]);
  });
});

describe("facetOptions", () => {
  const counts = new Map([
    ["bug", 2],
    ["task", 9],
    ["feature", 2],
  ]);

  it("sorts by count then name", () => {
    expect(facetOptions(counts, [], "").map((o) => o.value)).toEqual(["task", "bug", "feature"]);
  });

  it("marks selected values and keeps ones the data no longer has", () => {
    const o = facetOptions(counts, ["bug", "gone"], "");
    expect(o.find((x) => x.value === "bug")?.selected).toBe(true);
    expect(o.find((x) => x.value === "gone")).toMatchObject({ count: 0, selected: true });
  });

  it("filters by the search text, case-insensitively", () => {
    expect(facetOptions(counts, [], "  TA").map((o) => o.value)).toEqual(["task"]);
    expect(facetOptions(counts, [], "zzz")).toEqual([]);
  });

  it("puts Unassigned first and matches it by its label", () => {
    const m = new Map([
      ["ada", 5],
      ["", 1],
    ]);
    expect(facetOptions(m, [], "").map((o) => o.label)).toEqual(["Unassigned", "ada"]);
    expect(facetOptions(m, [], "unass").map((o) => o.value)).toEqual([""]);
  });
});
