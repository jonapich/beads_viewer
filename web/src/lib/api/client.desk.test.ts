import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getInbox,
  getInboxIfChanged,
  getPipeline,
  getPipelineIfChanged,
  getSignoffIfChanged,
  getSliceMetrics,
  getSliceMetricsIfChanged,
} from "./client";
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

describe("agent desk client", () => {
  it.each([
    ["inbox", getInbox, "/api/v1/inbox"],
    ["pipeline", getPipeline, "/api/v1/pipeline"],
    ["slice metrics", getSliceMetrics, "/api/v1/slice-metrics"],
  ] as const)("%s reads its endpoint", async (_name, get, path) => {
    fetchMock.mockResolvedValue(json({ ok: true }));
    await expect(get()).resolves.toEqual({ ok: true });
    expect(fetchMock.mock.calls[0]?.[0]).toBe(path);
  });

  it.each([
    ["inbox", getInboxIfChanged, "/api/v1/inbox"],
    ["signoff", getSignoffIfChanged, "/api/v1/signoff"],
    ["pipeline", getPipelineIfChanged, "/api/v1/pipeline"],
    ["slice metrics", getSliceMetricsIfChanged, "/api/v1/slice-metrics"],
  ] as const)("%s conditional GET sends the etag and resolves null on 304", async (_name, get, path) => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(get('"e"')).resolves.toBeNull();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(path);
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"e"');
  });

  it("a conditional GET returns the body with its etag", async () => {
    fetchMock.mockResolvedValue(json({ total: 0, ready_to_judge: 0, groups: [] }, { headers: { ETag: '"x~m1"' } }));
    await expect(getInboxIfChanged(null)).resolves.toEqual({ data: { total: 0, ready_to_judge: 0, groups: [] }, etag: '"x~m1"' });
  });
});
