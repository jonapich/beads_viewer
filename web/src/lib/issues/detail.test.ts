import { describe, expect, it } from "vitest";
import type { IssueDetail, Link } from "../api/types";
import {
  absTime,
  designOpenByDefault,
  linkHost,
  pillStatus,
  readOnlyNotice,
  relTime,
  shortDate,
  signoffPill,
  stageOf,
  typedLinks,
} from "./detail";
import { READ_ONLY_TITLE } from "./writes";

describe("absTime / shortDate", () => {
  it("formats in UTC", () => {
    expect(absTime("2026-11-23T02:32:00Z")).toBe("23 Nov 2026 02:32 UTC");
    expect(absTime("2026-03-01T09:05:00+02:00")).toBe("1 Mar 2026 07:05 UTC");
    expect(shortDate("2026-11-23T02:32:00Z")).toBe("23 Nov");
  });

  it("gives an empty string for a timestamp that does not parse", () => {
    expect(absTime("")).toBe("");
    expect(absTime("yesterday")).toBe("");
    expect(shortDate("nope")).toBe("");
  });
});

describe("relTime", () => {
  const now = Date.parse("2026-11-23T12:00:00Z");
  const ago = (ms: number) => new Date(now - ms).toISOString();

  it("steps through just now, minutes, hours and days", () => {
    expect(relTime(ago(5_000), now)).toBe("just now");
    expect(relTime(ago(59_000), now)).toBe("just now");
    expect(relTime(ago(60_000), now)).toBe("1 m ago");
    expect(relTime(ago(59 * 60_000), now)).toBe("59 m ago");
    expect(relTime(ago(60 * 60_000), now)).toBe("1 h ago");
    expect(relTime(ago(23 * 3_600_000), now)).toBe("23 h ago");
    expect(relTime(ago(24 * 3_600_000), now)).toBe("1 d ago");
    expect(relTime(ago(10 * 86_400_000), now)).toBe("10 d ago");
  });

  it("reads a future timestamp as just now and a bad one as empty", () => {
    expect(relTime(new Date(now + 60_000).toISOString(), now)).toBe("just now");
    expect(relTime("garbage", now)).toBe("");
  });
});

describe("designOpenByDefault", () => {
  it("is open up to 40 lines and collapsed beyond", () => {
    const lines = (n: number) => Array.from({ length: n }, (_, i) => `l${i}`).join("\n");
    expect(designOpenByDefault(undefined)).toBe(true);
    expect(designOpenByDefault("")).toBe(true);
    expect(designOpenByDefault(lines(40))).toBe(true);
    expect(designOpenByDefault(lines(41))).toBe(false);
  });
});

describe("typedLinks / linkHost", () => {
  const link = (source: string, url: string): Link => ({ kind: "mock", url, source });

  it("keeps only the links bd stored on the issue", () => {
    const links = [link("typed", "https://a.test/x"), link("comment", "https://b.test/y"), link("typed", "https://c.test/z")];
    expect(typedLinks({ links }).map((l) => l.url)).toEqual(["https://a.test/x", "https://c.test/z"]);
    expect(typedLinks({ links: [] })).toEqual([]);
  });

  it("shows the host of a link, or the text itself when it is not a URL", () => {
    expect(linkHost("https://claude.ai/code/artifact/abc")).toBe("claude.ai");
    expect(linkHost("http://localhost:7777/x")).toBe("localhost:7777");
    expect(linkHost("not a url")).toBe("not a url");
  });
});

describe("stageOf", () => {
  it("reads metadata.stage when it is a string", () => {
    expect(stageOf({ metadata: { stage: "code" } })).toBe("code");
    expect(stageOf({ metadata: { stage: 3 } })).toBe("");
    expect(stageOf({ metadata: {} })).toBe("");
    expect(stageOf({})).toBe("");
  });
});

describe("pillStatus", () => {
  const now = Date.parse("2026-11-23T12:00:00Z");
  const d = (over: Partial<Pick<IssueDetail, "status" | "blocked" | "defer_until">>) => ({ status: "open", blocked: false, ...over });

  it("follows the list's display status", () => {
    expect(pillStatus(d({}), now)).toBe("open");
    expect(pillStatus(d({ status: "in_progress" }), now)).toBe("in_progress");
    expect(pillStatus(d({ blocked: true }), now)).toBe("blocked");
    expect(pillStatus(d({ status: "closed", blocked: true }), now)).toBe("closed");
    expect(pillStatus(d({ defer_until: "2027-01-01T00:00:00Z" }), now)).toBe("deferred");
  });

  it("names review and pinned, which the list glyphs do not", () => {
    expect(pillStatus(d({ status: "review" }), now)).toBe("review");
    expect(pillStatus(d({ status: "pinned" }), now)).toBe("pinned");
  });
});

describe("signoffPill", () => {
  it("labels the four states and passes an unknown one through", () => {
    expect(signoffPill("pending").label).toBe("Pending");
    expect(signoffPill("approved").label).toBe("Approved");
    expect(signoffPill("disputed").label).toBe("Disputed");
    expect(signoffPill("waived").label).toBe("Waived");
    expect(signoffPill("odd")).toEqual({ label: "odd", glyph: "" });
  });
});

describe("readOnlyNotice", () => {
  const meta = (writes: boolean) => ({ capabilities: { writes, events: true } });

  it("stays quiet until /meta is loaded, so it does not flash", () => {
    expect(readOnlyNotice(null, null)).toBeNull();
    expect(readOnlyNotice(null, {})).toBeNull();
  });

  it("is quiet when the tracker takes writes", () => {
    expect(readOnlyNotice(meta(true), {})).toBeNull();
  });

  it("gives the generic notice when the tracker takes no writes", () => {
    expect(readOnlyNotice(meta(false), null)).toBe(READ_ONLY_TITLE);
    expect(readOnlyNotice(meta(false), {})).toBe(READ_ONLY_TITLE);
  });

  it("prefers the issue's own reason, even before /meta arrives", () => {
    expect(readOnlyNotice(meta(false), { read_only_reason: "no route" })).toBe("no route");
    expect(readOnlyNotice(meta(true), { read_only_reason: "no route" })).toBe("no route");
    expect(readOnlyNotice(null, { read_only_reason: "no route" })).toBe("no route");
  });
});
