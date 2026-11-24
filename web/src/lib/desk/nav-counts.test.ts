import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { navCounts, refreshNavCounts, startNavCounts } from "./nav-counts.svelte";
import { session } from "../session.svelte";

const meta = (awaiting: number, signoff: number) =>
  new Response(JSON.stringify({ counts: { open: 1, ready: 1, blocked: 0, closed: 0, awaiting, signoff_pending: signoff } }), {
    headers: { "Content-Type": "application/json", ETag: `"m-${awaiting}-${signoff}"` },
  });

let fetchMock: ReturnType<typeof vi.fn>;
let stop: () => void = () => {};

beforeEach(() => {
  session.status = "unknown";
  navCounts.awaiting = 0;
  navCounts.signoff_pending = 0;
  navCounts.loaded = false;
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});

describe("nav counts", () => {
  it("reads awaiting and signoff_pending from /meta", async () => {
    fetchMock.mockResolvedValue(meta(3, 2));
    stop = startNavCounts();
    await refreshNavCounts();
    expect(navCounts).toMatchObject({ awaiting: 3, signoff_pending: 2, loaded: true });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/meta");
  });

  it("refresh refetches with the last etag and picks up a change", async () => {
    fetchMock.mockResolvedValueOnce(meta(3, 2));
    stop = startNavCounts();
    await refreshNavCounts();
    fetchMock.mockResolvedValueOnce(meta(2, 2));
    await refreshNavCounts();
    expect(navCounts.awaiting).toBe(2);
    const last = fetchMock.mock.calls.at(-1) as [string, RequestInit];
    expect((last[1].headers as Record<string, string>)["If-None-Match"]).toBe('"m-3-2"');
  });

  it("keeps the counts on a 304", async () => {
    fetchMock.mockResolvedValueOnce(meta(1, 0));
    stop = startNavCounts();
    await refreshNavCounts();
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 304 }));
    await refreshNavCounts();
    expect(navCounts).toMatchObject({ awaiting: 1, signoff_pending: 0 });
  });

  it("treats a /meta without the desk counts (an older server) as zero", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ counts: { open: 1, ready: 1, blocked: 0, closed: 0 } }), { headers: { "Content-Type": "application/json" } }),
    );
    stop = startNavCounts();
    await refreshNavCounts();
    expect(navCounts).toMatchObject({ awaiting: 0, signoff_pending: 0, loaded: true });
  });

  it("refresh resolves at once when it was never started or has been stopped", async () => {
    await expect(refreshNavCounts()).resolves.toBeUndefined();
    fetchMock.mockResolvedValue(meta(1, 1));
    stop = startNavCounts();
    await refreshNavCounts();
    stop();
    await expect(refreshNavCounts()).resolves.toBeUndefined();
  });
});
