import { ApiError, getMetaIfChanged } from "../api/client";
import type { MetaResponse } from "../api/types";
import { POLL_MS } from "../issues/feed.svelte";
import { live, onLive } from "../live.svelte";

export { POLL_MS };

/** One conditional GET: null means the cached copy is current. */
export type Fetcher<T> = (etag: string | null, signal: AbortSignal) => Promise<{ data: T; etag: string | null } | null>;

interface Cached {
  data: unknown;
  etag: string | null;
}

// Module level and keyed by request path, so going Triage -> Insights -> Triage paints
// at once and the first request of the next mount is a conditional one.
const cache = new Map<string, Cached>();
const META_KEY = "/api/v1/meta";

/** Forgets every cached response (tests). */
export function resetViewFeedCache(): void {
  cache.clear();
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function toApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError(0, "unknown", err instanceof Error ? err.message : String(err));
}

/** What a ViewFeed drives: one fetch per tick that says whether it should be retried shortly. */
interface Loadable {
  error: ApiError | null;
  /** Resolves true when the call should be retried after POLL_MS. */
  load(signal: AbortSignal): Promise<boolean>;
}

/**
 * One API resource of a view (the triage, the plan, the insights at one limit).
 * Data is replaced immutably and held raw. A failed refetch keeps the data it
 * already has (after a write or a watcher reload a large tracker answers 503
 * until phase 2 finishes: the view keeps the old copy and says "Recomputing").
 */
export class Resource<T> implements Loadable {
  data = $state.raw<T | null>(null);
  /** The last failure that is not "still computing"; cleared by the next success. */
  error = $state.raw<ApiError | null>(null);
  /** 503 analysis_pending: the server is still computing it (data may be the previous copy). */
  pending = $state(false);

  #key: string;
  #fetch: Fetcher<T>;

  constructor(key: string, fetch: Fetcher<T>) {
    this.#key = key;
    this.#fetch = fetch;
    this.data = (cache.get(key)?.data as T | undefined) ?? null;
  }

  /** Points the resource at another request (the insights limit changed): paints that one's cached copy if any, else keeps the data on show. The owner refetches. */
  setSource(key: string, fetch: Fetcher<T>): void {
    this.#key = key;
    this.#fetch = fetch;
    const hit = cache.get(key);
    if (hit) this.data = hit.data as T;
  }

  async load(signal: AbortSignal): Promise<boolean> {
    const key = this.#key;
    try {
      const r = await this.#fetch(cache.get(key)?.etag ?? null, signal);
      if (signal.aborted || key !== this.#key) return false;
      if (r) {
        cache.set(key, { data: r.data, etag: r.etag });
        this.data = r.data;
      }
      this.error = null;
      this.pending = false;
      return false;
    } catch (err) {
      if (isAbort(err) || signal.aborted || key !== this.#key) return false;
      const e = toApiError(err);
      if (e.status === 503 && e.code === "not_ready") return true; // no tracker data yet: just loading
      if (e.status === 503 && e.code === "analysis_pending") {
        this.pending = true;
        this.error = null;
        return true;
      }
      this.pending = false;
      this.error = e;
      // Network failure: keep trying so the view recovers when bv serve comes back.
      return e.status === 0 && e.code === "network";
    }
  }
}

/**
 * Loads a view's resources and /meta for one mounted view. One tick fetches all
 * of them in parallel, each with If-None-Match. While the live stream is up every
 * event (the phase 2 swap included, it changes the ETags) is one conditional
 * refetch and nothing is polled. While it is connecting or down everything is
 * refetched every POLL_MS, and with no stream started ("off") only phase 2 is
 * polled for. A 503 not_ready or analysis_pending and a network failure retry
 * after POLL_MS; any other failure sets the resource's error and waits for the
 * next live event or reload().
 */
export class ViewFeed {
  meta = $state.raw<MetaResponse | null>(null);
  metaError = $state.raw<ApiError | null>(null);

  /** The server is still computing phase 2, so scores and metrics are absent or may change. */
  phase2Pending = $derived(
    this.meta !== null && !this.meta.phase2_ready && !this.meta.phase2_error && !this.meta.phase2_skipped,
  );
  /** The tracker takes writes (meta.capabilities.writes). */
  writes = $derived(this.meta?.capabilities?.writes ?? false);

  /** Refetches everything after an error (the Retry button). Replaced by start(). */
  reload: () => void = () => {};
  /**
   * Refetches everything now and resolves once a refetch that started after this
   * call has finished (never rejects; failures show in the resources' errors). A
   * write awaits this so its row stays busy until the view shows the result.
   * Resolves at once before start() and after stop.
   */
  refresh: () => Promise<void> = () => Promise.resolve();
  /** A live signal: refetch now, never dropped while a tick runs. Replaced by start(). */
  kick: () => void = () => {};

  readonly #resources: readonly Loadable[];

  constructor(resources: readonly Loadable[]) {
    this.#resources = resources;
    const m = cache.get(META_KEY);
    if (m) this.meta = m.data as MetaResponse;
  }

  /** Begins loading and polling; returns the cleanup that aborts both. Call from onMount. */
  start(): () => void {
    const ac = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let running = false;
    /** A signal arrived during a tick: run one more full refetch when it ends. */
    let again = false;
    /** Ticks started so far, and the refresh() callers waiting for a tick numbered `from` or later to end. */
    let started = 0;
    let waiters: Array<{ from: number; resolve: () => void }> = [];
    const settle = (finished: number): void => {
      const done = waiters.filter((w) => w.from <= finished);
      waiters = waiters.filter((w) => w.from > finished);
      for (const w of done) w.resolve();
    };

    const fetchMeta = async (): Promise<boolean> => {
      try {
        const r = await getMetaIfChanged(cache.get(META_KEY)?.etag ?? null, ac.signal);
        if (ac.signal.aborted) return false;
        this.metaError = null;
        if (r) {
          cache.set(META_KEY, { data: r.data, etag: r.etag });
          this.meta = r.data;
        }
        return false;
      } catch (err) {
        if (isAbort(err)) return false;
        const e = toApiError(err);
        if (e.status === 503 && e.code === "not_ready") return true;
        this.metaError = e;
        return e.status === 0 && e.code === "network";
      }
    };

    const tick = async (): Promise<void> => {
      const mine = ++started;
      running = true;
      // A catch-up tick (a signal arrived during the last one) can start while that tick's retry
      // timer is still armed: cancel it, or its poll chain would run beside this one.
      clearTimeout(timer);
      timer = undefined;
      try {
        const retries = await Promise.all([fetchMeta(), ...this.#resources.map((r) => r.load(ac.signal))]);
        if (ac.signal.aborted) return;
        const poll =
          live.status === "connecting" ||
          live.status === "down" ||
          (live.status === "off" && this.phase2Pending);
        if (retries.some(Boolean) || poll) timer = setTimeout(() => void tick(), POLL_MS);
      } finally {
        running = false;
        settle(mine);
        if (again && !ac.signal.aborted) {
          again = false;
          void tick();
        }
      }
    };

    this.kick = () => {
      if (ac.signal.aborted) return;
      if (running) {
        again = true;
        return;
      }
      clearTimeout(timer);
      void tick();
    };

    this.reload = () => {
      if (ac.signal.aborted) return;
      this.metaError = null;
      for (const r of this.#resources) r.error = null;
      this.kick();
    };

    this.refresh = () => {
      if (ac.signal.aborted) return Promise.resolve();
      return new Promise<void>((resolve) => {
        waiters.push({ from: started + 1, resolve });
        this.kick();
      });
    };

    const offLive = onLive(() => this.kick());
    void tick();
    return () => {
      offLive();
      ac.abort();
      clearTimeout(timer);
      settle(Infinity);
    };
  }
}
