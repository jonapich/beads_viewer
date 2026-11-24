import { tick } from "svelte";

/** A Svelte action: focuses the element when it is rendered (an editor that was just opened). */
export function autofocus(node: HTMLElement): void {
  node.focus({ preventScroll: true });
  if (node instanceof HTMLInputElement) node.select();
}

/**
 * After a write from a form (sign-off, dispute, answer) failed and the form stays: pressing its
 * button disabled it while the write ran, so focus fell to the page. Put it back in the form, on
 * its textarea, else its first enabled button. Acts only when focus is on the page, never from
 * where the user moved it. Call it once the DOM has settled (after `tick()`).
 */
export function refocusForm(form: ParentNode | null | undefined): void {
  const active = document.activeElement;
  if (active && active !== document.body) return;
  const target = form?.querySelector<HTMLElement>("textarea:not(:disabled)") ?? form?.querySelector<HTMLElement>("button:not(:disabled)");
  target?.focus({ preventScroll: true });
}

/**
 * After an editor closed: put focus on `target` (the button that opened it) unless the user has
 * already moved it somewhere else. Waits for the DOM to settle first, since the button is only
 * rendered once the editor is gone.
 */
export async function refocus(target: () => HTMLElement | undefined | null): Promise<void> {
  await tick();
  const active = document.activeElement;
  if (active && active !== document.body) return;
  target()?.focus({ preventScroll: true });
}
