import { describe, expect, it } from "vitest";
import fixture from "../api/__fixtures__/issues-tree.json";
import type { IssuesResponse } from "../api/types";
import { awaitsSignoff, buildIndex, canAnswer, displayStatus, isBlockedAny, needsAnswer } from "./model";
import { mkItem, mkResponse } from "./test-helpers";

const fx: IssuesResponse = fixture;

describe("buildIndex", () => {
  it("indexes the golden fixture", () => {
    const ix = buildIndex(fx);
    expect(ix.byId.size).toBe(12);
    expect(ix.byId.get("fx-b")?.title).toBe("Payment form");
    expect(ix.childrenByParent.get("fx-epic")).toEqual(["fx-a", "fx-b", "fx-c"]);
    expect(ix.childrenByParent.get("fx-epic2")).toEqual(["fx-x"]);
    expect(ix.closedChildren.get("fx-epic")).toBeUndefined();
    expect(ix.tree.roots).toHaveLength(7);
  });

  it("counts closed children and facets", () => {
    const resp = mkResponse(
      [
        mkItem("e", { issue_type: "epic", labels: ["x"] }),
        mkItem("a", { parent_id: "e", status: "closed", labels: ["x", "y"], assignee: "ada" }),
        mkItem("b", { parent_id: "e" }),
        mkItem("c", { parent_id: "gone" }),
      ],
      ["e", "c"],
    );
    const ix = buildIndex(resp);
    expect(ix.closedChildren.get("e")).toBe(1);
    expect(ix.childrenByParent.get("e")).toEqual(["a", "b"]);
    expect(ix.childrenByParent.has("gone")).toBe(false);
    expect(Object.fromEntries(ix.facets.types)).toEqual({ epic: 1, task: 3 });
    expect(Object.fromEntries(ix.facets.labels)).toEqual({ x: 2, y: 1 });
    expect(Object.fromEntries(ix.facets.assignees)).toEqual({ "": 3, ada: 1 });
  });

  it("falls back to a roots-only tree when the response has none", () => {
    const ix = buildIndex({ phase2_ready: true, total: 2, items: [mkItem("a"), mkItem("b")] });
    expect(ix.tree).toEqual({ roots: ["a", "b"], children: {} });
  });
});

describe("status helpers", () => {
  const now = Date.parse("2026-03-10T00:00:00Z");
  it("derives the displayed status", () => {
    expect(displayStatus(mkItem("a", { status: "closed", blocked: true }), now)).toBe("closed");
    expect(displayStatus(mkItem("a", { status: "in_progress" }), now)).toBe("in_progress");
    expect(displayStatus(mkItem("a", { blocked: true }), now)).toBe("blocked");
    expect(displayStatus(mkItem("a", { status: "blocked" }), now)).toBe("blocked");
    expect(displayStatus(mkItem("a", { status: "deferred" }), now)).toBe("deferred");
    expect(displayStatus(mkItem("a", { defer_until: "2026-03-30T00:00:00Z" }), now)).toBe("deferred");
    expect(displayStatus(mkItem("a", { defer_until: "2026-03-01T00:00:00Z" }), now)).toBe("open");
    expect(displayStatus(mkItem("a"), now)).toBe("open");
  });

  it("flags attention", () => {
    expect(isBlockedAny(mkItem("a", { blocked: true }))).toBe(true);
    expect(isBlockedAny(mkItem("a", { blocked: true, status: "closed" }))).toBe(false);
    expect(needsAnswer(mkItem("a", { labels: ["needs-design"] }))).toBe(true);
    expect(needsAnswer(mkItem("a", { labels: ["needs-signoff"] }))).toBe(false);
    expect(awaitsSignoff(mkItem("a", { status: "closed", signoff: "pending" }))).toBe(true);
    expect(awaitsSignoff(mkItem("a", { status: "open", signoff: "pending" }))).toBe(false);
  });
});

describe("write gating helpers", () => {
  it("awaitsSignoff: a closed issue whose sign-off is pending, nothing else", () => {
    expect(awaitsSignoff(mkItem("a", { status: "closed", signoff: "pending" }))).toBe(true);
    for (const signoff of ["approved", "disputed", "waived", "none", undefined]) {
      expect(awaitsSignoff(mkItem("a", { status: "closed", signoff }))).toBe(false);
    }
    expect(awaitsSignoff(mkItem("a", { status: "in_progress", signoff: "pending" }))).toBe(false);
  });

  it("canAnswer: an open issue labelled needs-decision or needs-design", () => {
    expect(canAnswer(mkItem("a", { labels: ["needs-decision"] }))).toBe(true);
    expect(canAnswer(mkItem("a", { labels: ["x", "needs-design"], status: "in_progress" }))).toBe(true);
    expect(canAnswer(mkItem("a", { labels: ["needs-signoff"] }))).toBe(false);
    expect(canAnswer(mkItem("a"))).toBe(false);
  });

  it("canAnswer is false for a closed issue that still carries the label (the server would refuse)", () => {
    const closed = mkItem("a", { status: "closed", labels: ["needs-decision"] });
    expect(needsAnswer(closed)).toBe(true);
    expect(canAnswer(closed)).toBe(false);
  });
});
