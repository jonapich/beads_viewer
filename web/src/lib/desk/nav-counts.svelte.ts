import { ApiError, getMetaIfChanged } from "../api/client";
import { POLL_MS } from "../issues/feed.svelte";
import { live, onLive } from "../live.svelte";

/**
 * The agent desk badge in the nav: questions awaiting the user plus sign-offs pending, from
 * meta.counts, so no page has to load the desk lists for it. It is started once by App and
 * follows the live stream like a view does: one conditional /meta refetch per event, and a poll
 * every POLL_MS while the stream is connecting or down.
 */
export const navCounts = $state({
  awaiting: 0,
  signoff_pending: 0,
  /** /meta has answered at least once. */
  loaded: false,
});

/**
 * What the server can do, from the same /meta read (the nav counts already follow it): files is
 * true when GET /api/v1/file serves local files, so file paths in bead text can become links;
 * reveal when POST /api/v1/file/reveal can show one in the file manager (Windows, writable tracker).
 */
export const appCaps = $state({ files: false, reveal: false });

let etag: string | null = null;
let refresh: () => Promise<void> = () => Promise.resolve();

/** Refetches the counts now (after a write on the desk) and resolves once they are in. Never rejects. */
export function refreshNavCounts(): Promise<void> {
  return refresh();
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

/** Starts following the counts; returns the cleanup. Call from App's onMount. */
export function startNavCounts(): () => void {
  const ac = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let running = false;
  let again = false;
  let waiters: Array<() => void> = [];

  const fetchOnce = async (): Promise<boolean> => {
    try {
      const r = await getMetaIfChanged(etag, ac.signal);
      if (ac.signal.aborted) return false;
      if (r) {
        etag = r.etag;
        navCounts.awaiting = r.data.counts?.awaiting ?? 0;
        navCounts.signoff_pending = r.data.counts?.signoff_pending ?? 0;
        appCaps.files = r.data.capabilities?.files ?? false;
        appCaps.reveal = r.data.capabilities?.reveal ?? false;
      }
      navCounts.loaded = true;
      return false;
    } catch (err) {
      if (isAbort(err) || ac.signal.aborted) return false;
      // No data yet or the server is unreachable: try again shortly. Any other failure waits for the next event.
      return err instanceof ApiError && ((err.status === 503 && err.code === "not_ready") || (err.status === 0 && err.code === "network"));
    }
  };

  const tick = async (): Promise<void> => {
    running = true;
    clearTimeout(timer);
    timer = undefined;
    const mine = waiters;
    waiters = [];
    try {
      const retry = await fetchOnce();
      if (ac.signal.aborted) return;
      if (retry || live.status === "connecting" || live.status === "down") timer = setTimeout(() => void tick(), POLL_MS);
    } finally {
      running = false;
      for (const w of mine) w();
      if (again && !ac.signal.aborted) {
        again = false;
        void tick();
      }
    }
  };

  const kick = (): void => {
    if (ac.signal.aborted) return;
    if (running) {
      again = true;
      return;
    }
    void tick();
  };

  refresh = () => {
    if (ac.signal.aborted) return Promise.resolve();
    return new Promise<void>((resolve) => {
      // A tick already running may have read /meta before the write landed: wait for the one after it.
      if (running) again = true;
      waiters.push(resolve);
      kick();
    });
  };

  const offLive = onLive(kick);
  void tick();
  return () => {
    offLive();
    ac.abort();
    clearTimeout(timer);
    refresh = () => Promise.resolve();
    for (const w of waiters) w();
    waiters = [];
    etag = null;
  };
}
