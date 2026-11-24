import { describe, expect, it } from "vitest";
import { decisionComment } from "./decision";

describe("decisionComment (mirror of agentdesk.DecisionComment)", () => {
  const b = { key: "b", text: "Adyen: better EU rates" };
  it("an option alone", () => {
    expect(decisionComment(b, "")).toBe("Decision: (b) Adyen: better EU rates");
    expect(decisionComment(b, "   ")).toBe("Decision: (b) Adyen: better EU rates");
  });
  it("an option with a note", () => {
    expect(decisionComment(b, "  for EU rates ")).toBe("Decision: (b) Adyen: better EU rates — for EU rates");
  });
  it("free text alone", () => {
    expect(decisionComment(null, " build it ourselves ")).toBe("Decision: build it ourselves");
    expect(decisionComment(undefined, "x")).toBe("Decision: x");
  });
});
