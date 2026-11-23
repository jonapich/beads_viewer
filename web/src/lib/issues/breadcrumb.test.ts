import { describe, expect, it } from "vitest";
import { ancestors, crumbLabel, trunc } from "./breadcrumb";
import { mkItem } from "./test-helpers";

const byId = (...items: ReturnType<typeof mkItem>[]) => new Map(items.map((i) => [i.id, i]));

describe("ancestors", () => {
  it("returns the parent chain, outermost first", () => {
    const m = byId(mkItem("root"), mkItem("mid", { parent_id: "root" }), mkItem("leaf", { parent_id: "mid" }));
    expect(ancestors("leaf", m).map((i) => i.id)).toEqual(["root", "mid"]);
    expect(ancestors("mid", m).map((i) => i.id)).toEqual(["root"]);
    expect(ancestors("root", m)).toEqual([]);
  });

  it("stops at a parent that is not in the list", () => {
    const m = byId(mkItem("a", { parent_id: "gone" }));
    expect(ancestors("a", m)).toEqual([]);
  });

  it("returns nothing for an unknown id", () => {
    expect(ancestors("nope", byId())).toEqual([]);
  });

  it("stops on a parent cycle without a hop cap", () => {
    const m = byId(mkItem("a", { parent_id: "b" }), mkItem("b", { parent_id: "c" }), mkItem("c", { parent_id: "a" }));
    expect(ancestors("a", m).map((i) => i.id)).toEqual(["c", "b"]);
  });

  it("follows chains deeper than 20 levels", () => {
    const items = Array.from({ length: 40 }, (_, i) => mkItem(`n${i}`, i > 0 ? { parent_id: `n${i - 1}` } : {}));
    expect(ancestors("n39", byId(...items))).toHaveLength(39);
  });

  it("follows a self parent once", () => {
    expect(ancestors("a", byId(mkItem("a", { parent_id: "a" })))).toEqual([]);
  });
});

describe("crumb label", () => {
  it("cuts titles to 28 characters", () => {
    expect(trunc("short", 28)).toBe("short");
    expect(crumbLabel("x".repeat(28))).toBe("x".repeat(28));
    expect(crumbLabel("x".repeat(40))).toBe("x".repeat(27) + "…");
  });
});
