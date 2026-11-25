import { beforeEach, describe, expect, it } from "vitest";
import { closeLightbox, groupOf, isPlainClick, itemFrom, lightbox, openLightbox, stepLightbox, type LightboxItem } from "./lightbox.svelte";

const el = (url: string, over: Record<string, string | undefined> = {}) => ({
  dataset: { lbUrl: url, lbKind: "image", lbAbs: `/r/${url}`, lbAlt: `alt ${url}`, ...over } as Record<string, string | undefined>,
});
const item = (n: number): LightboxItem => ({ url: `u${n}`, name: `n${n}.png`, abs: `/r/n${n}.png`, kind: "image", alt: "" });

beforeEach(() => closeLightbox());

describe("lightbox items", () => {
  it("reads an item from the data attributes, naming it after the file", () => {
    expect(itemFrom(el("/api/v1/file?path=x", { lbAbs: "C:\\shots\\cart.png" }))).toEqual({
      url: "/api/v1/file?path=x",
      name: "cart.png",
      abs: "C:\\shots\\cart.png",
      kind: "image",
      alt: "alt /api/v1/file?path=x",
    });
    expect(itemFrom(el("u", { lbName: "given.png" }))?.name).toBe("given.png");
    expect(itemFrom(el("v", { lbKind: "video" }))?.kind).toBe("video");
  });

  it("ignores an element with no url or an unknown kind", () => {
    expect(itemFrom(el("", {}))).toBeNull();
    expect(itemFrom(el("u", { lbKind: "pdf" }))).toBeNull();
    expect(itemFrom({ dataset: {} })).toBeNull();
  });

  it("builds the group in document order with the opener's place", () => {
    const [a, b, c] = [el("a"), el("b"), el("c")];
    const g = groupOf(b, [a, b, c]);
    expect(g.items.map((i) => i.url)).toEqual(["a", "b", "c"]);
    expect(g.index).toBe(1);
  });

  it("skips members that are not items, and falls back to the opener alone", () => {
    const [a, c] = [el("a"), el("c")];
    const junk = el("x", { lbKind: "pdf" });
    expect(groupOf(c, [a, junk, c])).toEqual({ items: [itemFrom(a), itemFrom(c)], index: 1 });
    expect(groupOf(a, []).items.map((i) => i.url)).toEqual(["a"]);
    expect(groupOf(junk, []).items).toEqual([]);
  });
});

describe("lightbox state", () => {
  it("opens at an index, clamped, and closes", () => {
    openLightbox([item(1), item(2), item(3)], 1);
    expect(lightbox.index).toBe(1);
    openLightbox([item(1), item(2)], 9);
    expect(lightbox.index).toBe(1);
    closeLightbox();
    expect(lightbox.index).toBe(-1);
    expect(lightbox.items).toEqual([]);
  });

  it("opens nothing for an empty group", () => {
    openLightbox([], 0);
    expect(lightbox.index).toBe(-1);
  });

  it("steps with the buttons and stops at the ends", () => {
    openLightbox([item(1), item(2), item(3)], 0);
    stepLightbox(-1);
    expect(lightbox.index).toBe(0);
    stepLightbox(1);
    stepLightbox(1);
    stepLightbox(1);
    expect(lightbox.index).toBe(2);
    stepLightbox(-1);
    expect(lightbox.index).toBe(1);
  });

  it("does not step while closed", () => {
    stepLightbox(1);
    expect(lightbox.index).toBe(-1);
  });
});

describe("plain clicks", () => {
  const click = (over: Partial<MouseEvent> = {}) => ({ button: 0, ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, defaultPrevented: false, ...over });
  it("opens the lightbox only for a plain left click", () => {
    expect(isPlainClick(click())).toBe(true);
    for (const over of [{ button: 1 }, { ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { altKey: true }, { defaultPrevented: true }]) {
      expect(isPlainClick(click(over))).toBe(false);
    }
  });
});