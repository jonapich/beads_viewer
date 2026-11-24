import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, getHealth, getJSON, getJSONIfChanged, getInsights, getInsightsIfChanged, getIssue, getIssueIfChanged, getIssues, getIssuesIfChanged, getMeta, getMetaIfChanged, getSignoff, getTriage, getTriageIfChanged, patchIssue, postAnswer, postSignoff, putPriority, search, sendJSON, setCloseCandidate } from "./client";
import { session } from "../session.svelte";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    ...init,
    headers: { "Content-Type": "application/json", ...init.headers },
  });

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  session.status = "unknown";
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("api client", () => {
  it("returns the body on 200 and marks the session ok", async () => {
    fetchMock.mockResolvedValue(json({ status: "ok", version: "v1" }));
    await expect(getHealth()).resolves.toEqual({ status: "ok", version: "v1" });
    expect(session.status).toBe("ok");
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/health");
    expect(init.credentials).toBe("same-origin");
    expect(init.cache).toBe("no-store");
  });

  it("sends If-None-Match and resolves null on 304", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getJSONIfChanged("/api/v1/x", '"abc"')).resolves.toBeNull();
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"abc"');
    expect(session.status).toBe("ok");
  });

  it("returns data and the etag on a changed conditional GET", async () => {
    fetchMock.mockResolvedValue(json({ n: 1 }, { headers: { ETag: '"v2"' } }));
    await expect(getJSONIfChanged("/api/v1/x", null)).resolves.toEqual({ data: { n: 1 }, etag: '"v2"' });
  });

  it("getMeta reads /api/v1/meta", async () => {
    fetchMock.mockResolvedValue(json({ phase2_ready: false }));
    await expect(getMeta()).resolves.toEqual({ phase2_ready: false });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/meta");
  });

  it("getMetaIfChanged sends the etag and resolves null on 304", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getMetaIfChanged('"h-p1"')).resolves.toBeNull();
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/meta");
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"h-p1"');

    fetchMock.mockResolvedValue(json({ phase2_ready: true }, { headers: { ETag: '"h-p2"' } }));
    await expect(getMetaIfChanged('"h-p1"')).resolves.toEqual({ data: { phase2_ready: true }, etag: '"h-p2"' });
  });

  it("getIssues defaults to the tree view and names the view in the query", async () => {
    fetchMock.mockImplementation(() => Promise.resolve(json({ items: [] })));
    await getIssues();
    await getIssues("flat");
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual(["/api/v1/issues?view=tree", "/api/v1/issues?view=flat"]);
  });

  it("getIssuesIfChanged sends the etag for the view", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getIssuesIfChanged("flat", '"x~flat"')).resolves.toBeNull();
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/issues?view=flat");
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"x~flat"');
  });

  it("search percent-encodes the query", async () => {
    fetchMock.mockResolvedValue(json({ query: "a b&c=d/é", total: 0, hits: [] }));
    await search("a b&c=d/é");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/search?q=a%20b%26c%3Dd%2F%C3%A9");
  });

  it("getIssue percent-encodes the id", async () => {
    fetchMock.mockResolvedValue(json({ id: "a b/c#d?e" }));
    await getIssue("a b/c#d?e");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/issues/a%20b%2Fc%23d%3Fe");
  });

  it("getIssueIfChanged sends the etag and getSignoff reads /signoff", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getIssueIfChanged("x-1", '"t"')).resolves.toBeNull();
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/issues/x-1");
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"t"');

    fetchMock.mockResolvedValue(json({ total: 0, groups: [] }));
    await expect(getSignoff()).resolves.toEqual({ total: 0, groups: [] });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/signoff");
  });

  it("getIssue surfaces the not_found envelope", async () => {
    fetchMock.mockResolvedValue(json({ error: { code: "not_found", message: "no such issue" } }, { status: 404 }));
    await expect(getIssue("nope")).rejects.toMatchObject({ status: 404, code: "not_found" });
  });

  it("getInsights adds ?limit only when given", async () => {
    fetchMock.mockImplementation(() => Promise.resolve(json({})));
    await getInsights();
    await getInsights(5);
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual(["/api/v1/insights", "/api/v1/insights?limit=5"]);
  });

  it("getInsightsIfChanged sends the etag for the limit", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getInsightsIfChanged(7, '"i~l7"')).resolves.toBeNull();
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/insights?limit=7");
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"i~l7"');
    await getInsightsIfChanged(undefined, null);
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/insights");
  });

  it("getTriage surfaces analysis_pending and supports conditional reads", async () => {
    fetchMock.mockResolvedValue(json({ error: { code: "analysis_pending", message: "still computing" } }, { status: 503 }));
    await expect(getTriage()).rejects.toMatchObject({ status: 503, code: "analysis_pending" });

    fetchMock.mockResolvedValue(new Response(null, { status: 304 }));
    await expect(getTriageIfChanged('"t"')).resolves.toBeNull();
    const init = fetchMock.mock.calls[1]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["If-None-Match"]).toBe('"t"');
  });

  it("maps 401 to signed-out with the server's error code", async () => {
    fetchMock.mockResolvedValue(json({ error: { code: "unauthorized", message: "open the URL" } }, { status: 401 }));
    const err = await getJSON("/api/v1/x").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 401, code: "unauthorized", message: "open the URL" });
    expect(session.status).toBe("signed-out");
  });

  it("maps a JSON error body on 500", async () => {
    fetchMock.mockResolvedValue(json({ error: { code: "internal", message: "boom" } }, { status: 500 }));
    await expect(getJSON("/api/v1/x")).rejects.toMatchObject({ status: 500, code: "internal", message: "boom" });
    expect(session.status).toBe("ok");
  });

  it("maps a non-JSON 502 to http_502", async () => {
    fetchMock.mockResolvedValue(new Response("<html>bad gateway</html>", { status: 502, statusText: "Bad Gateway" }));
    await expect(getJSON("/api/v1/x")).rejects.toMatchObject({
      status: 502,
      code: "http_502",
      message: "Bad Gateway",
    });
  });

  it("maps 2xx with a bad body to bad_response", async () => {
    fetchMock.mockResolvedValue(new Response("not json", { status: 200 }));
    await expect(getJSON("/api/v1/x")).rejects.toMatchObject({ status: 200, code: "bad_response" });
  });

  it("maps a network failure to offline", async () => {
    fetchMock.mockRejectedValue(new TypeError("fetch failed"));
    await expect(getJSON("/api/v1/x")).rejects.toMatchObject({ status: 0, code: "network" });
    expect(session.status).toBe("offline");
  });

  it("rethrows aborts without touching the session", async () => {
    const abort = new DOMException("aborted", "AbortError");
    fetchMock.mockRejectedValue(abort);
    await expect(getJSON("/api/v1/x")).rejects.toBe(abort);
    expect(session.status).toBe("unknown");
  });
});

describe("api client writes", () => {
  const ok = { id: "x-1", generation: 3, data_hash: "abc" };
  const sent = () => fetchMock.mock.calls[0] as [string, RequestInit];
  const header = (init: RequestInit, name: string) => (init.headers as Record<string, string>)[name];

  it("postSignoff POSTs the JSON body to the escaped id", async () => {
    fetchMock.mockResolvedValue(json({ ...ok, created_id: "x-2" }));
    await expect(postSignoff("a b/c", { approve: false, note: "broken" })).resolves.toEqual({ ...ok, created_id: "x-2" });
    const [path, init] = sent();
    expect(path).toBe("/api/v1/issues/a%20b%2Fc/signoff");
    expect(init.method).toBe("POST");
    expect(init.body).toBe('{"approve":false,"note":"broken"}');
    expect(header(init, "Content-Type")).toBe("application/json");
    expect(init.credentials).toBe("same-origin");
    expect(session.status).toBe("ok");
  });

  it("postAnswer POSTs the option and text", async () => {
    fetchMock.mockResolvedValue(json(ok));
    await postAnswer("x-1", { option: "b", text: "because" });
    const [path, init] = sent();
    expect(path).toBe("/api/v1/issues/x-1/answer");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ option: "b", text: "because" });
  });

  it("putPriority PUTs {priority}, P0 included", async () => {
    fetchMock.mockResolvedValue(json(ok));
    await putPriority("x-1", 0);
    const [path, init] = sent();
    expect(path).toBe("/api/v1/issues/x-1/priority");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe('{"priority":0}');
  });

  it("patchIssue PATCHes the escaped id, keeping an empty assignee and the verbatim base", async () => {
    fetchMock.mockResolvedValue(json(ok));
    const base = "2026-03-02T10:00:00.123456789Z";
    await expect(patchIssue("a b/c", { assignee: "", base_updated_at: base })).resolves.toEqual(ok);
    const [path, init] = sent();
    expect(path).toBe("/api/v1/issues/a%20b%2Fc");
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe(`{"assignee":"","base_updated_at":"${base}"}`);
    expect(header(init, "Content-Type")).toBe("application/json");
  });

  it("setCloseCandidate is PUT to mark and DELETE to unmark, with no body", async () => {
    fetchMock.mockImplementation(() => Promise.resolve(json(ok)));
    await setCloseCandidate("x-1", true);
    await setCloseCandidate("x-1", false);
    const [put, del] = fetchMock.mock.calls as [[string, RequestInit], [string, RequestInit]];
    expect(put[0]).toBe("/api/v1/issues/x-1/labels/close-candidate");
    expect(put[1].method).toBe("PUT");
    expect(del[0]).toBe("/api/v1/issues/x-1/labels/close-candidate");
    expect(del[1].method).toBe("DELETE");
    for (const [, init] of [put, del]) {
      expect(init.body).toBeUndefined();
      expect(header(init, "Content-Type")).toBeUndefined();
    }
  });

  it("sendJSON without a body sets no Content-Type", async () => {
    fetchMock.mockResolvedValue(json(ok));
    await sendJSON("PUT", "/api/v1/x");
    expect(header(sent()[1], "Content-Type")).toBeUndefined();
  });

  it("rejects with the server's error envelope (a refused write)", async () => {
    for (const [status, code, message] of [
      [403, "read_only", "source is read-only"],
      [409, "conflict", "x-1 is no longer awaiting sign-off"],
      [502, "tracker_failed", "Error: no such label"],
    ] as const) {
      fetchMock.mockResolvedValueOnce(json({ error: { code, message } }, { status }));
      await expect(postSignoff("x-1", { approve: true })).rejects.toMatchObject({ status, code, message });
    }
    expect(session.status).toBe("ok");
  });

  it("maps 401 on a write to signed-out and a network failure to offline", async () => {
    fetchMock.mockResolvedValueOnce(json({ error: { code: "unauthorized", message: "open the URL" } }, { status: 401 }));
    await expect(putPriority("x-1", 1)).rejects.toMatchObject({ status: 401 });
    expect(session.status).toBe("signed-out");
    fetchMock.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(putPriority("x-1", 1)).rejects.toMatchObject({ status: 0, code: "network" });
    expect(session.status).toBe("offline");
  });
});
