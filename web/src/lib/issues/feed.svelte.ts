import { ApiError, getIssuesIfChanged, getMetaIfChanged } from "../api/client";
import type { IssuesResponse, MetaResponse } from "../api/types";

/** How often /meta is polled while Phase 2 is still computing, and how long a "not ready" answer waits. */
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
 * The server does not reload its data until the SSE bead (beads-c2o.6), so
 * there is no revalidation on focus; only Phase 2 is polled for.
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
        if (retry || this.phase2Pending) timer = setTimeout(() => void tick(false), POLL_MS);
      } finally {
        running = false;
      }
    };

    this.reload = () => {
      if (running || ac.signal.aborted) return;
      clearTimeout(timer);
      this.issuesError = null;
      this.metaError = null;
      void tick(true);
    };

    void tick(true);
    return () => {
      ac.abort();
      clearTimeout(timer);
    };
  }
}
