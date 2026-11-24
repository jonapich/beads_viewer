import { ApiError, getIssueIfChanged, getMetaIfChanged } from "../api/client";
import type { IssueDetail, MetaResponse } from "../api/types";
import { live, onLive } from "../live.svelte";
import { POLL_MS } from "./feed.svelte";

export type DetailStatus = "loading" | "ok" | "missing" | "error";

interface Cached<T> {
  data: T;
  etag: string | null;
}

/** Details remembered for an instant paint when the page comes back to an issue it showed. */
const CACHE_MAX = 30;

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function toApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError(0, "unknown", err instanceof Error ? err.message : String(err));
}

/**
 * Loads one issue (GET /api/v1/issues/{id}) and /meta for the full-page detail view.
 * The id can change while the view stays mounted (/issues/a to /issues/b keeps the same
 * component): setId() switches. Asking for another id drops the old detail at once (or
 * paints the copy remembered for it); a refetch of the same id keeps what is shown until
 * the new copy arrives, so a live update never turns the page into a skeleton.
 *
 * While the live stream is up (lib/live.svelte.ts) every event triggers one conditional
 * refetch of both and nothing is polled; while it is connecting or down both are polled
 * every POLL_MS, and with no stream started ("off") only while Phase 2 is computing.
 * A 404 is "missing" (the issue is not in the tracker); the page shows its 404.
 */
export class DetailFeed {
  detail = $state.raw<IssueDetail | null>(null);
  meta = $state.raw<MetaResponse | null>(null);
  status = $state<DetailStatus>("loading");
  /** The last failure of the detail request (the 404 included); null once a request succeeds. */
  error = $state.raw<ApiError | null>(null);

  /** The server is still computing triage, so the detail's scores are absent or may change. */
  phase2Pending = $derived(
    this.meta !== null && !this.meta.phase2_ready && !this.meta.phase2_error && !this.meta.phase2_skipped,
  );

  /** Refetches after an error (the Retry button). Replaced by start(). */
  reload: () => void = () => {};

  /**
   * Refetches the issue and meta now and resolves once a refetch that started after this
   * call has finished (never rejects; failures show in `error`). A write awaits this so
   * its control stays busy until the page shows the result. Replaced by start(); resolves
   * at once before that and after stop.
   */
  refresh: () => Promise<void> = () => Promise.resolve();

  #id: string | null = null;
  /** Aborted when the id changes (and on stop): it cancels the requests of the old id. */
  #idAc = new AbortController();
  #cache = new Map<string, Cached<IssueDetail>>();
  #metaEtag: string | null = null;
  /** Set by start(): the id changed, so run a fresh tick. */
  #onId: () => void = () => {};

  /** Shows issue `id`. The same id again does nothing. */
  setId(id: string): void {
    if (id === this.#id) return;
    this.#id = id;
    this.#idAc.abort();
    this.#idAc = new AbortController();
    const cached = this.#cache.get(id);
    this.detail = cached?.data ?? null;
    this.status = cached ? "ok" : "loading";
    this.error = null;
    this.#onId();
  }

  #remember(id: string, entry: Cached<IssueDetail>): void {
    this.#cache.delete(id);
    this.#cache.set(id, entry);
    if (this.#cache.size > CACHE_MAX) {
      const oldest = this.#cache.keys().next();
      if (!oldest.done) this.#cache.delete(oldest.value);
    }
  }

  /** Begins loading and polling; returns the cleanup that aborts both. Call from onMount. */
  start(): () => void {
    const ac = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let running = false;
    /** An event or id change arrived during a tick: run one more when it ends. */
    let again = false;
    /** Ticks started so far, and the refresh() callers waiting for a tick numbered `from` or later to end. */
    let started = 0;
    let waiters: Array<{ from: number; resolve: () => void }> = [];
    const settle = (finished: number): void => {
      const done = waiters.filter((w) => w.from <= finished);
      waiters = waiters.filter((w) => w.from > finished);
      for (const w of done) w.resolve();
    };

    /** Resolves true when the call should be retried shortly. */
    const fetchDetail = async (id: string, signal: AbortSignal): Promise<boolean> => {
      try {
        const r = await getIssueIfChanged(id, this.#cache.get(id)?.etag ?? null, signal);
        if (ac.signal.aborted || id !== this.#id) return false;
        if (r) {
          this.#remember(id, { data: r.data, etag: r.etag });
          this.detail = r.data;
        }
        // A 304 means the remembered copy, which setId() put on show, is current.
        this.status = "ok";
        this.error = null;
        return false;
      } catch (err) {
        if (isAbort(err) || ac.signal.aborted || id !== this.#id) return false;
        const e = toApiError(err);
        // 503 not_ready: the server has no data yet, which is just loading.
        if (e.status === 503 && e.code === "not_ready") return true;
        if (e.status === 404) {
          this.#cache.delete(id);
          this.detail = null;
          this.status = "missing";
          this.error = e;
          return false;
        }
        // A failed refetch keeps the page that is up; only an empty page turns into the error state.
        if (this.detail === null) this.status = "error";
        this.error = e;
        // Network failure: keep trying so the page recovers when bv serve comes back.
        return e.status === 0 && e.code === "network";
      }
    };

    const fetchMeta = async (signal: AbortSignal): Promise<boolean> => {
      try {
        const r = await getMetaIfChanged(this.#metaEtag, signal);
        if (ac.signal.aborted) return false;
        if (r) {
          this.#metaEtag = r.etag;
          this.meta = r.data;
        }
        return false;
      } catch (err) {
        if (isAbort(err)) return false;
        const e = toApiError(err);
        return (e.status === 503 && e.code === "not_ready") || (e.status === 0 && e.code === "network");
      }
    };

    const tick = async (): Promise<void> => {
      const mine = ++started;
      running = true;
      // A catch-up tick (an event or id change arrived during the last one) can start while that
      // tick's retry timer is still armed: cancel it, or its poll chain would run beside this one.
      clearTimeout(timer);
      timer = undefined;
      const id = this.#id;
      const signal = this.#idAc.signal;
      try {
        if (id === null) return; // setId() starts it
        const [detailRetry, metaRetry] = await Promise.all([fetchDetail(id, signal), fetchMeta(signal)]);
        // The id changed meanwhile: the catch-up tick (again) loads the new one.
        if (ac.signal.aborted || id !== this.#id) return;
        const poll =
          live.status === "connecting" ||
          live.status === "down" ||
          (live.status === "off" && this.phase2Pending);
        if (detailRetry || metaRetry || poll) timer = setTimeout(() => void tick(), POLL_MS);
      } finally {
        running = false;
        settle(mine);
        if (again && !ac.signal.aborted) {
          again = false;
          void tick();
        }
      }
    };

    /** A live signal, status change or id change: refetch now, never dropped while a tick runs. */
    const kick = (): void => {
      if (ac.signal.aborted) return;
      if (running) {
        again = true;
        return;
      }
      clearTimeout(timer);
      void tick();
    };

    this.#onId = kick;

    this.reload = () => {
      if (running || ac.signal.aborted) return;
      clearTimeout(timer);
      this.error = null;
      if (this.detail === null) this.status = "loading";
      void tick();
    };

    this.refresh = () => {
      if (ac.signal.aborted) return Promise.resolve();
      return new Promise<void>((resolve) => {
        waiters.push({ from: started + 1, resolve });
        kick();
      });
    };

    const offLive = onLive(kick);
    void tick();
    return () => {
      offLive();
      ac.abort();
      this.#idAc.abort();
      clearTimeout(timer);
      this.#onId = () => {};
      settle(Infinity);
    };
  }
}
