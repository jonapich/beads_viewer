import type { Action } from "svelte/action";

/**
 * One delegated click listener for a view's rows: a click on a `[data-row]` opens its
 * issue, unless it landed on a button, link or form control (those do their own thing).
 * The title button is the keyboard path. An action, so the container needs no handler
 * attribute (and no a11y suppression).
 */
export const rowClicks: Action<HTMLElement, (id: string) => void> = (node, open) => {
  let handler = open;
  const onClick = (e: MouseEvent): void => {
    const target = e.target as Element | null;
    if (!target || e.defaultPrevented || target.closest("button, a, input, select, textarea, summary")) return;
    const id = target.closest("[data-row]")?.getAttribute("data-id");
    if (id) handler(id);
  };
  node.addEventListener("click", onClick);
  return {
    update(next) {
      handler = next;
    },
    destroy() {
      node.removeEventListener("click", onClick);
    },
  };
};
