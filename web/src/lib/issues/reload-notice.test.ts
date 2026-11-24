import { describe, expect, it } from "vitest";
import { clockTime, reloadNotice } from "./reload-notice";

describe("reloadNotice", () => {
  it("is null without a reload error", () => {
    expect(reloadNotice(null)).toBeNull();
    expect(reloadNotice({ loaded_at: "2026-03-10T12:00:00Z" })).toBeNull();
  });

  it("names the error and the load time in local HH:MM:SS", () => {
    const iso = new Date(2026, 2, 10, 9, 5, 7).toISOString();
    expect(clockTime(iso)).toBe("09:05:07");
    expect(reloadNotice({ reload_error: "bad json", loaded_at: iso })).toBe(
      "Couldn't reload the tracker: bad json. Showing the data loaded at 09:05:07.",
    );
  });

  it("drops the time when loaded_at does not parse", () => {
    expect(reloadNotice({ reload_error: "x", loaded_at: "nope" })).toBe(
      "Couldn't reload the tracker: x. Showing the data loaded.",
    );
  });
});
