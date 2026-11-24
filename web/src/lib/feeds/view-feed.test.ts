import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getPlanIfChanged, getTriageIfChanged } from "../api/client";
import type { MetaResponse, PlanResponse, TriageResponse } from "../api/types";
import { asEventSource, FakeEventSource } from "../fake-event-source";
import { startLive } from "../live.svelte";
import { session } from "../session.svelte";
import { POLL_MS, Resource, resetViewFeedCache, ViewFeed } from "./view-feed.svelte";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });

const triage = (issueCount: number): TriageResponse => ({ issue_count: issueCount }) as TriageResponse;
const plan = (n: number): PlanResponse => ({ total_actionable: n, tracks: [] }) as unknown as PlanResponse;
const meta = (phase2: boolean, over: Partial<MetaResponse> = {}): MetaResponse =>
  ({ phase2_ready: phase2, phase2_skipped: false, capabilities: { writes: true }, ...over }) as MetaResponse;

const pending = () => json({ error: { code: "analysis_pending", message: "still computing" } }, { status: 503 });
const notReady = () => json({ error: { code: "not_ready", message: "no tracker data is loaded" } }, { status: 503 });
const failed = () => json({ error: { code: "analysis_failed", message: "triage analysis failed: boom" } }, { status: 500 });
const notModified = () => new Response(null, { status: 304 });

const triagePath = "/api/v1/triage";
const planPath = "/api/v1/plan";
const metaPath = "/api/v1/meta";

let fetchMock: ReturnType<typeof vi.fn>;
let queues: Record<string, Array<() => Response>>;
let sent: Array<{ path: string; inm: string | undefined }>;

/** Queues responses for a path; the last one repeats. */
const reply = (path: string, ...rs: Array<() => Response>) => {
  queues[path] = rs;
};
const count = (path: string) => sent.filter((s) => s.path === path).length;
const inms = (path: string) => sent.filter((s) => s.path === path).map((s) => s.inm);

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.reset();
  resetViewFeedCache();
  session.status = "unknown";
  queues = {};
  sent = [];
  fetchMock = vi.fn((path: string, init: RequestInit) => {
    sent.push({ path, inm: (init.headers as Record<string, string>)["If-None-Match"] });
    const q = queues[path];
    const next = q && q.length > 1 ? q.shift() : q?.[0];
    if (!next) return Promise.reject(new Error(`unexpected ${path}`));
    return Promise.resolve(next());
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const stopAll: Array<() => void> = [];
afterEach(() => {
  while (stopAll.length) stopAll.pop()?.();
});

const mount = (res: Resource<unknown>[]) => {
  const feed = new ViewFeed(res);
  stopAll.push(feed.start());
  return feed;
};

describe("Resource and ViewFeed", () => {
  it("loads the resources and meta in parallel with conditional requests, and a 304 keeps the data", async () => {
    reply(triagePath, () => json(triage(3), { headers: { ETag: '"t1"' } }), notModified);
    reply(planPath, () => json(plan(2), { headers: { ETag: '"p1"' } }), notModified);
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }), notModified);
    const t = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    const p = new Resource<PlanResponse>(planPath, getPlanIfChanged);
    const feed = mount([t, p]);
    await vi.advanceTimersByTimeAsync(0);
    expect(t.data?.issue_count).toBe(3);
    expect(p.data?.total_actionable).toBe(2);
    expect(feed.meta?.phase2_ready).toBe(true);
    expect(feed.writes).toBe(true);
    expect(inms(triagePath)).toEqual([undefined]);

    void feed.refresh();
    await vi.advanceTimersByTimeAsync(0);
    expect(inms(triagePath)).toEqual([undefined, '"t1"']);
    expect(inms(planPath)).toEqual([undefined, '"p1"']);
    expect(inms(metaPath)).toEqual([undefined, '"m1"']);
    expect(t.data?.issue_count).toBe(3);
    expect(t.error).toBeNull();
  });

  it("503 analysis_pending marks pending, keeps the data, retries after POLL_MS and clears on success", async () => {
    reply(triagePath, () => json(triage(3), { headers: { ETag: '"t1"' } }), pending, () => json(triage(4), { headers: { ETag: '"t2"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }));
    const t = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    const feed = mount([t]);
    await vi.advanceTimersByTimeAsync(0);
    expect(t.data?.issue_count).toBe(3);

    feed.kick();
    await vi.advanceTimersByTimeAsync(0);
    expect(t.pending).toBe(true);
    expect(t.error).toBeNull();
    expect(t.data?.issue_count).toBe(3); // the old copy stays on show

    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(t.pending).toBe(false);
    expect(t.data?.issue_count).toBe(4);
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 5); // no more retries
    expect(sent.length).toBe(before);
  });

  it("503 on the first fetch leaves no data and no error (loading), 503 not_ready retries too", async () => {
    reply(triagePath, pending, notReady, () => json(triage(1)));
    reply(metaPath, () => json(meta(true)));
    const t = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    mount([t]);
    await vi.advanceTimersByTimeAsync(0);
    expect(t.data).toBeNull();
    expect(t.pending).toBe(true);
    expect(t.error).toBeNull();
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(t.data).toBeNull();
    expect(t.pending).toBe(true); // not_ready keeps the previous flag: still loading
    expect(t.error).toBeNull();
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(t.data?.issue_count).toBe(1);
    expect(t.pending).toBe(false);
  });

  it("500 sets the error, keeps the data, does not retry, and reload() refetches", async () => {
    reply(triagePath, () => json(triage(3), { headers: { ETag: '"t1"' } }), failed, () => json(triage(5), { headers: { ETag: '"t2"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }), notModified);
    const t = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    const feed = mount([t]);
    await vi.advanceTimersByTimeAsync(0);
    feed.kick();
    await vi.advanceTimersByTimeAsync(0);
    expect(t.error?.status).toBe(500);
    expect(t.error?.code).toBe("analysis_failed");
    expect(t.pending).toBe(false);
    expect(t.data?.issue_count).toBe(3);

    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 3);
    expect(sent.length).toBe(before);

    feed.reload();
    await vi.advanceTimersByTimeAsync(0);
    expect(t.error).toBeNull();
    expect(t.data?.issue_count).toBe(5);
  });

  it("a network failure sets the error and keeps retrying until the server is back", async () => {
    let up = false;
    fetchMock.mockImplementation((path: string) => {
      if (!up) return Promise.reject(new TypeError("Failed to fetch"));
      return Promise.resolve(path === metaPath ? json(meta(true)) : json(plan(1)));
    });
    const p = new Resource<PlanResponse>(planPath, getPlanIfChanged);
    const feed = mount([p]);
    await vi.advanceTimersByTimeAsync(0);
    expect(p.error?.code).toBe("network");
    expect(feed.metaError?.code).toBe("network");
    expect(p.data).toBeNull();
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(p.error?.code).toBe("network");
    up = true;
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(p.error).toBeNull();
    expect(feed.metaError).toBeNull();
    expect(p.data?.total_actionable).toBe(1);
  });

  it("polls only for phase 2 when no stream is started, and stops once it lands", async () => {
    reply(triagePath, () => json(triage(1), { headers: { ETag: '"t1"' } }), () => json(triage(2), { headers: { ETag: '"t2"' } }));
    reply(metaPath, () => json(meta(false), { headers: { ETag: '"m1"' } }), () => json(meta(true), { headers: { ETag: '"m2"' } }));
    const t = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    const feed = mount([t]);
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.phase2Pending).toBe(true);
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.phase2Pending).toBe(false);
    expect(t.data?.issue_count).toBe(2);
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
  });

  it("does not poll when phase 2 was skipped or failed", async () => {
    for (const m of [meta(false, { phase2_skipped: true }), meta(false, { phase2_error: "boom" })]) {
      resetViewFeedCache();
      sent.length = 0;
      reply(triagePath, () => json(triage(1)));
      reply(metaPath, () => json(m));
      const feed = new ViewFeed([new Resource<TriageResponse>(triagePath, getTriageIfChanged)]);
      const stop = feed.start();
      await vi.advanceTimersByTimeAsync(POLL_MS * 3);
      expect(feed.phase2Pending).toBe(false);
      expect(count(metaPath)).toBe(1);
      stop();
    }
  });

  it("refetches once per live event and does not poll while live", async () => {
    reply(triagePath, () => json(triage(1), { headers: { ETag: '"t1"' } }));
    reply(metaPath, () => json(meta(false), { headers: { ETag: '"m1"' } })); // would poll without a stream
    const t = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    mount([t]);
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    await vi.advanceTimersByTimeAsync(0);
    const t0 = count(triagePath);
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(triagePath)).toBe(t0);

    reply(triagePath, () => json(triage(2), { headers: { ETag: '"t2"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m2"' } }));
    FakeEventSource.last.send("snapshot", { generation: 1, hash: "h", phase: "phase2" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(triagePath)).toBe(t0 + 1);
    expect(inms(triagePath).at(-1)).toBe('"t1"');
    expect(t.data?.issue_count).toBe(2);
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(triagePath)).toBe(t0 + 1);
  });

  it("runs one more tick when an event arrives during a tick", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    fetchMock.mockImplementation(async (path: string, init: RequestInit) => {
      sent.push({ path, inm: (init.headers as Record<string, string>)["If-None-Match"] });
      if (path === triagePath && count(triagePath) === 1) await gate;
      return path === metaPath ? json(meta(true)) : json(triage(1));
    });
    mount([new Resource<TriageResponse>(triagePath, getTriageIfChanged)]);
    await vi.advanceTimersByTimeAsync(0);
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    FakeEventSource.last.send("snapshot", { generation: 3, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(triagePath)).toBe(1); // nothing runs concurrently
    release();
    await vi.advanceTimersByTimeAsync(0);
    expect(count(triagePath)).toBe(2); // one catch-up tick, not dropped, not tripled
  });

  it("paints the next mount from the cache and revalidates with the ETag", async () => {
    reply(triagePath, () => json(triage(3), { headers: { ETag: '"t1"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }));
    const warm = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    const warmFeed = new ViewFeed([warm]);
    const stopWarm = warmFeed.start();
    await vi.advanceTimersByTimeAsync(0);
    stopWarm();

    sent.length = 0;
    reply(triagePath, notModified);
    reply(metaPath, notModified);
    const back = new Resource<TriageResponse>(triagePath, getTriageIfChanged);
    const backFeed = new ViewFeed([back]);
    expect(back.data?.issue_count).toBe(3); // painted before any request
    expect(backFeed.meta?.phase2_ready).toBe(true);
    stopAll.push(backFeed.start());
    await vi.advanceTimersByTimeAsync(0);
    expect(inms(triagePath)).toEqual(['"t1"']);
    expect(inms(metaPath)).toEqual(['"m1"']);
    expect(back.data?.issue_count).toBe(3);
    expect(back.error).toBeNull();
  });

  it("setSource paints the cached copy of the new key, or keeps the data when there is none", async () => {
    const path = (n: number) => `/api/v1/insights?limit=${n}` as const;
    const fetcher = (n: number) => async () => ({ data: { n } as unknown as TriageResponse, etag: `"e${n}"` });
    reply(metaPath, () => json(meta(true)));
    const r = new Resource<TriageResponse>(path(25), fetcher(25));
    const feed = mount([r]);
    await vi.advanceTimersByTimeAsync(0);
    expect((r.data as unknown as { n: number }).n).toBe(25);

    r.setSource(path(100), fetcher(100));
    expect((r.data as unknown as { n: number }).n).toBe(25); // nothing cached for 100: keeps the data
    feed.kick();
    await vi.advanceTimersByTimeAsync(0);
    expect((r.data as unknown as { n: number }).n).toBe(100);

    r.setSource(path(25), fetcher(25));
    expect((r.data as unknown as { n: number }).n).toBe(25); // painted from the cache at once
  });

  it("refresh resolves once a tick that started after the call has finished, and at once after stop", async () => {
    reply(triagePath, () => json(triage(1)));
    reply(metaPath, () => json(meta(true)));
    const feed = new ViewFeed([new Resource<TriageResponse>(triagePath, getTriageIfChanged)]);
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    let done = false;
    void feed.refresh().then(() => (done = true));
    expect(done).toBe(false);
    await vi.advanceTimersByTimeAsync(0);
    expect(done).toBe(true);
    stop();
    await expect(feed.refresh()).resolves.toBeUndefined();
  });

  it("stops everything on cleanup", async () => {
    reply(triagePath, () => json(triage(1)));
    reply(metaPath, () => json(meta(false)));
    const feed = new ViewFeed([new Resource<TriageResponse>(triagePath, getTriageIfChanged)]);
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const before = sent.length;
    stop();
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
  });
});
