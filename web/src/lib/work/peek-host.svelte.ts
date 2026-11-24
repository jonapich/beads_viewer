import { tick } from "svelte";
import { postAnswer, postSignoff, putPriority, setCloseCandidate } from "../api/client";
import type { ListItem } from "../api/types";
import { IssueActions } from "../issues/actions.svelte";
import { toasts } from "../toasts.svelte";

/** The fields the priority popover reads from an issue. */
export type PopItem = Pick<ListItem, "id" | "priority" | "status" | "labels" | "signoff">;

/** The open priority popover: where it was opened from, and from which control. */
export interface Pop {
  id: string;
  anchor: DOMRect;
  el: HTMLElement;
  from: "row" | "drawer";
  /** The priority the drawer showed, used when the view has no row for this issue. */
  priority: number;
}

export interface PeekHostEnv {
  /** The tracker takes writes. */
  writable: () => boolean;
  /** The feed's refresh: resolves once the view has been refetched. */
  refresh: () => Promise<void>;
  /** The view's own copy of an issue (its row), if it has one. */
  lookup: (id: string) => PopItem | undefined;
}

/**
 * The state the Triage, Plan and Insights views share around the peek drawer: which
 * issue is open, the priority popover, and the quick actions. These views are not
 * virtualised, so unlike Issues.svelte there is no row key or window to track.
 */
export class PeekHost {
  /** The issue open in the drawer. */
  peek = $state<string | null>(null);
  pop = $state.raw<Pop | null>(null);
  /** Set when the peeked issue turned out to be gone (404): shown above the view until dismissed. */
  missing = $state<string | null>(null);
  readonly actions: IssueActions;
  readonly #env: PeekHostEnv;

  /** The issue the popover is for: the view's row, else (from the drawer) what the drawer showed. */
  popItem = $derived.by((): PopItem | undefined => {
    const p = this.pop;
    if (!p) return undefined;
    const row = this.#env.lookup(p.id);
    if (row) return row;
    return p.from === "drawer" ? { id: p.id, priority: p.priority, status: "open", labels: [] } : undefined;
  });

  constructor(env: PeekHostEnv) {
    this.#env = env;
    this.actions = new IssueActions({
      client: { postSignoff, postAnswer, putPriority, setCloseCandidate },
      toasts,
      writable: env.writable,
      refresh: env.refresh,
      sticky: () => {},
    });
  }

  open(id: string): void {
    this.missing = null;
    this.peek = id;
  }

  close(): void {
    this.peek = null;
  }

  /** The drawer's 404: close it and say so. */
  peekMissing(id: string): void {
    this.missing = `${id} is no longer in the tracker.`;
    this.peek = null;
  }

  /** The same button again closes the popover. */
  togglePop(id: string, el: HTMLElement, from: Pop["from"], priority = 2): void {
    if (this.pop && this.pop.el === el) {
      this.pop = null;
      return;
    }
    this.pop = { id, anchor: el.getBoundingClientRect(), el, from, priority };
  }

  /** Which row's pill has its popover open, for aria-expanded. */
  rowPopOpen(id: string): boolean {
    return this.pop !== null && this.pop.from === "row" && this.pop.id === id;
  }

  /** The popover goes with its issue (it left the view) and with write access. */
  dropStalePop(): void {
    if (this.pop && (!this.popItem || !this.#env.writable())) this.pop = null;
  }

  /**
   * A live update can reorder or move rows while the popover is open, and its anchor is a rect
   * measured at opening: measure it again from the opener, or close the popover when the opener
   * is gone. Call it after the view's data changed.
   */
  async remeasure(): Promise<void> {
    const p = this.pop;
    if (!p || p.from !== "row") return;
    await tick();
    if (this.pop !== p) return;
    if (!p.el.isConnected) {
      // Closing unmounts the popover: if it holds focus, park it on the view, not <body>.
      const held = !!document.getElementById("row-actions-pop")?.contains(document.activeElement);
      this.pop = null;
      if (held) {
        await tick();
        document.querySelector<HTMLElement>("[data-work-scroll]")?.focus({ preventScroll: true });
      }
      return;
    }
    const a = p.el.getBoundingClientRect();
    if (a.left !== p.anchor.left || a.top !== p.anchor.top || a.width !== p.anchor.width || a.height !== p.anchor.height) {
      this.pop = { ...p, anchor: a };
    }
  }

  async closePop(restoreFocus: boolean): Promise<void> {
    const p = this.pop;
    this.pop = null;
    if (!restoreFocus || !p) return;
    await tick();
    const target = p.el.isConnected
      ? p.el
      : document.querySelector<HTMLElement>(`[data-row][data-id="${CSS.escape(p.id)}"] [data-pop-anchor="prio"]`);
    target?.focus({ preventScroll: true });
  }

  /**
   * A write that re-renders the view moves the row's DOM node, which drops focus to <body> for a
   * keyboard user. Once it has landed, put focus back on the pill the popover was opened from,
   * unless the user has moved it elsewhere meanwhile.
   */
  async refocus(write: Promise<boolean>, p: Pop | null): Promise<void> {
    await write;
    await tick();
    if (!p || (document.activeElement && document.activeElement !== document.body)) return;
    const target = p.el.isConnected
      ? p.el
      : document.querySelector<HTMLElement>(
          p.from === "drawer"
            ? `aside.drawer [data-pop-anchor="prio"]`
            : `[data-row][data-id="${CSS.escape(p.id)}"] [data-pop-anchor="prio"]`,
        );
    target?.focus({ preventScroll: true });
  }

  /** A priority was picked in the popover: close it, write, and keep focus sensible. */
  pickPriority(id: string, priority: number): void {
    const from = this.pop;
    void this.closePop(true);
    void this.refocus(this.actions.setPriority(id, priority), from);
  }
}
