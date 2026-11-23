<script lang="ts" generics="T">
  import { untrack, type Snippet } from "svelte";
  import { windowRange } from "../lib/issues/rows";

  interface Props {
    items: readonly T[];
    /** Fixed row height in px. The list does its index arithmetic with it and hands it to the rows' CSS as --row-h. */
    rowHeight: number;
    key: (item: T) => string;
    row: Snippet<[T, number]>;
    /** Extra rows mounted above and below the viewport. */
    overscan?: number;
    /** The scroller's measured width in px (for width-dependent row layouts). */
    width?: number;
    /** Accessible name of the scroll region. */
    label?: string;
    /** One native click listener on the scroller, for delegating row clicks (no onclick on a div). */
    onclick?: (e: MouseEvent) => void;
  }

  let { items, rowHeight, key, row, overscan = 8, width = $bindable(0), label = "List", onclick }: Props = $props();

  let scroller = $state<HTMLDivElement>();
  let sizer = $state<HTMLDivElement>();
  let win = $state<HTMLDivElement>();
  let scrollTop = $state(0);
  let viewH = $state(0);

  // The browser clamps scrollTop when the list shrinks, but only after layout; windowRange
  // clamps it meanwhile, so the window stays inside the data.
  const range = $derived(windowRange(scrollTop, viewH, items.length, rowHeight, overscan));
  const start = $derived(range.start);
  const slice = $derived(items.slice(range.start, range.end));

  // Every offset reaches the DOM as a CSS custom property set through CSSOM
  // (el.style.setProperty). CSSOM writes are not a parsed style attribute, so the
  // "no style attributes or style: directives" CSP rule still holds. Keep all such
  // writes in this one place so they can be audited.
  function setVar(el: HTMLElement, name: string, value: string): void {
    el.style.setProperty(name, value);
  }

  $effect(() => {
    if (!scroller || !sizer || !win) return;
    setVar(scroller, "--row-h", `${rowHeight}px`);
    setVar(sizer, "--vl-total", `${items.length * rowHeight}px`);
    setVar(win, "--vl-offset", `${start * rowHeight}px`);
  });

  // The sizer may just have shrunk (a collapse, a filter): take whatever scrollTop the
  // browser settled on. Runs after the effect above, so the new height is in place.
  $effect(() => {
    void items.length;
    void rowHeight;
    untrack(() => {
      if (scroller && scroller.scrollTop !== scrollTop) scrollTop = scroller.scrollTop;
    });
  });

  $effect(() => {
    const el = scroller;
    if (!el) return;
    const measure = () => {
      viewH = el.clientHeight;
      width = el.clientWidth;
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  });

  $effect(() => {
    const el = scroller;
    const handler = onclick;
    if (!el || !handler) return;
    el.addEventListener("click", handler);
    return () => el.removeEventListener("click", handler);
  });

  export function getScrollTop(): number {
    return scroller?.scrollTop ?? 0;
  }

  export function setScrollTop(px: number): void {
    if (!scroller) return;
    scroller.scrollTop = Math.max(0, px);
    scrollTop = scroller.scrollTop;
  }

  export function scrollToIndex(i: number, align: "center" | "nearest" = "nearest"): void {
    if (!scroller) return;
    const y = i * rowHeight;
    const h = scroller.clientHeight;
    if (align === "center") setScrollTop(y - (h - rowHeight) / 2);
    else if (y < scroller.scrollTop) setScrollTop(y);
    else if (y + rowHeight > scroller.scrollTop + h) setScrollTop(y + rowHeight - h);
  }
</script>

<div class="vl" role="region" aria-label={label} bind:this={scroller} onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}>
  <div class="vl-sizer" bind:this={sizer}>
    <div class="vl-win" bind:this={win}>
      {#each slice as it, i (key(it))}
        {@render row(it, start + i)}
      {/each}
    </div>
  </div>
</div>

<style>
  .vl {
    flex: 1;
    min-height: 0;
    overflow: auto;
    /* Rows query this container (column hiding by width), keyed "list". */
    container: list / inline-size;
  }
  .vl-sizer {
    height: var(--vl-total, 0px);
  }
  .vl-win {
    transform: translateY(var(--vl-offset, 0px));
  }
</style>
