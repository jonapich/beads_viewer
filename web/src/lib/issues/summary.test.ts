import { describe, expect, it } from "vitest";
import { summarize, summaryText } from "./summary";
import { mkItem } from "./test-helpers";

describe("summary", () => {
  const items = [
    mkItem("a"),
    mkItem("b", { ready: false, blocked: true }),
    mkItem("c", { status: "blocked", ready: false }),
    mkItem("d", { status: "closed", ready: false, blocked: true }),
  ];

  it("counts open, ready and blocked (a closed issue is never blocked)", () => {
    expect(summarize(items)).toEqual({ total: 4, open: 3, ready: 1, blocked: 2 });
    expect(summarize([])).toEqual({ total: 0, open: 0, ready: 0, blocked: 0 });
  });

  it("words the tree summary", () => {
    expect(summaryText(summarize(items), "tree", "")).toBe("4 issues · 3 open · 1 ready · 2 blocked");
    expect(summaryText(summarize([items[0] as never]), "tree", "")).toBe("1 issue · 1 open · 1 ready · 0 blocked");
  });

  it("words flat results for a query, and ignores the query in tree mode", () => {
    expect(summaryText(summarize(items), "flat", " sign ")).toBe("4 results for “sign”");
    expect(summaryText(summarize([items[0] as never]), "flat", "sign")).toBe("1 result for “sign”");
    expect(summaryText(summarize(items), "flat", "")).toBe("4 issues · 3 open · 1 ready · 2 blocked");
    expect(summaryText(summarize(items), "tree", "sign")).toBe("4 issues · 3 open · 1 ready · 2 blocked");
  });
});
