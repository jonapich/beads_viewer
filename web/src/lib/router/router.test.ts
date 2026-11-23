import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { replaceQuery, route } from "./router.svelte";

let current: URL;
let replaceState: ReturnType<typeof vi.fn>;

beforeEach(() => {
  current = new URL("http://127.0.0.1:7777/issues?state=all#top");
  replaceState = vi.fn((_state: unknown, _title: string, url: URL) => {
    current = new URL(url.href);
  });
  vi.stubGlobal("location", {
    get href() {
      return current.href;
    },
  });
  vi.stubGlobal("history", { state: { n: 1 }, replaceState });
  route.query = new URLSearchParams(current.search);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("replaceQuery", () => {
  it("replaces the query without pushing, keeping the path, hash and history state", () => {
    replaceQuery(new URLSearchParams("q=sign&p=0&p=1"));
    expect(replaceState).toHaveBeenCalledTimes(1);
    const [state, , url] = replaceState.mock.calls[0] as [unknown, string, URL];
    expect(state).toEqual({ n: 1 });
    expect(url.pathname).toBe("/issues");
    expect(url.hash).toBe("#top");
    expect(url.search).toBe("?q=sign&p=0&p=1");
    expect(route.query.getAll("p")).toEqual(["0", "1"]);
    expect(route.query.get("q")).toBe("sign");
  });

  it("clears the query", () => {
    replaceQuery(new URLSearchParams());
    expect(current.search).toBe("");
    expect(route.query.toString()).toBe("");
  });

  it("does nothing when the URL would not change", () => {
    replaceQuery(new URLSearchParams("state=all"));
    expect(replaceState).not.toHaveBeenCalled();
  });
});
