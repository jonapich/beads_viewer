import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TOAST_MS, Toasts } from "./toasts.svelte";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("Toasts", () => {
  it("shows a toast and removes it after its time", () => {
    const t = new Toasts();
    t.push("success", "Saved");
    expect(t.items.map((i) => i.text)).toEqual(["Saved"]);
    vi.advanceTimersByTime(TOAST_MS.success - 1);
    expect(t.items).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(t.items).toHaveLength(0);
  });

  it("keeps a danger toast longer than a success one, and dismisses on demand", () => {
    const t = new Toasts();
    const id = t.push("danger", "bd failed");
    t.push("success", "ok");
    vi.advanceTimersByTime(TOAST_MS.success);
    expect(t.items.map((i) => i.kind)).toEqual(["danger"]);
    t.dismiss(id);
    expect(t.items).toHaveLength(0);
  });
});
