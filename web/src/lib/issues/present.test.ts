import { describe, expect, it } from "vitest";
import { ago, initials, percent, triageText } from "./present";
import { isNarrow, NARROW_W, ROW_H, rowHeight, windowRange } from "./rows";

describe("windowRange", () => {
  it("is empty for an empty list or a zero row height", () => {
    expect(windowRange(0, 800, 0, 32, 8)).toEqual({ start: 0, end: 0 });
    expect(windowRange(500, 800, 0, 32, 8)).toEqual({ start: 0, end: 0 });
    expect(windowRange(0, 800, 10, 0, 8)).toEqual({ start: 0, end: 0 });
  });

  it("windows the viewport plus overscan", () => {
    // 800px / 32px = 25 rows visible, 8 overscan below
    expect(windowRange(0, 800, 1000, 32, 8)).toEqual({ start: 0, end: 33 });
    // scrollTop 3200 = row 100
    expect(windowRange(3200, 800, 1000, 32, 8)).toEqual({ start: 92, end: 133 });
    // partial rows round outward
    expect(windowRange(3210, 800, 1000, 32, 8)).toEqual({ start: 92, end: 134 });
  });

  it("mounts only the overscan when the viewport is not measured yet", () => {
    expect(windowRange(0, 0, 1000, 32, 8)).toEqual({ start: 0, end: 8 });
  });

  it("clamps a list that shrank under a large scrollTop", () => {
    // 20 rows x 32 = 640px total, viewport 800: nothing to scroll, whole list shown
    expect(windowRange(64000, 800, 20, 32, 8)).toEqual({ start: 0, end: 20 });
    // 100 rows = 3200px, viewport 800: max scrollTop 2400 = row 75
    expect(windowRange(64000, 800, 100, 32, 8)).toEqual({ start: 67, end: 100 });
  });

  it("never ends past the count and never starts after the end", () => {
    for (const count of [1, 5, 40, 1000]) {
      for (const top of [-50, 0, 1, 999, 1e6]) {
        const { start, end } = windowRange(top, 800, count, 32, 8);
        expect(end).toBeLessThanOrEqual(count);
        expect(start).toBeLessThanOrEqual(end);
        expect(start).toBeGreaterThanOrEqual(0);
      }
    }
  });
});

describe("rows", () => {
  it("is the single source of row heights", () => {
    expect(rowHeight("tree", false)).toBe(32);
    expect(rowHeight("flat", false)).toBe(44);
    expect(rowHeight("tree", true)).toBe(52);
    expect(rowHeight("flat", true)).toBe(64);
    expect(ROW_H.tree).toBe(32);
  });

  it("is narrow below the container breakpoint only", () => {
    expect(isNarrow(NARROW_W - 1)).toBe(true);
    expect(isNarrow(NARROW_W)).toBe(false);
    expect(isNarrow(0)).toBe(false); // not measured yet
  });
});

describe("present", () => {
  const now = Date.parse("2026-03-10T12:00:00Z");

  it("formats ages", () => {
    expect(ago("2026-03-10T11:59:50Z", now)).toBe("1m");
    expect(ago("2026-03-10T11:15:00Z", now)).toBe("45m");
    expect(ago("2026-03-10T07:00:00Z", now)).toBe("5h");
    expect(ago("2026-03-08T12:00:00Z", now)).toBe("2d");
    expect(ago("2026-03-11T12:00:00Z", now)).toBe("1m");
    expect(ago("garbage", now)).toBe("");
  });

  it("derives initials", () => {
    expect(initials("ada")).toBe("AD");
    expect(initials("Ada Lovelace")).toBe("AL");
    expect(initials("ada.lovelace")).toBe("AL");
    expect(initials("x")).toBe("X");
  });

  it("computes progress percent, never 100 while something is open", () => {
    expect(percent(0, 0)).toBe(0);
    expect(percent(3, 12)).toBe(25);
    expect(percent(199, 200)).toBe(99);
    expect(percent(12, 12)).toBe(100);
  });

  it("shows an em dash for a missing triage score", () => {
    expect(triageText(undefined)).toBe("—");
    expect(triageText(0.2644995)).toBe("0.26");
    expect(triageText(0)).toBe("0.00");
  });
});
