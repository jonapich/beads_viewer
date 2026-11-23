import { afterEach, describe, expect, it, vi } from "vitest";
import type { MetaResponse } from "../api/types";
import { loadPrefs, MAX_EXPANDED, parsePrefs, prefsKey, savePrefs, serializePrefs } from "./persist";

afterEach(() => {
  vi.unstubAllGlobals();
});

const meta = (source: Partial<MetaResponse["source"]>): MetaResponse =>
  ({ source: { path: "/p/.beads/issues.jsonl", kind: "jsonl", tracker: "bd", project_dir: "/p", ...source } }) as MetaResponse;

describe("prefs", () => {
  it("keys by project dir, else path", () => {
    expect(prefsKey(meta({}))).toBe("bv.issues.v1:/p");
    expect(prefsKey(meta({ project_dir: "" }))).toBe("bv.issues.v1:/p/.beads/issues.jsonl");
    expect(prefsKey(null)).toBe("bv.issues.v1:");
  });

  it("round-trips expansion and sort", () => {
    const p = { expanded: ["a", "b"], sort: "triage" as const };
    expect(parsePrefs(serializePrefs(p))).toEqual(p);
    expect(parsePrefs(serializePrefs({ expanded: [] }))).toEqual({ expanded: [] });
  });

  it("ignores a stored filters key (filters are URL-only)", () => {
    expect(parsePrefs('{"expanded":["a"],"filters":{"state":"closed"}}')).toEqual({ expanded: ["a"] });
  });

  it("rejects bad input and drops bad fields", () => {
    expect(parsePrefs(null)).toBeNull();
    expect(parsePrefs("not json")).toBeNull();
    expect(parsePrefs("42")).toBeNull();
    expect(parsePrefs('{"sort":"triage"}')).toBeNull();
    expect(parsePrefs('{"expanded":["a",3,null],"sort":"bogus"}')).toEqual({ expanded: ["a"] });
  });

  it("caps the expanded list", () => {
    const many = Array.from({ length: MAX_EXPANDED + 5 }, (_, i) => `id${i}`);
    expect(parsePrefs(serializePrefs({ expanded: many }))?.expanded).toHaveLength(MAX_EXPANDED);
  });
});

describe("storage access", () => {
  it("loads and saves through localStorage", () => {
    const store = new Map<string, string>();
    vi.stubGlobal("localStorage", { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) });
    expect(loadPrefs("k")).toBeNull();
    savePrefs("k", { expanded: ["x"], sort: "updated" });
    expect(loadPrefs("k")).toEqual({ expanded: ["x"], sort: "updated" });
  });

  it("swallows storage errors", () => {
    const boom = () => {
      throw new Error("blocked");
    };
    vi.stubGlobal("localStorage", { getItem: boom, setItem: boom });
    expect(loadPrefs("k")).toBeNull();
    expect(() => savePrefs("k", { expanded: [] })).not.toThrow();
  });

  it("copes with storage being absent", () => {
    expect(loadPrefs("k")).toBeNull();
    expect(() => savePrefs("k", { expanded: [] })).not.toThrow();
  });
});
