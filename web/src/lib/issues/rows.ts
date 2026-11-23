import type { Mode } from "./filter";

/**
 * Row heights in px: the single source of truth. VirtualList does its index
 * arithmetic with these and hands the same number to the CSS as --row-h, so
 * the two cannot disagree.
 */
export const ROW_H = { tree: 32, flat: 44, treeNarrow: 52, flatNarrow: 64 } as const;

/** A scroller narrower than this (the mock's container breakpoint) uses the two-line rows. */
export const NARROW_W = 560;

export function isNarrow(width: number): boolean {
  return width > 0 && width < NARROW_W;
}

export function rowHeight(mode: Mode, narrow: boolean): number {
  if (mode === "tree") return narrow ? ROW_H.treeNarrow : ROW_H.tree;
  return narrow ? ROW_H.flatNarrow : ROW_H.flat;
}

/**
 * The rows to mount: [start, end) over a list of `count` fixed-height rows.
 * `scrollTop` is clamped to the scrollable range first, so a list that just
 * shrank under a large scrollTop still yields a window inside the data.
 */
export function windowRange(
  scrollTop: number,
  viewH: number,
  count: number,
  rowH: number,
  overscan: number,
): { start: number; end: number } {
  if (count <= 0 || rowH <= 0) return { start: 0, end: 0 };
  const top = Math.min(Math.max(0, scrollTop), Math.max(0, count * rowH - viewH));
  const start = Math.max(0, Math.floor(top / rowH) - overscan);
  const end = Math.min(count, Math.ceil((top + viewH) / rowH) + overscan);
  return { start, end: Math.max(start, end) };
}
