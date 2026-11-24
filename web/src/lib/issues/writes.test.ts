import { describe, expect, it } from "vitest";
import { actionState, CLOSE_CANDIDATE, MAX_TEXT, tooLong } from "./writes";
import { mkItem } from "./test-helpers";

describe("actionState", () => {
  const signoff = mkItem("s", { status: "closed", signoff: "pending" });
  const question = mkItem("q", { labels: ["needs-design"] });
  const plain = mkItem("p");
  const closedPlain = mkItem("c", { status: "closed" });

  it("offers priority and close-candidate on every row, closed ones included", () => {
    for (const it of [signoff, question, plain, closedPlain]) {
      const s = actionState(it, true);
      expect(s.priority).toBe(true);
      expect(s.closeCandidate).toBe(true);
    }
  });

  it("offers sign-off only on a closed issue awaiting it", () => {
    expect(actionState(signoff, true)).toMatchObject({ approve: true, dispute: true, answer: false });
    for (const it of [question, plain, closedPlain, mkItem("x", { status: "closed", signoff: "approved" })]) {
      expect(actionState(it, true)).toMatchObject({ approve: false, dispute: false });
    }
  });

  it("offers answer only on an open issue waiting for a decision", () => {
    expect(actionState(question, true).answer).toBe(true);
    expect(actionState(plain, true).answer).toBe(false);
    expect(actionState(mkItem("x", { status: "closed", labels: ["needs-decision"] }), true).answer).toBe(false);
  });

  it("offers nothing when the tracker takes no writes", () => {
    for (const it of [signoff, question, plain]) {
      expect(actionState(it, false)).toMatchObject({
        priority: false,
        closeCandidate: false,
        approve: false,
        dispute: false,
        answer: false,
      });
    }
  });

  it("tells which way the close-candidate toggle goes", () => {
    expect(actionState(plain, true).isCandidate).toBe(false);
    expect(actionState(mkItem("m", { labels: ["a", CLOSE_CANDIDATE] }), true).isCandidate).toBe(true);
  });
});

describe("tooLong", () => {
  it("counts UTF-8 bytes, not characters", () => {
    expect(tooLong("a".repeat(MAX_TEXT))).toBe(false);
    expect(tooLong("a".repeat(MAX_TEXT + 1))).toBe(true);
    // 3-byte characters: 3334 of them are 10002 bytes, though only 3334 UTF-16 units.
    expect(tooLong("€".repeat(3334))).toBe(true);
    expect(tooLong("€".repeat(3333))).toBe(false);
  });
});
