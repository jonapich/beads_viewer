// Pure rules of the full-page issue detail view (views/IssueDetail.svelte). Everything
// the page decides from the API's IssueDetail lives here so it is unit-tested.
import type { BlockerCounts, IssueComment, IssueDetail, Link, ListItem, MetaResponse, RelatedRef, SignoffItem } from "../api/types";
import { displayStatus, type DisplayStatus } from "./model";
import { READ_ONLY_TITLE } from "./writes";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

const two = (n: number): string => (n < 10 ? `0${n}` : String(n));

function parse(iso: string): Date | null {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** "23 Nov 2026 02:32 UTC"; "" for a timestamp that does not parse. */
export function absTime(iso: string): string {
  const d = parse(iso);
  if (!d) return "";
  return `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]} ${d.getUTCFullYear()} ${two(d.getUTCHours())}:${two(d.getUTCMinutes())} UTC`;
}

/** "23 Nov"; "" for a timestamp that does not parse. */
export function shortDate(iso: string): string {
  const d = parse(iso);
  return d ? `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]}` : "";
}

/** "just now", "5 m ago", "3 h ago", "2 d ago"; a future timestamp reads as just now, an unparsable one as "". */
export function relTime(iso: string, now: number): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "";
  const s = Math.max(0, (now - t) / 1000);
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} h ago`;
  return `${Math.floor(h / 24)} d ago`;
}

/** A design longer than this many lines starts collapsed. */
export const DESIGN_OPEN_MAX_LINES = 40;

export function designOpenByDefault(design: string | undefined): boolean {
  return (design ?? "").split("\n").length <= DESIGN_OPEN_MAX_LINES;
}

/** The links bd stored on the issue itself (metadata link.<kind>); those found in comment text are left out. */
export function typedLinks(d: Pick<IssueDetail, "links">): Link[] {
  return d.links.filter((l) => l.source === "typed");
}

/** The host of a link for display; the URL itself when it does not parse. */
export function linkHost(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

/** The workflow stage (metadata.stage), when it is a string. */
export function stageOf(d: Pick<IssueDetail, "metadata">): string {
  const s = d.metadata?.["stage"];
  return typeof s === "string" ? s : "";
}

/** Status as the header pill draws it: bd's glyph set, plus the two statuses the pill palette names. */
export type PillStatus = DisplayStatus | "review" | "pinned";

export const PILL_NAME: Record<PillStatus, string> = {
  open: "Open",
  in_progress: "In progress",
  blocked: "Blocked",
  closed: "Closed",
  deferred: "Deferred",
  review: "Review",
  pinned: "Pinned",
};

export const PILL_GLYPH: Record<PillStatus, string> = {
  open: "○",
  in_progress: "◐",
  blocked: "●",
  closed: "✓",
  deferred: "❄",
  review: "◑",
  pinned: "",
};

export function pillStatus(d: Pick<IssueDetail, "status" | "blocked" | "defer_until">, now: number): PillStatus {
  if (d.status === "review" || d.status === "pinned") return d.status;
  return displayStatus(d, now);
}

/** The sign-off pill in the rail: the mock's label and glyph per state; an unknown state shows as is. */
export function signoffPill(state: string): { label: string; glyph: string } {
  switch (state) {
    case "pending":
      return { label: "Pending", glyph: "●" };
    case "approved":
      return { label: "Approved", glyph: "✓" };
    case "disputed":
      return { label: "Disputed", glyph: "!" };
    case "waived":
      return { label: "Waived", glyph: "∅" };
    default:
      return { label: state, glyph: "" };
  }
}

/**
 * The read-only notice for the top of the page: the issue's own reason when it has one,
 * else the generic one when the tracker takes no writes. Null while /meta has not
 * arrived (so the notice does not flash on load) or when writes are possible.
 */
export function readOnlyNotice(
  meta: Pick<MetaResponse, "capabilities"> | null,
  detail: Pick<IssueDetail, "read_only_reason"> | null,
): string | null {
  if (detail?.read_only_reason) return detail.read_only_reason;
  if (meta && !meta.capabilities?.writes) return READ_ONLY_TITLE;
  return null;
}

/** Kinds of comment that start expanded; every other kind starts as a one-line summary. */
const OPEN_KINDS: ReadonlySet<string> = new Set(["signoff", "user", "answer"]);

/** Whether a comment starts expanded: a plain comment (no kind) always is. */
export function defaultCommentOpen(kind: string | undefined): boolean {
  return !kind || OPEN_KINDS.has(kind);
}

function firstSentence(s: string): string {
  const i = s.indexOf(". ");
  return i > 0 ? s.slice(0, i + 1) : s;
}

/** A string, or the first string of a list of them; "" for anything else. */
function stringOf(v: unknown): string {
  if (typeof v === "string") return v;
  if (Array.isArray(v)) {
    const first = v.find((x): x is string => typeof x === "string");
    return first ?? "";
  }
  return "";
}

/**
 * The one line shown for a collapsed kinded comment. Not the API's `summary`: that starts
 * with the kind, which the badge beside it already shows, and a sign-off's JSON comes out
 * as "{calls_made, ...}". A JSON body gives what_changed, else summary, else disputed,
 * else its first string value, cut at the first sentence; a text body gives its first
 * non-blank line.
 */
export function commentSummary(c: Pick<IssueComment, "data" | "text">): string {
  const j = c.data;
  if (j) {
    let s = stringOf(j["what_changed"]) || stringOf(j["summary"]) || stringOf(j["disputed"]);
    if (!s) {
      for (const v of Object.values(j)) {
        if (typeof v === "string") {
          s = v;
          break;
        }
      }
    }
    return firstSentence(s.trim());
  }
  return (c.text ?? "").split("\n").find((l) => l.trim() !== "")?.trim() ?? "";
}

/** The kinds present in the comments with how many of each, in order of first appearance (the filter chips). */
export function kindCounts(comments: ReadonlyArray<Pick<IssueComment, "kind">>): Array<{ kind: string; n: number }> {
  const out: Array<{ kind: string; n: number }> = [];
  for (const c of comments) {
    if (!c.kind) continue;
    const hit = out.find((k) => k.kind === c.kind);
    if (hit) hit.n++;
    else out.push({ kind: c.kind, n: 1 });
  }
  return out;
}

export type HistoryKind = "created" | "started" | "stage" | "signoff" | "closed";

export interface HistoryEvent {
  at: string;
  label: string;
  kind: HistoryKind;
}

/**
 * The timeline, oldest first: created, started, every stage change (metadata
 * stage_at.<stage>), the sign-off request (signoff_at; "waived" when metadata.signoff says
 * so) and closed. Timestamps that do not parse are left out.
 */
export function historyEvents(d: Pick<IssueDetail, "created_at" | "started_at" | "closed_at" | "metadata">): HistoryEvent[] {
  const ev: HistoryEvent[] = [{ at: d.created_at, label: "Created", kind: "created" }];
  if (d.started_at) ev.push({ at: d.started_at, label: "Started", kind: "started" });
  const meta = d.metadata ?? {};
  for (const [k, v] of Object.entries(meta)) {
    if (k.startsWith("stage_at.") && typeof v === "string") ev.push({ at: v, label: `Stage → ${k.slice("stage_at.".length)}`, kind: "stage" });
  }
  const so = meta["signoff_at"];
  if (typeof so === "string") ev.push({ at: so, label: `Sign-off ${meta["signoff"] === "waived" ? "waived" : "requested"}`, kind: "signoff" });
  if (d.closed_at) ev.push({ at: d.closed_at, label: "Closed", kind: "closed" });
  return ev.filter((e) => Number.isFinite(Date.parse(e.at))).sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
}

/** "calls_made" -> "Calls made". */
export function humanKey(k: string): string {
  const s = k.replace(/_/g, " ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** "owner/repo abc1234" -> the commit on GitHub; null for any other text. */
export function commitHref(s: string): string | null {
  const m = /^([\w.-]+\/[\w.-]+)\s+([0-9a-f]{7,40})$/.exec(s);
  return m ? `https://github.com/${m[1]}/commit/${m[2]}` : null;
}

/** A JSON value that is not an object or array (null counts). */
export function isScalar(v: unknown): boolean {
  return v === null || typeof v !== "object";
}

/** "2 open · 1 resolved" for the Blocked by group; "" when there is nothing to count. */
export function blockedByMeta(c: BlockerCounts): string {
  return [c.active > 0 ? `${c.active} open` : "", c.resolved > 0 ? `${c.resolved} resolved` : ""].filter(Boolean).join(" · ");
}

/** The Blocks group's meta: how many dependents the issue still holds up; "" for none. */
export function blocksMeta(activeDependents: number): string {
  return activeDependents > 0 ? `${activeDependents} still blocked` : "";
}

/**
 * The tag on a Related row. The row is the other issue: direction "out" means this issue
 * declared the link, so for discovered-from the row is where this one was discovered from.
 */
export function relatedTag(r: Pick<RelatedRef, "type" | "direction">): string {
  if (r.type === "discovered-from") return r.direction === "in" ? "discovered from this" : "discovered from";
  return r.type.replace(/-/g, " ");
}

/** How many rows the Relationships section lists (parent, children, blockers, dependents, related). */
export function relationshipCount(
  d: Pick<IssueDetail, "parent" | "children" | "blocked_by" | "blocks" | "related">,
): number {
  return (d.parent ? 1 : 0) + d.children.length + d.blocked_by.length + d.blocks.length + d.related.length;
}

/**
 * The slice of the issue the shared write gates (actionState, awaitsSignoff, canAnswer) and the
 * priority popover read, the same shape a list row has, so the drawer and the page cannot drift.
 */
export function actionTarget(d: IssueDetail): Pick<ListItem, "id" | "priority" | "status" | "labels" | "signoff"> {
  return { id: d.id, priority: d.priority, status: d.status, labels: d.labels, signoff: d.signoff?.state };
}

/** Writes need a writable tracker and an issue whose own route is writable (the detail says why not). */
export function isReadOnly(writes: boolean, d: Pick<IssueDetail, "read_only_reason"> | null): boolean {
  return !writes || !!d?.read_only_reason;
}

/** A "removed or renamed" part that just says nothing was: drawn faint. */
export function isNothing(lines: readonly string[] | undefined): boolean {
  return lines?.length === 1 && /^nothing\.?$/i.test((lines[0] ?? "").trim());
}

/** The sign-off comment filled at least one of the card's parts (else its raw text is shown). */
export function signoffHasParts(so: SignoffItem): boolean {
  return (
    so.calls.length > 0 ||
    [so.calls_made, so.what_changed, so.removed_or_renamed, so.where_to_see, so.captures, so.commits].some((p) => (p?.length ?? 0) > 0)
  );
}

/**
 * Whether the "done" note must keep standing in for a sign-off or answer card: the write was
 * accepted ("saved, but reloading the tracker failed" included) and the detail still has the
 * `updated_at` it had when the write started, so it is the same stale snapshot and the action
 * would be offered again. A newer updated_at (the refetch landed, or someone changed it since)
 * lets the card back.
 */
export function noteHolds(resolvedAt: string | undefined, updatedAt: string): boolean {
  return resolvedAt !== undefined && resolvedAt === updatedAt;
}
