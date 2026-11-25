import type { ToastKind } from "./toasts.svelte";

/** How long the copy button shows its check mark after a successful copy. */
export const COPIED_MS = 1500;

/** The parts of the browser the copy needs; tests pass fakes. */
export interface CopyEnv {
  clipboard: { writeText(text: string): Promise<void> } | undefined;
  toasts: { push(kind: ToastKind, text: string): unknown };
}

/** Why a copy failed, in a few words: the browser's own message, else that the clipboard is unavailable. */
export function copyFailure(err: unknown): string {
  return err instanceof Error && err.message.trim() !== "" ? err.message.trim() : "the clipboard is not available";
}

/**
 * Copies an issue id (plain text, nothing around it) and says so in a toast: "Copied <id>", or a
 * warning with the reason. Resolves true when the id is on the clipboard.
 */
export async function copyIssueId(id: string, env: CopyEnv): Promise<boolean> {
  try {
    if (!env.clipboard) throw new Error("");
    await env.clipboard.writeText(id);
  } catch (err) {
    env.toasts.push("warning", `Couldn't copy: ${copyFailure(err)}`);
    return false;
  }
  env.toasts.push("success", `Copied ${id}`);
  return true;
}