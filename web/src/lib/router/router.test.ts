import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { replaceQuery, route, startRouter, type RouteDef } from "./router.svelte";

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

describe("route redirects", () => {
  const Page = (() => {}) as unknown as RouteDef["component"];
  const table: RouteDef[] = [
    { pattern: "/", title: "Home", component: Page, redirect: "/desk" },
    { pattern: "/desk", title: "Desk", component: Page },
    { pattern: "*", title: "Nope", component: Page },
  ];
  let stop: () => void = () => {};

  beforeEach(() => {
    vi.stubGlobal("window", { addEventListener: vi.fn(), removeEventListener: vi.fn() });
    vi.stubGlobal("document", { addEventListener: vi.fn(), removeEventListener: vi.fn(), title: "" });
  });
  afterEach(() => stop());

  it("replaces / with its target, dropping the query and hash, and resolves the target", () => {
    current = new URL("http://127.0.0.1:7777/?tab=x#h");
    stop = startRouter(table);
    expect(replaceState).toHaveBeenCalledTimes(1);
    const [, , url] = replaceState.mock.calls[0] as [unknown, string, URL];
    expect(url.pathname + url.search + url.hash).toBe("/desk");
    expect(route.path).toBe("/desk");
    expect(route.def?.title).toBe("Desk");
  });

  it("leaves other paths alone", () => {
    current = new URL("http://127.0.0.1:7777/desk?tab=signoff");
    stop = startRouter(table);
    expect(replaceState).not.toHaveBeenCalled();
    expect(route.path).toBe("/desk");
    expect(route.query.get("tab")).toBe("signoff");
  });

  it("does not loop on a redirect to its own path", () => {
    current = new URL("http://127.0.0.1:7777/");
    stop = startRouter([{ pattern: "/", title: "Self", component: Page, redirect: "/" }]);
    expect(replaceState).not.toHaveBeenCalled();
    expect(route.path).toBe("/");
  });
});
