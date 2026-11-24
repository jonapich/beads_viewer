import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { asEventSource, FakeEventSource } from "./fake-event-source";
import { backoffDelay, BACKOFF_MS, live, onLive, startLive } from "./live.svelte";
import { session } from "./session.svelte";

const healthBody = () =>
  new Response(JSON.stringify({ status: "ok", version: "v" }), { headers: { "Content-Type": "application/json" } });

let health: () => Response;
let healthCalls: number;

beforeEach(() => {
  vi.useFakeTimers();
  FakeEventSource.reset();
  session.status = "unknown";
  health = healthBody;
  healthCalls = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(() => {
      healthCalls++;
      return Promise.resolve(health());
    }),
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const start = () => startLive({ EventSource: asEventSource });

describe("startLive", () => {
  it("connects to the events path and reports connecting, live and off", () => {
    const seen: string[] = [];
    const off = onLive(() => seen.push(live.status));
    const stop = start();
    expect(FakeEventSource.last.url).toBe("/api/v1/events");
    expect(live.status).toBe("connecting");
    FakeEventSource.last.open();
    expect(live.status).toBe("live");
    stop();
    expect(live.status).toBe("off");
    expect(FakeEventSource.last.closed).toBe(true);
    expect(seen).toEqual(["connecting", "live", "off"]);
    off();
  });

  it("stores snapshot events and calls subscribers on each", () => {
    let calls = 0;
    const off = onLive(() => calls++);
    const stop = start();
    FakeEventSource.last.open();
    const before = calls;
    const ev = { generation: 2, hash: "h", phase: "phase1" };
    FakeEventSource.last.send("snapshot", ev);
    expect(live.last).toEqual(ev);
    expect(calls).toBe(before + 1);
    FakeEventSource.last.send("snapshot", { ...ev, phase: "phase2" });
    expect(calls).toBe(before + 2);
    FakeEventSource.last.send("snapshot", "not json");
    expect(calls).toBe(before + 2);
    expect(live.last?.phase).toBe("phase2");
    stop();
    expect(live.last).toBeNull();
    off();
  });

  it("shows connecting while the browser retries by itself, without a new connection", () => {
    const stop = start();
    FakeEventSource.last.open();
    FakeEventSource.last.fail(false);
    expect(live.status).toBe("connecting");
    expect(FakeEventSource.instances).toHaveLength(1);
    FakeEventSource.last.open();
    expect(live.status).toBe("live");
    stop();
  });

  it("goes down when the stream is refused, checks health, and reconnects with backoff", async () => {
    const stop = start();
    FakeEventSource.last.fail(true);
    expect(live.status).toBe("down");
    expect(FakeEventSource.last.closed).toBe(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(healthCalls).toBe(1);

    // backoffDelay(0), then [1], then [2]; each failed attempt waits longer.
    for (const i of [0, 1, 2]) {
      const delay = backoffDelay(i);
      expect(FakeEventSource.instances).toHaveLength(i + 1);
      await vi.advanceTimersByTimeAsync(delay - 1);
      expect(FakeEventSource.instances).toHaveLength(i + 1);
      await vi.advanceTimersByTimeAsync(1);
      expect(FakeEventSource.instances).toHaveLength(i + 2);
      expect(live.status).toBe("connecting");
      FakeEventSource.last.fail(true);
      await vi.advanceTimersByTimeAsync(0);
      expect(live.status).toBe("down");
    }

    // A successful open resets the backoff.
    await vi.advanceTimersByTimeAsync(backoffDelay(3));
    FakeEventSource.last.open();
    expect(live.status).toBe("live");
    FakeEventSource.last.fail(true);
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(backoffDelay(0));
    expect(live.status).toBe("connecting");
    stop();
  });

  it("caps the backoff at the last step", async () => {
    const stop = start();
    for (let i = 0; i < BACKOFF_MS.length + 2; i++) {
      FakeEventSource.last.fail(true);
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(backoffDelay(i));
    }
    expect(FakeEventSource.instances).toHaveLength(BACKOFF_MS.length + 3);
    stop();
  });

  it("does not reconnect when the session is signed out", async () => {
    health = () => new Response(JSON.stringify({ error: { code: "unauthorized", message: "no" } }), { status: 401, headers: { "Content-Type": "application/json" } });
    const stop = start();
    FakeEventSource.last.fail(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(session.status).toBe("signed-out");
    await vi.advanceTimersByTimeAsync(backoffDelay(BACKOFF_MS.length) * 2);
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(live.status).toBe("down");
    stop();
  });

  it("stops reconnecting after cleanup", async () => {
    const stop = start();
    FakeEventSource.last.fail(true);
    await vi.advanceTimersByTimeAsync(0);
    stop();
    await vi.advanceTimersByTimeAsync(backoffDelay(0) * 3);
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(live.status).toBe("off");
  });

  it("reports down when EventSource does not exist", () => {
    vi.stubGlobal("EventSource", undefined);
    const stop = startLive();
    expect(live.status).toBe("down");
    stop();
    expect(live.status).toBe("off");
  });
});
