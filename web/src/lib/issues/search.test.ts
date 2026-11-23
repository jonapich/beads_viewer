import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MAX_QUERY_BYTES, queryTooLong, SearchRunner } from "./search.svelte";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });
const hits = (...ids: string[]) => json({ query: "q", total: ids.length, hits: ids.map((id, i) => ({ id, score: 10 - i })) });

let fetchMock: ReturnType<typeof vi.fn>;
let calls: Array<{ url: string; signal: AbortSignal | null | undefined; resolve: (r: Response) => void }>;

beforeEach(() => {
  calls = [];
  fetchMock = vi.fn(
    (url: string, init: RequestInit) =>
      new Promise<Response>((resolve, reject) => {
        calls.push({ url, signal: init.signal, resolve });
        init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("queryTooLong", () => {
  it("counts bytes, not characters", () => {
    expect(queryTooLong("a".repeat(MAX_QUERY_BYTES))).toBe(false);
    expect(queryTooLong("a".repeat(MAX_QUERY_BYTES + 1))).toBe(true);
    expect(queryTooLong("é".repeat(MAX_QUERY_BYTES / 2))).toBe(false);
    expect(queryTooLong("é".repeat(MAX_QUERY_BYTES / 2 + 1))).toBe(true);
  });
});

describe("SearchRunner", () => {
  it("delivers hits as id -> rank position", async () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("  cart ");
    expect(r.pending).toBe(true);
    expect(calls[0]?.url).toBe("/api/v1/search?q=cart");
    calls[0]?.resolve(hits("b", "a"));
    await flush();
    expect(r.pending).toBe(false);
    expect([...(onrank.mock.calls[0]?.[0] as Map<string, number>)]).toEqual([
      ["b", 0],
      ["a", 1],
    ]);
  });

  it("aborts the previous request and keeps its ranking out", async () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("ca");
    r.run("cart");
    expect(calls[0]?.signal?.aborted).toBe(true);
    calls[0]?.resolve(hits("stale"));
    calls[1]?.resolve(hits("fresh"));
    await flush();
    expect(onrank).toHaveBeenCalledTimes(1);
    expect([...(onrank.mock.calls[0]?.[0] as Map<string, number>).keys()]).toEqual(["fresh"]);
  });

  it("clears the ranking for an empty query without a request", () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("   ");
    expect(onrank).toHaveBeenCalledWith(null);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(r.pending).toBe(false);
  });

  it("an empty query also cancels a request in flight", async () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("cart");
    r.run("");
    expect(calls[0]?.signal?.aborted).toBe(true);
    calls[0]?.resolve(hits("x"));
    await flush();
    expect(onrank).toHaveBeenCalledTimes(1);
    expect(onrank).toHaveBeenCalledWith(null);
  });

  it("skips the call for a query over 256 bytes and leaves the ranking alone", () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("x".repeat(257));
    expect(r.tooLong).toBe(true);
    expect(r.pending).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(onrank).not.toHaveBeenCalled();
    r.run("ok");
    expect(r.tooLong).toBe(false);
  });

  it("records a failure and keeps the previous ranking", async () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("cart");
    calls[0]?.resolve(json({ error: { code: "internal", message: "boom" } }, { status: 500 }));
    await flush();
    expect(r.error?.status).toBe(500);
    expect(r.pending).toBe(false);
    expect(onrank).not.toHaveBeenCalled();
  });

  it("stop() aborts quietly", async () => {
    const onrank = vi.fn();
    const r = new SearchRunner(onrank);
    r.run("cart");
    r.stop();
    await flush();
    expect(r.pending).toBe(false);
    expect(r.error).toBeNull();
    expect(onrank).not.toHaveBeenCalled();
  });
});
