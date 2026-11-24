import { describe, expect, it } from "vitest";
import type { InboxResponse, PipelineResponse, SignoffResponse, SliceMetricsResponse } from "../api/types";
import {
  answerSummary,
  badgeCount,
  badgeTitle,
  defaultTab,
  firstUnhandled,
  flatRows,
  holderText,
  inboxGroups,
  isListTab,
  isLongText,
  judgeLink,
  metricsCells,
  nextUnhandled,
  parseTab,
  pipelineFlag,
  pipelineSummary,
  readyCount,
  reconcile,
  selectionFor,
  signoffGroups,
  stageCounts,
  summaryParts,
  timeLeft,
  unhandledCount,
  type DeskGroup,
  type DeskRow,
  type Outcomes,
} from "./desk";
import inbox from "../api/__fixtures__/inbox.json";
import signoff from "../api/__fixtures__/signoff.json";
import pipeline from "../api/__fixtures__/pipeline.json";
import sliceMetrics from "../api/__fixtures__/slice-metrics.json";

const row = (id: string, extra: Partial<DeskRow> = {}): DeskRow => ({
  id,
  title: id,
  kind: "decision",
  priority: 2,
  ready: false,
  links: 0,
  noNote: false,
  ...extra,
});
const group = (epic: string, ...ids: string[]): DeskGroup => ({ epic, rows: ids.map((i) => row(i)) });
const ids = (gs: readonly DeskGroup[]) => gs.map((g) => `${g.epic}:${g.rows.map((r) => r.id).join(",")}`);
const done = (...list: string[]): Outcomes => new Map(list.map((i) => [i, { kind: "answered" as const }]));

describe("tabs", () => {
  it("parses only known tabs", () => {
    expect(parseTab("inbox")).toBe("inbox");
    expect(parseTab("signoff")).toBe("signoff");
    expect(parseTab("pipeline")).toBe("pipeline");
    expect(parseTab("metrics")).toBe("metrics");
    expect(parseTab("labels")).toBeNull();
    expect(parseTab("")).toBeNull();
    expect(parseTab(null)).toBeNull();
  });

  it("opens on the inbox, or on the sign-offs when the inbox is empty and some are pending", () => {
    expect(defaultTab(3, 2)).toBe("inbox");
    expect(defaultTab(0, 2)).toBe("signoff");
    expect(defaultTab(0, 0)).toBe("inbox");
    expect(defaultTab(0, null)).toBe("inbox");
    expect(defaultTab(null, 5)).toBe("inbox"); // not loaded yet: the inbox stands in
    expect(defaultTab(1, 0)).toBe("inbox");
  });
});

describe("rows from the API", () => {
  it("maps the inbox fixture: kinds, ready flag, waiting-on and links", () => {
    const groups = inboxGroups(inbox as InboxResponse);
    expect(groups.map((g) => g.epic)).toEqual(["fx-epic", ""]);
    expect(groups[0]?.title).toBe("Checkout epic");
    const [style] = groups[0]?.rows ?? [];
    expect(style).toMatchObject({ id: "fx-style", kind: "design", ready: true, links: 1, priority: 1 });
    const none = groups[1]?.rows ?? [];
    expect(none[0]).toMatchObject({ id: "fx-decide", kind: "decision", ready: false, links: 0 });
    expect(none[1]).toMatchObject({ id: "fx-logo", kind: "design", ready: false });
    expect(none[1]?.waitingOn).toBe("a poet direction, then an HTML mock");
    expect(groups[1]?.title).toBeUndefined();
  });

  it("maps the sign-off fixture", () => {
    const groups = signoffGroups(signoff as SignoffResponse);
    const rows = flatRows(groups);
    expect(rows).toHaveLength((signoff as SignoffResponse).total);
    expect(rows[0]).toMatchObject({ id: "fx-done", kind: "signoff", noNote: false });
    expect(rows[0]?.at).toBeTruthy();
  });

  it("flags a sign-off with no sign-off comment", () => {
    const resp: SignoffResponse = {
      total: 1,
      groups: [{ epic: "", items: [{ id: "x", title: "X", priority: 2, state: "pending", has_signoff_comment: false, calls: [] }] }],
    };
    expect(flatRows(signoffGroups(resp))[0]?.noNote).toBe(true);
  });
});

describe("reconcile", () => {
  const prev = [group("e1", "a", "b", "c"), group("", "d", "e")];

  it("is the incoming list when nothing is kept", () => {
    const incoming = [group("e1", "a", "c")];
    expect(ids(reconcile(prev, incoming, new Set()))).toEqual(["e1:a,c"]);
  });

  it("keeps a handled row in its place", () => {
    const incoming = [group("e1", "a", "c"), group("", "d", "e")];
    expect(ids(reconcile(prev, incoming, new Set(["b"])))).toEqual(["e1:a,b,c", ":d,e"]);
  });

  it("keeps a handled first row first and a handled last row last", () => {
    const incoming = [group("e1", "b", "c"), group("", "d")];
    expect(ids(reconcile(prev, incoming, new Set(["a", "e"])))).toEqual(["e1:a,b,c", ":d,e"]);
  });

  it("keeps consecutive handled rows in order", () => {
    const incoming = [group("e1", "a"), group("", "d", "e")];
    expect(ids(reconcile(prev, incoming, new Set(["b", "c"])))).toEqual(["e1:a,b,c", ":d,e"]);
  });

  it("brings back a group that emptied, before the first later group that is listed", () => {
    const three = [group("e1", "a"), group("e2", "b"), group("", "c")];
    const incoming = [group("", "c")];
    expect(ids(reconcile(three, incoming, new Set(["a"])))).toEqual(["e1:a", ":c"]);
    expect(ids(reconcile(three, [group("e1", "a")], new Set(["b"])))).toEqual(["e1:a", "e2:b"]);
  });

  it("drops a row that left without the user's write", () => {
    const incoming = [group("e1", "a", "c"), group("", "d", "e")];
    expect(ids(reconcile(prev, incoming, new Set(["x"])))).toEqual(["e1:a,c", ":d,e"]);
  });

  it("does not duplicate a kept row that is still listed", () => {
    const incoming = [group("e1", "a", "b", "c"), group("", "d", "e")];
    expect(ids(reconcile(prev, incoming, new Set(["b"])))).toEqual(["e1:a,b,c", ":d,e"]);
  });

  it("is stable when applied again to its own result", () => {
    const incoming = [group("e1", "a", "c"), group("", "d", "e")];
    const once = reconcile(prev, incoming, new Set(["b"]));
    expect(ids(reconcile(once, incoming, new Set(["b"])))).toEqual(ids(once));
  });

  it("does not change its inputs", () => {
    const incoming = [group("e1", "a", "c")];
    reconcile(prev, incoming, new Set(["b"]));
    expect(ids(incoming)).toEqual(["e1:a,c"]);
    expect(ids(prev)).toEqual(["e1:a,b,c", ":d,e"]);
  });
});

describe("selection and auto-advance", () => {
  const groups = [group("e1", "a", "b", "c"), group("", "d", "e")];

  it("counts and finds unhandled rows", () => {
    expect(unhandledCount(groups, done("b", "d"))).toBe(3);
    expect(firstUnhandled(groups, done("a", "b"))).toBe("c");
    expect(firstUnhandled(groups, done("a", "b", "c", "d", "e"))).toBeNull();
  });

  it("moves to the next row below that is not handled", () => {
    expect(nextUnhandled(groups, "a", done("a"))).toBe("b");
    expect(nextUnhandled(groups, "a", done("a", "b"))).toBe("c");
    expect(nextUnhandled(groups, "c", done("c"))).toBe("d"); // across groups
  });

  it("falls back to the nearest row above when none is left below", () => {
    expect(nextUnhandled(groups, "e", done("e"))).toBe("d");
    expect(nextUnhandled(groups, "e", done("e", "d"))).toBe("c");
  });

  it("returns null when everything is handled", () => {
    expect(nextUnhandled(groups, "b", done("a", "b", "c", "d", "e"))).toBeNull();
    expect(nextUnhandled([group("e1", "a")], "a", done("a"))).toBeNull();
  });

  it("selects the named row, the first unhandled, or flags a vanished one", () => {
    expect(selectionFor(groups, done(), "c")).toEqual({ id: "c", vanished: false });
    expect(selectionFor(groups, done("a"), null)).toEqual({ id: "b", vanished: false });
    expect(selectionFor(groups, done("a", "b", "c", "d", "e"), null)).toEqual({ id: "a", vanished: false });
    expect(selectionFor(groups, done(), "gone")).toEqual({ id: "gone", vanished: true });
    expect(selectionFor([], done(), null)).toEqual({ id: null, vanished: false });
  });

  it("counts ready rows that are not handled", () => {
    const rows = [row("a", { ready: true }), row("b", { ready: true }), row("c")];
    expect(readyCount(rows, done("b"))).toBe(1);
  });
});

describe("pane helpers", () => {
  it("picks the first mock or artifact link to judge", () => {
    const l = (kind: string, url: string) => ({ kind, url, source: "typed" });
    expect(judgeLink([l("doc", "u1"), l("artifact", "u2"), l("mock", "u3")])?.url).toBe("u2");
    expect(judgeLink([l("commit", "u1")])).toBeUndefined();
    expect(judgeLink([])).toBeUndefined();
  });

  it("summarises an answer", () => {
    const opts = [
      { key: "a", text: "Stripe" },
      { key: "b", text: "Adyen", recommended: true },
    ];
    expect(answerSummary(opts, "a", "")).toBe("(a) Stripe");
    expect(answerSummary(opts, "", "")).toBe("(b) Adyen"); // the recommended one stands in
    expect(answerSummary(opts, "a", " cheaper ")).toBe("(a) Stripe — cheaper");
    expect(answerSummary([], "", " free text ")).toBe("free text");
  });

  it("collapses a long description", () => {
    expect(isLongText("one\ntwo")).toBe(false);
    expect(isLongText("1\n2\n3\n4\n5\n6")).toBe(false);
    expect(isLongText("1\n2\n3\n4\n5\n6\n7")).toBe(true);
    expect(isLongText("x".repeat(601))).toBe(true);
  });

  it("builds the summary line", () => {
    expect(summaryParts({ inbox: 3, ready: 1, signoff: 2 }).map((p) => `${p.n} ${p.text} ${p.tab}`)).toEqual([
      "3 questions await you inbox",
      "1 ready to judge inbox",
      "2 sign-offs pending signoff",
    ]);
    expect(summaryParts({ inbox: 1, ready: 0, signoff: 1 }).map((p) => p.text)).toEqual(["question awaits you", "sign-off pending"]);
    const withPipe = summaryParts({ inbox: 0, ready: 0, signoff: 0, pipeline: { total: 4, stale: 1 } });
    expect(withPipe.at(-1)).toEqual({ n: 4, text: "in progress (1 stale)", tab: "pipeline" });
    expect(summaryParts({ inbox: 0, ready: 0, signoff: 0, pipeline: { total: 2, stale: 0 } }).at(-1)?.text).toBe("in progress");
    expect(summaryParts({ inbox: 0, ready: 0, signoff: 0, pipeline: null }).some((p) => p.tab === "pipeline")).toBe(false);
  });
});

describe("nav badge", () => {
  it("adds the questions and the sign-offs, 0 until meta arrived", () => {
    expect(badgeCount({ awaiting: 3, signoff_pending: 2 })).toBe(5);
    expect(badgeCount({ awaiting: 0, signoff_pending: 0 })).toBe(0);
    expect(badgeCount(null)).toBe(0);
    expect(badgeCount(undefined)).toBe(0);
  });

  it("titles it with both counts", () => {
    expect(badgeTitle({ awaiting: 3, signoff_pending: 2 })).toBe("3 questions · 2 sign-offs");
    expect(badgeTitle({ awaiting: 1, signoff_pending: 1 })).toBe("1 question · 1 sign-off");
  });
});

describe("tab kinds", () => {
  it("separates the tabs you act on from the tables you look at", () => {
    expect(isListTab("inbox")).toBe(true);
    expect(isListTab("signoff")).toBe(true);
    expect(isListTab("pipeline")).toBe(false);
    expect(isListTab("metrics")).toBe(false);
  });
});

describe("pipeline", () => {
  const NOW = Date.parse("2026-03-10T12:00:00Z");

  it("orders the stage counts: known stages in pipeline order, others by name, unstaged last", () => {
    expect(stageCounts({ unstaged: 1, review: 2, code: 1, zeta: 1, alpha: 3, scout: 0 })).toEqual([
      { stage: "code", n: 1 },
      { stage: "review", n: 2 },
      { stage: "alpha", n: 3 },
      { stage: "zeta", n: 1 },
      { stage: "unstaged", n: 1 },
    ]);
  });

  it("summarises the fixture: total, stale, then the stages", () => {
    const s = pipelineSummary(pipeline as PipelineResponse);
    expect(s.counts).toEqual(["3 in progress", "1 stale", "code 1", "review 1", "unstaged 1"]);
    expect(s.hint).toBe("Stale: no activity for 2h, or the lease expired.");
    expect(pipelineSummary({ total: 0, stale: 0, by_stage: {}, stale_after: "2h" }).counts).toEqual(["0 in progress"]);
  });

  it("formats the time left on a lease", () => {
    expect(timeLeft("2026-03-10T12:12:00Z", NOW)).toBe("12m");
    expect(timeLeft("2026-03-10T12:00:20Z", NOW)).toBe("1m");
    expect(timeLeft("2026-03-10T13:05:00Z", NOW)).toBe("1h05m");
    expect(timeLeft("2026-03-10T14:00:00Z", NOW)).toBe("2h");
    expect(timeLeft("2026-03-12T14:00:00Z", NOW)).toBe("2d");
    expect(timeLeft("2026-03-10T12:00:00Z", NOW)).toBe("");
    expect(timeLeft("2026-03-09T12:00:00Z", NOW)).toBe("");
    expect(timeLeft("not a date", NOW)).toBe("");
  });

  it("flags a stale row with its reason and a live lease with the time left", () => {
    const rows = (pipeline as PipelineResponse).rows;
    expect(pipelineFlag(rows[0]!, NOW)).toEqual({ tone: "stale", text: "⚠ no activity for 8d2h" });
    expect(pipelineFlag(rows[1]!, NOW)).toEqual({ tone: "", text: "" }); // staged, no lease
    expect(pipelineFlag(rows[2]!, NOW)).toEqual({ tone: "lease", text: "lease 12m left" });
    expect(pipelineFlag({ stale: true, lease_expires_at: "2026-03-10T12:12:00Z" }, NOW).tone).toBe("stale"); // stale wins
  });

  it("joins the holder and the claim session", () => {
    expect(holderText({ holder: "ada", session: "s-1" })).toBe("ada · s-1");
    expect(holderText({ holder: "ada" })).toBe("ada");
    expect(holderText({ session: "s-1" })).toBe("s-1");
    expect(holderText({})).toBe("");
  });
});

describe("metrics", () => {
  it("formats a row in the TUI's columns", () => {
    const r = sliceMetrics as SliceMetricsResponse;
    expect(metricsCells(r.epics[0]!)).toEqual({
      epic: "fx-epic",
      title: "Checkout epic",
      slices: "2",
      outcome: "1/1",
      revPerSlice: "2.0",
      firstPass: "50%",
      tests: "6",
      red: "1",
      minPerSlice: "60",
      tokens: "32/4/1 (1?)", // 500 rounds up; one slice logged no tokens
      esc: "1",
    });
  });

  it("formats the total row and a no-epic row, and survives zero slices", () => {
    const r = sliceMetrics as SliceMetricsResponse;
    expect(metricsCells(r.total)).toMatchObject({ epic: "all", slices: "3", outcome: "2/1", firstPass: "33%", tokens: "32/4/1 (2?)" });
    const none = { ...r.total, epic: "", epic_title: undefined, slices: 0, review_rounds: 0, first_pass_approvals: 0, slice_wall_minutes: 0 };
    expect(metricsCells(none)).toMatchObject({ epic: "(no epic)", title: "", revPerSlice: "0.0", firstPass: "0%", minPerSlice: "0" });
  });
});