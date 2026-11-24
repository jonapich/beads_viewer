import { tick } from "svelte";

/** A Svelte action: focuses the element when it is rendered (an editor that was just opened). */
export function autofocus(node: HTMLElement): void {
  node.focus({ preventScroll: true });
  if (node instanceof HTMLInputElement) node.select();
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
