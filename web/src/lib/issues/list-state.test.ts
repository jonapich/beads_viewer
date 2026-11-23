import { describe, expect, it } from "vitest";
import { defaultFilters } from "./filter";
import { buildIndex } from "./model";
import { IssueListState, type HydrateArgs } from "./list-state.svelte";
import { parsePrefs } from "./persist";
import { fromQuery } from "./url";
import { mkItem, mkResponse } from "./test-helpers";

const ix = buildIndex(
  mkResponse([mkItem("e"), mkItem("a", { parent_id: "e" }), mkItem("b"), mkItem("c", { parent_id: "b" })], ["e", "b"], {
    e: ["a"],
    b: ["c"],
  }),
);

const args = (search: string, over: Partial<HydrateArgs> = {}): HydrateArgs => {
  return { tree: ix.tree, byId: ix.byId, url: fromQuery(new URLSearchParams(search)), prefs: null, ...over };
};

describe("IssueListState mode", () => {
  it("is tree until a query or filter, then flat", () => {
    const s = new IssueListState();
    expect(s.mode).toBe("tree");
    s.setQuery("cart");
    expect(s.mode).toBe("flat");
    s.setQuery("");
    expect(s.mode).toBe("tree");
    s.setFilters({ ...defaultFilters(), state: "closed" });
    expect(s.mode).toBe("flat");
    s.clearFilters();
    expect(s.mode).toBe("tree");
  });

  it("forces Tree while filtered and forgets it when the trigger clears", () => {
    const s = new IssueListState();
    s.setQuery("x");
    s.setMode("tree");
    expect(s.mode).toBe("tree");
    expect(s.pruning).toBe(true);
    s.setQuery("");
    expect(s.modePick).toBeNull();
    s.setQuery("y");
    expect(s.mode).toBe("flat");
  });

  it("keeps an explicit Flat across clearing", () => {
    const s = new IssueListState();
    s.setMode("flat");
    s.setQuery("x");
    s.setQuery("");
    expect(s.mode).toBe("flat");
    s.setMode("tree");
    expect(s.mode).toBe("tree");
    expect(s.modePick).toBeNull();
  });
});

describe("IssueListState expansion", () => {
  it("toggles per id, immutably", () => {
    const s = new IssueListState();
    const before = s.expanded;
    s.toggle("e");
    expect(s.isExpanded("e")).toBe(true);
    expect(before.has("e")).toBe(false);
    s.toggle("e");
    expect(s.isExpanded("e")).toBe(false);
  });

  it("expands and collapses all", () => {
    const s = new IssueListState();
    s.expandAll(new Set(["e", "b"]));
    expect(s.isExpanded("b")).toBe(true);
    s.collapseAll(new Set(["e", "b"]));
    expect(s.isExpanded("b")).toBe(false);
  });

  it("opens everything while pruning and collapses by exception", () => {
    const s = new IssueListState();
    s.setQuery("x");
    s.setMode("tree");
    expect(s.isExpanded("e")).toBe(true);
    s.toggle("e");
    expect(s.isExpanded("e")).toBe(false);
    expect(s.expanded.has("e")).toBe(false);
    s.expandAll(new Set(["e"]));
    expect(s.isExpanded("e")).toBe(true);
    s.collapseAll(new Set(["e", "b"]));
    expect(s.isExpanded("b")).toBe(false);
  });
});

describe("IssueListState.hydrate", () => {
  it("defaults to the roots with children and the default filters", () => {
    const s = new IssueListState();
    s.hydrate(args(""));
    expect(s.hydrated).toBe(true);
    expect([...s.expanded].sort()).toEqual(["b", "e"]);
    expect(s.filters).toEqual(defaultFilters());
    expect(s.mode).toBe("tree");
  });

  it("applies the URL over remembered prefs, and drops unknown expanded ids", () => {
    const s = new IssueListState();
    s.hydrate(
      args("state=closed&sort=updated&peek=a&mode=flat", { prefs: { expanded: ["e", "gone"], sort: "triage" } }),
    );
    expect(s.filters.state).toBe("closed");
    expect(s.sortPick).toBe("updated");
    expect(s.peek).toBe("a");
    expect(s.modePick).toBe("flat");
    expect([...s.expanded]).toEqual(["e"]);
  });

  it("falls back to the remembered sort", () => {
    const s = new IssueListState();
    s.hydrate(args("", { prefs: { expanded: [], sort: "triage" } }));
    expect(s.sortPick).toBe("triage");
    expect(s.expanded.size).toBe(0);
  });

  it("gives default filters on a bare URL even if storage holds some (filters are URL-only)", () => {
    const stored = { expanded: [], sort: "updated", filters: { state: "closed", type: ["bug"] } };
    const prefs = parsePrefs(JSON.stringify(stored));
    const s = new IssueListState();
    s.hydrate(args("", { prefs }));
    expect(s.filters).toEqual(defaultFilters());
    expect(s.mode).toBe("tree");
  });
  it("drops a forced tree pick when nothing is filtered", () => {
    const s = new IssueListState();
    s.hydrate(args("mode=tree"));
    expect(s.modePick).toBeNull();
    const f = new IssueListState();
    f.hydrate(args("mode=tree&state=all"));
    expect(f.modePick).toBe("tree");
  });
});

describe("IssueListState.applyUrl", () => {
  const url = (search: string) => fromQuery(new URLSearchParams(search));

  it("adopts a bare URL: defaults back, forced tree reset, expansion kept", () => {
    const s = new IssueListState();
    s.hydrate(args("state=closed&mode=tree&peek=a&sort=updated"));
    s.expandAll(new Set(["e"]));
    expect(s.mode).toBe("tree");
    expect(s.pruning).toBe(true);
    s.applyUrl(url(""));
    expect(s.filters).toEqual(defaultFilters());
    expect(s.modePick).toBeNull();
    expect(s.mode).toBe("tree");
    expect(s.pruning).toBe(false);
    expect(s.peek).toBeNull();
    expect(s.sortPick).toBeNull();
    expect(s.query).toBe("");
    expect(s.expanded.has("e")).toBe(true);
  });

  it("adopts filters from another /issues entry and falls back to the remembered sort", () => {
    const s = new IssueListState();
    s.hydrate(args(""));
    s.applyUrl(url("state=all&type=bug&mode=flat"), "triage");
    expect(s.filters).toEqual({ ...defaultFilters(), state: "all", type: ["bug"] });
    expect(s.modePick).toBe("flat");
    expect(s.sortPick).toBe("triage");
    s.applyUrl(url("sort=updated"), "triage");
    expect(s.sortPick).toBe("updated");
  });
});
describe("IssueListState reveal and back", () => {
  it("leaves flat results, opens the path, and Back restores the mode", () => {
    const s = new IssueListState();
    s.hydrate(args(""));
    s.setQuery("x");
    expect(s.mode).toBe("flat");
    s.beginReveal(120);
    expect(s.mode).toBe("tree");
    expect(s.backTo).toEqual({ scrollTop: 120, modePick: null });
    expect(s.pruning).toBe(true);
    s.toggle("e"); // collapsed from the all-open view
    s.expandPath(["e"]);
    expect(s.isExpanded("e")).toBe(true);
    expect(s.expanded.has("e")).toBe(true);

    expect(s.back()).toBe(120);
    expect(s.backTo).toBeNull();
    expect(s.mode).toBe("flat");
    expect(s.back()).toBeNull();
  });

  it("restores an explicit Flat pick on Back", () => {
    const s = new IssueListState();
    s.hydrate(args(""));
    s.setMode("flat");
    s.beginReveal(0);
    expect(s.mode).toBe("tree");
    s.back();
    expect(s.modePick).toBe("flat");
    expect(s.mode).toBe("flat");
  });

  it("does not offer Back when revealing from the tree, and clears it on any manual change", () => {
    const s = new IssueListState();
    s.hydrate(args(""));
    s.beginReveal(5);
    expect(s.backTo).toBeNull();
    s.setQuery("x");
    s.beginReveal(5);
    expect(s.backTo).not.toBeNull();
    s.setQuery("y");
    expect(s.backTo).toBeNull();
    s.setQuery("");
    s.setMode("flat");
    s.beginReveal(1);
    s.setMode("flat");
    expect(s.backTo).toBeNull();
    s.setMode("flat");
    s.beginReveal(1);
    s.setFilters({ ...defaultFilters(), state: "all" });
    expect(s.backTo).toBeNull();
    s.setMode("flat");
    s.beginReveal(1);
    s.applyUrl(fromQuery(new URLSearchParams("")));
    expect(s.backTo).toBeNull();
  });
});
