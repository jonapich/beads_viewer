import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { IssueDetail, IssuePatchRequest, WriteResponse } from "../api/types";
import { clip, EDIT_STATUSES, EDIT_TYPES, editProblem, fieldValue, IssueEdits, normalizeDraft, withCurrent, type EditsClient } from "./edits.svelte";
import { MAX_DESCRIPTION, MAX_LINE_FIELD, normalizeDescription } from "./writes";

const T0 = "2026-03-02T10:00:00.123456789Z";
const T1 = "2026-03-02T11:00:00.5Z";

const issue = (over: Partial<IssueDetail> = {}): IssueDetail =>
  ({
    id: "x-1",
    title: "Cart model",
    description: "Cart model description.",
    status: "open",
    priority: 2,
    issue_type: "task",
    assignee: "ada",
    updated_at: T0,
    ...over,
  }) as IssueDetail;

const resp = (over: Partial<WriteResponse> = {}): WriteResponse => ({ id: "x-1", generation: 2, data_hash: "h", ...over });

function setup(over: { writable?: boolean; patch?: EditsClient["patchIssue"]; refresh?: (state: { detail: IssueDetail }) => Promise<void> | void } = {}) {
  const state = { detail: issue(), blocked: false };
  const log: string[] = [];
  const sent: Array<[string, IssuePatchRequest]> = [];
  const toasts: Array<[string, string]> = [];
  const patchIssue: EditsClient["patchIssue"] =
    over.patch ??
    vi.fn(async (id, req) => {
      sent.push([id, req]);
      return resp();
    });
  const edits = new IssueEdits({
    client: { patchIssue },
    toasts: { push: (kind, text) => toasts.push([kind, text]) },
    writable: () => over.writable ?? true,
    refresh: async () => {
      log.push("refresh");
      await over.refresh?.(state);
    },
    current: (id) => (id === state.detail.id ? state.detail : null),
    blocked: () => state.blocked,
  });
  return { edits, state, sent, toasts, log, patchIssue };
}

describe("pure helpers", () => {
  it("withCurrent adds a value the list lacks, and only then", () => {
    expect(withCurrent(EDIT_TYPES, "bug")).toEqual([...EDIT_TYPES]);
    expect(withCurrent(EDIT_STATUSES, "hooked")).toEqual([...EDIT_STATUSES, "hooked"]);
    expect(withCurrent(EDIT_TYPES, "")).toEqual([...EDIT_TYPES]);
  });

  it("the status menu offers bd's work statuses: no review, pinned or hooked", () => {
    expect(EDIT_STATUSES).toEqual(["open", "in_progress", "blocked", "deferred", "closed"]);
    expect(EDIT_TYPES).toEqual(["bug", "feature", "task", "epic", "chore"]);
  });

  it("fieldValue reads what the editor compares; an unassigned issue is an empty string", () => {
    const d = issue({ assignee: undefined, description: "\n\nBody \n\n" });
    expect(fieldValue(d, "assignee")).toBe("");
    expect(fieldValue(d, "description")).toBe("Body");
    expect(fieldValue(d, "type")).toBe("task");
    expect(fieldValue(d, "status")).toBe("open");
    expect(fieldValue(d, "title")).toBe("Cart model");
  });

  it("normalizeDescription matches the server: leading newlines and trailing whitespace go, nothing else", () => {
    expect(normalizeDescription("\n\n  indented\nbody \t\r\n\n")).toBe("  indented\nbody");
    expect(normalizeDescription("   ")).toBe("");
    expect(normalizeDraft("title", "  x \n")).toBe("x");
    expect(normalizeDraft("description", "a\n\n")).toBe("a");
  });

  it("editProblem repeats the server's checks, counting bytes", () => {
    expect(editProblem("title", "ok")).toBe("");
    expect(editProblem("title", "   ")).toMatch(/can't be empty/);
    expect(editProblem("title", "a".repeat(MAX_LINE_FIELD))).toBe("");
    expect(editProblem("title", "a".repeat(MAX_LINE_FIELD + 1))).toMatch(/500 bytes max/);
    expect(editProblem("title", "é".repeat(MAX_LINE_FIELD / 2 + 1))).toMatch(/500 bytes max/);
    expect(editProblem("assignee", "")).toBe("");
    expect(editProblem("assignee", "a".repeat(MAX_LINE_FIELD + 1))).toMatch(/500 bytes max/);
    expect(editProblem("description", "")).toBe("");
    expect(editProblem("description", "d".repeat(MAX_DESCRIPTION))).toBe("");
    // The cap counts the normalised text, as the server does.
    expect(editProblem("description", `${"d".repeat(MAX_DESCRIPTION)}\n\n\n`)).toBe("");
    expect(editProblem("description", "d".repeat(MAX_DESCRIPTION + 1))).toBe("Too long to save from bv (16,000 bytes max)");
    expect(editProblem("description", "é".repeat(MAX_DESCRIPTION / 2 + 1))).toMatch(/16,000 bytes max/);
  });

  it("clip flattens whitespace and cuts at 90 characters", () => {
    expect(clip("a\n  b")).toBe("a b");
    expect(clip("x".repeat(200))).toHaveLength(90);
    expect(clip("x".repeat(200)).endsWith("…")).toBe(true);
    expect(clip("short")).toBe("short");
  });
});

describe("IssueEdits", () => {
  it("opens on the current value with the detail's updated_at as the base", () => {
    const { edits, state } = setup();
    expect(edits.open("x-1", "title", state.detail)).toBe(true);
    expect(edits.session("x-1", "title")).toEqual({ draft: "Cart model", original: "Cart model", base: T0 });
    // An open editor is left as it is, draft included.
    edits.setDraft("x-1", "title", "New");
    edits.open("x-1", "title", state.detail);
    expect(edits.session("x-1", "title")?.draft).toBe("New");
  });

  it("saves one field: PATCH with the base, toast, closes, waits for the refetch", async () => {
    const { edits, state, sent, toasts, log } = setup();
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "  Cart model v2 ");
    await expect(edits.save("x-1", "title")).resolves.toBe(true);
    expect(sent).toEqual([["x-1", { base_updated_at: T0, title: "Cart model v2" }]]);
    expect(toasts).toEqual([["success", "Title of x-1 saved"]]);
    expect(log).toEqual(["refresh"]);
    expect(edits.session("x-1", "title")).toBeUndefined();
    expect(edits.isBusy("x-1")).toBe(false);
  });

  it("each field sends its own key and toast", async () => {
    const cases: Array<[Parameters<IssueEdits["open"]>[1], string, IssuePatchRequest, string]> = [
      ["status", "in_progress", { base_updated_at: T0, status: "in_progress" }, "Status of x-1 set to In progress"],
      ["status", "closed", { base_updated_at: T0, status: "closed" }, "Status of x-1 set to Closed"],
      ["assignee", "grace", { base_updated_at: T0, assignee: "grace" }, "Assignee of x-1 set to grace"],
      ["assignee", "", { base_updated_at: T0, assignee: "" }, "Assignee of x-1 cleared"],
      ["type", "bug", { base_updated_at: T0, type: "bug" }, "Type of x-1 set to bug"],
      ["description", "\nNew body\n\n", { base_updated_at: T0, description: "New body" }, "Description of x-1 saved"],
    ];
    for (const [field, value, req, toast] of cases) {
      const { edits, state, sent, toasts } = setup();
      edits.open("x-1", field, state.detail);
      edits.setDraft("x-1", field, value);
      await expect(edits.save("x-1", field)).resolves.toBe(true);
      expect(sent[0]?.[1]).toEqual(req);
      expect(toasts[0]).toEqual(["success", toast]);
    }
  });

  it("a status outside the pill names shows as is in the toast", async () => {
    const { edits, state, toasts } = setup();
    edits.open("x-1", "status", state.detail);
    edits.setDraft("x-1", "status", "hooked");
    await edits.save("x-1", "status");
    expect(toasts[0]).toEqual(["success", "Status of x-1 set to hooked"]);
  });

  it("canSave needs a real change and a valid draft", () => {
    const { edits, state } = setup();
    edits.open("x-1", "title", state.detail);
    expect(edits.canSave("x-1", "title")).toBe(false); // unchanged
    edits.setDraft("x-1", "title", "  Cart model  ");
    expect(edits.canSave("x-1", "title")).toBe(false); // the same once trimmed
    edits.setDraft("x-1", "title", "   ");
    expect(edits.canSave("x-1", "title")).toBe(false);
    edits.setDraft("x-1", "title", "Cart");
    expect(edits.canSave("x-1", "title")).toBe(true);

    edits.open("x-1", "assignee", state.detail);
    edits.setDraft("x-1", "assignee", "");
    expect(edits.canSave("x-1", "assignee")).toBe(true); // clearing a set assignee

    edits.open("x-1", "description", state.detail);
    edits.setDraft("x-1", "description", "\nCart model description.\n\n");
    expect(edits.canSave("x-1", "description")).toBe(false); // equal once normalised
    edits.setDraft("x-1", "description", "d".repeat(MAX_DESCRIPTION + 1));
    expect(edits.canSave("x-1", "description")).toBe(false);
    edits.setDraft("x-1", "description", "d".repeat(MAX_DESCRIPTION));
    expect(edits.canSave("x-1", "description")).toBe(true);
  });

  it("clearing an assignee that is already empty is not a change", () => {
    const { edits, state } = setup();
    state.detail = issue({ assignee: undefined });
    edits.open("x-1", "assignee", state.detail);
    expect(edits.canSave("x-1", "assignee")).toBe(false);
  });

  it("save refuses an invalid draft without a request, and says why", async () => {
    const { edits, state, sent } = setup();
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "");
    await expect(edits.save("x-1", "title")).resolves.toBe(false);
    expect(sent).toEqual([]);
    expect(edits.error("x-1", "title")?.message).toMatch(/can't be empty/);
  });

  it("a live refetch during the edit does not move the base", async () => {
    const { edits, state, sent } = setup();
    edits.open("x-1", "title", state.detail);
    state.detail = issue({ updated_at: T1, status: "blocked" }); // someone else changed it
    edits.setDraft("x-1", "title", "Mine");
    await edits.save("x-1", "title");
    expect(sent[0]?.[1].base_updated_at).toBe(T0);
  });

  it("the base is echoed verbatim, nanoseconds included", async () => {
    const { edits, state, sent } = setup();
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Mine");
    await edits.save("x-1", "title");
    expect(sent[0]?.[1].base_updated_at).toBe("2026-03-02T10:00:00.123456789Z");
  });

  it("stays busy until the refetch has landed, and runs one write per issue at a time", async () => {
    let land: () => void = () => {};
    const { edits, state, sent } = setup({ refresh: () => new Promise<void>((r) => (land = r)) });
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "A");
    edits.open("x-1", "assignee", state.detail);
    edits.setDraft("x-1", "assignee", "grace");
    const first = edits.save("x-1", "title");
    await vi.waitFor(() => expect(edits.isBusy("x-1")).toBe(true));
    expect(edits.isSaving("x-1", "title")).toBe(true);
    expect(edits.isSaving("x-1", "assignee")).toBe(false);
    await expect(edits.save("x-1", "assignee")).resolves.toBe(false);
    expect(edits.canEdit("x-1")).toBe(false);
    expect(sent).toHaveLength(1);
    land();
    await first;
    expect(edits.isBusy("x-1")).toBe(false);
  });

  it("a read-only tracker refuses without a request", async () => {
    const { edits, state, sent } = setup({ writable: false });
    expect(edits.canEdit("x-1")).toBe(false);
    expect(edits.open("x-1", "title", state.detail)).toBe(false);
    await expect(edits.quick("x-1", "status", "closed")).resolves.toBe(false);
    expect(sent).toEqual([]);
  });

  it("another kind of write in flight (the quick actions) blocks opening, saving and quick picks, without a request", async () => {
    const { edits, state, sent } = setup();
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Mine");
    expect(edits.canSave("x-1", "title")).toBe(true);
    state.blocked = true;
    expect(edits.canEdit("x-1")).toBe(false);
    expect(edits.canSave("x-1", "title")).toBe(false);
    expect(edits.open("x-1", "assignee", state.detail)).toBe(false);
    await expect(edits.save("x-1", "title")).resolves.toBe(false);
    await expect(edits.quick("x-1", "status", "closed")).resolves.toBe(false);
    await expect(edits.saveMine("x-1", "title")).resolves.toBe(false);
    expect(sent).toEqual([]);
    expect(edits.isBusy("x-1")).toBe(false); // isBusy is the editors' own saves only
    state.blocked = false;
    expect(edits.canSave("x-1", "title")).toBe(true);
    await expect(edits.save("x-1", "title")).resolves.toBe(true);
  });

  it("a refused Save mine does not move the base", async () => {
    const { edits, state, sent } = setup();
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Mine");
    state.detail = issue({ updated_at: T1 });
    state.blocked = true;
    await expect(edits.saveMine("x-1", "title")).resolves.toBe(false);
    expect(edits.session("x-1", "title")?.base).toBe(T0);
    expect(sent).toEqual([]);
    state.blocked = false;
    await expect(edits.saveMine("x-1", "title")).resolves.toBe(true);
    expect(sent[0]?.[1].base_updated_at).toBe(T1);
  });

  it("an editor that is already open while the warning lock holds cannot be saved", async () => {
    const patch = vi.fn(async () => resp({ warning: "Saved, but reloading failed" }));
    const { edits, state } = setup({ patch });
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "First");
    edits.open("x-1", "assignee", state.detail);
    edits.setDraft("x-1", "assignee", "grace");
    await edits.save("x-1", "title"); // lands, the reload failed: still T0
    expect(patch).toHaveBeenCalledTimes(1);
    expect(edits.locked("x-1")).toBe(true);
    expect(edits.session("x-1", "assignee")).toBeDefined();
    expect(edits.canSave("x-1", "assignee")).toBe(false);
    await expect(edits.save("x-1", "assignee")).resolves.toBe(false);
    expect(patch).toHaveBeenCalledTimes(1);
    // The next refetch that lands unlocks it again.
    state.detail = issue({ title: "First", updated_at: T1 });
    expect(edits.canSave("x-1", "assignee")).toBe(true);
  });

  it("a 409 panel blocks Save (Enter) until Save mine or Discard", async () => {
    const { edits, state } = setup({
      patch: vi.fn(async () => {
        throw new ApiError(409, "conflict", "stale");
      }),
    });
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Mine");
    expect(edits.canSave("x-1", "title")).toBe(true);
    await edits.save("x-1", "title");
    expect(edits.error("x-1", "title")?.kind).toBe("conflict");
    expect(edits.canSave("x-1", "title")).toBe(false);
    edits.setDraft("x-1", "title", "Mine, edited again");
    expect(edits.canSave("x-1", "title")).toBe(false);
    // Discard my edit is cancel; a fresh editor can save again.
    edits.cancel("x-1", "title");
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Fresh");
    expect(edits.canSave("x-1", "title")).toBe(true);
  });

  it("a plain error leaves Save enabled", async () => {
    const { edits, state } = setup({ patch: vi.fn(async () => Promise.reject(new ApiError(502, "tracker_failed", "bd said no"))) });
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Mine");
    await edits.save("x-1", "title");
    expect(edits.error("x-1", "title")?.kind).toBe("error");
    expect(edits.canSave("x-1", "title")).toBe(true);
  });

  it("a refused write keeps the draft and shows the message, with the full text", async () => {
    const { edits, state, toasts, log } = setup({
      patch: vi.fn(async () => {
        throw new ApiError(502, "tracker_failed", 'Error: invalid status "review"\nvalid: open, closed');
      }),
    });
    edits.open("x-1", "status", state.detail);
    edits.setDraft("x-1", "status", "review");
    await expect(edits.save("x-1", "status")).resolves.toBe(false);
    expect(edits.session("x-1", "status")?.draft).toBe("review");
    expect(edits.error("x-1", "status")).toEqual({ kind: "error", message: 'Error: invalid status "review"\nvalid: open, closed' });
    expect(toasts).toEqual([]);
    expect(log).toEqual([]); // no refetch for a plain error
    expect(edits.isBusy("x-1")).toBe(false);
    edits.dismissError("x-1", "status");
    expect(edits.error("x-1", "status")).toBeUndefined();
    expect(edits.session("x-1", "status")?.draft).toBe("review");
  });

  it("a network failure without a message still says something", async () => {
    const { edits, state } = setup({ patch: vi.fn(async () => Promise.reject(new Error(""))) });
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "x");
    await edits.save("x-1", "title");
    expect(edits.error("x-1", "title")?.message).toBe("The change was not saved.");
  });

  describe("409", () => {
    const conflict = () =>
      vi.fn(async () => {
        throw new ApiError(409, "conflict", "x-1 changed since you opened it; reload and try again");
      });

    it("refetches first, then shows what the field is now beside the draft", async () => {
      const { edits, state, log } = setup({
        patch: conflict(),
        refresh: (s) => void (s.detail = issue({ title: "Renamed elsewhere", updated_at: T1 })),
      });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      await expect(edits.save("x-1", "title")).resolves.toBe(false);
      expect(log).toEqual(["refresh"]);
      expect(edits.error("x-1", "title")).toMatchObject({ kind: "conflict", now: "Renamed elsewhere", sameField: false });
      expect(edits.session("x-1", "title")?.draft).toBe("Mine");
    });

    it("says so when the field itself did not change", async () => {
      const { edits, state } = setup({
        patch: conflict(),
        refresh: (s) => void (s.detail = issue({ priority: 0, updated_at: T1 })),
      });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      await edits.save("x-1", "title");
      expect(edits.error("x-1", "title")).toMatchObject({ kind: "conflict", now: "Cart model", sameField: true });
    });

    it("Save mine rebases on the refreshed updated_at and saves", async () => {
      let calls = 0;
      const sent: IssuePatchRequest[] = [];
      const { edits, state } = setup({
        patch: vi.fn(async (_id, req) => {
          sent.push(req);
          if (++calls === 1) throw new ApiError(409, "conflict", "stale");
          return resp();
        }),
        refresh: (s) => {
          if (calls === 1) s.detail = issue({ title: "Renamed elsewhere", updated_at: T1 });
        },
      });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      await edits.save("x-1", "title");
      await expect(edits.saveMine("x-1", "title")).resolves.toBe(true);
      expect(sent.map((r) => r.base_updated_at)).toEqual([T0, T1]);
      expect(sent[1]).toMatchObject({ title: "Mine" });
      expect(edits.session("x-1", "title")).toBeUndefined();
      expect(edits.error("x-1", "title")).toBeUndefined();
    });

    it("stays busy through the conflict refetch, and Save mine that conflicts again shows the newer value and keeps the draft", async () => {
      let release: () => void = () => {};
      let phase = 0; // 0: first save; 1: Save mine; both conflict
      const { edits, state } = setup({
        patch: vi.fn(async () => {
          phase++;
          throw new ApiError(409, "conflict", "stale");
        }),
        refresh: (s) => {
          if (phase === 1) {
            s.detail = issue({ title: "First other edit", updated_at: T1 });
            return new Promise<void>((r) => (release = r));
          }
          s.detail = issue({ title: "Second other edit", updated_at: "2026-03-02T12:00:00Z" });
        },
      });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      const first = edits.save("x-1", "title");
      await vi.waitFor(() => expect(phase).toBe(1));
      // The 409 came back and the refetch is still running: the editor is still busy.
      await vi.waitFor(() => expect(state.detail.title).toBe("First other edit"));
      expect(edits.isBusy("x-1")).toBe(true);
      expect(edits.error("x-1", "title")).toBeUndefined();
      release();
      await first;
      expect(edits.isBusy("x-1")).toBe(false);
      expect(edits.error("x-1", "title")).toMatchObject({ kind: "conflict", now: "First other edit" });

      await expect(edits.saveMine("x-1", "title")).resolves.toBe(false);
      expect(edits.error("x-1", "title")).toMatchObject({ kind: "conflict", now: "Second other edit", sameField: false });
      expect(edits.session("x-1", "title")?.draft).toBe("Mine");
      // It was retried on the refreshed base, not the first one.
      expect(edits.session("x-1", "title")?.base).toBe(T1);
    });

    it("Discard my edit is cancel: the draft and the error go", async () => {
      const { edits, state } = setup({ patch: conflict() });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      await edits.save("x-1", "title");
      edits.cancel("x-1", "title");
      expect(edits.session("x-1", "title")).toBeUndefined();
      expect(edits.error("x-1", "title")).toBeUndefined();
    });
  });

  describe("saved, but the reload failed", () => {
    it("toasts the warning and keeps every editor of the issue off while the detail is the same snapshot", async () => {
      const { edits, state, toasts } = setup({ patch: vi.fn(async () => resp({ warning: "Saved, but reloading failed" })) });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      await expect(edits.save("x-1", "title")).resolves.toBe(true);
      expect(toasts).toEqual([
        ["success", "Title of x-1 saved"],
        ["warning", "Saved, but reloading failed"],
      ]);
      // The refetch changed nothing: still T0.
      expect(edits.locked("x-1")).toBe(true);
      expect(edits.canEdit("x-1")).toBe(false);
      expect(edits.open("x-1", "assignee", state.detail)).toBe(false);
      await expect(edits.quick("x-1", "status", "closed")).resolves.toBe(false);
      // The next refetch that lands lets them back.
      state.detail = issue({ title: "Mine", updated_at: T1 });
      expect(edits.locked("x-1")).toBe(false);
      expect(edits.canEdit("x-1")).toBe(true);
    });
  });

  describe("rebase after our own save", () => {
    it("moves another open editor to the new updated_at when its field did not change", async () => {
      const { edits, state, sent } = setup({
        refresh: (s) => void (s.detail = issue({ title: "Mine", updated_at: T1 })),
      });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      edits.open("x-1", "description", state.detail);
      edits.setDraft("x-1", "description", "My new description");
      await edits.save("x-1", "title");
      expect(edits.session("x-1", "description")).toMatchObject({ draft: "My new description", base: T1 });
      await edits.save("x-1", "description");
      expect(sent[1]?.[1]).toEqual({ base_updated_at: T1, description: "My new description" });
    });

    it("leaves an editor whose field changed underneath on its old base, so it meets the 409", async () => {
      const { edits, state } = setup({
        refresh: (s) => void (s.detail = issue({ title: "Mine", description: "Changed elsewhere", updated_at: T1 })),
      });
      edits.open("x-1", "title", state.detail);
      edits.setDraft("x-1", "title", "Mine");
      edits.open("x-1", "description", state.detail);
      edits.setDraft("x-1", "description", "My new description");
      await edits.save("x-1", "title");
      expect(edits.session("x-1", "description")?.base).toBe(T0);
    });
  });

  describe("rebaseOpen (after the page's own quick actions)", () => {
    it("moves an open editor to the new updated_at when its field did not change", () => {
      const { edits, state } = setup();
      edits.open("x-1", "description", state.detail);
      edits.setDraft("x-1", "description", "My new description");
      state.detail = issue({ priority: 0, updated_at: T1 }); // our own priority PUT, refetched
      edits.rebaseOpen("x-1");
      expect(edits.session("x-1", "description")).toMatchObject({ draft: "My new description", base: T1 });
    });

    it("leaves an editor whose field changed underneath on its old base", () => {
      const { edits, state } = setup();
      edits.open("x-1", "description", state.detail);
      state.detail = issue({ description: "Changed elsewhere", updated_at: T1 });
      edits.rebaseOpen("x-1");
      expect(edits.session("x-1", "description")?.base).toBe(T0);
    });

    it("skips the `except` field and an issue that is not on show", () => {
      const { edits, state } = setup();
      edits.open("x-1", "title", state.detail);
      edits.open("x-1", "assignee", state.detail);
      state.detail = issue({ updated_at: T1 });
      edits.rebaseOpen("other-1");
      expect(edits.session("x-1", "title")?.base).toBe(T0);
      edits.rebaseOpen("x-1", "title");
      expect(edits.session("x-1", "title")?.base).toBe(T0);
      expect(edits.session("x-1", "assignee")?.base).toBe(T1);
    });
  });

  describe("quick (status menu)", () => {
    it("opens a fresh session at the pick and saves it", async () => {
      const { edits, state, sent } = setup();
      state.detail = issue({ updated_at: T1 });
      await expect(edits.quick("x-1", "status", "closed")).resolves.toBe(true);
      expect(sent).toEqual([["x-1", { base_updated_at: T1, status: "closed" }]]);
      expect(edits.session("x-1", "status")).toBeUndefined();
    });

    it("the current status does nothing, and closes an open status session", async () => {
      const { edits, state, sent } = setup();
      edits.open("x-1", "status", state.detail);
      await expect(edits.quick("x-1", "status", "open")).resolves.toBe(false);
      expect(sent).toEqual([]);
      expect(edits.session("x-1", "status")).toBeUndefined();
    });

    it("the type goes through a draft and Save, as the select does: choosing writes nothing", async () => {
      const { edits, state, sent } = setup();
      edits.open("x-1", "type", state.detail);
      expect(edits.canSave("x-1", "type")).toBe(false); // unchanged
      edits.setDraft("x-1", "type", "chore"); // the select's change event
      expect(sent).toEqual([]);
      expect(edits.canSave("x-1", "type")).toBe(true);
      await expect(edits.save("x-1", "type")).resolves.toBe(true);
      expect(sent).toEqual([["x-1", { base_updated_at: T0, type: "chore" }]]);
      // Picking the type it already has back is not a change.
      edits.open("x-1", "type", state.detail);
      edits.setDraft("x-1", "type", "task");
      expect(edits.canSave("x-1", "type")).toBe(false);
    });

    it("after a failure the session stays, and a second pick starts over on the current base", async () => {
      let calls = 0;
      const sent: IssuePatchRequest[] = [];
      const { edits, state } = setup({
        patch: vi.fn(async (_id, req) => {
          sent.push(req);
          if (++calls === 1) throw new ApiError(502, "tracker_failed", "cannot close: open children");
          return resp();
        }),
      });
      await expect(edits.quick("x-1", "status", "closed")).resolves.toBe(false);
      expect(edits.session("x-1", "status")?.draft).toBe("closed");
      expect(edits.error("x-1", "status")?.message).toBe("cannot close: open children");
      state.detail = issue({ updated_at: T1 });
      await expect(edits.quick("x-1", "status", "blocked")).resolves.toBe(true);
      expect(sent[1]).toEqual({ base_updated_at: T1, status: "blocked" });
      expect(edits.error("x-1", "status")).toBeUndefined();
    });

    it("is refused for an issue that is not on show", async () => {
      const { edits, sent } = setup();
      await expect(edits.quick("other-1", "status", "closed")).resolves.toBe(false);
      expect(sent).toEqual([]);
    });
  });
});
