import type { SnapshotEvent } from "./api/types";
import { refreshHealth } from "./health";
import { session } from "./session.svelte";

/**
 * The live-update stream (GET /api/v1/events, beads-c2o.6). It carries signals
 * only: a subscriber refetches what it shows when told.
 *
 * - "off": not started.
 * - "connecting": opening, or the browser is retrying by itself (the server asks
 *   for a 2 s retry).
 * - "live": the stream is open; subscribers need not poll.
 * - "down": the stream was refused (EventSource gives up on an HTTP error such as
 *   401 or 503) and a reconnect is scheduled with backoff. Subscribers poll.
 */
export type LiveStatus = "off" | "connecting" | "live" | "down";

export const live = $state({
  status: "off" as LiveStatus,
  /** The last snapshot event received. */
  last: null as SnapshotEvent | null,
});

/** Reconnect delays after the stream was refused; the last one repeats. */
export const BACKOFF_MS = [1000, 2000, 5000, 10000, 30000];

/** The wait before reconnect attempt n (0-based). */
export function backoffDelay(n: number): number {
  return BACKOFF_MS[Math.min(n, BACKOFF_MS.length - 1)] ?? 30000;
}

const EVENTS_PATH = "/api/v1/events";
/** EventSource.CLOSED, spelled out so a fake constructor needs no statics. */
const READY_CLOSED = 2;

const listeners = new Set<() => void>();

/** Calls fn on every snapshot event and every status change. Returns the unsubscribe. */
export function onLive(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function emit(): void {
  for (const fn of [...listeners]) fn();
}

function setStatus(status: LiveStatus): void {
  if (live.status === status) return;
  live.status = status;
  emit();
}

export interface StartLiveOptions {
  /** The EventSource constructor; tests pass a fake. */
  EventSource?: typeof EventSource;
}

/** Opens the stream and keeps it open. Returns the cleanup. Call from App's onMount. */
export function startLive(opts: StartLiveOptions = {}): () => void {
  const ES = opts.EventSource ?? globalThis.EventSource;
  if (!ES) {
    setStatus("down"); // no EventSource: subscribers poll
    return () => setStatus("off");
  }

  let es: EventSource | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let attempt = 0;
  let stopped = false;

  const connect = (): void => {
    if (stopped) return;
    setStatus("connecting");
    const source = new ES(EVENTS_PATH);
    es = source;

    source.onopen = () => {
      if (stopped || es !== source) return;
      attempt = 0;
      setStatus("live");
    };
    source.addEventListener("snapshot", (e) => {
      if (stopped || es !== source) return;
      let ev: SnapshotEvent;
      try {
        ev = JSON.parse((e as MessageEvent<string>).data) as SnapshotEvent;
      } catch {
        return; // a malformed event is not worth tearing the stream down for
      }
      live.last = ev;
      emit();
    });
    source.onerror = () => {
      if (stopped || es !== source) return;
      if (source.readyState !== READY_CLOSED) {
        setStatus("connecting"); // the browser retries on its own
        return;
      }
      // Refused (401, 503, ...): the browser will not retry. A 401 shows up in
      // the session, so ask it, and back off unless the user is signed out.
      source.close();
      es = null;
      setStatus("down");
      void refreshHealth()
        .catch(() => {})
        .then(() => {
          if (stopped || session.status === "signed-out") return;
          timer = setTimeout(connect, backoffDelay(attempt++));
        });
    };
  };

  connect();
  return () => {
    stopped = true;
    clearTimeout(timer);
    es?.close();
    es = null;
    live.last = null;
    setStatus("off");
  };
}
