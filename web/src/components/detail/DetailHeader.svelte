<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { absTime, PILL_GLYPH, PILL_NAME, pillStatus, relTime, shortDate } from "../../lib/issues/detail";
  import { editProblem, type IssueEdits } from "../../lib/issues/edits.svelte";
  import { toasts } from "../../lib/toasts.svelte";
  import PriorityPill from "../issues/PriorityPill.svelte";
  import TypeIcon from "../issues/TypeIcon.svelte";
  import EditButtons from "./EditButtons.svelte";
  import EditError from "./EditError.svelte";
  import { autofocus, refocus } from "./focus";
  import StatusMenu from "./StatusMenu.svelte";

  interface Props {
    /** The inline editors' state (title and status are edited here). */
    edits: IssueEdits;
    detail: IssueDetail;
    /** Wall-clock time for the relative "Updated ..." text; changes with every refetch. */
    now: number;
    /** A write on this issue is in flight. */
    busy: boolean;
    /** The tracker (or this issue) takes no writes. */
    readOnly: boolean;
    /** The priority popover is open from this pill. */
    prioOpen: boolean;
    /** The priority pill was pressed: open the popover at it. */
    onpickpriority: (anchor: HTMLElement) => void;
  }

  let { edits, detail, now, busy, readOnly, prioOpen, onpickpriority }: Props = $props();

  const status = $derived(pillStatus(detail, now));
  const parent = $derived(detail.parent);

  // ---- title: a pencil beside it opens an input with Save and Cancel (Enter saves, Esc cancels).
  const canEdit = $derived(!readOnly && edits.canEdit(detail.id));
  const titleEdit = $derived(readOnly ? undefined : edits.session(detail.id, "title"));
  const titleError = $derived(edits.error(detail.id, "title"));
  const titleSaving = $derived(edits.isSaving(detail.id, "title"));
  const titleProblem = $derived(titleEdit ? editProblem("title", titleEdit.draft) : "");
  let pencil = $state<HTMLButtonElement>();
  let titleInput = $state<HTMLInputElement>();

  function openTitle(): void {
    edits.open(detail.id, "title", detail);
  }

  /**
   * Save (Enter, the button), Retry, or Save mine: one path, so focus always ends up somewhere
   * sensible. A success hands it to the pencil; a failure leaves the editor open with the focused
   * button (Retry, Save mine) gone, so it goes back to the input. refocus() acts only when focus
   * has fallen to the page, never from where the user moved it.
   */
  async function commitTitle(mode: "save" | "mine" | "retry" = "save"): Promise<void> {
    if (mode === "save" && !edits.canSave(detail.id, "title")) return;
    const ok = await (mode === "mine" ? edits.saveMine(detail.id, "title") : edits.save(detail.id, "title"));
    void refocus(() => (ok ? pencil : titleInput));
  }

  function cancelTitle(): void {
    edits.cancel(detail.id, "title");
    void refocus(() => pencil);
  }

  // ---- status: the pill is a button that opens a menu; picking a status saves it at once.
  let menu = $state.raw<{ anchor: DOMRect; el: HTMLElement } | null>(null);
  const statusSaving = $derived(edits.isSaving(detail.id, "status"));
  const statusEdit = $derived(readOnly ? undefined : edits.session(detail.id, "status"));
  const statusError = $derived(edits.error(detail.id, "status"));

  // The menu goes with its issue and with write access.
  $effect(() => {
    if (readOnly) menu = null;
  });
  $effect(() => {
    void detail.id;
    menu = null;
  });

  function toggleMenu(el: HTMLElement): void {
    if (!canEdit) return;
    menu = menu && menu.el === el ? null : { anchor: el.getBoundingClientRect(), el };
  }

  function closeMenu(restore: boolean): void {
    const m = menu;
    menu = null;
    if (restore) m?.el.focus({ preventScroll: true });
  }

  function pickStatus(value: string): void {
    const m = menu;
    closeMenu(true);
    void edits.quick(detail.id, "status", value).then(() => refocus(() => m?.el));
  }

  /** Retry or Save mine on the status panel; focus goes back to the pill (the panel's button is gone). */
  async function commitStatus(mode: "mine" | "retry"): Promise<void> {
    await (mode === "mine" ? edits.saveMine(detail.id, "status") : edits.save(detail.id, "status"));
    void refocus(() => document.querySelector<HTMLElement>("button[data-status-anchor]"));
  }

  async function copyId(): Promise<void> {
    try {
      await navigator.clipboard.writeText(detail.id);
      toasts.push("success", `Copied ${detail.id}`);
    } catch {
      toasts.push("danger", "Couldn't copy the id.");
    }
  }
</script>

<header class="head">
  {#if parent}
    <nav class="crumb" aria-label="Parent">
      {#if parent.missing}
        <span class="mono">{parent.id}</span>
      {:else}
        <a href="/issues/{encodeURIComponent(parent.id)}" title={parent.title ?? parent.id}>{parent.title ?? parent.id}</a>
        <span class="mono">{parent.id}</span>
      {/if}
    </nav>
  {/if}

  <div class="titlerow">
    <span class="ticon"><TypeIcon type={detail.issue_type} /></span>
    {#if titleEdit}
      <form
        class="titleedit"
        onsubmit={(e) => {
          e.preventDefault();
          void commitTitle();
        }}
      >
        <!-- readonly, not disabled, while a write runs: a disabled input drops the keyboard focus. -->
        <input
          class="title-input"
          type="text"
          aria-label="Title"
          autocomplete="off"
          spellcheck="true"
          value={titleEdit.draft}
          readonly={busy}
          aria-busy={titleSaving ? "true" : undefined}
          bind:this={titleInput}
          use:autofocus
          oninput={(e) => edits.setDraft(detail.id, "title", e.currentTarget.value)}
          onkeydown={(e) => {
            if (e.key === "Escape" && !busy) {
              e.preventDefault();
              cancelTitle();
            }
          }}
        />
        <EditButtons saving={titleSaving} disabled={!edits.canSave(detail.id, "title")} {busy} oncancel={cancelTitle} />
        {#if titleProblem}<p class="hint">{titleProblem}</p>{/if}
        {#if titleError}
          <EditError
            error={titleError}
            draft={titleEdit.draft}
            busy={busy}
            onretry={() => void commitTitle(titleError.kind === "conflict" ? "mine" : "retry")}
            ondismiss={() => (titleError.kind === "conflict" ? cancelTitle() : edits.dismissError(detail.id, "title"))}
          />
        {/if}
      </form>
    {:else}
      <h1 class="title" tabindex="-1">{detail.title}</h1>
      {#if !readOnly}
        <button
          type="button"
          class="pencil"
          bind:this={pencil}
          aria-label="Edit title"
          title="Edit title"
          aria-disabled={canEdit ? undefined : "true"}
          onclick={() => {
            if (canEdit) openTitle();
          }}
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 13.5l.7-3L11 2.7a1.4 1.4 0 012 2L5.2 12.8z" /><path d="M9.8 3.9l2.3 2.3" /></svg>
        </button>
      {/if}
    {/if}
    <span class="idg">
      {detail.id}
      <button type="button" class="iconbtn" aria-label="Copy id" title="Copy id" onclick={() => void copyId()}>
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="5" width="8" height="8" rx="1.5" /><path d="M3 10V4a1 1 0 011-1h6" /></svg>
      </button>
    </span>
  </div>

  <div class="pillrow">
    {#if readOnly}
      <span class="pill st" data-status={status}>
        {#if PILL_GLYPH[status]}<span aria-hidden="true">{PILL_GLYPH[status]}</span>{/if}{PILL_NAME[status]}
      </span>
    {:else}
      <!-- A busy or held pill is aria-disabled, not disabled, so the focus a closed menu hands back stays. -->
      <button
        type="button"
        class="pill st stbtn"
        data-status={status}
        data-status-anchor
        aria-haspopup="listbox"
        aria-expanded={menu !== null}
        aria-controls={menu ? "status-menu" : undefined}
        aria-label="Status {PILL_NAME[status]}, change"
        aria-busy={statusSaving ? "true" : undefined}
        aria-disabled={canEdit ? undefined : "true"}
        title="Change status"
        onclick={(e) => toggleMenu(e.currentTarget)}
      >
        {#if statusSaving}<span class="spin" aria-hidden="true"></span>{:else if PILL_GLYPH[status]}<span aria-hidden="true">{PILL_GLYPH[status]}</span>{/if}{PILL_NAME[status]}
        <svg class="chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 3.5l3 3 3-3" /></svg>
      </button>
    {/if}
    <PriorityPill priority={detail.priority} {busy} expanded={prioOpen} disabled={readOnly} onpick={onpickpriority} />
    <span class="dt" title={absTime(detail.updated_at)}>Updated {relTime(detail.updated_at, now)}</span>
    {#if detail.status === "closed" && detail.closed_at}
      <span class="dt" title={absTime(detail.closed_at)}>· Closed {shortDate(detail.closed_at)}</span>
    {/if}
  </div>
  {#if statusEdit && statusError}
    <EditError
      error={statusError}
      draft={statusEdit.draft}
      {busy}
      onretry={() => void commitStatus(statusError.kind === "conflict" ? "mine" : "retry")}
      ondismiss={() => edits.cancel(detail.id, "status")}
    />
  {/if}
</header>

{#if menu && !readOnly}
  <StatusMenu issueId={detail.id} current={detail.status} anchor={menu.anchor} onpick={pickStatus} onclose={closeMenu} />
{/if}

<style>
  .crumb {
    font-size: var(--text-sm);
    color: var(--text-muted);
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
    margin-bottom: var(--space-2);
  }
  .crumb .mono {
    flex: 0 0 auto;
    white-space: nowrap;
    font-family: var(--font-mono);
  }
  .crumb a {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 60ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-block;
    color: var(--text-muted);
    text-decoration: none;
    vertical-align: bottom;
  }
  .crumb a:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .titlerow {
    display: flex;
    align-items: flex-start;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .ticon {
    flex: none;
    margin-top: 6px;
    display: inline-flex;
  }
  .title {
    flex: 1 1 0;
    min-width: 0;
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
    line-height: 1.3;
    overflow-wrap: anywhere;
  }
  .title:focus {
    outline: none;
  }
  .pencil {
    flex: none;
    width: 28px;
    height: 28px;
    margin-top: 1px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
  }
  .titlerow:hover .pencil,
  .pencil:focus-visible {
    opacity: 1;
  }
  @media (hover: none) {
    .pencil {
      opacity: 1;
    }
  }
  .pencil:hover {
    background: var(--bg-hover);
  }
  .pencil[aria-disabled="true"] {
    cursor: not-allowed;
  }
  .titleedit {
    flex: 1 1 0;
    min-width: 0;
  }
  .title-input {
    display: block;
    width: 100%;
    margin: 0;
    padding: 2px 8px;
    border: 1px solid var(--accent);
    border-radius: var(--radius-md);
    background: var(--bg);
    color: inherit;
    font: inherit;
    font-size: var(--text-xl);
    font-weight: 600;
    line-height: 1.3;
  }
  .title-input:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--accent) 35%, transparent);
    outline-offset: 0;
  }
  .title-input:disabled {
    opacity: 0.7;
  }
  .hint {
    margin: 4px 0 0;
    font-size: var(--text-xs);
    color: var(--danger);
  }
  .idg {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    margin-top: 5px;
    margin-left: auto;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .iconbtn {
    border: 0;
    background: transparent;
    width: 24px;
    height: 24px;
    border-radius: var(--radius-sm);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    cursor: pointer;
  }
  .iconbtn:hover {
    background: var(--bg-hover);
  }
  @media (max-width: 479px) {
    .idg {
      flex-basis: 100%;
      margin: 0 0 0 24px;
    }
  }
  .pillrow {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }
  /* PriorityPill sits last in a list row (order: 9); here it keeps the DOM order: status, priority, updated. */
  .pillrow :global(.pp) {
    order: 0;
  }
  .dt {
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .pill {
    --c: var(--text-muted);
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 22px;
    padding: 0 var(--space-2);
    border-radius: 11px;
    font-size: var(--text-sm);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 14%, transparent);
    border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
  }
  .stbtn {
    font-family: inherit;
    cursor: pointer;
  }
  .stbtn:hover:not([aria-disabled="true"]) {
    border-color: color-mix(in srgb, var(--c) 60%, transparent);
  }
  .stbtn[aria-disabled="true"] {
    cursor: not-allowed;
    opacity: 0.7;
  }
  .stbtn[aria-busy="true"] {
    cursor: progress;
  }
  .stbtn .chev {
    flex: none;
    opacity: 0.8;
  }
  .spin {
    width: 10px;
    height: 10px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin {
      animation-duration: 2s;
    }
  }
  .pill[data-status="open"] {
    --c: var(--status-open);
  }
  .pill[data-status="in_progress"] {
    --c: var(--status-progress);
  }
  .pill[data-status="blocked"] {
    --c: var(--status-blocked);
  }
  .pill[data-status="review"] {
    --c: var(--status-review);
  }
  .pill[data-status="closed"] {
    --c: var(--status-closed);
  }
  .pill[data-status="deferred"] {
    --c: var(--status-deferred);
  }
  .pill[data-status="pinned"] {
    --c: var(--status-pinned);
  }
</style>
