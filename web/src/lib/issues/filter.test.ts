import { describe, expect, it } from "vitest";
import {
  defaultFilters,
  isDefaultFilters,
  isTriggered,
  nextModePick,
  passesFilters,
  resolveMode,
  stateMatches,
  type ListFilters,
  type Mode,
} from "./filter";
import { mkItem } from "./test-helpers";

describe("State filter", () => {
  it("Active is not closed, plus closed awaiting sign-off", () => {
    expect(stateMatches(mkItem("a"), "active")).toBe(true);
    expect(stateMatches(mkItem("a", { status: "in_progress" }), "active")).toBe(true);
    expect(stateMatches(mkItem("a", { status: "closed" }), "active")).toBe(false);
    expect(stateMatches(mkItem("a", { status: "closed", signoff: "none" }), "active")).toBe(false);
    expect(stateMatches(mkItem("a", { status: "closed", signoff: "approved" }), "active")).toBe(false);
    expect(stateMatches(mkItem("a", { status: "closed", signoff: "waived" }), "active")).toBe(false);
    expect(stateMatches(mkItem("a", { status: "closed", signoff: "pending" }), "active")).toBe(true);
    expect(stateMatches(mkItem("a", { status: "closed", signoff: "disputed" }), "active")).toBe(true);
  });

  it("covers open, ready, closed and all", () => {
    const closed = mkItem("c", { status: "closed", signoff: "pending" });
    const open = mkItem("o");
    const blocked = mkItem("b", { ready: false, blocked: true });
    expect(stateMatches(closed, "open")).toBe(false);
    expect(stateMatches(open, "open")).toBe(true);
    expect(stateMatches(open, "ready")).toBe(true);
    expect(stateMatches(blocked, "ready")).toBe(false);
    expect(stateMatches(closed, "closed")).toBe(true);
    expect(stateMatches(open, "closed")).toBe(false);
    expect(stateMatches(closed, "all")).toBe(true);
  });
});

describe("passesFilters", () => {
  const f = (over: Partial<ListFilters>): ListFilters => ({ ...defaultFilters(), ...over });

  it("passes everything active by default", () => {
    expect(isDefaultFilters(defaultFilters())).toBe(true);
    expect(passesFilters(mkItem("a"), defaultFilters())).toBe(true);
  });

  it("is OR within a chip and AND across chips", () => {
    const item = mkItem("a", { priority: 1, issue_type: "bug", labels: ["x", "y"], assignee: "ada" });
    expect(passesFilters(item, f({ priority: [0, 1] }))).toBe(true);
    expect(passesFilters(item, f({ priority: [0, 2] }))).toBe(false);
    expect(passesFilters(item, f({ label: ["y", "z"] }))).toBe(true);
    expect(passesFilters(item, f({ label: ["z"] }))).toBe(false);
    expect(passesFilters(item, f({ type: ["bug"], label: ["x"], assignee: ["ada"] }))).toBe(true);
    expect(passesFilters(item, f({ type: ["bug"], label: ["z"] }))).toBe(false);
  });

  it("treats the empty assignee as unassigned", () => {
    expect(passesFilters(mkItem("a"), f({ assignee: [""] }))).toBe(true);
    expect(passesFilters(mkItem("a", { assignee: "ada" }), f({ assignee: [""] }))).toBe(false);
    expect(passesFilters(mkItem("a", { assignee: "ada" }), f({ assignee: ["", "ada"] }))).toBe(true);
  });
});

describe("mode", () => {
  const filtered = { ...defaultFilters(), state: "closed" as const };

  it("is triggered by a query or any non-default filter", () => {
    expect(isTriggered("", defaultFilters())).toBe(false);
    expect(isTriggered("  ", defaultFilters())).toBe(false);
    expect(isTriggered("x", defaultFilters())).toBe(true);
    expect(isTriggered("", filtered)).toBe(true);
    expect(isTriggered("", { ...defaultFilters(), assignee: [""] })).toBe(true);
  });

  it("resolves to flat while triggered unless the user picked", () => {
    expect(resolveMode(null, false)).toBe("tree");
    expect(resolveMode(null, true)).toBe("flat");
    expect(resolveMode("tree", true)).toBe("tree");
    expect(resolveMode("flat", false)).toBe("flat");
  });

  it("nextModePick: auto flat, forced tree resets on clear, explicit flat survives", () => {
    let pick: Mode | null = null;
    // typing a query: automatic, nothing to remember
    expect(resolveMode(pick, true)).toBe("flat");
    // the user forces Tree while flat results show
    pick = nextModePick("tree", true);
    expect(pick).toBe("tree");
    expect(resolveMode(pick, true)).toBe("tree");
    // clearing the query drops the forced pick, so the next query goes flat again
    pick = nextModePick(pick, false);
    expect(pick).toBeNull();
    expect(resolveMode(pick, true)).toBe("flat");
    // an explicit Flat survives clearing
    pick = nextModePick("flat", true);
    pick = nextModePick(pick, false);
    expect(pick).toBe("flat");
    expect(resolveMode(pick, false)).toBe("flat");
    // picking Tree with nothing triggered is just the automatic mode
    expect(nextModePick("tree", false)).toBeNull();
  });
});

describe("sticky rows", () => {
  const approved = mkItem("a", { status: "closed", signoff: "approved" });
  it("keeps a just-written issue in Active, and only there", () => {
    const sticky = new Set(["a"]);
    expect(stateMatches(approved, "active")).toBe(false);
    expect(stateMatches(approved, "active", sticky)).toBe(true);
    expect(stateMatches(approved, "active", new Set(["other"]))).toBe(false);
    expect(stateMatches(approved, "open", sticky)).toBe(false);
    expect(stateMatches(approved, "closed", sticky)).toBe(true);
  });

  it("passesFilters honours it for the state only; the other facets still apply", () => {
    const sticky = new Set(["a"]);
    expect(passesFilters(approved, defaultFilters(), sticky)).toBe(true);
    expect(passesFilters(approved, { ...defaultFilters(), priority: [0] }, sticky)).toBe(false);
  });
});
