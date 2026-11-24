import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IssueDetail, MetaResponse } from "../api/types";
import { asEventSource, FakeEventSource } from "../fake-event-source";
import { startLive } from "../live.svelte";
import { session } from "../session.svelte";
import { DetailFeed } from "./detail-feed.svelte";
import { POLL_MS } from "./feed.svelte";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });

const detail = (id: string, over: Partial<IssueDetail> = {}): IssueDetail =>
  ({
    phase2_ready: true,
    id,
    title: `Title of ${id}`,
    description: "",
    status: "open",
    priority: 2,
    issue_type: "task",
    created_at: "2026-03-01T10:00:00Z",
    updated_at: "2026-03-02T10:00:00Z",
    labels: [],
    children: [],
    blocked_by: [],
    blocks: [],
    related: [],
    blocker_counts: { active: 0, resolved: 0 },
    active_dependents: 0,
    ready: true,
    blocked: false,
    links: [],
    comments: [],
    ...over,
  }) as IssueDetail;

const meta = (phase2: boolean, over: Partial<MetaResponse> = {}): MetaResponse =>
  ({
    phase2_ready: phase2,
    phase2_skipped: false,
    capabilities: { writes: true, events: true },
    ...over,
  }) as MetaResponse;

const notFound = () => json({ error: { code: "not_found", message: "no such issue" } }, { status: 404 });
const notReady = () => json({ error: { code: "not_ready", message: "no tracker data is loaded" } }, { status: 503 });

let fetchMock: ReturnType<typeof vi.fn>;
let queues: Record<string, Array<() => Response>>;
let sent: Array<{ path: string; inm: string | undefined }>;

/** Queues responses for a path; the last one repeats. */
const reply = (path: string, ...rs: Array<() => Response>) => {
  queues[path] = rs;
};

const issuePath = (id: string) => `/api/v1/issues/${encodeURIComponent(id)}`;
const metaPath = "/api/v1/meta";
const count = (path: string) => sent.filter((s) => s.path === path).length;

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.reset();
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
  reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }));
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("DetailFeed", () => {
  it("loads the issue and meta for the id", async () => {
    reply(issuePath("a"), () => json(detail("a"), { headers: { ETag: '"a1"' } }));
    const feed = new DetailFeed();
    feed.setId("a");
    expect(feed.status).toBe("loading");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("ok");
    expect(feed.detail?.id).toBe("a");
    expect(feed.meta?.capabilities.writes).toBe(true);
    expect(feed.error).toBeNull();
    expect(count(issuePath("a"))).toBe(1);
    expect(count(metaPath)).toBe(1);
    stop();
  });

  it("percent-encodes the id", async () => {
    reply(issuePath("a/b c"), () => json(detail("a/b c")));
    const feed = new DetailFeed();
    feed.setId("a/b c");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(sent.map((s) => s.path)).toContain("/api/v1/issues/a%2Fb%20c");
    expect(feed.detail?.id).toBe("a/b c");
    stop();
  });

  it("waits for an id before it loads anything", async () => {
    const feed = new DetailFeed();
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(POLL_MS * 3);
    expect(sent).toHaveLength(0);
    reply(issuePath("a"), () => json(detail("a")));
    feed.setId("a");
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.detail?.id).toBe("a");
    stop();
  });

  it("reports a 404 as missing and clears the detail", async () => {
    reply(issuePath("gone"), notFound);
    const feed = new DetailFeed();
    feed.setId("gone");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("missing");
    expect(feed.detail).toBeNull();
    expect(feed.error?.status).toBe(404);
    // a 404 is final: no polling while the stream is off and Phase 2 is done
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
    stop();
  });

  it("treats 503 not_ready as loading and retries", async () => {
    reply(issuePath("a"), notReady, () => json(detail("a")));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("loading");
    expect(feed.error).toBeNull();
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.status).toBe("ok");
    expect(feed.detail?.id).toBe("a");
    stop();
  });

  it("goes to the error state on an HTTP failure and reloads on request", async () => {
    reply(issuePath("a"), () => json({ error: { code: "internal", message: "boom" } }, { status: 500 }), () => json(detail("a")));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("error");
    expect(feed.error?.status).toBe(500);
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 3);
    expect(sent.length).toBe(before);

    feed.reload();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("ok");
    expect(feed.error).toBeNull();
    expect(feed.detail?.id).toBe("a");
    stop();
  });

  it("keeps retrying while the server is unreachable, then recovers", async () => {
    let up = false;
    fetchMock.mockImplementation((path: string) => {
      if (!up) return Promise.reject(new TypeError("Failed to fetch"));
      return Promise.resolve(path === metaPath ? json(meta(true)) : json(detail("a")));
    });
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("error");
    expect(feed.error?.code).toBe("network");
    up = true;
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.status).toBe("ok");
    expect(feed.error).toBeNull();
    stop();
  });

  it("revalidates with the ETag and keeps the detail on a 304", async () => {
    reply(issuePath("a"), () => json(detail("a"), { headers: { ETag: '"a1"' } }), () => new Response(null, { status: 304 }));
    reply(metaPath, () => json(meta(true), { headers: { ETag: '"m1"' } }), () => new Response(null, { status: 304 }));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const first = feed.detail;
    void feed.refresh();
    await vi.advanceTimersByTimeAsync(0);
    expect(sent.filter((s) => s.path === issuePath("a")).map((s) => s.inm)).toEqual([undefined, '"a1"']);
    expect(sent.filter((s) => s.path === metaPath).map((s) => s.inm)).toEqual([undefined, '"m1"']);
    expect(feed.detail).toBe(first);
    expect(feed.status).toBe("ok");
    stop();
  });

  it("clears the detail for another id, keeps one etag per id, and paints a remembered copy at once", async () => {
    reply(issuePath("a"), () => json(detail("a", { title: "A1" }), { headers: { ETag: '"a1"' } }), () => new Response(null, { status: 304 }));
    reply(issuePath("b"), () => json(detail("b"), { headers: { ETag: '"b1"' } }));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.detail?.title).toBe("A1");

    feed.setId("b");
    expect(feed.detail).toBeNull(); // not a's page under b's id
    expect(feed.status).toBe("loading");
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.detail?.id).toBe("b");
    expect(sent.filter((s) => s.path === issuePath("b"))[0]?.inm).toBeUndefined(); // b's request carries no etag of a

    feed.setId("a");
    expect(feed.detail?.title).toBe("A1"); // remembered
    expect(feed.status).toBe("ok");
    await vi.advanceTimersByTimeAsync(0);
    expect(sent.filter((s) => s.path === issuePath("a")).at(-1)?.inm).toBe('"a1"');
    expect(feed.detail?.title).toBe("A1");
    stop();
  });

  it("setId with the same id does nothing", async () => {
    reply(issuePath("a"), () => json(detail("a")));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const before = sent.length;
    feed.setId("a");
    await vi.advanceTimersByTimeAsync(0);
    expect(sent.length).toBe(before);
    expect(feed.detail?.id).toBe("a");
    stop();
  });

  it("ignores a late answer for the id it left", async () => {
    let releaseA: () => void = () => {};
    const gateA = new Promise<void>((r) => (releaseA = r));
    fetchMock.mockImplementation(async (path: string, init: RequestInit) => {
      sent.push({ path, inm: (init.headers as Record<string, string>)["If-None-Match"] });
      if (path === issuePath("a")) {
        await gateA; // the mock ignores the abort signal, like a response already on the wire
        return json(detail("a"));
      }
      return path === metaPath ? json(meta(true)) : json(detail("b"));
    });
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    feed.setId("b");
    releaseA();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.detail?.id).toBe("b");
    expect(feed.status).toBe("ok");
    stop();
  });

  it("keeps the shown detail when a refetch of the same id fails", async () => {
    reply(issuePath("a"), () => json(detail("a")), () => json({ error: { code: "internal", message: "boom" } }, { status: 500 }));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    void feed.refresh();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.detail?.id).toBe("a");
    expect(feed.status).toBe("ok");
    expect(feed.error?.status).toBe(500);
    stop();
  });

  it("turns a vanished issue into missing on a refetch", async () => {
    reply(issuePath("a"), () => json(detail("a")), notFound);
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    void feed.refresh();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.status).toBe("missing");
    expect(feed.detail).toBeNull();
    stop();
  });

  it("polls meta and the issue while Phase 2 is computing with no stream, and stops when it lands", async () => {
    reply(issuePath("a"), () => json(detail("a", { phase2_ready: false }), { headers: { ETag: '"a1"' } }), () => json(detail("a"), { headers: { ETag: '"a2"' } }));
    reply(metaPath, () => json(meta(false), { headers: { ETag: '"m1"' } }), () => json(meta(true), { headers: { ETag: '"m2"' } }));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.phase2Pending).toBe(true);
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(feed.phase2Pending).toBe(false);
    expect(feed.detail?.phase2_ready).toBe(true);
    const before = sent.length;
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
    stop();
  });

  it("stops everything on cleanup", async () => {
    reply(issuePath("a"), () => json(detail("a", { phase2_ready: false })));
    reply(metaPath, () => json(meta(false)));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const before = sent.length;
    stop();
    feed.setId("b"); // a stopped feed does not load
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(sent.length).toBe(before);
  });
});

describe("DetailFeed live updates", () => {
  const stopAll: Array<() => void> = [];
  afterEach(() => {
    while (stopAll.length) stopAll.pop()?.();
  });

  const goLive = async () => {
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    await vi.advanceTimersByTimeAsync(0);
  };

  it("refetches once per snapshot event and does not poll while live", async () => {
    reply(issuePath("a"), () => json(detail("a", { title: "old" }), { headers: { ETag: '"a1"' } }));
    reply(metaPath, () => json(meta(false), { headers: { ETag: '"m1"' } })); // Phase 2 pending would poll without a stream
    const feed = new DetailFeed();
    feed.setId("a");
    stopAll.push(feed.start());
    await goLive();
    const issue0 = count(issuePath("a"));
    const meta0 = count(metaPath);

    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(issuePath("a"))).toBe(issue0);
    expect(count(metaPath)).toBe(meta0);

    reply(issuePath("a"), () => json(detail("a", { title: "new" }), { headers: { ETag: '"a2"' } }));
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath("a"))).toBe(issue0 + 1);
    expect(count(metaPath)).toBe(meta0 + 1);
    expect(feed.detail?.title).toBe("new");
    expect(sent.filter((s) => s.path === issuePath("a")).at(-1)?.inm).toBe('"a1"');
  });

  it("runs one more tick when an event arrives during a tick", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    fetchMock.mockImplementation(async (path: string, init: RequestInit) => {
      sent.push({ path, inm: (init.headers as Record<string, string>)["If-None-Match"] });
      if (path === issuePath("a") && count(issuePath("a")) === 1) await gate; // the first request hangs
      return path === metaPath ? json(meta(true)) : json(detail("a"));
    });
    const feed = new DetailFeed();
    feed.setId("a");
    stopAll.push(feed.start());
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath("a"))).toBe(1);

    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    FakeEventSource.last.send("snapshot", { generation: 3, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath("a"))).toBe(1); // nothing runs concurrently

    release();
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath("a"))).toBe(2); // exactly one catch-up tick
  });

  it("polls while the stream is connecting or down, and stops once it is live again", async () => {
    reply("/api/v1/health", () => json({ status: "ok", version: "v" }));
    reply(issuePath("a"), () => json(detail("a")));
    const feed = new DetailFeed();
    feed.setId("a");
    stopAll.push(feed.start());
    await vi.advanceTimersByTimeAsync(0);
    stopAll.push(startLive({ EventSource: asEventSource }));
    await vi.advanceTimersByTimeAsync(0); // connecting: polls
    const n1 = count(issuePath("a"));
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(count(issuePath("a"))).toBeGreaterThanOrEqual(n1 + 2);

    FakeEventSource.last.open(); // live: no polling
    await vi.advanceTimersByTimeAsync(0);
    const n2 = count(issuePath("a"));
    await vi.advanceTimersByTimeAsync(POLL_MS * 5);
    expect(count(issuePath("a"))).toBe(n2);

    FakeEventSource.last.fail(true); // down: polling resumes
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(count(issuePath("a"))).toBeGreaterThanOrEqual(n2 + 2);
    expect(feed.error).toBeNull();
  });

  it("stops listening to the stream on cleanup", async () => {
    reply(issuePath("a"), () => json(detail("a")));
    const feed = new DetailFeed();
    feed.setId("a");
    const stopFeed = feed.start();
    await goLive();
    stopFeed();
    const before = sent.length;
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);
    expect(sent.length).toBe(before);
  });
});

describe("DetailFeed.refresh", () => {
  it("refetches and resolves once that refetch is in", async () => {
    reply(issuePath("a"), () => json(detail("a", { title: "one" }), { headers: { ETag: '"a1"' } }), () => json(detail("a", { title: "two" }), { headers: { ETag: '"a2"' } }));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(feed.detail?.title).toBe("one");

    let done = false;
    const p = feed.refresh().then(() => (done = true));
    expect(done).toBe(false);
    await vi.advanceTimersByTimeAsync(0);
    await p;
    expect(feed.detail?.title).toBe("two");
    stop();
  });

  it("asked while a tick runs, waits for the next full tick, not the one in flight", async () => {
    let releaseFirst: () => void = () => {};
    let calls = 0;
    fetchMock.mockImplementation((path: string) => {
      if (path === metaPath) return Promise.resolve(json(meta(true)));
      calls++;
      if (calls === 1) {
        return new Promise<Response>((resolve) => {
          releaseFirst = () => resolve(json(detail("a", { title: "one" })));
        });
      }
      return Promise.resolve(json(detail("a", { title: "two" })));
    });
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0); // the first tick waits on the issue

    const p = feed.refresh();
    releaseFirst();
    await vi.advanceTimersByTimeAsync(0);
    await p;
    expect(calls).toBe(2);
    expect(feed.detail?.title).toBe("two");
    stop();
  });

  it("resolves at once before start and after stop, and a pending one is released by stop", async () => {
    await expect(new DetailFeed().refresh()).resolves.toBeUndefined();
    reply(issuePath("a"), () => json(detail("a")));
    const feed = new DetailFeed();
    feed.setId("a");
    const stop = feed.start();
    await vi.advanceTimersByTimeAsync(0);
    const pending = feed.refresh();
    stop();
    await expect(pending).resolves.toBeUndefined();
    await expect(feed.refresh()).resolves.toBeUndefined();
  });
});
