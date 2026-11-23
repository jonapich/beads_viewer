import { ApiError, search } from "../api/client";

/** The server rejects queries over 256 bytes (pkg/api/search.go); the client skips the call instead. */
export const MAX_QUERY_BYTES = 256;

export function queryTooLong(q: string): boolean {
  return new TextEncoder().encode(q).length > MAX_QUERY_BYTES;
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

/**
 * Runs GET /api/v1/search for the list's text query. Each run aborts the one
 * before it and keeps the previous ranking until the new one arrives, so the
 * list does not flash empty while typing. `onrank` gets the hits as
 * id -> position (best first), or null when the query is empty.
 */
export class SearchRunner {
  /** A request is in flight. */
  pending = $state(false);
  /** The query exceeds MAX_QUERY_BYTES: no request was made. */
  tooLong = $state(false);
  error = $state.raw<ApiError | null>(null);

  #ac: AbortController | null = null;
  #onrank: (rank: Map<string, number> | null) => void;

  constructor(onrank: (rank: Map<string, number> | null) => void) {
    this.#onrank = onrank;
  }

  run(raw: string): void {
    const q = raw.trim();
    this.#ac?.abort();
    this.#ac = null;
    this.tooLong = false;
    this.error = null;
    if (q === "") {
      this.pending = false;
      this.#onrank(null);
      return;
    }
    if (queryTooLong(q)) {
      this.pending = false;
      this.tooLong = true;
      return;
    }
    const ac = new AbortController();
    this.#ac = ac;
    this.pending = true;
    search(q, ac.signal).then(
      (res) => {
        if (ac.signal.aborted) return;
        this.pending = false;
        this.#onrank(new Map(res.hits.map((h, i) => [h.id, i])));
      },
      (err: unknown) => {
        if (ac.signal.aborted || isAbort(err)) return;
        this.pending = false;
        this.error = err instanceof ApiError ? err : new ApiError(0, "unknown", err instanceof Error ? err.message : String(err));
      },
    );
  }

  stop(): void {
    this.#ac?.abort();
    this.#ac = null;
    this.pending = false;
  }
}
