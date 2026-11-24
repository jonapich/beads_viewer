import { ApiError } from "../api/client";
import type { IssueDetail, IssuePatchRequest, WriteResponse } from "../api/types";
import type { ToastKind } from "../toasts.svelte";
import { noteHolds, PILL_NAME, type PillStatus } from "./detail";
import { MAX_DESCRIPTION, MAX_LINE_FIELD, normalizeDescription, utf8Length } from "./writes";

/**
 * What the status menu and the type select offer. The TUI's O editor is free text, so these
 * are bv's choice (beads-c2o.46): bd's work statuses without pinned/hooked, and the types bv has
 * icons for. The current value is always added when it is something else. Custom values stay a
 * bd/TUI job.
 */
export const EDIT_STATUSES: readonly string[] = ["open", "in_progress", "blocked", "deferred", "closed"];
export const EDIT_TYPES: readonly string[] = ["bug", "feature", "task", "epic", "chore"];

/** `list`, with `current` added at the end when it is not in it (a custom value the tracker allows). */
export function withCurrent(list: readonly string[], current: string): string[] {
  return current === "" || list.includes(current) ? [...list] : [...list, current];
}

/** The fields the O editor edits and the inline editors cover (priority has its own popover). */
export type EditField = "title" | "status" | "assignee" | "type" | "description";

const FIELDS: readonly EditField[] = ["title", "status", "assignee", "type", "description"];

/** One open editor: the text being edited, the value it started from, and the updated_at it started on. */
export interface EditSession {
  draft: string;
  /** The field's value when the editor opened (a description already normalised). */
  original: string;
  /** detail.updated_at at open, echoed as base_updated_at. A live refetch never moves it. */
  base: string;
}

/** A failed save, shown inline next to the editor. */
export interface EditError {
  kind: "conflict" | "error";
  message: string;
  /** conflict: the field's current value after the refetch. */
  now?: string;
  /** conflict: the field itself still has the value the editor opened with; something else changed. */
  sameField?: boolean;
}

export interface EditsClient {
  patchIssue(id: string, req: IssuePatchRequest): Promise<WriteResponse>;
}

export interface EditsEnv {
  client: EditsClient;
  toasts: { push(kind: ToastKind, text: string): unknown };
  /** meta.capabilities.writes and the issue's own route: with false every edit is refused without a request. */
  writable: () => boolean;
  /** DetailFeed.refresh: resolves once the detail has been refetched. */
  refresh: () => Promise<void>;
  /** The detail on show for `id`, null when it is not. */
  current: (id: string) => IssueDetail | null;
  /**
   * Another kind of write on `id` is in flight (the quick actions). Writes on one issue run one at
   * a time on the server, so an edit started now would carry a base the other write is about to
   * make stale: no editor opens or saves meanwhile.
   */
  blocked: (id: string) => boolean;
}

export const editKey = (id: string, field: EditField): string => `${id}:${field}`;

/** The field as the editor shows and compares it. */
export function fieldValue(d: IssueDetail, field: EditField): string {
  switch (field) {
    case "title":
      return d.title;
    case "status":
      return d.status;
    case "assignee":
      return d.assignee ?? "";
    case "type":
      return d.issue_type;
    case "description":
      return normalizeDescription(d.description);
  }
}

/** What the server will see of a draft: trimmed, a description normalised. */
export function normalizeDraft(field: EditField, draft: string): string {
  return field === "description" ? normalizeDescription(draft) : draft.trim();
}

/** Why a draft can't be saved ("" when it can): the server's checks, repeated so Save is disabled first. */
export function editProblem(field: EditField, draft: string): string {
  const v = normalizeDraft(field, draft);
  if (field === "description") {
    return utf8Length(v) > MAX_DESCRIPTION ? `Too long to save from bv (${MAX_DESCRIPTION.toLocaleString("en-US")} bytes max)` : "";
  }
  if (v === "" && field !== "assignee") return field === "title" ? "The title can't be empty" : `The ${field} can't be empty`;
  return utf8Length(v) > MAX_LINE_FIELD ? `Too long to save from bv (${MAX_LINE_FIELD} bytes max)` : "";
}

/** One line of a value for the conflict panel: whitespace collapsed, cut at `max` characters. */
export function clip(text: string, max = 90): string {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > max ? `${flat.slice(0, max - 1)}…` : flat;
}

const statusLabel = (status: string): string => PILL_NAME[status as PillStatus] ?? status;

function body(field: EditField, value: string, base: string): IssuePatchRequest {
  switch (field) {
    case "title":
      return { base_updated_at: base, title: value };
    case "status":
      return { base_updated_at: base, status: value };
    case "assignee":
      return { base_updated_at: base, assignee: value };
    case "type":
      return { base_updated_at: base, type: value };
    case "description":
      return { base_updated_at: base, description: value };
  }
}

function successText(id: string, field: EditField, value: string): string {
  switch (field) {
    case "title":
      return `Title of ${id} saved`;
    case "status":
      return `Status of ${id} set to ${statusLabel(value)}`;
    case "assignee":
      return value === "" ? `Assignee of ${id} cleared` : `Assignee of ${id} set to ${value}`;
    case "type":
      return `Type of ${id} set to ${value}`;
    case "description":
      return `Description of ${id} saved`;
  }
}

/**
 * The inline field editors of the issue detail (beads-c2o.14): title, status, assignee, type
 * and description, each through PATCH /api/v1/issues/{id}, one request at a time per issue.
 * Holds the open editors (keyed `${id}:${field}`), their inline errors, and the "saved, but the
 * reload failed" hold, so a stale detail can't re-offer an edit.
 */
export class IssueEdits {
  private sessions = $state.raw<Readonly<Record<string, EditSession>>>({});
  private errors = $state.raw<Readonly<Record<string, EditError>>>({});
  /** The issue's updated_at when a write landed but the reload failed; editors stay off while the detail still has it. */
  private staleAt = $state.raw<Readonly<Record<string, string>>>({});
  /** Issues with a save in flight, and the field it saves. */
  private saving = $state.raw<Readonly<Record<string, EditField>>>({});

  private readonly env: EditsEnv;

  constructor(env: EditsEnv) {
    this.env = env;
  }

  session(id: string, field: EditField): EditSession | undefined {
    return this.sessions[editKey(id, field)];
  }

  error(id: string, field: EditField): EditError | undefined {
    return this.errors[editKey(id, field)];
  }

  isBusy(id: string): boolean {
    return id in this.saving;
  }

  /** This field's save is the one in flight. */
  isSaving(id: string, field: EditField): boolean {
    return this.saving[id] === field;
  }

  /** The last write landed but the detail is still the snapshot it started from: editing would offer a stale value. */
  locked(id: string): boolean {
    return noteHolds(this.staleAt[id], this.env.current(id)?.updated_at ?? "");
  }

  /** Whether an editor can be opened now. */
  canEdit(id: string): boolean {
    return this.env.writable() && !this.isBusy(id) && !this.env.blocked(id) && !this.locked(id);
  }

  /**
   * Whether Save is enabled for an open editor. Not while a 409 panel is up: the old base would
   * only conflict again, so the ways forward are Save mine and Discard my edit.
   */
  canSave(id: string, field: EditField): boolean {
    const s = this.session(id, field);
    if (!s || this.isBusy(id) || this.env.blocked(id) || this.locked(id)) return false;
    if (this.errors[editKey(id, field)]?.kind === "conflict") return false;
    return editProblem(field, s.draft) === "" && normalizeDraft(field, s.draft) !== s.original;
  }

  /** Opens an editor on the issue's current value; one already open is left as it is. */
  open(id: string, field: EditField, detail: IssueDetail): boolean {
    if (!this.canEdit(id) && !this.session(id, field)) return false;
    const key = editKey(id, field);
    if (this.sessions[key]) return true;
    const value = fieldValue(detail, field);
    this.sessions = { ...this.sessions, [key]: { draft: value, original: value, base: detail.updated_at } };
    this.dropError(key);
    return true;
  }

  setDraft(id: string, field: EditField, draft: string): void {
    const key = editKey(id, field);
    const s = this.sessions[key];
    if (s) this.sessions = { ...this.sessions, [key]: { ...s, draft } };
  }

  /** Closes the editor and drops its draft and error. */
  cancel(id: string, field: EditField): void {
    const key = editKey(id, field);
    const next = { ...this.sessions };
    delete next[key];
    this.sessions = next;
    this.dropError(key);
  }

  /** Hides the error and keeps the draft. */
  dismissError(id: string, field: EditField): void {
    this.dropError(editKey(id, field));
  }

  private dropError(key: string): void {
    if (!(key in this.errors)) return;
    const next = { ...this.errors };
    delete next[key];
    this.errors = next;
  }

  private setSaving(id: string, field: EditField | null): void {
    const next = { ...this.saving };
    if (field) next[id] = field;
    else delete next[id];
    this.saving = next;
  }

  /**
   * Picks a value and saves it at once (the status menu, whose pick is an explicit click): opens the editor on the
   * detail on show (so the base is its updated_at now), sets the draft and saves. The same value
   * again does nothing. On a failure the editor stays open, so the panel can retry or discard.
   */
  async quick(id: string, field: EditField, value: string): Promise<boolean> {
    const detail = this.env.current(id);
    if (!detail || this.isBusy(id) || this.env.blocked(id)) return false;
    if (value === fieldValue(detail, field)) {
      this.cancel(id, field);
      return false;
    }
    // A fresh session every time: its base is the updated_at on show now, not that of an earlier attempt.
    this.cancel(id, field);
    if (!this.open(id, field, detail)) return false;
    this.setDraft(id, field, value);
    return this.save(id, field);
  }

  /**
   * Saves one editor: busy, PATCH, toast, then wait for the refetch. Resolves true when the server
   * accepted it. A failure keeps the draft and shows an inline error; a 409 refetches first, so the
   * panel can show what the field is now.
   */
  async save(id: string, field: EditField): Promise<boolean> {
    const key = editKey(id, field);
    const s = this.sessions[key];
    if (!s || this.isBusy(id) || this.env.blocked(id) || !this.env.writable() || this.locked(id)) return false;
    const problem = editProblem(field, s.draft);
    if (problem !== "") {
      this.errors = { ...this.errors, [key]: { kind: "error", message: problem } };
      return false;
    }
    const value = normalizeDraft(field, s.draft);

    this.setSaving(id, field);
    this.dropError(key);
    try {
      let resp: WriteResponse;
      try {
        resp = await this.env.client.patchIssue(id, body(field, value, s.base));
      } catch (err) {
        await this.failed(id, field, s, err);
        return false;
      }
      this.env.toasts.push("success", successText(id, field, value));
      if (resp.warning) {
        this.env.toasts.push("warning", resp.warning);
        // The reload failed, so the refetch below changes nothing: keep every editor of the issue
        // off while the detail is still the snapshot this write started from.
        this.staleAt = { ...this.staleAt, [id]: s.base };
      }
      this.cancel(id, field);
      await this.env.refresh();
      this.rebaseOpen(id, field);
      return true;
    } finally {
      this.setSaving(id, null);
    }
  }

  private async failed(id: string, field: EditField, s: EditSession, err: unknown): Promise<void> {
    const key = editKey(id, field);
    if (err instanceof ApiError && err.code === "conflict") {
      // The server reloaded before answering 409; refetch so "now" is what the page will show.
      await this.env.refresh();
      const detail = this.env.current(id);
      const now = detail ? fieldValue(detail, field) : "";
      this.errors = {
        ...this.errors,
        [key]: { kind: "conflict", message: err.message, now, sameField: now === s.original },
      };
      return;
    }
    const message = err instanceof Error && err.message !== "" ? err.message : "The change was not saved.";
    this.errors = { ...this.errors, [key]: { kind: "error", message } };
  }

  /**
   * "Save mine" after a 409: the refetch has already brought the new values in, so rebase the
   * draft on the issue as it is now and save it.
   */
  async saveMine(id: string, field: EditField): Promise<boolean> {
    const key = editKey(id, field);
    const s = this.sessions[key];
    const detail = this.env.current(id);
    // The same refusals as save(), before the base moves: a refused Save mine changes nothing.
    if (!s || !detail || this.isBusy(id) || this.env.blocked(id) || !this.env.writable() || this.locked(id)) return false;
    this.sessions = { ...this.sessions, [key]: { ...s, base: detail.updated_at } };
    return this.save(id, field);
  }

  /**
   * After a write of ours and its refetch (a field save here, or a quick action of the page): the
   * open editors of the issue, `except` the one just saved, whose field still has the value they
   * opened with move to the new updated_at, so our own priority change does not make a description
   * edit that is still valid conflict. An editor whose field changed keeps its old base and meets
   * the 409.
   */
  rebaseOpen(id: string, except?: EditField): void {
    const detail = this.env.current(id);
    if (!detail) return;
    let next: Record<string, EditSession> | null = null;
    for (const f of FIELDS) {
      if (f === except) continue;
      const key = editKey(id, f);
      const s = this.sessions[key];
      if (s && s.original === fieldValue(detail, f) && s.base !== detail.updated_at) {
        next ??= { ...this.sessions };
        next[key] = { ...s, base: detail.updated_at };
      }
    }
    if (next) this.sessions = next;
  }
}
