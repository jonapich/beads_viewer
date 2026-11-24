import { ApiError, getIssuesIfChanged, getMetaIfChanged } from "../api/client";
import type { IssuesResponse, MetaResponse } from "../api/types";
import { live, onLive } from "../live.svelte";

/** How often /meta is polled while the live stream is not up (or Phase 2 is computing without one), and how long a "not ready" answer waits. */
export const POLL_MS = 2000;

interface Cached<T> {
  data: T;
  etag: string | null;
}

// Module level, so coming back from /issues/:id paints the list at once and the
// first request is a conditional one that usually answers 304.
let issuesCache: Cached<IssuesResponse> | null = null;
let metaCache: Cached<MetaResponse> | null = null;

/** Forgets the cached responses (tests). */
export function resetIssuesFeedCache(): void {
  issuesCache = null;
  metaCache = null;
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function toApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError(0, "unknown", err instanceof Error ? err.message : String(err));
}

/**
 * Loads the issue list (view=tree) and /meta for one mounted Issues view.
 * Data is replaced immutably and held raw: never a deep proxy over 20k items.
 * While the live stream is up (lib/live.svelte.ts) every event, including the
 * Phase 2 swap, triggers one conditional refetch of both and nothing is polled.
 * While it is connecting or down, /meta is polled every POLL_MS: its ETag
 * changes with every generation and Phase 2 swap, which then refetches the list.
 * With no stream started ("off") only Phase 2 is polled for.
 */
export class IssuesFeed {
  data = $state.raw<IssuesResponse | null>(issuesCache?.data ?? null);
  meta = $state.raw<MetaResponse | null>(metaCache?.data ?? null);
  issuesError = $state.raw<ApiError | null>(null);
  metaError = $state.raw<ApiError | null>(null);
  /** The error to show: the list failing outranks /meta failing. */
  error = $derived(this.issuesError ?? this.metaError);

  /** The server is still computing triage, so scores are absent or may change. */
  phase2Pending = $derived(
    this.meta !== null && !this.meta.phase2_ready && !this.meta.phase2_error && !this.meta.phase2_skipped,
  );

  /** Reloads after an error (the Retry button). Replaced by start(). */
  reload: () => void = () => {};

  /** Begins loading and polling; returns the cleanup that aborts both. Call from onMount. */
  start(): () => void {
    const ac = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let running = false;
    let issuesStale = false;
    /** An event arrived during a tick: run one more full refetch when it ends. */
    let again = false;

    /** Resolves true when the call should be retried shortly. */
    const failed = (err: unknown, which: "issues" | "meta"): boolean => {
      if (isAbort(err)) return false;
      const e = toApiError(err);
      // 503 not_ready: the server has no data yet, which is just loading.
      if (e.status === 503 && e.code === "not_ready") return true;
      if (which === "issues") this.issuesError = e;
      else this.metaError = e;
      // Network failure: keep trying so the view recovers when bv serve comes back.
      return e.status === 0 && e.code === "network";
    };

    const fetchIssues = async (): Promise<boolean> => {
      issuesStale = false;
      try {
        const r = await getIssuesIfChanged("tree", issuesCache?.etag ?? null, ac.signal);
        if (ac.signal.aborted) return false;
        if (r) {
          issuesCache = { data: r.data, etag: r.etag };
          this.data = r.data;
        }
        this.issuesError = null;
        return false;
      } catch (err) {
        const retry = failed(err, "issues");
        issuesStale = retry;
        return retry;
      }
    };

    const fetchMeta = async (): Promise<{ retry: boolean; changed: boolean }> => {
      try {
        const r = await getMetaIfChanged(metaCache?.etag ?? null, ac.signal);
        if (ac.signal.aborted) return { retry: false, changed: false };
        this.metaError = null;
        if (!r) return { retry: false, changed: false };
        metaCache = { data: r.data, etag: r.etag };
        this.meta = r.data;
        return { retry: false, changed: true };
      } catch (err) {
        return { retry: failed(err, "meta"), changed: false };
      }
    };

    const tick = async (withIssues: boolean): Promise<void> => {
      running = true;
      timer = undefined;
      try {
        const [issuesRetry, m] = await Promise.all([
          withIssues || issuesStale ? fetchIssues() : Promise.resolve(false),
          fetchMeta(),
        ]);
        if (ac.signal.aborted) return;
        let retry = issuesRetry || m.retry;
        // Any new meta (Phase 2 landing changes its ETag, not data_hash) can change scores.
        if (!withIssues && m.changed) retry = (await fetchIssues()) || retry;
        if (ac.signal.aborted) return;
        const poll =
          live.status === "connecting" ||
          live.status === "down" ||
          (live.status === "off" && this.phase2Pending);
        if (retry || poll) timer = setTimeout(() => void tick(false), POLL_MS);
      } finally {
        running = false;
        if (again && !ac.signal.aborted) {
          again = false;
          void tick(true);
        }
      }
    };

    /** A live signal or status change: refetch now, never dropped while a tick runs. */
    const kick = (): void => {
      if (ac.signal.aborted) return;
      if (running) {
        again = true;
        return;
      }
      clearTimeout(timer);
      void tick(true);
    };

    this.reload = () => {
      if (running || ac.signal.aborted) return;
      clearTimeout(timer);
      this.issuesError = null;
      this.metaError = null;
      void tick(true);
    };

    const offLive = onLive(kick);
    void tick(true);
    return () => {
      offLive();
      ac.abort();
      clearTimeout(timer);
    };
  }
}
