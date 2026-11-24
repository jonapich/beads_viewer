// Pure rules of the full-page issue detail view (views/IssueDetail.svelte). Everything
// the page decides from the API's IssueDetail lives here so it is unit-tested.
import type { IssueDetail, Link, MetaResponse } from "../api/types";
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
