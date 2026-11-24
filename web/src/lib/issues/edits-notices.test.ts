import { describe, expect, it, vi } from "vitest";
import type { IssueDetail, WriteResponse } from "../api/types";
import { displayValue, IssueEdits, LOCK_NOTICE } from "./edits.svelte";

const T0 = "2026-03-02T10:00:00.123456789Z";
const T1 = "2026-03-02T11:00:00.5Z";

const issue = (over: Partial<IssueDetail> = {}): IssueDetail =>
  ({ id: "x-1", title: "Title", status: "in_progress", issue_type: "task", assignee: "ada", description: "", updated_at: T0, ...over }) as IssueDetail;

describe("displayValue", () => {
  it("shows a status by its label and leaves other fields alone", () => {
    expect(displayValue("status", "in_progress")).toBe("In progress");
    expect(displayValue("status", "open")).toBe("Open");
    expect(displayValue("status", "hooked")).toBe("hooked"); // a custom status has no label
    expect(displayValue("type", "in_progress")).toBe("in_progress");
    expect(displayValue("title", "in_progress")).toBe("in_progress");
  });
});

describe("lockNotice", () => {
  it("says why editing is off while the post-warning lock holds, and nothing once it lifts", async () => {
    const state = { detail: issue() };
    const edits = new IssueEdits({
      client: { patchIssue: vi.fn(async (): Promise<WriteResponse> => ({ id: "x-1", generation: 2, data_hash: "h", warning: "reload failed" })) },
      toasts: { push: () => undefined },
      writable: () => true,
      refresh: async () => undefined,
      current: (id) => (id === state.detail.id ? state.detail : null),
      blocked: () => false,
    });
    expect(edits.lockNotice("x-1")).toBe("");
    edits.open("x-1", "title", state.detail);
    edits.setDraft("x-1", "title", "Mine");
    await edits.save("x-1", "title");
    expect(edits.lockNotice("x-1")).toBe(LOCK_NOTICE);
    expect(edits.lockNotice("other-1")).toBe("");
    state.detail = issue({ title: "Mine", updated_at: T1 });
    expect(edits.lockNotice("x-1")).toBe("");
  });
});
