import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssuesResponse, MetaResponse } from "../api/types";
import { asEventSource, FakeEventSource } from "../fake-event-source";
import { startLive } from "../live.svelte";
import { session } from "../session.svelte";
import { IssuesFeed, POLL_MS, resetIssuesFeedCache } from "./feed.svelte";
import { mkItem, mkResponse } from "./test-helpers";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });

const issues = (phase2: boolean, ...ids: string[]): IssuesResponse => ({
  ...mkResponse(
    ids.map((id) => mkItem(id)),
    ids,
  ),
  phase2_ready: phase2,
});

const meta = (phase2: boolean, over: Partial<MetaResponse> = {}): MetaResponse =>
  ({
    phase2_ready: phase2,
    phase2_skipped: false,
    source: { path: "/p/.beads/issues.jsonl", kind: "jsonl", tracker: "bd", project_dir: "/p" },
    ...over,
  }) as MetaResponse;

const notReady = () => json({ error: { code: "not_ready", message: "no tracker data is loaded" } }, { status: 503 });

let fetchMock: ReturnType<typeof vi.fn>;
let queues: Record<string, Array<() => Response>>;
let sent: Array<{ path: string; inm: string | undefined }>;

/** Queues responses for a path; the last one repeats. */
const reply = (path: string, ...rs: Array<() => Response>) => {
  queues[path] = rs;
};

const issuesPath = "/api/v1/issues?view=tree";
const metaPath = "/api/v1/meta";

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.reset();
  resetIssuesFeedCache();
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

const count = (path: string) => sent.filter((s) => s.path === path).length;

describe("IssuesFeed", () => {
  it("loads issues and meta in parallel, then polls meta until Phase 2 lands and refetches the list", async () => {
    reply(issuesPath, () => json(issues(false, "a"), { headers: { ETag: '"i1"' } }), () => json(issues(true, "a", "b"), { headers: { ETag: '"i2"' } }));
    reply(
      metaPath,
      () => json(meta(false), { headers: { ETag: '"m1"' } }),
      () => new Response(null, { status: 304 }),
      () => json(meta(true), { headers: { ETag: '"m2"' } }),
    );
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.data?.items).toHaveLength(1);
    expect(feed.meta?.phase2_ready).toBe(false);
    expect(feed.phase2Pending).toBe(true);
    expect(count(issuesPath)).toBe(1);
    expect(count(metaPath)).toBe(1);

    // still computing: 304, nothing changes, keeps polling
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(count(metaPath)).toBe(2);
    expect(sent.filter((s) => s.path === metaPath)[1]?.inm).toBe('"m1"');
    expect(count(issuesPath)).toBe(1);

    // Phase 2 lands: new meta, so the list is refetched conditionally
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.meta?.phase2_ready).toBe(true);
    expect(feed.phase2Pending).toBe(false);
    expect(count(issuesPath)).toBe(2);
    expect(sent.filter((s) => s.path === issuesPath)[1]?.inm).toBe('"i1"');
    expect(feed.data?.items).toHaveLength(2);

    // and polling stops
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(metaPath)).toBe(3);
    expect(count(issuesPath)).toBe(2);
    stop();
  });

  it("does not poll when Phase 2 is already done, was skipped or failed", async () => {
    for (const m of [meta(true), meta(false, { phase2_skipped: true }), meta(false, { phase2_error: "boom" })]) {
      resetIssuesFeedCache();
      sent.length = 0;
      reply(issuesPath, () => json(issues(true, "a")));
      reply(metaPath, () => json(m));
      const stop = new IssuesFeed().start();
      await vi.advanceTimersByTimeAsync(POLL_MS * 3);
      expect(count(metaPath)).toBe(1);
      stop();
    }
  });

  it("treats 503 not_ready as loading and retries", async () => {
    reply(issuesPath, notReady, () => json(issues(true, "a"), { headers: { ETag: '"i1"' } }));
    reply(metaPath, notReady, () => json(meta(true), { headers: { ETag: '"m1"' } }));
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.data).toBeNull();
    expect(feed.error).toBeNull();
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.error).toBeNull();
    expect(feed.data?.items).toHaveLength(1);
    expect(feed.meta?.phase2_ready).toBe(true);
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
    stop();
  });

  it("keeps retrying while the server is unreachable, then recovers", async () => {
    let up = false;
    fetchMock.mockImplementation((path: string) => {
      if (!up) return Promise.reject(new TypeError("Failed to fetch"));
      return Promise.resolve(path === metaPath ? json(meta(true)) : json(issues(true, "a")));
    });
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.error?.code).toBe("network");
    expect(feed.data).toBeNull();
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.error?.code).toBe("network");
    up = true;
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.error).toBeNull();
    expect(feed.data?.items).toHaveLength(1);
    stop();
  });

  it("stops on an HTTP error and reloads on request", async () => {
    reply(issuesPath, () => json({ error: { code: "internal", message: "boom" } }, { status: 500 }), () => json(issues(true, "a")));
    reply(metaPath, () => json(meta(true)));
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.error?.status).toBe(500);
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 3);
    expect(sent.length).toBe(before);

    feed.reload();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.error).toBeNull();
    expect(feed.data?.items).toHaveLength(1);
    stop();
  });

  it("seeds a new feed from the cache and revalidates with the ETag", async () => {
    reply(issuesPath, () => json(issues(true, "a", "b"), { headers: { ETag: '"i1"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }));
    const warm = new IssuesFeed();
    const stopWarm = warm.start();
    await vi.advanceTimersByTimeAsync(0);
    stopWarm();
    expect(warm.data?.items).toHaveLength(2);

    sent.length = 0;
    reply(issuesPath, () => new Response(null, { status: 304 }));
    reply(metaPath, () => new Response(null, { status: 304 }));
    const back = new IssuesFeed();
    expect(back.data?.items).toHaveLength(2); // painted before any request
    expect(back.meta?.phase2_ready).toBe(true);
    const stop = back.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(sent.find((s) => s.path === issuesPath)?.inm).toBe('"i1"');
    expect(sent.find((s) => s.path === metaPath)?.inm).toBe('"m1"');
    expect(back.data?.items).toHaveLength(2);
    expect(back.error).toBeNull();
    stop();
  });

  it("stops everything on cleanup", async () => {
    reply(issuesPath, () => json(issues(false, "a")));
    reply(metaPath, () => json(meta(false)));
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const before = sent.length;
    stop();
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
  });
});

describe("IssuesFeed live updates", () => {
  const stopAll: Array<() => void> = [];
  afterEach(() => {
    while (stopAll.length) stopAll.pop()?.();
  });

  const run = async (): Promise<IssuesFeed> => {
    reply(issuesPath, () => json(issues(true, "a"), { headers: { ETag: '"i1"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }));
    const feed = new IssuesFeed();
    stopAll.push(feed.start());
    await vi.advanceTimersByTimeAsync(0);
    return feed;
  };
  const goLive = async () => {
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    await vi.advanceTimersByTimeAsync(0);
  };

  it("refetches once per snapshot event and does not poll while live", async () => {
    // Phase 2 is still pending in meta, which would poll without a stream.
    reply(issuesPath, () => json(issues(false, "a"), { headers: { ETag: '"i1"' } }));
    reply(metaPath, () => json(meta(false), { headers: { ETag: '"m1"' } }));
    const feed = new IssuesFeed();
    stopAll.push(feed.start());
    await goLive();
    expect(feed.phase2Pending).toBe(true);
    const issues0 = count(issuesPath);
    const meta0 = count(metaPath);

    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(issuesPath)).toBe(issues0);
    expect(count(metaPath)).toBe(meta0);

    reply(issuesPath, () => json(issues(true, "a", "b"), { headers: { ETag: '"i2"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m2"' } }));
    FakeEventSource.last.send("snapshot", { generation: 1, hash: "h", phase: "phase2" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(issues0 + 1);
    expect(count(metaPath)).toBe(meta0 + 1);
    expect(feed.data?.items).toHaveLength(2);
    expect(feed.phase2Pending).toBe(false);
    expect(sent.filter((s) => s.path === issuesPath).at(-1)?.inm).toBe('"i1"');

    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(issuesPath)).toBe(issues0 + 1);
  });

  it("runs one more tick when an event arrives during a tick", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    fetchMock.mockImplementation(async (path: string, init: RequestInit) => {
      sent.push({ path, inm: (init.headers as Record<string, string>)["If-None-Match"] });
      if (path === issuesPath && count(issuesPath) === 1) await gate; // the first list request hangs
      return path === metaPath ? json(meta(true), { headers: { ETag: '"m1"' } }) : json(issues(true, "a"), { headers: { ETag: '"i1"' } });
    });
    const feed = new IssuesFeed();
    stopAll.push(feed.start());
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(1);

    // status change to live and two events, all during the first tick
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    FakeEventSource.last.send("snapshot", { generation: 3, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(1); // nothing runs concurrently

    release();
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(2); // exactly one catch-up tick, not dropped, not tripled
    expect(count(metaPath)).toBe(2);
    expect(feed.data?.items).toHaveLength(1);
  });

  it("polls while the stream is connecting or down, and stops once it is live again", async () => {
    reply("/api/v1/health", () => json({ status: "ok", version: "v" }));
    const feed = await run();
    stopAll.push(startLive({ EventSource: asEventSource }));
    await vi.advanceTimersByTimeAsync(0); // connecting: polls
    const m1 = count(metaPath);
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(count(metaPath)).toBeGreaterThanOrEqual(m1 + 2);

    FakeEventSource.last.open(); // live: no polling
    await vi.advanceTimersByTimeAsync(0);
    const m2 = count(metaPath);
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(metaPath)).toBe(m2);

    FakeEventSource.last.fail(true); // down: polling resumes
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(count(metaPath)).toBeGreaterThanOrEqual(m2 + 3);
    expect(feed.error).toBeNull();
  });

  it("stops listening to the stream on cleanup", async () => {
    reply(issuesPath, () => json(issues(true, "a")));
    reply(metaPath, () => json(meta(true)));
    const feed = new IssuesFeed();
    const stopFeed = feed.start();
    await goLive();
    stopFeed();
    const before = sent.length;
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(sent.length).toBe(before);
  });
});

describe("IssuesFeed.refresh", () => {
  it("refetches the list and resolves once that refetch is in", async () => {
    reply(issuesPath, () => json(issues(true, "a"), { headers: { ETag: '"i1"' } }), () => json(issues(true, "a", "b"), { headers: { ETag: '"i2"' } }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }), () => new Response(null, { status: 304 }));
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.data?.items).toHaveLength(1);

    let done = false;
    const p = feed.refresh().then(() => (done = true));
    expect(done).toBe(false);
    await vi.advanceTimersByTimeAsync(0);
    await p;
    expect(feed.data?.items).toHaveLength(2);
    expect(count(issuesPath)).toBe(2);
    expect(sent.filter((s) => s.path === issuesPath)[1]?.inm).toBe('"i1"');
    stop();
  });

  it("asked while a tick runs, waits for the next full tick, not the one in flight", async () => {
    let releaseFirst: () => void = () => {};
    let issueCalls = 0;
    fetchMock.mockImplementation((path: string) => {
      if (path === metaPath) return Promise.resolve(json(meta(true), { headers: { ETag: '"m1"' } }));
      issueCalls++;
      if (issueCalls === 1) {
        return new Promise<Response>((resolve) => {
          releaseFirst = () => resolve(json(issues(true, "a")));
        });
      }
      return Promise.resolve(json(issues(true, "a", "b")));
    });
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0); // the first tick waits on the list

    const p = feed.refresh();
    releaseFirst();
    await vi.advanceTimersByTimeAsync(0);
    await p;
    expect(issueCalls).toBe(2);
    expect(feed.data?.items).toHaveLength(2);
    stop();
  });

  it("resolves at once before start and after stop", async () => {
    await expect(new IssuesFeed().refresh()).resolves.toBeUndefined();
    reply(issuesPath, () => json(issues(true, "a")));
    reply(metaPath, () => json(meta(true)));
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    stop();
    await expect(feed.refresh()).resolves.toBeUndefined();
  });

  it("a refresh pending when the feed stops is released", async () => {
    fetchMock.mockImplementation((path: string) =>
      path === metaPath ? Promise.resolve(json(meta(true))) : new Promise<Response>(() => {}),
    );
    const feed = new IssuesFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const p = feed.refresh();
    stop();
    await expect(p).resolves.toBeUndefined();
  });
});
