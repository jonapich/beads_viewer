import { describe, expect, it } from "vitest";
import { match } from "./match";

describe("match", () => {
  it("matches exact paths", () => {
    expect(match("/issues", "/issues")).toEqual({});
    expect(match("/", "/")).toEqual({});
  });

  it("rejects different paths and lengths", () => {
    expect(match("/issues", "/triage")).toBeNull();
    expect(match("/issues", "/issues/x")).toBeNull();
    expect(match("/issues/:id", "/issues")).toBeNull();
  });

  it("captures params", () => {
    expect(match("/issues/:id", "/issues/bd-12")).toEqual({ id: "bd-12" });
  });

  it("decodes captured params", () => {
    expect(match("/issues/:id", "/issues/a%2Fb%20c")).toEqual({ id: "a/b c" });
  });

  it("returns null for malformed percent-encoding", () => {
    expect(match("/issues/:id", "/issues/%E0%A4%A")).toBeNull();
  });

  it("ignores a trailing slash", () => {
    expect(match("/issues", "/issues/")).toEqual({});
    expect(match("/issues/:id", "/issues/x/")).toEqual({ id: "x" });
  });

  it("wildcard matches anything", () => {
    expect(match("*", "/nope/at/all")).toEqual({});
    expect(match("*", "/")).toEqual({});
  });
});