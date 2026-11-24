import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MetaResponse } from "../api/types";
import { asEventSource, FakeEventSource } from "../fake-event-source";
import { startLive } from "../live.svelte";
import { session } from "../session.svelte";
import { IssuesFeed, POLL_MS, resetIssuesFeedCache } from "./feed.svelte";

// beads-c2o.56: a live event that lands during a tick which has already scheduled its retry
// timer starts a catch-up tick at once. That tick must cancel the armed timer, or the old timer
// fires later and runs a second poll chain beside it (and every further event adds another).

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });

const issuesPath = "/api/v1/issues?view=tree";
const metaPath = "/api/v1/meta";
const notReady = () => json({ error: { code: "not_ready", message: "no tracker data is loaded" } }, { status: 503 });
const meta = { phase2_ready: true, phase2_skipped: false, capabilities: { writes: true, events: true } } as MetaResponse;

let fetchMock: ReturnType<typeof vi.fn>;
let sent: string[];
const count = (path: string) => sent.filter((p) => p === path).length;
const stopAll: Array<() => void> = [];

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.reset();
  resetIssuesFeedCache();
  session.status = "unknown";
  sent = [];
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  while (stopAll.length) stopAll.pop()?.();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("IssuesFeed catch-up tick", () => {
  it("cancels the armed retry timer, so only one poll chain runs", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    fetchMock.mockImplementation(async (path: string, init: RequestInit) => {
      sent.push(path);
      // /meta answers 304 once its ETag is held, so a changed meta does not add a second list fetch per tick.
      if (path === metaPath && (init.headers as Record<string, string>)["If-None-Match"]) return new Response(null, { status: 304 });
      if (path === issuesPath && count(issuesPath) === 1) await gate; // the first list request is held
      return path === metaPath ? json(meta, { headers: { ETag: '"m1"' } }) : notReady(); // a retry is always due
    });
    const feed = new IssuesFeed();
    stopAll.push(feed.start());
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(1);

    // A live event lands during the held tick (the stream is up, so only the retry would poll).
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(1); // nothing runs concurrently

    release(); // tick 1 ends with a retry armed, then the catch-up tick runs at once
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuesPath)).toBe(2);

    await vi.advanceTimersByTimeAsync(POLL_MS); // one chain: one more request, not two
    expect(count(issuesPath)).toBe(3);
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(count(issuesPath)).toBe(4);
    expect(feed.data).toBeNull();
  });
});
