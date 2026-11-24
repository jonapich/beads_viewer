import { session } from "../session.svelte";
import type {
  AnswerRequest,
  ApiErrorBody,
  HealthResponse,
  InsightsResponse,
  IssueDetail,
  IssuePatchRequest,
  IssuesResponse,
  MetaResponse,
  PriorityRequest,
  SearchResponse,
  SignoffRequest,
  SignoffResponse,
  TriageResponse,
  WriteResponse,
} from "./types";

export type ApiPath = `/api/v1/${string}`;

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

interface RequestOptions {
  signal?: AbortSignal;
  headers?: Record<string, string>;
  body?: BodyInit;
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

async function readErrorBody(res: Response): Promise<ApiErrorBody["error"] | null> {
  if (!(res.headers.get("Content-Type") ?? "").includes("json")) return null;
  try {
    const body = (await res.json()) as Partial<ApiErrorBody> | null;
    const e = body?.error;
    if (e && typeof e.code === "string" && typeof e.message === "string") return e;
  } catch {
    // fall through to the generic error
  }
  return null;
}

/**
 * Performs one API call and keeps the session status in step. Resolves with
 * the response for any 2xx or 304; throws ApiError otherwise. Aborted calls
 * rethrow the abort and leave the session alone.
 */
async function request(method: string, path: ApiPath, opts: RequestOptions = {}): Promise<Response> {
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      cache: "no-store",
      credentials: "same-origin",
      signal: opts.signal,
      body: opts.body,
      headers: { Accept: "application/json", ...opts.headers },
    });
  } catch (err) {
    if (isAbort(err)) throw err;
    session.status = "offline";
    throw new ApiError(0, "network", "bv serve is not reachable");
  }

  if (res.ok || res.status === 304) {
    session.status = "ok";
    return res;
  }

  const e = await readErrorBody(res);
  if (res.status === 401) {
    session.status = "signed-out";
    throw new ApiError(401, e?.code ?? "unauthorized", e?.message ?? "unauthorized");
  }
  // The server answered, so it is reachable even though the call failed.
  session.status = "ok";
  // No envelope: the status text, which is empty over HTTP/2, so never a blank message.
  throw new ApiError(res.status, e?.code ?? `http_${res.status}`, e?.message ?? (res.statusText || `Request failed (HTTP ${res.status})`));
}

async function parseJSON<T>(res: Response): Promise<T> {
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError(res.status, "bad_response", "response was not valid JSON");
  }
}

export async function getJSON<T>(path: ApiPath, opts: { signal?: AbortSignal } = {}): Promise<T> {
  return parseJSON<T>(await request("GET", path, { signal: opts.signal }));
}

/**
 * Conditional GET. Resolves null on 304 (the caller's cached copy is current);
 * otherwise the parsed body with its ETag for the next call.
 */
export async function getJSONIfChanged<T>(
  path: ApiPath,
  etag: string | null,
  opts: { signal?: AbortSignal } = {},
): Promise<{ data: T; etag: string | null } | null> {
  const res = await request("GET", path, {
    signal: opts.signal,
    headers: etag ? { "If-None-Match": etag } : undefined,
  });
  if (res.status === 304) return null;
  return { data: await parseJSON<T>(res), etag: res.headers.get("ETag") };
}

export const getHealth = (signal?: AbortSignal) => getJSON<HealthResponse>("/api/v1/health", { signal });

export const getMeta = (signal?: AbortSignal) => getJSON<MetaResponse>("/api/v1/meta", { signal });

/**
 * Conditional GET of /meta: null means unchanged. Views poll this (every ~2 s)
 * while phase2_ready is false and refetch their data when the ETag changes.
 */
export const getMetaIfChanged = (etag: string | null, signal?: AbortSignal) =>
  getJSONIfChanged<MetaResponse>("/api/v1/meta", etag, { signal });

export type IssuesView = "tree" | "flat";

/** Every issue of the current view; `tree` adds the adjacency. Filter client-side. */
export const getIssues = (view: IssuesView = "tree", signal?: AbortSignal) =>
  getJSON<IssuesResponse>(`/api/v1/issues?view=${view}`, { signal });

/** Conditional GET of /issues: null means the cached copy for this view is current. */
export const getIssuesIfChanged = (view: IssuesView, etag: string | null, signal?: AbortSignal) =>
  getJSONIfChanged<IssuesResponse>(`/api/v1/issues?view=${view}`, etag, { signal });

/** The TUI's fuzzy list filter: every hit in rank order. q must be 1..256 bytes. */
export const search = (q: string, signal?: AbortSignal) =>
  getJSON<SearchResponse>(`/api/v1/search?q=${encodeURIComponent(q)}`, { signal });

/** One issue in full. The id is percent-encoded; an unknown id rejects with ApiError 404 "not_found". */
export const getIssue = (id: string, signal?: AbortSignal) =>
  getJSON<IssueDetail>(`/api/v1/issues/${encodeURIComponent(id)}`, { signal });

/** Conditional GET of one issue: null means the cached copy is current. */
export const getIssueIfChanged = (id: string, etag: string | null, signal?: AbortSignal) =>
  getJSONIfChanged<IssueDetail>(`/api/v1/issues/${encodeURIComponent(id)}`, etag, { signal });

/** Closed beads awaiting sign-off, grouped by epic. */
export const getSignoff = (signal?: AbortSignal) => getJSON<SignoffResponse>("/api/v1/signoff", { signal });

/**
 * Triage. Rejects with ApiError 503 "analysis_pending" while it is still being
 * computed (retry after ~2 s), 500 "analysis_failed" if Phase 2 failed.
 */
export const getTriage = (signal?: AbortSignal) => getJSON<TriageResponse>("/api/v1/triage", { signal });

/** Conditional GET of triage: null means the cached copy is current. */
export const getTriageIfChanged = (etag: string | null, signal?: AbortSignal) =>
  getJSONIfChanged<TriageResponse>("/api/v1/triage", etag, { signal });

/** Graph insights; each list holds at most `limit` entries (server default 25, max 500). */
export const getInsights = (limit?: number, signal?: AbortSignal) =>
  getJSON<InsightsResponse>(limit === undefined ? "/api/v1/insights" : `/api/v1/insights?limit=${limit}`, { signal });

/** Conditional GET of insights for one limit: null means the cached copy is current. */
export const getInsightsIfChanged = (limit: number | undefined, etag: string | null, signal?: AbortSignal) =>
  getJSONIfChanged<InsightsResponse>(
    limit === undefined ? "/api/v1/insights" : `/api/v1/insights?limit=${limit}`,
    etag,
    { signal },
  );

/**
 * A write: JSON in (when there is a body), JSON out. The browser adds the Origin
 * header the server checks on every non-GET; the session cookie goes along. Errors
 * reject with ApiError carrying the server's {error:{code,message}}.
 */
export async function sendJSON<T>(method: "POST" | "PUT" | "PATCH" | "DELETE", path: ApiPath, body?: unknown): Promise<T> {
  const res = await request(
    method,
    path,
    body === undefined ? {} : { body: JSON.stringify(body), headers: { "Content-Type": "application/json" } },
  );
  return parseJSON<T>(res);
}

const issueActionPath = (id: string, tail: string): ApiPath => `/api/v1/issues/${encodeURIComponent(id)}/${tail}`;

/** Approves (approve: true) or disputes (approve: false, with a note) a closed issue awaiting sign-off. */
export const postSignoff = (id: string, req: SignoffRequest) =>
  sendJSON<WriteResponse>("POST", issueActionPath(id, "signoff"), req);

/** Answers a needs-decision / needs-design issue: an option key, free text, or both. */
export const postAnswer = (id: string, req: AnswerRequest) =>
  sendJSON<WriteResponse>("POST", issueActionPath(id, "answer"), req);

/** Sets the priority, 0 (P0) to 4 (P4). */
export const putPriority = (id: string, priority: number) =>
  sendJSON<WriteResponse>("PUT", issueActionPath(id, "priority"), { priority } satisfies PriorityRequest);

/**
 * Edits title, status, priority, assignee, type and description in one request
 * (the TUI's O editor). Rejects with ApiError 409 "conflict" when base_updated_at
 * no longer matches the issue; the data is then already reloaded, so refetch it.
 */
export const patchIssue = (id: string, req: IssuePatchRequest) =>
  sendJSON<WriteResponse>("PATCH", `/api/v1/issues/${encodeURIComponent(id)}`, req);

/** Marks (PUT) or unmarks (DELETE) an issue as a close candidate. */
export const setCloseCandidate = (id: string, on: boolean) =>
  sendJSON<WriteResponse>(on ? "PUT" : "DELETE", issueActionPath(id, "labels/close-candidate"));
