import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, getHealth, getJSON, getJSONIfChanged, getMeta, getMetaIfChanged } from "./client";
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