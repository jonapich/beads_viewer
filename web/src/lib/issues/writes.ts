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
  return new TextEncoder().encode(text).length > MAX_TEXT;
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

export function actionState(item: ListItem, writes: boolean): ActionState {
  return {
    priority: writes,
    closeCandidate: writes,
    isCandidate: item.labels.includes(CLOSE_CANDIDATE),
    approve: writes && awaitsSignoff(item),
    dispute: writes && awaitsSignoff(item),
    answer: writes && canAnswer(item),
  };
}
