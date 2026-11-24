import type { AnswerRequest, SignoffRequest, WriteResponse } from "../api/types";
import type { ToastKind } from "../toasts.svelte";

/** The write calls (lib/api/client.ts); tests pass a fake. */
export interface WriteClient {
  postSignoff(id: string, req: SignoffRequest): Promise<WriteResponse>;
  postAnswer(id: string, req: AnswerRequest): Promise<WriteResponse>;
  putPriority(id: string, priority: number): Promise<WriteResponse>;
  setCloseCandidate(id: string, on: boolean): Promise<WriteResponse>;
}

export interface ActionsEnv {
  client: WriteClient;
  toasts: { push(kind: ToastKind, text: string): unknown };
  /** meta.capabilities.writes: with false every action is refused without a request. */
  writable: () => boolean;
  /** IssuesFeed.refresh: resolves once the list has been refetched. */
  refresh: () => Promise<void>;
  /** The row stays in the list under the "Active" filter until the filters change. */
  sticky: (id: string) => void;
}

/** A draft key: the dispute note or the answer text of one issue. */
export const noteKey = (id: string) => `d:${id}`;
export const answerKey = (id: string) => `a:${id}`;
/** The picked option of a decision (the key letter), "" when the user has not picked. */
export const optionKey = (id: string) => `o:${id}`;

/**
 * The five quick actions the list and the peek drawer offer (sign-off approve and
 * dispute, answer, priority, close-candidate), one request at a time per issue.
 * Holds the UI state around them: which issues are busy, which dispute form is
 * open, and the unsent drafts (they survive the drawer re-rendering on a data
 * refresh and are dropped only when the write succeeded).
 */
export class IssueActions {
  /** Issues with a write in flight; replaced immutably. */
  busy = $state.raw<ReadonlySet<string>>(new Set());
  /** The issue whose dispute note form is open, if any. */
  disputeOpen = $state<string | null>(null);
  private drafts = $state.raw<Readonly<Record<string, string>>>({});
  /** The follow-up bead a dispute filed, by disputed issue id (the full-page view links to it). */
  followUps = $state.raw<Readonly<Record<string, string>>>({});

  private readonly env: ActionsEnv;

  constructor(env: ActionsEnv) {
    this.env = env;
  }

  getDraft(key: string): string {
    return this.drafts[key] ?? "";
  }

  setDraft(key: string, value: string): void {
    this.drafts = { ...this.drafts, [key]: value };
  }

  private dropDrafts(...keys: string[]): void {
    const next = { ...this.drafts };
    for (const k of keys) delete next[k];
    this.drafts = next;
  }

  isBusy(id: string): boolean {
    return this.busy.has(id);
  }

  private setBusy(id: string, on: boolean): void {
    const next = new Set(this.busy);
    if (on) next.add(id);
    else next.delete(id);
    this.busy = next;
  }

  /**
   * One write: busy, call, toast, then wait for the refetch. Resolves true when the
   * server accepted it. A refused or failed write toasts the server's message (bd's
   * stderr for a tracker failure) and resolves false.
   */
  private async run(id: string, call: () => Promise<WriteResponse>, success: (r: WriteResponse) => string): Promise<boolean> {
    if (this.busy.has(id) || !this.env.writable()) return false;
    this.setBusy(id, true);
    try {
      let resp: WriteResponse;
      try {
        resp = await call();
      } catch (err) {
        this.env.toasts.push("danger", err instanceof Error && err.message !== "" ? err.message : "The change was not saved.");
        return false;
      }
      this.env.toasts.push("success", success(resp));
      if (resp.warning) this.env.toasts.push("warning", resp.warning);
      // Before the refetch lands, so the row is not filtered out of the list under the cursor.
      this.env.sticky(id);
      await this.env.refresh();
      return true;
    } finally {
      this.setBusy(id, false);
    }
  }

  setPriority(id: string, priority: number): Promise<boolean> {
    return this.run(id, () => this.env.client.putPriority(id, priority), () => `Priority of ${id} set to P${priority}`);
  }

  /** `on`: mark as a close candidate; false removes the label. */
  setCloseCandidate(id: string, on: boolean): Promise<boolean> {
    return this.run(
      id,
      () => this.env.client.setCloseCandidate(id, on),
      () => (on ? `Marked ${id} close-candidate` : `Removed close-candidate from ${id}`),
    );
  }

  approve(id: string): Promise<boolean> {
    return this.run(id, () => this.env.client.postSignoff(id, { approve: true }), () => `Signed off ${id}`);
  }

  /** The note is required (the server refuses a blank one); the draft and the form go on success. */
  async dispute(id: string, note: string): Promise<boolean> {
    const text = note.trim();
    if (text === "") return false;
    const ok = await this.run(
      id,
      () => this.env.client.postSignoff(id, { approve: false, note: text }),
      (r) => {
        if (r.created_id) this.followUps = { ...this.followUps, [id]: r.created_id };
        return r.created_id ? `Disputed ${id} (follow-up ${r.created_id} filed)` : `Disputed ${id}`;
      },
    );
    if (ok) {
      this.dropDrafts(noteKey(id));
      if (this.disputeOpen === id) this.disputeOpen = null;
    }
    return ok;
  }

  /** Needs an option or non-blank text; the drafts go on success. */
  async answer(id: string, req: AnswerRequest): Promise<boolean> {
    const option = req.option ?? "";
    const text = (req.text ?? "").trim();
    if (option === "" && text === "") return false;
    const body: AnswerRequest = { ...(option !== "" ? { option } : {}), ...(text !== "" ? { text } : {}) };
    const ok = await this.run(id, () => this.env.client.postAnswer(id, body), () => `Answer posted on ${id}`);
    if (ok) this.dropDrafts(answerKey(id), optionKey(id));
    return ok;
  }

  openDispute(id: string): void {
    this.disputeOpen = id;
  }

  closeDispute(): void {
    this.disputeOpen = null;
  }
}
