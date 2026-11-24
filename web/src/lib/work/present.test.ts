import { describe, expect, it } from "vitest";
import { barPct, formatMetric, plural, severityTone } from "./present";

describe("barPct", () => {
  it.each([
    [0, 10, 0],
    [5, 10, 50],
    [10, 10, 100],
    [3.04, 10, 30],
    [12, 10, 100], // clamped
    [-1, 10, 0], // clamped
    [1, 0, 0], // no maximum
    [1, -5, 0],
    [Number.NaN, 10, 0],
    [0.996, 1, 100], // not capped at 99 like percent()
  ])("barPct(%s, %s) = %s", (v, max, want) => {
    expect(barPct(v, max)).toBe(want);
  });
});

describe("formatMetric", () => {
  it.each([
    [3, "int", "3"],
    [2.6, "int", "3"],
    [0, "int", "0"],
    [0, "dec", "0"],
    [0.0004, "dec", "<0.001"],
    [0.001, "dec", "0.001"],
    [0.0094, "dec", "0.009"],
    [0.01, "dec", "0.01"],
    [0.456, "dec", "0.46"],
    [12.345, "dec", "12.35"],
  ] as const)("formatMetric(%s, %s) = %s", (v, f, want) => {
    expect(formatMetric(v, f)).toBe(want);
  });
});

describe("severityTone", () => {
  it.each([
    ["critical", "danger"],
    ["error", "danger"],
    ["warning", "warning"],
    ["info", "info"],
    ["", "info"],
    ["something-new", "info"],
  ] as const)("%s -> %s", (sev, want) => {
    expect(severityTone(sev)).toBe(want);
  });
});

describe("plural", () => {
  it("picks the form", () => {
    expect(plural(1, "issue", "issues")).toBe("1 issue");
    expect(plural(0, "issue", "issues")).toBe("0 issues");
    expect(plural(7, "issue", "issues")).toBe("7 issues");
  });
});
