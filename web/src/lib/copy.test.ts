import { describe, expect, it, vi } from "vitest";
import { copyFailure, copyIssueId, copyText } from "./copy";
import type { ToastKind } from "./toasts.svelte";

function env(writeText?: (t: string) => Promise<void>) {
  const pushed: Array<[ToastKind, string]> = [];
  return {
    pushed,
    env: {
      clipboard: writeText ? { writeText } : undefined,
      toasts: { push: (k: ToastKind, t: string) => pushed.push([k, t]) },
    },
  };
}

describe("copyIssueId", () => {
  it("writes the plain id and toasts Copied", async () => {
    const write = vi.fn().mockResolvedValue(undefined);
    const e = env(write);
    await expect(copyIssueId("beads-c2o.11", e.env)).resolves.toBe(true);
    expect(write).toHaveBeenCalledExactlyOnceWith("beads-c2o.11");
    expect(e.pushed).toEqual([["success", "Copied beads-c2o.11"]]);
  });

  it("warns with the browser's reason when the write is refused", async () => {
    const e = env(() => Promise.reject(new Error("Write permission denied.")));
    await expect(copyIssueId("x-1", e.env)).resolves.toBe(false);
    expect(e.pushed).toEqual([["warning", "Couldn't copy: Write permission denied."]]);
  });

  it("warns when there is no clipboard at all", async () => {
    const e = env();
    await expect(copyIssueId("x-1", e.env)).resolves.toBe(false);
    expect(e.pushed).toEqual([["warning", "Couldn't copy: the clipboard is not available"]]);
  });

  it("copes with a rejection that is not an Error", async () => {
    const e = env(() => Promise.reject("nope"));
    await expect(copyIssueId("x-1", e.env)).resolves.toBe(false);
    expect(e.pushed[0]?.[1]).toBe("Couldn't copy: the clipboard is not available");
  });
});

describe("copyFailure", () => {
  it("uses the message, trimmed, else a fallback", () => {
    expect(copyFailure(new Error(" denied "))).toBe("denied");
    expect(copyFailure(new Error(""))).toBe("the clipboard is not available");
    expect(copyFailure(undefined)).toBe("the clipboard is not available");
  });
});

describe("copyText", () => {
  it("copies any text with the message it is given", async () => {
    const write = vi.fn().mockResolvedValue(undefined);
    const e = env(write);
    await expect(copyText("C:\\shots\\a.png", "Copied path", e.env)).resolves.toBe(true);
    expect(write).toHaveBeenCalledExactlyOnceWith("C:\\shots\\a.png");
    expect(e.pushed).toEqual([["success", "Copied path"]]);
  });

  it("warns the same way as an id copy", async () => {
    const e = env(() => Promise.reject(new Error("denied")));
    await expect(copyText("x", "Copied path", e.env)).resolves.toBe(false);
    expect(e.pushed).toEqual([["warning", "Couldn't copy: denied"]]);
  });
});
