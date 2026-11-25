import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { WriteResponse } from "../api/types";
import { answerKey, IssueActions, noteKey, optionKey, type ActionsEnv, type WriteClient } from "./actions.svelte";

const resp = (over: Partial<WriteResponse> = {}): WriteResponse => ({ id: "x-1", generation: 2, data_hash: "h", ...over });

function setup(over: { writable?: boolean; client?: Partial<WriteClient>; refresh?: () => Promise<void> } = {}) {
  const log: string[] = [];
  const client: WriteClient = {
    postSignoff: vi.fn(async (id, req) => (log.push(`signoff ${id} ${JSON.stringify(req)}`), resp())),
    postAnswer: vi.fn(async (id, req) => (log.push(`answer ${id} ${JSON.stringify(req)}`), resp())),
    putPriority: vi.fn(async (id, p) => (log.push(`priority ${id} ${p}`), resp())),
    setCloseCandidate: vi.fn(async (id, on) => (log.push(`candidate ${id} ${on}`), resp())),
    ...over.client,
  };
  const toasts: Array<[string, string]> = [];
  const sticky: string[] = [];
  const env: ActionsEnv = {
    client,
    toasts: { push: (kind, text) => toasts.push([kind, text]) },
    writable: () => over.writable ?? true,
    refresh: over.refresh ?? (async () => void log.push("refresh")),
    sticky: (id) => (log.push(`sticky ${id}`), sticky.push(id)),
  };
  return { actions: new IssueActions(env), client, toasts, sticky, log };
}

describe("IssueActions", () => {
  it("sets a priority: calls, toasts, marks the row sticky and waits for the refetch", async () => {
    const { actions, toasts, log } = setup();
    await expect(actions.setPriority("x-1", 2)).resolves.toBe(true);
    expect(toasts).toEqual([["success", "Priority of x-1 set to P2"]]);
    expect(log).toEqual(["priority x-1 2", "sticky x-1", "refresh"]);
    expect(actions.isBusy("x-1")).toBe(false);
  });

  it("stays busy until the refetch has landed", async () => {
    let land: () => void = () => {};
    const { actions } = setup({ refresh: () => new Promise<void>((r) => (land = r)) });
    const done = actions.setPriority("x-1", 0);
    await vi.waitFor(() => expect(actions.isBusy("x-1")).toBe(true));
    land();
    await done;
    expect(actions.isBusy("x-1")).toBe(false);
  });

  it("ignores a second write on the same issue while one is in flight, not on another issue", async () => {
    let release: (r: WriteResponse) => void = () => {};
    const { actions, client } = setup({
      client: { putPriority: vi.fn(() => new Promise<WriteResponse>((r) => (release = r))) },
    });
    const first = actions.setPriority("x-1", 1);
    await vi.waitFor(() => expect(actions.isBusy("x-1")).toBe(true));
    await expect(actions.setPriority("x-1", 3)).resolves.toBe(false);
    await expect(actions.setCloseCandidate("x-1", true)).resolves.toBe(false);
    expect(client.setCloseCandidate).not.toHaveBeenCalled();
    const other = actions.setCloseCandidate("x-2", true);
    await other;
    release(resp());
    await first;
    expect(client.putPriority).toHaveBeenCalledTimes(1);
  });

  it("does nothing without write access", async () => {
    const { actions, client, toasts } = setup({ writable: false });
    for (const r of [
      actions.setPriority("x-1", 1),
      actions.setCloseCandidate("x-1", true),
      actions.approve("x-1"),
      actions.dispute("x-1", "note"),
      actions.answer("x-1", { text: "t" }),
    ]) {
      await expect(r).resolves.toBe(false);
    }
    expect(Object.values(client).every((f) => (f as ReturnType<typeof vi.fn>).mock.calls.length === 0)).toBe(true);
    expect(toasts).toEqual([]);
  });

  it("toggles close-candidate both ways with the mock's messages", async () => {
    const { actions, toasts, log } = setup();
    await actions.setCloseCandidate("x-1", true);
    await actions.setCloseCandidate("x-1", false);
    expect(log.filter((l) => l.startsWith("candidate"))).toEqual(["candidate x-1 true", "candidate x-1 false"]);
    expect(toasts.map((t) => t[1])).toEqual(["Marked x-1 close-candidate", "Removed close-candidate from x-1"]);
  });

  it("approves a sign-off", async () => {
    const { actions, toasts, log } = setup();
    await expect(actions.approve("x-1")).resolves.toBe(true);
    expect(log[0]).toBe('signoff x-1 {"approve":true}');
    expect(toasts).toEqual([["success", "Signed off x-1"]]);
  });

  it("disputes with a trimmed note, names the follow-up, and drops the draft and the open form", async () => {
    const { actions, toasts, log } = setup({
      client: { postSignoff: vi.fn(async () => resp({ created_id: "x-9" })) },
    });
    actions.openDispute("x-1");
    actions.setDraft(noteKey("x-1"), "  it drops items  ");
    await expect(actions.dispute("x-1", "  it drops items  ")).resolves.toBe(true);
    expect(toasts).toEqual([["success", "Disputed x-1 (follow-up x-9 filed)"]]);
    expect(actions.getDraft(noteKey("x-1"))).toBe("");
    expect(actions.disputeOpen).toBeNull();
    expect(log).toContain("refresh");
  });

  it("remembers the follow-up a dispute filed, per disputed issue", async () => {
    const { actions } = setup({
      client: { postSignoff: vi.fn(async (id: string) => resp(id === "x-1" ? { created_id: "x-9" } : {})) },
    });
    expect(actions.followUps).toEqual({});
    await actions.dispute("x-1", "wrong");
    await actions.dispute("x-2", "also wrong");
    await actions.approve("x-3");
    expect(actions.followUps).toEqual({ "x-1": "x-9" });
  });

  it("refuses a blank dispute note without a request", async () => {
    const { actions, client } = setup();
    await expect(actions.dispute("x-1", "  \n ")).resolves.toBe(false);
    expect(client.postSignoff).not.toHaveBeenCalled();
  });

  it("answers with an option, text, or both, and nothing otherwise", async () => {
    const { actions, client, toasts } = setup();
    await expect(actions.answer("x-1", {})).resolves.toBe(false);
    await expect(actions.answer("x-1", { text: "   " })).resolves.toBe(false);
    expect(client.postAnswer).not.toHaveBeenCalled();

    actions.setDraft(answerKey("x-1"), "because");
    actions.setDraft(optionKey("x-1"), "b");
    await expect(actions.answer("x-1", { option: "b", text: " because " })).resolves.toBe(true);
    expect(client.postAnswer).toHaveBeenCalledWith("x-1", { option: "b", text: "because" });
    expect(toasts).toEqual([["success", "Answer posted on x-1"]]);
    expect(actions.getDraft(answerKey("x-1"))).toBe("");
    expect(actions.getDraft(optionKey("x-1"))).toBe("");

    await actions.answer("x-1", { option: "a" });
    expect(client.postAnswer).toHaveBeenLastCalledWith("x-1", { option: "a" });
  });

  it("toasts the server's message on a refused write, keeps the drafts and the open form, and frees the row", async () => {
    const { actions, toasts, sticky, log } = setup({
      client: {
        postSignoff: vi.fn(async () => {
          throw new ApiError(502, "tracker_failed", "Error: bd exploded");
        }),
      },
    });
    actions.openDispute("x-1");
    actions.setDraft(noteKey("x-1"), "note");
    await expect(actions.dispute("x-1", "note")).resolves.toBe(false);
    expect(toasts).toEqual([["danger", "Error: bd exploded"]]);
    expect(actions.getDraft(noteKey("x-1"))).toBe("note");
    expect(actions.disputeOpen).toBe("x-1");
    expect(actions.isBusy("x-1")).toBe(false);
    expect(sticky).toEqual([]);
    expect(log).not.toContain("refresh");
  });

  it("falls back to a generic message for a rejection with no usable message", async () => {
    for (const reject of [() => Promise.reject("boom"), () => Promise.reject(new Error(""))]) {
      const { actions, toasts } = setup({ client: { putPriority: vi.fn(reject) } });
      await expect(actions.setPriority("x-1", 1)).resolves.toBe(false);
      expect(toasts).toEqual([["danger", "The change was not saved."]]);
    }
  });

  it("toasts a warning next to the success when the write saved but the refresh failed", async () => {
    const { actions, toasts } = setup({
      client: { putPriority: vi.fn(async () => resp({ warning: "Saved, but refreshing the export failed" })) },
    });
    await expect(actions.setPriority("x-1", 1)).resolves.toBe(true);
    expect(toasts).toEqual([
      ["success", "Priority of x-1 set to P1"],
      ["warning", "Saved, but refreshing the export failed"],
    ]);
  });

  it("keeps drafts apart per issue and per kind", () => {
    const { actions } = setup();
    actions.setDraft(noteKey("a"), "1");
    actions.setDraft(answerKey("a"), "2");
    actions.setDraft(noteKey("b"), "3");
    expect([noteKey("a"), answerKey("a"), noteKey("b"), noteKey("c")].map((k) => actions.getDraft(k))).toEqual(["1", "2", "3", ""]);
  });
});

describe("IssueActions onFailed", () => {
  const withFailed = (client: Partial<WriteClient>, writable = true) => {
    const s = setup({ client, writable });
    const failed: Array<[string, string]> = [];
    const env = {
      client: s.client,
      toasts: { push: (kind: string, text: string) => s.toasts.push([kind, text]) },
      writable: () => writable,
      refresh: async () => {},
      sticky: () => {},
      onFailed: (id: string, message: string) => failed.push([id, message]),
    } as ActionsEnv;
    return { actions: new IssueActions(env), failed, toasts: s.toasts };
  };

  it("reports a refused write with the id and the toast message", async () => {
    const { actions, failed, toasts } = withFailed({ postSignoff: vi.fn(async () => Promise.reject(new ApiError(409, "conflict", "bd said no"))) });
    await expect(actions.approve("x-1")).resolves.toBe(false);
    expect(failed).toEqual([["x-1", "bd said no"]]);
    expect(toasts).toEqual([["danger", "bd said no"]]);
  });

  it("falls back to the generic message for an error with none", async () => {
    const { actions, failed } = withFailed({ postAnswer: vi.fn(async () => Promise.reject("boom")) });
    await actions.answer("x-1", { text: "t" });
    expect(failed).toEqual([["x-1", "The change was not saved."]]);
  });

  it("is not called for a write that was never sent, nor for one that succeeded", async () => {
    const ro = withFailed({}, false);
    await ro.actions.approve("x-1");
    await ro.actions.answer("x-1", {});
    const ok = withFailed({});
    await ok.actions.approve("x-1");
    expect(ro.failed).toEqual([]);
    expect(ok.failed).toEqual([]);
  });
});
