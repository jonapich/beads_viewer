// Pure rules of the agent desk (views/Desk.svelte): the tabs, the rows of the lists, how a
// list keeps handled rows in place across a reload, and where the selection moves after a
// write. Everything the view decides from the API data lives here so it is unit-tested.
import type {
  DecisionOption,
  InboxResponse,
  Link,
  MetaCounts,
  PipelineResponse,
  PipelineRow,
  SignoffResponse,
  SliceMetrics,
} from "../api/types";

/** The desk's tabs, in order: the two you act on, then the two you only look at. */
export const DESK_TABS = ["inbox", "signoff", "pipeline", "metrics"] as const;
export type DeskTab = (typeof DESK_TABS)[number];

export const TAB_LABEL: Record<DeskTab, string> = { inbox: "Inbox", signoff: "Sign-off", pipeline: "Pipeline", metrics: "Metrics" };

/** The tabs that list rows to act on (a selection, a pane); the others are read-only tables. */
export function isListTab(tab: DeskTab): tab is "inbox" | "signoff" {
  return tab === "inbox" || tab === "signoff";
}

/** The tab named by ?tab=, or null when it is absent or unknown. */
export function parseTab(raw: string | null | undefined): DeskTab | null {
  return (DESK_TABS as readonly string[]).includes(raw ?? "") ? (raw as DeskTab) : null;
}

/**
 * The tab the desk opens on when the URL names none: the inbox, because an unanswered question
 * blocks an agent while a sign-off sits on a bead that is already closed; the sign-offs when the
 * inbox is empty and some are pending. A total that has not loaded yet (null) counts as unknown,
 * and the inbox stands in.
 */
export function defaultTab(inbox: number | null, signoff: number | null): DeskTab {
  return inbox === 0 && (signoff ?? 0) > 0 ? "signoff" : "inbox";
}

export type RowKind = "decision" | "design" | "signoff";

/** One row of a desk list, from an inbox item or a pending sign-off. */
export interface DeskRow {
  id: string;
  title: string;
  kind: RowKind;
  priority: number;
  /** A design bead with a mock or artifact to judge. */
  ready: boolean;
  /** For a design bead not ready to judge: the artifact it waits for. */
  waitingOn?: string;
  /** Links on the bead (inbox rows). */
  links: number;
  /** When it was last updated (inbox) or closed (sign-off); an RFC 3339 instant. */
  at?: string;
  /** A sign-off row whose bead carries no `bd-kind: signoff` comment. */
  noNote: boolean;
}

/** The rows of one epic, in the server's order. */
export interface DeskGroup {
  /** Epic id; "" for beads with no epic. */
  epic: string;
  title?: string;
  rows: DeskRow[];
}

export function inboxGroups(resp: InboxResponse): DeskGroup[] {
  return resp.groups.map((g) => ({
    epic: g.epic,
    ...(g.epic_title ? { title: g.epic_title } : {}),
    rows: g.items.map((it) => ({
      id: it.id,
      title: it.title,
      kind: it.awaiting_labels.includes("needs-decision") ? "decision" : "design",
      priority: it.priority,
      ready: it.ready_to_judge,
      ...(it.waiting_on ? { waitingOn: it.waiting_on } : {}),
      links: it.links?.length ?? 0,
      at: it.updated_at,
      noNote: false,
    })),
  }));
}

export function signoffGroups(resp: SignoffResponse): DeskGroup[] {
  return resp.groups.map((g) => ({
    epic: g.epic,
    ...(g.epic_title ? { title: g.epic_title } : {}),
    rows: g.items.map((it) => ({
      id: it.id,
      title: it.title,
      kind: "signoff" as const,
      priority: it.priority,
      ready: false,
      links: it.links?.length ?? 0,
      ...(it.closed_at ? { at: it.closed_at } : {}),
      noNote: !it.has_signoff_comment,
    })),
  }));
}

/** How long a handled row stays in the list, muted, before it leaves (whoever handled it). */
export const LINGER_MS = 2000;

/** What happened to a row this visit: the user did it here, or it left the list without a write from this desk. */
export type OutcomeKind = "answered" | "approved" | "disputed" | "elsewhere";
export interface Outcome {
  kind: OutcomeKind;
  /** For an answer: what was posted, as "(b) keep the cart". */
  note?: string;
}
export type Outcomes = ReadonlyMap<string, Outcome>;

export const OUTCOME_LABEL: Record<OutcomeKind, string> = {
  answered: "Answered",
  approved: "Approved",
  disputed: "Disputed",
  elsewhere: "Handled elsewhere",
};

/** Every row in display order. */
export function flatRows(groups: readonly DeskGroup[]): DeskRow[] {
  return groups.flatMap((g) => g.rows);
}

/**
 * The list to show after a reload. It is `incoming`, in the server's order, plus every row that
 * was on show and is in `pinned` but has left the server's list: a bead that was just handled
 * stays in its place (muted by the view) while it lingers. Pin a row when the write is sent, not
 * when it is answered: the server reloads and announces the change before it answers the request,
 * so a refetch can arrive first, and an unpinned row would be dropped for good. Such a row goes
 * back after the nearest row before it that is still listed, in its epic's group; the group goes
 * back before the first later group that is listed, else at the end. A row that left and is not
 * pinned drops out (see departedIds).
 */
export function reconcile(prev: readonly DeskGroup[], incoming: readonly DeskGroup[], pinned: ReadonlySet<string>): DeskGroup[] {
  const out: DeskGroup[] = incoming.map((g) => ({ ...g, rows: [...g.rows] }));
  if (pinned.size === 0) return out;
  const present = new Set(out.flatMap((g) => g.rows.map((r) => r.id)));
  prev.forEach((pg, gi) => {
    pg.rows.forEach((row, ri) => {
      if (!pinned.has(row.id) || present.has(row.id)) return;
      let group = out.find((g) => g.epic === pg.epic);
      if (!group) {
        group = { epic: pg.epic, ...(pg.title ? { title: pg.title } : {}), rows: [] };
        const later = new Set(prev.slice(gi + 1).map((g) => g.epic));
        const at = out.findIndex((g) => later.has(g.epic));
        out.splice(at < 0 ? out.length : at, 0, group);
      }
      let at = 0;
      for (let k = ri - 1; k >= 0; k--) {
        const idx = group.rows.findIndex((r) => r.id === pg.rows[k]?.id);
        if (idx >= 0) {
          at = idx + 1;
          break;
        }
      }
      group.rows.splice(at, 0, row);
      present.add(row.id);
    });
  });
  return out;
}

/**
 * The rows that were on show and are not in the new list although this desk did not write them
 * (not pinned): answered or signed off in the drawer, on the issue page or with bd. The view marks
 * them handled elsewhere and lets them linger like any handled row. Empty on the first load.
 */
export function departedIds(prev: readonly DeskGroup[], incoming: readonly DeskGroup[], pinned: ReadonlySet<string>): string[] {
  if (prev.length === 0) return [];
  const listed = new Set(flatRows(incoming).map((r) => r.id));
  return flatRows(prev)
    .map((r) => r.id)
    .filter((id) => !listed.has(id) && !pinned.has(id));
}

/** The list without row `id`; a group left empty goes with it. */
export function removeRow(groups: readonly DeskGroup[], id: string): DeskGroup[] {
  return groups.map((g) => ({ ...g, rows: g.rows.filter((r) => r.id !== id) })).filter((g) => g.rows.length > 0);
}
/** Rows the user has not handled this visit. */
export function unhandledCount(groups: readonly DeskGroup[], outcomes: Outcomes): number {
  return flatRows(groups).filter((r) => !outcomes.has(r.id)).length;
}

/** Unhandled rows ready to judge, for a group header or the summary line. */
export function readyCount(rows: readonly DeskRow[], outcomes: Outcomes): number {
  return rows.filter((r) => r.ready && !outcomes.has(r.id)).length;
}

/** The first row not handled yet, or null. */
export function firstUnhandled(groups: readonly DeskGroup[], outcomes: Outcomes): string | null {
  return flatRows(groups).find((r) => !outcomes.has(r.id))?.id ?? null;
}

/**
 * Where the selection goes after `current` was handled: the next row below it that is not
 * handled, else the nearest one above it, else null (nothing is left to do).
 */
export function nextUnhandled(groups: readonly DeskGroup[], current: string, outcomes: Outcomes): string | null {
  const rows = flatRows(groups);
  const at = rows.findIndex((r) => r.id === current);
  const after = rows.slice(at + 1).find((r) => !outcomes.has(r.id));
  if (after) return after.id;
  const before = rows
    .slice(0, Math.max(at, 0))
    .reverse()
    .find((r) => !outcomes.has(r.id));
  return before?.id ?? null;
}

export interface Selection {
  /** The id the pane shows; null when the list is empty. */
  id: string | null;
  /** The URL names an id that is not in the list (it was answered elsewhere, or never was). */
  vanished: boolean;
}

/**
 * The selected row for ?id=: the one named when listed; one that is not listed stays named and
 * is flagged as vanished (the pane says so and offers the next item); with none named, the
 * first row not handled, else the first row.
 */
export function selectionFor(groups: readonly DeskGroup[], outcomes: Outcomes, wanted: string | null): Selection {
  const rows = flatRows(groups);
  if (wanted) return { id: wanted, vanished: !rows.some((r) => r.id === wanted) };
  return { id: firstUnhandled(groups, outcomes) ?? rows[0]?.id ?? null, vanished: false };
}

/** The link a design bead is judged against: the first mock or artifact. */
export function judgeLink(links: readonly Link[]): Link | undefined {
  return links.find((l) => l.kind === "mock" || l.kind === "artifact");
}

/** "(b) keep the cart", "(b) keep the cart — note" or the free text: what an answer posted. */
export function answerSummary(options: readonly DecisionOption[], picked: string, text: string): string {
  const opt = options.find((o) => o.key === picked) ?? options.find((o) => o.recommended);
  const note = text.trim();
  if (opt && note !== "") return `(${opt.key}) ${opt.text} — ${note}`;
  if (opt) return `(${opt.key}) ${opt.text}`;
  return note;
}

/** A description of more lines or characters than this starts collapsed in the pane. */
const CLAMP_LINES = 6;
const CLAMP_CHARS = 600;

export function isLongText(text: string): boolean {
  return text.split("\n").length > CLAMP_LINES || text.length > CLAMP_CHARS;
}

export interface SummaryPart {
  n: number;
  text: string;
  tab: DeskTab;
}

/** The parts of the page's summary line; each links to its tab. Parts with nothing to say are left out. */
export function summaryParts(c: {
  inbox: number;
  ready: number;
  signoff: number;
  /** The claimed work, once the pipeline has loaded. */
  pipeline?: { total: number; stale: number } | null;
}): SummaryPart[] {
  const parts: SummaryPart[] = [];
  parts.push({ n: c.inbox, text: c.inbox === 1 ? "question awaits you" : "questions await you", tab: "inbox" });
  if (c.ready > 0) parts.push({ n: c.ready, text: "ready to judge", tab: "inbox" });
  parts.push({ n: c.signoff, text: c.signoff === 1 ? "sign-off pending" : "sign-offs pending", tab: "signoff" });
  if (c.pipeline) {
    parts.push({ n: c.pipeline.total, text: c.pipeline.stale > 0 ? `in progress (${c.pipeline.stale} stale)` : "in progress", tab: "pipeline" });
  }
  return parts;
}

/** What the nav badge counts: questions plus sign-offs, 0 until /meta has arrived. */
export function badgeCount(c: Pick<MetaCounts, "awaiting" | "signoff_pending"> | null | undefined): number {
  return (c?.awaiting ?? 0) + (c?.signoff_pending ?? 0);
}

/** The badge's tooltip: "3 questions · 2 sign-offs". */
export function badgeTitle(c: Pick<MetaCounts, "awaiting" | "signoff_pending"> | null | undefined): string {
  const q = c?.awaiting ?? 0;
  const s = c?.signoff_pending ?? 0;
  return `${q} ${q === 1 ? "question" : "questions"} · ${s} ${s === 1 ? "sign-off" : "sign-offs"}`;
}

// ---- Pipeline

/** Server stage order (agentdesk.Stages); a stage outside it sorts after these, "unstaged" last. */
const STAGE_ORDER = ["scout", "plan", "fx", "code", "test", "review", "verify", "done"];

/** The pipeline's per-stage counts in stage order: known stages first, then others by name, "unstaged" last. */
export function stageCounts(byStage: Readonly<Record<string, number>>): Array<{ stage: string; n: number }> {
  const rank = (s: string): number => (s === "unstaged" ? STAGE_ORDER.length + 1 : STAGE_ORDER.includes(s) ? STAGE_ORDER.indexOf(s) : STAGE_ORDER.length);
  return Object.entries(byStage)
    .filter(([, n]) => n > 0)
    .map(([stage, n]) => ({ stage, n }))
    .sort((a, b) => rank(a.stage) - rank(b.stage) || a.stage.localeCompare(b.stage));
}

/** "2h", "12m", "1h05m", "3d" until `iso` from `now`; "" when it is not in the future or does not parse. */
export function timeLeft(iso: string, now: number): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t) || t <= now) return "";
  const m = Math.max(1, Math.floor((t - now) / 60000));
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 === 0 ? `${h}h` : `${h}h${String(m % 60).padStart(2, "0")}m`;
  return `${Math.floor(h / 24)}d`;
}

export interface PipelineFlag {
  /** "stale": drawn as a warning tag; "lease": faint text; "": nothing to say. */
  tone: "stale" | "lease" | "";
  text: string;
}

/** The Flag column of a pipeline row: the stale reason, else the time left on a live lease. */
export function pipelineFlag(row: Pick<PipelineRow, "stale" | "stale_reason" | "lease_expires_at">, now: number): PipelineFlag {
  if (row.stale) return { tone: "stale", text: `⚠ ${row.stale_reason ?? "stale"}` };
  const left = row.lease_expires_at ? timeLeft(row.lease_expires_at, now) : "";
  return left ? { tone: "lease", text: `lease ${left} left` } : { tone: "", text: "" };
}

/** The pipeline's summary line parts: "4 in progress", "1 stale", then "code 2" per stage. */
export function pipelineSummary(p: Pick<PipelineResponse, "total" | "stale" | "by_stage" | "stale_after">): { counts: string[]; hint: string } {
  const counts = [`${p.total} in progress`];
  if (p.stale > 0) counts.push(`${p.stale} stale`);
  for (const s of stageCounts(p.by_stage)) counts.push(`${s.stage} ${s.n}`);
  return { counts, hint: `Stale: no activity for ${p.stale_after}, or the lease expired.` };
}

/** The holder cell: assignee and claim session, whichever exist. */
export function holderText(row: Pick<PipelineRow, "holder" | "session">): string {
  return [row.holder, row.session].filter(Boolean).join(" · ");
}

// ---- Metrics

export interface MetricsCells {
  epic: string;
  title: string;
  slices: string;
  /** committed/stopped */
  outcome: string;
  /** Review rounds per slice. */
  revPerSlice: string;
  /** Slices approved on the first review round, whole percent. */
  firstPass: string;
  tests: string;
  red: string;
  /** Wall minutes per slice. */
  minPerSlice: string;
  /** Thousands of tokens "coder/reviewer/other", with how many slices logged none. */
  tokens: string;
  esc: string;
}

/** Thousands of tokens, rounded as the TUI does. */
function kTokens(v: number): string {
  return String(Math.floor((v + 500) / 1000));
}

/** One row of the Metrics table, in the TUI's columns (pkg/ui/agent_desk.go). */
export function metricsCells(m: SliceMetrics): MetricsCells {
  const per = (n: number): number => (m.slices > 0 ? n / m.slices : 0);
  const unknown = m.slices_with_unknown_tokens > 0 ? ` (${m.slices_with_unknown_tokens}?)` : "";
  return {
    epic: m.epic === "" ? "(no epic)" : m.epic,
    title: m.epic_title ?? "",
    slices: String(m.slices),
    outcome: `${m.committed}/${m.stopped}`,
    revPerSlice: per(m.review_rounds).toFixed(1),
    firstPass: `${Math.floor(100 * per(m.first_pass_approvals) + 0.5)}%`,
    tests: String(m.test_runs),
    red: String(m.test_red_runs),
    minPerSlice: per(m.slice_wall_minutes).toFixed(0),
    tokens: `${kTokens(m.coder_tokens)}/${kTokens(m.reviewer_tokens)}/${kTokens(m.other_agent_tokens)}${unknown}`,
    esc: String(m.escaped_bugs),
  };
}
