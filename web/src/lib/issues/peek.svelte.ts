import { ApiError, getIssue } from "../api/client";
import type { IssueDetail } from "../api/types";

export type PeekStatus = "loading" | "ok" | "missing" | "error";

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

/**
 * Loads GET /api/v1/issues/{id} for the peek drawer. Asking for another id
 * drops the old detail at once; asking again for the same id (a data refresh)
 * keeps what is shown until the new copy arrives. A 404 means the issue is
 * gone from the tracker ("missing").
 */
export class PeekLoader {
  detail = $state.raw<IssueDetail | null>(null);
  status = $state<PeekStatus>("loading");
  error = $state.raw<ApiError | null>(null);

  #ac: AbortController | null = null;
  #id: string | null = null;

  load(id: string | null): void {
    this.#ac?.abort();
    this.#ac = null;
    if (id === null) {
      this.#id = null;
      this.detail = null;
      this.error = null;
      this.status = "loading";
      return;
    }
    if (id !== this.#id) {
      this.#id = id;
      this.detail = null;
      this.error = null;
      this.status = "loading";
    }
    const ac = new AbortController();
    this.#ac = ac;
    getIssue(id, ac.signal).then(
      (d) => {
        if (ac.signal.aborted) return;
        this.detail = d;
        this.error = null;
        this.status = "ok";
      },
      (err: unknown) => {
        if (ac.signal.aborted || isAbort(err)) return;
        const e = err instanceof ApiError ? err : new ApiError(0, "unknown", err instanceof Error ? err.message : String(err));
        if (e.status === 404) {
          this.detail = null;
          this.status = "missing";
        } else if (this.detail === null) {
          this.status = "error";
        }
        this.error = e;
      },
    );
  }

  stop(): void {
    this.#ac?.abort();
    this.#ac = null;
  }
}
