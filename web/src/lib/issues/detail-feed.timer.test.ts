import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MetaResponse } from "../api/types";
import { asEventSource, FakeEventSource } from "../fake-event-source";
import { startLive } from "../live.svelte";
import { session } from "../session.svelte";
import { DetailFeed } from "./detail-feed.svelte";
import { POLL_MS } from "./feed.svelte";

// beads-c2o.56: a live event that lands during a tick which has already scheduled its retry
// timer starts a catch-up tick at once. That tick must cancel the armed timer, or the old timer
// fires later and runs a second poll chain beside it (and every further event adds another).

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });

const issuePath = "/api/v1/issues/a";
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

describe("DetailFeed catch-up tick", () => {
  it("cancels the armed retry timer, so only one poll chain runs", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    fetchMock.mockImplementation(async (path: string) => {
      sent.push(path);
      if (path === issuePath && count(issuePath) === 1) await gate; // the first detail request is held
      return path === metaPath ? json(meta, { headers: { ETag: '"m1"' } }) : notReady(); // a retry is always due
    });
    const feed = new DetailFeed();
    feed.setId("a");
    stopAll.push(feed.start());
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath)).toBe(1);

    // A live event lands during the held tick (the stream is up, so only the retry would poll).
    stopAll.push(startLive({ EventSource: asEventSource }));
    FakeEventSource.last.open();
    FakeEventSource.last.send("snapshot", { generation: 2, hash: "h", phase: "phase1" });
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath)).toBe(1); // nothing runs concurrently

    release(); // tick 1 ends with a retry armed, then the catch-up tick runs at once
    await vi.advanceTimersByTimeAsync(0);
    expect(count(issuePath)).toBe(2);

    await vi.advanceTimersByTimeAsync(POLL_MS); // one chain: one more request, not two
    expect(count(issuePath)).toBe(3);
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(count(issuePath)).toBe(4);
    expect(feed.status).toBe("loading"); // 503 not_ready is just loading
  });
});
