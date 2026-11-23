import type { DetailTriage, IssueRef, SignoffCall } from "../api/types";
import type { DisplayStatus } from "./model";

export type PeekSection = "signoff" | "answer";

/** "3/12 closed" for the drawer's Children block. */
export function childProgress(children: readonly IssueRef[]): { done: number; total: number } {
  let done = 0;
  for (const c of children) if (c.status === "closed") done++;
  return { done, total: children.length };
}

/** The top reasons behind the triage score (at most `n`), falling back to the primary one. */
export function topReasons(triage: DetailTriage | undefined, n = 3): string[] {
  const r = triage?.reasons;
  if (!r) return [];
  const all = r.all.length > 0 ? r.all : r.primary ? [r.primary] : [];
  return all.slice(0, n);
}

/** Sign-off states shown as a green badge; the rest (pending, disputed) draw attention. */
export function signoffTone(state: string): "ok" | "bad" | "pending" {
  if (state === "approved" || state === "waived") return "ok";
  if (state === "disputed") return "bad";
  return "pending";
}

/** The glyph status of a referenced issue (a blocker, a child) from its raw status; unknown reads as open. */
export function refStatus(status: string | undefined): DisplayStatus {
  return status === "closed" || status === "in_progress" || status === "blocked" || status === "deferred" ? status : "open";
}

/** The sign-off badge text. */
export function signoffLabel(state: string): string {
  switch (state) {
    case "pending":
      return "Awaiting sign-off";
    case "approved":
      return "Approved";
    case "disputed":
      return "Disputed";
    case "waived":
      return "Waived";
    default:
      return state;
  }
}

/**
 * How one structured sign-off call is drawn: the call is bold only when it comes with an
 * alternative or a why (then it is a headline over its detail); a call-only entry is a
 * whole sentence and reads in regular weight.
 */
export function callView(c: SignoffCall): { bold: boolean; alternative: string; why: string } {
  const alternative = c.alternative ?? "";
  const why = c.why ?? "";
  return { bold: alternative !== "" || why !== "", alternative, why };
}
