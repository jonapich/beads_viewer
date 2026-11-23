import { describe, expect, it } from "vitest";
import { highlight } from "./highlight";

describe("highlight", () => {
  it("returns the text whole without a query", () => {
    expect(highlight("Payment form", "")).toEqual([{ t: "Payment form", m: false }]);
    expect(highlight("Payment form", "   ")).toEqual([{ t: "Payment form", m: false }]);
    expect(highlight("", "x")).toEqual([{ t: "", m: false }]);
  });

  it("marks case-insensitive substrings and keeps the original case", () => {
    expect(highlight("Payment form", "PAY")).toEqual([
      { t: "Pay", m: true },
      { t: "ment form", m: false },
    ]);
    expect(highlight("a cart is a cart", "cart")).toEqual([
      { t: "a ", m: false },
      { t: "cart", m: true },
      { t: " is a ", m: false },
      { t: "cart", m: true },
    ]);
  });

  it("matches the whole query, not its words", () => {
    expect(highlight("sign off the cart", "off sign")).toEqual([{ t: "sign off the cart", m: false }]);
  });

  it("treats regex metacharacters literally", () => {
    expect(highlight("a.b (c) [d] a+b", "(c)")).toEqual([
      { t: "a.b ", m: false },
      { t: "(c)", m: true },
      { t: " [d] a+b", m: false },
    ]);
    expect(highlight("axb", "a.b")).toEqual([{ t: "axb", m: false }]);
  });

  it("returns no HTML, only segments", () => {
    expect(highlight("<b>x</b>", "x")).toEqual([
      { t: "<b>", m: false },
      { t: "x", m: true },
      { t: "</b>", m: false },
    ]);
  });
});
