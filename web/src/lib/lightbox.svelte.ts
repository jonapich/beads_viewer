// The image lightbox (beads-c2o.64): one dialog for the whole app, opened from any thumbnail or
// image link. The group it steps through is whatever sits in the same [data-lightbox-group]
// container as the opener (one Markdown text, one captures list), in document order. No keyboard
// shortcuts beyond what <dialog> gives (Esc closes): stepping is by buttons.

export interface LightboxItem {
  url: string;
  /** The file name. */
  name: string;
  /** The absolute path, for Copy path. */
  abs: string;
  kind: "image" | "video";
  alt: string;
}

export const lightbox = $state({
  items: [] as LightboxItem[],
  /** -1 while closed. */
  index: -1,
  /** What had focus when it opened, so closing can give it back. */
  opener: null as HTMLElement | null,
});

/** The attributes a thumbnail or image link carries for the lightbox. */
export interface LightboxSource {
  dataset: Readonly<Record<string, string | undefined>>;
}

/** Reads one item from an element's data-lb-* attributes; null when it has no url or an unknown kind. */
export function itemFrom(el: LightboxSource): LightboxItem | null {
  const d = el.dataset;
  const kind = d["lbKind"];
  if (!d["lbUrl"] || (kind !== "image" && kind !== "video")) return null;
  const abs = d["lbAbs"] ?? "";
  return { url: d["lbUrl"], name: d["lbName"] ?? (abs.split(/[\\/]/).pop() || d["lbUrl"]), abs, kind, alt: d["lbAlt"] ?? "" };
}

/** The group of an opener: its siblings in the same container, in order, and the opener's place among them. */
export function groupOf(opener: LightboxSource, siblings: readonly LightboxSource[]): { items: LightboxItem[]; index: number } {
  const items: LightboxItem[] = [];
  let index = 0;
  for (const el of siblings) {
    const item = itemFrom(el);
    if (!item) continue;
    if (el === opener) index = items.length;
    items.push(item);
  }
  if (items.length === 0) {
    const own = itemFrom(opener);
    return { items: own ? [own] : [], index: 0 };
  }
  return { items, index };
}

export function openLightbox(items: LightboxItem[], index: number, opener: HTMLElement | null = null): void {
  if (items.length === 0) return;
  lightbox.items = items;
  lightbox.index = Math.min(Math.max(index, 0), items.length - 1);
  lightbox.opener = opener;
}

/** Opens the lightbox on an element carrying data-lb-* attributes, with its group. */
export function openFromElement(el: HTMLElement): void {
  const container = el.closest("[data-lightbox-group]");
  const siblings = container ? [...container.querySelectorAll<HTMLElement>("[data-lb-url]")] : [el];
  const { items, index } = groupOf(el, siblings);
  openLightbox(items, index, el);
}

export function closeLightbox(): void {
  lightbox.index = -1;
  lightbox.items = [];
  const opener = lightbox.opener;
  lightbox.opener = null;
  if (opener?.isConnected) opener.focus({ preventScroll: true });
}

/** Moves one image on (+1) or back (-1); stops at the ends. */
export function stepLightbox(by: 1 | -1): void {
  const next = lightbox.index + by;
  if (lightbox.index >= 0 && next >= 0 && next < lightbox.items.length) lightbox.index = next;
}

/** A click that should open the lightbox: a plain left click, not a modified one (ctrl/shift/middle open a new tab). */
export function isPlainClick(e: Pick<MouseEvent, "button" | "ctrlKey" | "metaKey" | "shiftKey" | "altKey" | "defaultPrevented">): boolean {
  return e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey && !e.defaultPrevented;
}