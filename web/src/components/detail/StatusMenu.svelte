<script lang="ts">
  import { onMount } from "svelte";
  import { PILL_GLYPH, PILL_NAME, type PillStatus } from "../../lib/issues/detail";
  import { EDIT_STATUSES, withCurrent } from "../../lib/issues/edits.svelte";

  interface Props {
    issueId: string;
    /** The issue's status as stored (not the pill's derived one). */
    current: string;
    /** The pill that opened it. */
    anchor: DOMRect;
    /** A value was picked (never the current one). */
    onpick: (status: string) => void;
    /** `restoreFocus`: put focus back on the pill (Esc, Tab off an end, or the current value picked). */
    onclose: (restoreFocus: boolean) => void;
  }

  let { issueId, current, anchor, onpick, onclose }: Props = $props();

  const options = $derived(withCurrent(EDIT_STATUSES, current));
  const label = (s: string): string => PILL_NAME[s as PillStatus] ?? s;
  const glyph = (s: string): string => PILL_GLYPH[s as PillStatus] ?? "";

  let el = $state<HTMLElement>();

  // Placed like the priority popover: fixed, through CSSOM (no style attribute under the CSP),
  // below the pill or flipped above it, clamped into the window sideways.
  function place(): void {
    const target = el;
    if (!target) return;
    const a = anchor;
    const w = target.offsetWidth;
    const h = target.offsetHeight;
    const x = Math.max(8, Math.min(a.left, window.innerWidth - w - 8));
    let y = a.bottom + 4;
    if (y + h > window.innerHeight - 8 && a.top - 4 - h >= 8) y = a.top - 4 - h;
    target.style.setProperty("--pop-x", `${x}px`);
    target.style.setProperty("--pop-y", `${Math.max(8, y)}px`);
  }

  const rows = (): HTMLButtonElement[] => (el ? [...el.querySelectorAll<HTMLButtonElement>("button[role='option']")] : []);

  onMount(() => {
    place();
    (el?.querySelector<HTMLButtonElement>("button[aria-selected='true']") ?? rows()[0])?.focus({ preventScroll: true });
  });

  function onKeyDown(e: KeyboardEvent): void {
    const items = rows();
    const at = items.indexOf(document.activeElement as HTMLButtonElement);
    if (e.key === "Tab") {
      // The menu is the last thing in the page: Tab off either end closes it and hands focus back to the pill.
      if (items.length === 0 || (e.shiftKey ? at <= 0 : at === items.length - 1)) {
        e.preventDefault();
        onclose(true);
      }
    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const next = e.key === "ArrowDown" ? Math.min(items.length - 1, at + 1) : Math.max(0, at - 1);
      items[next]?.focus({ preventScroll: true });
    } else if (e.key === "Home" || e.key === "End") {
      e.preventDefault();
      items[e.key === "Home" ? 0 : items.length - 1]?.focus({ preventScroll: true });
    }
  }

  function onFocusOut(e: FocusEvent): void {
    const to = e.relatedTarget;
    if (!(to instanceof Element) || el?.contains(to) || to.closest("[data-status-anchor]")) return;
    onclose(false);
  }

  // Outside pointer, Esc, scroll and resize close it; a press on the pill is left to its own click, which toggles.
  $effect(() => {
    const onPointer = (e: PointerEvent) => {
      const t = e.target;
      if (!(t instanceof Element) || el?.contains(t) || t.closest("[data-status-anchor]")) return;
      onclose(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      onclose(true);
    };
    const onScroll = (e: Event) => {
      if (e.target instanceof Node && el?.contains(e.target)) return;
      onclose(false);
    };
    const onResize = () => onclose(false);
    document.addEventListener("pointerdown", onPointer, true);
    document.addEventListener("keydown", onKey, true);
    document.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      document.removeEventListener("pointerdown", onPointer, true);
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
    };
  });

  function pick(s: string): void {
    // Picking the current status does nothing, as the TUI does for an unchanged field.
    if (s === current) onclose(true);
    else onpick(s);
  }
</script>

<div
  class="pop"
  id="status-menu"
  role="listbox"
  aria-label="Status of {issueId}"
  tabindex="-1"
  onkeydown={onKeyDown}
  onfocusout={onFocusOut}
  bind:this={el}
>
  {#each options as s (s)}
    <button type="button" class="row" role="option" aria-selected={s === current} data-status={s} onclick={() => pick(s)}>
      <span class="g" aria-hidden="true">{glyph(s)}</span>
      <span class="l">{label(s)}</span>
      {#if s === current}
        <svg class="ck" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 6.4l2.4 2.4 4.6-5" /></svg>
      {/if}
    </button>
  {/each}
</div>

<style>
  .pop {
    position: fixed;
    left: var(--pop-x, 0px);
    top: var(--pop-y, 0px);
    z-index: 50;
    min-width: 180px;
    max-width: calc(100vw - 16px);
    max-height: 340px;
    overflow: auto;
    padding: 4px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 28px;
    padding: 0 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  .row:hover,
  .row:focus-visible {
    background: var(--bg-hover);
  }
  .g {
    width: 14px;
    text-align: center;
    color: var(--text-muted);
  }
  .l {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .ck {
    flex: none;
    color: var(--accent);
  }
  .row[data-status="open"] .g {
    color: var(--status-open);
  }
  .row[data-status="in_progress"] .g {
    color: var(--status-progress);
  }
  .row[data-status="blocked"] .g {
    color: var(--status-blocked);
  }
  .row[data-status="closed"] .g {
    color: var(--status-closed);
  }
  .row[data-status="deferred"] .g {
    color: var(--status-deferred);
  }
</style>
