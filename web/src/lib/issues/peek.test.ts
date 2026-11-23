import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import decision from "../api/__fixtures__/issue-detail-decision.json";
import signoff from "../api/__fixtures__/issue-detail-signoff.json";
import type { IssueDetail, IssueRef } from "../api/types";
import { callView, childProgress, refStatus, signoffLabel, signoffTone, topReasons } from "./peek";
import { PeekLoader } from "./peek.svelte";

const dec: IssueDetail = decision;
const sig: IssueDetail = signoff;
const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { ...init, headers: { "Content-Type": "application/json", ...init.headers } });

describe("peek helpers", () => {
  it("counts closed children", () => {
    const kids: IssueRef[] = [
      { id: "a", status: "closed" },
      { id: "b", status: "open" },
      { id: "c", status: "closed" },
    ];
    expect(childProgress(kids)).toEqual({ done: 2, total: 3 });
    expect(childProgress([])).toEqual({ done: 0, total: 0 });
  });

  it("takes the top three triage reasons, else the primary", () => {
    expect(topReasons(dec.triage)).toHaveLength(3);
    expect(topReasons(dec.triage, 2)).toEqual(dec.triage?.reasons?.all.slice(0, 2));
    expect(topReasons(undefined)).toEqual([]);
    expect(topReasons({ ...(dec.triage as NonNullable<IssueDetail["triage"]>), reasons: { all: [], primary: "p", action_hint: "" } })).toEqual(["p"]);
    expect(topReasons({ ...(dec.triage as NonNullable<IssueDetail["triage"]>), reasons: undefined })).toEqual([]);
  });

  it("maps a referenced issue status to a glyph status", () => {
    expect(refStatus("closed")).toBe("closed");
    expect(refStatus("in_progress")).toBe("in_progress");
    expect(refStatus("open")).toBe("open");
    expect(refStatus(undefined)).toBe("open");
    expect(refStatus("weird")).toBe("open");
  });

  it("bolds a sign-off call only when it has an alternative or a why", () => {
    expect(callView({ call: "Whole sentence of what was decided." })).toEqual({ bold: false, alternative: "", why: "" });
    expect(callView({ call: "x", alternative: "", why: "" }).bold).toBe(false);
    expect(callView({ call: "x", alternative: "y" })).toEqual({ bold: true, alternative: "y", why: "" });
    expect(callView({ call: "x", why: "z" })).toEqual({ bold: true, alternative: "", why: "z" });
    const fx = sig.signoff?.calls[0] as NonNullable<IssueDetail["signoff"]>["calls"][number];
    expect(callView(fx).bold).toBe(true); // the fixture call has both
  });

  it("words the sign-off badge", () => {
    expect(signoffLabel("pending")).toBe("Awaiting sign-off");
    expect(signoffLabel("approved")).toBe("Approved");
    expect(signoffLabel("disputed")).toBe("Disputed");
    expect(signoffLabel("waived")).toBe("Waived");
    expect(signoffLabel("odd")).toBe("odd");
  });

  it("tones the sign-off state", () => {
    expect(signoffTone("approved")).toBe("ok");
    expect(signoffTone("waived")).toBe("ok");
    expect(signoffTone("disputed")).toBe("bad");
    expect(signoffTone("pending")).toBe("pending");
    expect(sig.signoff?.state).toBe("pending");
  });
});

describe("PeekLoader", () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  let pending: Array<{ url: string; signal: AbortSignal | null | undefined; resolve: (r: Response) => void }>;

  beforeEach(() => {
    pending = [];
    fetchMock = vi.fn(
      (url: string, init: RequestInit) =>
        new Promise<Response>((resolve, reject) => {
          pending.push({ url, signal: init.signal, resolve });
          init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        }),
    );
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());
  const flush = () => new Promise((r) => setTimeout(r, 0));

  it("loads a detail", async () => {
    const l = new PeekLoader();
    l.load("fx-decide");
    expect(l.status).toBe("loading");
    expect(pending[0]?.url).toBe("/api/v1/issues/fx-decide");
    pending[0]?.resolve(json(dec));
    await flush();
    expect(l.status).toBe("ok");
    expect(l.detail?.id).toBe("fx-decide");
  });

  it("drops the old detail for another id and ignores its late response", async () => {
    const l = new PeekLoader();
    l.load("fx-decide");
    pending[0]?.resolve(json(dec));
    await flush();
    l.load("fx-done");
    expect(l.detail).toBeNull();
    expect(l.status).toBe("loading");
    l.load("fx-done"); // same id again aborts the first request
    expect(pending[1]?.signal?.aborted).toBe(true);
    pending[1]?.resolve(json(dec));
    pending[2]?.resolve(json(sig));
    await flush();
    expect(l.detail?.id).toBe("fx-done");
  });

  it("keeps the shown detail while the same id reloads", async () => {
    const l = new PeekLoader();
    l.load("fx-decide");
    pending[0]?.resolve(json(dec));
    await flush();
    l.load("fx-decide");
    expect(l.detail?.id).toBe("fx-decide");
    expect(l.status).toBe("ok");
  });

  it("reports a 404 as missing", async () => {
    const l = new PeekLoader();
    l.load("gone");
    pending[0]?.resolve(json({ error: { code: "not_found", message: "no such issue" } }, { status: 404 }));
    await flush();
    expect(l.status).toBe("missing");
    expect(l.detail).toBeNull();
  });

  it("reports other failures as an error, keeping a detail already shown", async () => {
    const l = new PeekLoader();
    l.load("x");
    pending[0]?.resolve(json({ error: { code: "internal", message: "boom" } }, { status: 500 }));
    await flush();
    expect(l.status).toBe("error");
    expect(l.error?.status).toBe(500);

    const m = new PeekLoader();
    m.load("fx-decide");
    pending[1]?.resolve(json(dec));
    await flush();
    m.load("fx-decide");
    pending[2]?.resolve(json({ error: { code: "internal", message: "boom" } }, { status: 500 }));
    await flush();
    expect(m.status).toBe("ok");
    expect(m.detail?.id).toBe("fx-decide");
  });

  it("clears on null and stops quietly", async () => {
    const l = new PeekLoader();
    l.load("x");
    l.load(null);
    expect(pending[0]?.signal?.aborted).toBe(true);
    expect(l.detail).toBeNull();
    l.load("y");
    l.stop();
    await flush();
    expect(l.error).toBeNull();
  });
});
