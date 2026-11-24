import { describe, expect, it } from "vitest";
import type { IssueDetail, Link } from "../api/types";
import {
  absTime,
  blockedByMeta,
  blocksMeta,
  commentSummary,
  commitHref,
  defaultCommentOpen,
  designOpenByDefault,
  historyEvents,
  humanKey,
  isScalar,
  kindCounts,
  linkHost,
  pillStatus,
  readOnlyNotice,
  relatedTag,
  relationshipCount,
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

describe("defaultCommentOpen", () => {
  it("expands sign-off, user, answer and plain comments, and collapses the other kinds", () => {
    for (const k of ["signoff", "user", "answer", undefined, ""]) expect(defaultCommentOpen(k), String(k)).toBe(true);
    for (const k of ["brief", "advice", "coder", "review", "verify", "whatever"]) expect(defaultCommentOpen(k), k).toBe(false);
  });
});

describe("commentSummary", () => {
  it("takes what_changed, then summary, then disputed from a JSON body", () => {
    expect(commentSummary({ text: "", data: { what_changed: "Added X", summary: "S" } })).toBe("Added X");
    expect(commentSummary({ text: "", data: { summary: "S", disputed: "D" } })).toBe("S");
    expect(commentSummary({ text: "", data: { disputed: "D" } })).toBe("D");
  });

  it("takes the first line of a list value", () => {
    expect(commentSummary({ text: "", data: { what_changed: ["First change", "Second"] } })).toBe("First change");
  });

  it("falls back to the first string value of the object", () => {
    expect(commentSummary({ text: "", data: { n: 3, calls_made: [{ call: "x" }], note: "A note" } })).toBe("A note");
  });

  it("cuts at the first sentence", () => {
    expect(commentSummary({ text: "", data: { summary: "Did it. Then more." } })).toBe("Did it.");
    expect(commentSummary({ text: "", data: { summary: "v1.2 shipped" } })).toBe("v1.2 shipped");
  });

  it("is empty for a JSON body with no string at all", () => {
    expect(commentSummary({ text: "{...}", data: {} })).toBe("");
    expect(commentSummary({ text: "", data: { n: 1, list: [1, 2] } })).toBe("");
  });

  it("takes the first non-blank line of a text body, without cutting the sentence", () => {
    expect(commentSummary({ text: "\n  \n  Goal: do it. Then more.\nsecond" })).toBe("Goal: do it. Then more.");
    expect(commentSummary({ text: "" })).toBe("");
  });
});

describe("kindCounts", () => {
  it("counts each kind in order of first appearance and skips plain comments", () => {
    expect(kindCounts([{ kind: "brief" }, {}, { kind: "coder" }, { kind: "brief" }, { kind: undefined }])).toEqual([
      { kind: "brief", n: 2 },
      { kind: "coder", n: 1 },
    ]);
    expect(kindCounts([])).toEqual([]);
  });
});

describe("historyEvents", () => {
  const base = { created_at: "2026-03-01T10:00:00Z" };

  it("lists created, started, stages, sign-off and closed, oldest first", () => {
    const ev = historyEvents({
      ...base,
      started_at: "2026-03-02T10:00:00Z",
      closed_at: "2026-03-06T10:00:00Z",
      metadata: {
        stage: "code",
        "stage_at.code": "2026-03-04T10:00:00Z",
        "stage_at.advise": "2026-03-03T10:00:00Z",
        signoff_at: "2026-03-05T10:00:00Z",
        signoff: "pending",
        link_unrelated: "x",
      },
    });
    expect(ev.map((e) => [e.kind, e.label])).toEqual([
      ["created", "Created"],
      ["started", "Started"],
      ["stage", "Stage → advise"],
      ["stage", "Stage → code"],
      ["signoff", "Sign-off requested"],
      ["closed", "Closed"],
    ]);
  });

  it("says waived when the sign-off was waived", () => {
    const ev = historyEvents({ ...base, metadata: { signoff_at: "2026-03-05T10:00:00Z", signoff: "waived" } });
    expect(ev.at(-1)?.label).toBe("Sign-off waived");
  });

  it("is just created for a bare issue and ignores timestamps that do not parse", () => {
    expect(historyEvents(base).map((e) => e.kind)).toEqual(["created"]);
    expect(historyEvents({ ...base, metadata: { "stage_at.x": "soon", "stage_at.y": 7 } }).map((e) => e.kind)).toEqual(["created"]);
  });
});

describe("JSON view helpers", () => {
  it("humanKey spaces and capitalises", () => {
    expect(humanKey("calls_made")).toBe("Calls made");
    expect(humanKey("a")).toBe("A");
    expect(humanKey("")).toBe("");
  });

  it("commitHref links only an 'owner/repo sha' string", () => {
    expect(commitHref("jonapich/beads_viewer 933665b")).toBe("https://github.com/jonapich/beads_viewer/commit/933665b");
    expect(commitHref("o/r abcdef0123456789abcdef0123456789abcdef01")).not.toBeNull();
    for (const s of ["933665b", "o/r xyz1234", "o/r abc12", "https://evil.test/x abcdef1", "o/r abcdef1 extra", "a/b/c abcdef1", ""]) {
      expect(commitHref(s), s).toBeNull();
    }
  });

  it("isScalar is false only for objects and arrays", () => {
    expect([null, 1, "s", true, undefined].every(isScalar)).toBe(true);
    expect([{}, [], [1]].some(isScalar)).toBe(false);
  });
});

describe("relationship group text", () => {
  it("states the blocker counts the API computed", () => {
    expect(blockedByMeta({ active: 2, resolved: 1 })).toBe("2 open · 1 resolved");
    expect(blockedByMeta({ active: 0, resolved: 3 })).toBe("3 resolved");
    expect(blockedByMeta({ active: 1, resolved: 0 })).toBe("1 open");
    expect(blockedByMeta({ active: 0, resolved: 0 })).toBe("");
    expect(blocksMeta(2)).toBe("2 still blocked");
    expect(blocksMeta(0)).toBe("");
  });

  it("counts every row of the Relationships section", () => {
    const ref = { id: "x" };
    expect(relationshipCount({ children: [], blocked_by: [], blocks: [], related: [] })).toBe(0);
    expect(
      relationshipCount({
        parent: ref,
        children: [ref, ref],
        blocked_by: [{ id: "b", active: true }],
        blocks: [{ id: "c", active: false }],
        related: [{ id: "r", type: "related", direction: "out" }],
      }),
    ).toBe(6);
  });

  it("tags a related row by its type and direction", () => {
    expect(relatedTag({ type: "discovered-from", direction: "out" })).toBe("discovered from");
    expect(relatedTag({ type: "discovered-from", direction: "in" })).toBe("discovered from this");
    expect(relatedTag({ type: "related", direction: "out" })).toBe("related");
    expect(relatedTag({ type: "relates-to", direction: "in" })).toBe("relates to");
  });
});
