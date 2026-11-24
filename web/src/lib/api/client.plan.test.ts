import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getPlan, getPlanIfChanged } from "./client";
import { session } from "../session.svelte";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    ...init,
    headers: { "Content-Type": "application/json", ...init.headers },
  });

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  session.status = "unknown";
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("plan client", () => {
  it("getPlan reads /api/v1/plan", async () => {
    const body = { tracks: [], total_actionable: 0, total_blocked: 0, total_deferred_or_parked: 0, total_in_progress: 0, summary: { highest_impact: "", impact_reason: "", unblocks_count: 0 } };
    fetchMock.mockResolvedValue(json(body));
    await expect(getPlan()).resolves.toEqual(body);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/plan");
  });

  it("getPlanIfChanged sends the etag and resolves null on 304", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getPlanIfChanged('"p"')).resolves.toBeNull();
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/plan");
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"p"');
  });
});
