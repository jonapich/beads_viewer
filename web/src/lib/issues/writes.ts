import type { ListItem } from "../api/types";
import { awaitsSignoff, canAnswer } from "./model";

/** Tooltip of every write control while the tracker cannot be written to. */
export const READ_ONLY_TITLE = "Read-only: this tracker can't be changed from bv";
export const SIGNOFF_ONLY_TITLE = "Only for closed issues awaiting sign-off";
export const ANSWER_ONLY_TITLE = "Only for issues labelled needs-decision or needs-design";

/** The label the close-candidate toggle sets (the TUI's 'x'). */
export const CLOSE_CANDIDATE = "close-candidate";

/**
 * Longest dispute note or answer text the server takes, in UTF-8 bytes (pkg/api/write.go).
 * A textarea's maxlength counts UTF-16 code units, and one unit can take up to 3 bytes, so
 * maxlength alone does not keep a note under it: it only stops a runaway paste. The forms
 * count bytes with tooLong() and disable Send/Post, with a hint.
 */
export const MAX_TEXT = 10000;

/** The text is over the server's byte cap. */
export function tooLong(text: string): boolean {
  return utf8Length(text) > MAX_TEXT;
}

/** UTF-8 bytes of a string: what the server's caps count. */
export function utf8Length(text: string): number {
  return new TextEncoder().encode(text).length;
}

/**
 * Longest description PATCH /issues/{id} takes, in UTF-8 bytes of the normalised text
 * (pkg/api/patch.go maxDescription): it travels to bd as one command-line argument.
 */
export const MAX_DESCRIPTION = 16000;

/** Longest title, status, type or assignee PATCH takes, in UTF-8 bytes (pkg/api/patch.go maxLineField). */
export const MAX_LINE_FIELD = 500;

/**
 * A description as the TUI's O editor and the server compare and send it: leading newlines and
 * trailing spaces, tabs and newlines are dropped (pkg/api/patch.go normalizeDescription).
 */
export function normalizeDescription(text: string): string {
  return text.replace(/^\n+/, "").replace(/[ \t\n\r]+$/, "");
}

/** What the quick actions may do for one row, given whether the tracker takes writes. */
export interface ActionState {
  /** Priority P0-P4 and close-candidate: every row, closed ones included. */
  priority: boolean;
  closeCandidate: boolean;
  /** The label is on the issue now, so the toggle removes it. */
  isCandidate: boolean;
  approve: boolean;
  dispute: boolean;
  answer: boolean;
}

export function actionState(item: Pick<ListItem, "labels" | "status" | "signoff">, writes: boolean): ActionState {
  return {
    priority: writes,
    closeCandidate: writes,
    isCandidate: item.labels.includes(CLOSE_CANDIDATE),
    approve: writes && awaitsSignoff(item),
    dispute: writes && awaitsSignoff(item),
    answer: writes && canAnswer(item),
  };
}
