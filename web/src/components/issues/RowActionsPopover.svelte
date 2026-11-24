<script lang="ts">
  import { onMount, untrack } from "svelte";
  import type { ListItem } from "../../lib/api/types";
  import { actionState, ANSWER_ONLY_TITLE, SIGNOFF_ONLY_TITLE } from "../../lib/issues/writes";

  interface Props {
    /** "prio": the priority pills only. "menu": the pills plus close-candidate, sign-off and answer. */
    kind: "prio" | "menu";
    item: Pick<ListItem, "id" | "priority" | "status" | "labels" | "signoff">;
    /** The pressed pill or "..." button, to open next to. */
    anchor: DOMRect;
    /** The tracker takes writes. The popover is only opened for writable trackers; this keeps its buttons honest. */
    writes: boolean;
    /** `restoreFocus`: put focus back on the button that opened it (Esc, or a choice made). */
    onclose: (restoreFocus: boolean) => void;
    onpriority: (priority: number) => void;
    oncandidate: (on: boolean) => void;
    onapprove: () => void;
    /** Opens the drawer at the dispute note form. */
    ondispute: () => void;
    /** Opens the drawer at the answer form. */
    onanswer: () => void;
  }

  let { kind, item, anchor, writes, onclose, onpriority, oncandidate, onapprove, ondispute, onanswer }: Props = $props();

  const st = $derived(actionState(item, writes));

  let el = $state<HTMLElement>();

  // Fixed position, set through CSSOM (el.style.setProperty), not a style attribute, so the
  // CSP stays default-src 'self'. Below the anchor, flipped above when it would run off the
  // bottom, and clamped into the window sideways. Measured with the content in. Effects run
  // before paint, so the popover is never seen at 0,0.
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

  // Placed first, then focused: focus() does nothing on an element that is not rendered where
  // it can take it. A new popover (another row or kind) is a new instance (see {#key} in
  // Issues.svelte), so this runs once per opening: the current priority pill for "prio"
  // (where the TUI cursor starts), else the first usable control.
  onMount(() => {
    place();
    const target = el;
    const first =
      (kind === "prio" ? target?.querySelector<HTMLButtonElement>(".rp.cur:not(:disabled)") : null) ??
      target?.querySelector<HTMLButtonElement>("button:not(:disabled)");
    first?.focus({ preventScroll: true });
  });

  // The content changed size (a priority landed, the label toggled): place it again.
  $effect(() => {
    void item;
    void anchor;
    untrack(place);
  });

  // Tabbing out of it closes it; a click elsewhere is the pointerdown handler's. The popover is
  // the last thing in the page, so Tab from its last control would wrap to the browser's own UI
  // and leave it open behind the focus: Tab off either end closes it and hands focus back to the
  // button that opened it, where the next Tab carries on through the list.
  function onKeyDown(e: KeyboardEvent): void {
    if (e.key !== "Tab" || !el) return;
    const items = [...el.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")];
    const at = items.indexOf(document.activeElement as HTMLButtonElement);
    if (items.length === 0 || (e.shiftKey ? at <= 0 : at === items.length - 1)) {
      e.preventDefault();
      onclose(true);
    }
  }

  // Focus that lands somewhere else by other means (a script, a click on a control) closes it too.
  function onFocusOut(e: FocusEvent): void {
    const to = e.relatedTarget;
    if (!(to instanceof Element) || el?.contains(to) || to.closest("[data-pop-anchor]")) return;
    onclose(false);
  }

  // Outside pointer, Esc and scrolling close it. A press on the buttons that open it is left to
  // their own click, which toggles. Esc is handled in the capture phase and marked handled, so
  // the drawer's Esc handler leaves this press alone.
  $effect(() => {
    const onPointer = (e: PointerEvent) => {
      const t = e.target;
      if (!(t instanceof Element) || el?.contains(t) || t.closest("[data-pop-anchor]")) return;
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

  function pick(p: number): void {
    // The TUI does nothing for the priority the issue already has.
    if (p === item.priority) onclose(true);
    else onpriority(p);
  }
</script>

<div class="pop" id="row-actions-pop" role="dialog" aria-label="Actions for {item.id}" tabindex="-1" onkeydown={onKeyDown} onfocusout={onFocusOut} bind:this={el}>
  <div class="hd">{kind === "prio" ? `Priority of ${item.id}` : "Priority"}</div>
  <div class="pills">
    {#each [0, 1, 2, 3, 4] as p (p)}
      <button
        type="button"
        class="rp"
        class:cur={p === item.priority}
        data-p={p}
        aria-pressed={p === item.priority}
        aria-label="Priority P{p}"
        disabled={!st.priority}
        onclick={() => pick(p)}>P{p}</button
      >
    {/each}
  </div>
  {#if kind === "menu"}
    <hr />
    <button type="button" class="opt" disabled={!st.closeCandidate} onclick={() => oncandidate(!st.isCandidate)}>
      {st.isCandidate ? "Remove close-candidate" : "Mark close-candidate"}
    </button>
    <!-- A refused option is aria-disabled, not disabled: it stays reachable by keyboard, so its reason
         (the tooltip, and the hidden text that aria-describedby names) is announced. -->
    <button
      type="button"
      class="opt"
      aria-disabled={st.approve ? undefined : "true"}
      aria-describedby={st.approve ? undefined : "pop-why-signoff"}
      title={st.approve ? "" : SIGNOFF_ONLY_TITLE}
      onclick={() => {
        if (st.approve) onapprove();
      }}
    >
      Approve
    </button>
    <button
      type="button"
      class="opt"
      aria-disabled={st.dispute ? undefined : "true"}
      aria-describedby={st.dispute ? undefined : "pop-why-signoff"}
      title={st.dispute ? "" : SIGNOFF_ONLY_TITLE}
      onclick={() => {
        if (st.dispute) ondispute();
      }}
    >
      Dispute…
    </button>
    <button
      type="button"
      class="opt"
      aria-disabled={st.answer ? undefined : "true"}
      aria-describedby={st.answer ? undefined : "pop-why-answer"}
      title={st.answer ? "" : ANSWER_ONLY_TITLE}
      onclick={() => {
        if (st.answer) onanswer();
      }}
    >
      Answer…
    </button>
    {#if !st.approve || !st.dispute}<span class="sr" id="pop-why-signoff">{SIGNOFF_ONLY_TITLE}</span>{/if}
    {#if !st.answer}<span class="sr" id="pop-why-answer">{ANSWER_ONLY_TITLE}</span>{/if}
  {/if}
</div>

<style>
  .pop {
    position: fixed;
    left: var(--pop-x, 0px);
    top: var(--pop-y, 0px);
    z-index: 50;
    min-width: 190px;
    max-width: calc(100vw - 16px);
    max-height: 340px;
    overflow: auto;
    padding: 6px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
  }
  .hd {
    padding: 2px 8px;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .pills {
    display: flex;
    gap: 4px;
    padding: 4px 8px 6px;
  }
  .rp {
    width: 32px;
    height: 22px;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 20px;
    text-align: center;
    cursor: pointer;
  }
  .rp[data-p="0"] {
    background: var(--danger);
    border-color: var(--danger);
    color: var(--on-accent);
  }
  .rp[data-p="1"] {
    background: var(--warning-subtle);
    color: var(--warning);
  }
  .rp[data-p="2"] {
    background: var(--bg-subtle);
  }
  .rp:hover:not(:disabled) {
    border-color: var(--border-strong);
  }
  .rp.cur {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }
  .rp:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  hr {
    margin: 4px 0;
    border: 0;
    border-top: 1px solid var(--border);
  }
  .opt {
    display: flex;
    align-items: center;
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
  .opt:hover:not(:disabled):not([aria-disabled="true"]) {
    background: var(--bg-hover);
  }
  .opt:disabled,
  .opt[aria-disabled="true"] {
    color: var(--text-faint);
    cursor: not-allowed;
  }
  .opt:focus-visible {
    background: var(--bg-hover);
  }
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
