<script lang="ts">
  import { tick, untrack } from "svelte";
  import type { ListItem } from "../../lib/api/types";
  import { answerKey, noteKey, optionKey, type IssueActions } from "../../lib/issues/actions.svelte";
  import { awaitsSignoff, canAnswer, displayStatus } from "../../lib/issues/model";
  import { callView, childProgress, signoffLabel, signoffTone, topReasons, type PeekSection } from "../../lib/issues/peek";
  import { PeekLoader } from "../../lib/issues/peek.svelte";
  import { percent, STATUS_NAME, triageText } from "../../lib/issues/present";
  import { refocusForm } from "../detail/focus";
  import DecisionForm from "./DecisionForm.svelte";
  import PriorityPill from "./PriorityPill.svelte";
  import RefRow from "./RefRow.svelte";
  import SignoffActions from "./SignoffActions.svelte";
  import StatusGlyph from "./StatusGlyph.svelte";

  interface Props {
    id: string;
    /** The list row, when the issue is in the list: it paints the drawer before the detail arrives. */
    item: ListItem | undefined;
    /** A block to scroll to and flash (opened from a row's "Sign off" / "Answer" chip). */
    section: PeekSection | null;
    now: number;
    /** Changes when the list data is refreshed (Phase 2 landed): the detail is fetched again. */
    reloadKey: unknown;
    onclose: () => void;
    /** Open another issue in the drawer (a blocker, a child). */
    onopen: (id: string) => void;
    /** The issue is gone from the tracker (404). */
    onmissing: (id: string) => void;
    /** The quick actions: their state (busy, drafts, the open dispute form) and calls. */
    actions: IssueActions;
    /** The tracker takes writes (meta.capabilities.writes). */
    writes: boolean;
    /** The priority pill was pressed: open the row actions popover at it. */
    onpickpriority: (id: string, anchor: HTMLElement) => void;
    /** The priority popover is open from this drawer's pill. */
    prioOpen: boolean;
  }

  let { id, item, section, now, reloadKey, onclose, onopen, onmissing, actions, writes, onpickpriority, prioOpen }: Props = $props();

  const loader = new PeekLoader();
  let root = $state<HTMLElement>();
  let heading = $state<HTMLElement>();
  let highlighted = $state<PeekSection | null>(null);
  let scrolledFor = "";
  let focusedFor = "";
  let hlTimer: ReturnType<typeof setTimeout> | undefined;

  /** Below 1280px the drawer is an overlay or sheet, so it takes focus; the push panel leaves focus in the list. */
  const overlayMode = () => matchMedia("(max-width: 1279px)").matches;

  let firstLoad = true;
  let lastId = "";
  $effect(() => {
    const current = id;
    void reloadKey;
    untrack(() => {
      loader.load(current);
      // Only a different issue moves focus to the heading: a data refresh (every live update)
      // must not pull it out of a textarea the user is typing in.
      const changed = current !== lastId;
      lastId = current;
      // Another issue opened from inside an overlay drawer (a ref click): the clicked button is
      // gone, so keep focus in the drawer.
      const holdsFocus = !!root && (document.activeElement === document.body || root.contains(document.activeElement));
      if (!firstLoad && changed && overlayMode() && holdsFocus) heading?.focus({ preventScroll: true });
      firstLoad = false;
    });
  });

  // Focus: move into the drawer on open (overlay/sheet only), and hand it back when the user
  // closes it (X or Esc), so a keyboard user is not dropped on <body> when the focused element
  // unmounts. Not done in a teardown: by then the focused node is already gone.
  let opener: HTMLElement | null = null;
  $effect(() => {
    opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (overlayMode()) heading?.focus({ preventScroll: true });
  });

  /**
   * A write that succeeds can remove the control that had focus (the dispute form, the answer
   * form): focus would fall to <body>. Park it on the drawer heading, so a keyboard user stays
   * in the drawer. One that fails leaves the form, but the button that was pressed was disabled
   * while the write ran, so focus fell too: put it back in that section's form.
   */
  async function keepFocus(write: Promise<boolean>, sec: PeekSection): Promise<void> {
    const ok = await write;
    await tick();
    // Only when it really fell: if the user moved focus elsewhere meanwhile, leave it.
    if (document.activeElement && document.activeElement !== document.body) return;
    if (ok) heading?.focus({ preventScroll: true });
    else refocusForm(root?.querySelector(`[data-sec="${sec}"]`));
  }

  async function close(): Promise<void> {
    const hadFocus = !!root?.contains(document.activeElement);
    const peeked = id;
    onclose();
    if (!hadFocus) return;
    await tick();
    restoreFocus(opener, peeked);
  }
  function restoreFocus(opener: HTMLElement | null, peeked: string): void {
    const target =
      (opener && opener !== document.body && opener.isConnected ? opener : null) ??
      document.querySelector<HTMLElement>(`[data-row][data-id="${CSS.escape(peeked)}"] .t-btn`) ??
      document.querySelector<HTMLElement>('[role="region"][aria-label="Issues"]');
    target?.focus({ preventScroll: true });
  }

  $effect(() => {
    if (loader.status === "missing") untrack(() => onmissing(id));
  });

  $effect(() => () => {
    loader.stop();
    clearTimeout(hlTimer);
  });

  // Esc closes the drawer, unless something else (an open filter menu) already took it.
  $effect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented) void close();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  const detail = $derived(loader.detail && loader.detail.id === id ? loader.detail : null);
  const base = $derived(detail ?? item);
  const title = $derived(detail?.title ?? item?.title ?? "");
  const status = $derived(base ? displayStatus(base, now) : "open");
  const labels = $derived(base?.labels ?? []);
  // The list item is fresher than the detail, which keeps the old copy until its own refetch
  // lands: after a write the list says "done" first. Gate the actions on it, so a write that
  // just succeeded is not offered again.
  const signoffState = $derived(item ? (item.signoff ?? "") : (detail?.signoff?.state ?? ""));
  // The server sends no decision for a closed issue even if it still carries the label.
  const hasDecision = $derived(item ? canAnswer(item) : !!detail?.decision);
  // Writes need a writable tracker, and an issue whose own route is writable (the detail says why not).
  const readOnly = $derived(!writes || !!detail?.read_only_reason);
  const busy = $derived(actions.isBusy(id));
  const awaitsSignoffNow = $derived(item ? awaitsSignoff(item) : base?.status === "closed" && signoffState === "pending");
  const progress = $derived(childProgress(detail?.children ?? []));
  const reasons = $derived(topReasons(detail?.triage));

  // Scroll the requested block into view once, and flash it.
  $effect(() => {
    const s = section;
    void detail;
    if (!s) {
      scrolledFor = "";
      focusedFor = "";
      return;
    }
    if (!root) return;
    const key = `${id}:${s}`;
    const el = root.querySelector(`[data-sec="${s}"]`);
    if (!el) return;
    if (scrolledFor !== key) {
      scrolledFor = key;
      el.scrollIntoView({ block: "nearest" });
      highlighted = s;
      clearTimeout(hlTimer);
      hlTimer = setTimeout(() => (highlighted = null), 1300);
    }
    // Opened from the row menu (Dispute..., Answer...): put the cursor in the form, once its
    // textarea exists (the answer form waits for the detail). element.focus(), not autofocus,
    // which does nothing on a re-render.
    if (focusedFor !== key) {
      const ta = el.querySelector<HTMLTextAreaElement>("textarea:not(:disabled)");
      if (ta) {
        focusedFor = key;
        ta.focus({ preventScroll: true });
      }
    }
  });
</script>

<aside class="drawer" aria-label="Issue details" bind:this={root}>
  <div class="dh">
    <div class="ttl">
      <div class="id">{id}</div>
      <h2 tabindex="-1" bind:this={heading}>{title || (loader.status === "loading" ? "Loading…" : id)}</h2>
    </div>
    <button type="button" class="x" aria-label="Close details" onclick={() => void close()}>✕</button>
  </div>

  {#if detail?.read_only_reason}
    <div class="note-ro">{detail.read_only_reason}</div>
  {/if}

  <div class="db">
    {#if loader.status === "missing"}
      <!-- One-frame fallback: onmissing closes the peek and Issues shows the real notice. -->
      <p class="hint">This issue is no longer in the tracker.</p>
    {:else if base}
      <div class="sec">
        <dl class="fields">
          <dt>Status</dt>
          <dd>
            <StatusGlyph {status} />
            {STATUS_NAME[status]}
            {#if base.ready && base.status === "open" && base.issue_type !== "epic"}<span class="tag ready">Ready</span>{/if}
          </dd>
          <dt>Priority</dt>
          <dd>
            <PriorityPill
              priority={item?.priority ?? base.priority}
              {busy}
              expanded={prioOpen}
              disabled={readOnly}
              onpick={(el) => onpickpriority(id, el)}
            />
          </dd>
          <dt>Type</dt>
          <dd>{base.issue_type}</dd>
          <dt>Assignee</dt>
          <dd>{#if base.assignee}{base.assignee}{:else}<span class="faint">Unassigned</span>{/if}</dd>
          <dt>Labels</dt>
          <dd>
            {#each labels as l (l)}<span class="lbl">{l}</span>{:else}<span class="faint">None</span>{/each}
          </dd>
        </dl>
      </div>
    {/if}

    {#if signoffState !== ""}
      <div class="sec" class:hl={highlighted === "signoff"} data-sec="signoff">
        <h3>Sign-off</h3>
        <span class="badge" data-tone={signoffTone(signoffState)}>{signoffLabel(signoffState)}</span>
        {#if awaitsSignoffNow}
          <SignoffActions
            issueId={id}
            {busy}
            disabled={readOnly}
            open={actions.disputeOpen === id}
            note={actions.getDraft(noteKey(id))}
            onapprove={() => void keepFocus(actions.approve(id), "signoff")}
            onopen={() => actions.openDispute(id)}
            oncancel={() => actions.closeDispute()}
            onnote={(v) => actions.setDraft(noteKey(id), v)}
            ondispute={(note) => void keepFocus(actions.dispute(id, note), "signoff")}
          />
        {/if}
        {#if detail?.signoff}
          {@const so = detail.signoff}
          {#if so.what_changed?.length}
            <p class="lead">What changed</p>
            <ul class="plain">{#each so.what_changed as w, i (i)}<li>{w}</li>{/each}</ul>
          {/if}
          {#if so.calls.length > 0}
            <p class="lead">Calls made</p>
            <ul class="plain">
              {#each so.calls as c, i (i)}
                {@const v = callView(c)}
                <li>
                  {#if v.bold}<strong>{c.call}</strong>{:else}{c.call}{/if}
                  {#if v.alternative}<span class="faint"> (over {v.alternative})</span>{/if}
                  {#if v.why}<div class="faint">{v.why}</div>{/if}
                </li>
              {/each}
            </ul>
          {:else if so.calls_made?.length}
            <p class="lead">Calls made</p>
            <ul class="plain">{#each so.calls_made as c, i (i)}<li>{c}</li>{/each}</ul>
          {/if}
          {#if so.removed_or_renamed?.length}
            <p class="lead">Removed / renamed</p>
            <ul class="plain">{#each so.removed_or_renamed as w, i (i)}<li>{w}</li>{/each}</ul>
          {/if}
          {#if so.where_to_see?.length}
            <p class="lead">Where to see it</p>
            <ul class="plain">{#each so.where_to_see as w, i (i)}<li>{w}</li>{/each}</ul>
          {/if}
        {/if}
      </div>
    {/if}
    {#if hasDecision}
      <div class="sec" class:hl={highlighted === "answer"} data-sec="answer">
        <h3>Needs your decision</h3>
        {#if detail?.decision}
          {@const dc = detail.decision}
          <p class="q">{dc.question ?? title}</p>
          {#if dc.waiting_on}<p class="faint">Waiting on: {dc.waiting_on}</p>{/if}
          <DecisionForm
            issueId={id}
            decision={dc}
            {busy}
            disabled={readOnly}
            option={actions.getDraft(optionKey(id))}
            text={actions.getDraft(answerKey(id))}
            onoption={(k) => actions.setDraft(optionKey(id), k)}
            ontext={(v) => actions.setDraft(answerKey(id), v)}
            onanswer={(a) => void keepFocus(actions.answer(id, a), "answer")}
          />
        {:else if loader.status === "loading"}
          <p class="faint">Loading…</p>
        {/if}
      </div>
    {/if}

    {#if detail}
      {#if detail.blocked_by.length > 0}
        <div class="sec">
          <h3>Blocked by</h3>
          {#each detail.blocked_by as r (r.id)}<RefRow ref={r} muted={!r.active} {onopen} />{/each}
        </div>
      {/if}
      {#if detail.blocks.length > 0}
        <div class="sec">
          <h3>Blocks</h3>
          {#each detail.blocks as r (r.id)}<RefRow ref={r} muted={!r.active} {onopen} />{/each}
        </div>
      {/if}
      {#if detail.children.length > 0}
        <div class="sec">
          <h3>Children <span class="faint">{progress.done}/{progress.total} closed</span></h3>
          <span class="prog" aria-hidden="true"><i class="pct-{percent(progress.done, progress.total)}"></i></span>
          {#each detail.children as r (r.id)}<RefRow ref={r} muted={r.status === "closed"} {onopen} />{/each}
        </div>
      {/if}
      {#if detail.triage}
        <div class="sec">
          <h3>Triage <span class="mono">{triageText(detail.triage.score)}</span></h3>
          {#if reasons.length > 0}
            <ul class="reasons">{#each reasons as r, i (i)}<li>{r}</li>{/each}</ul>
          {/if}
        </div>
      {/if}
    {:else if loader.status === "loading"}
      <p class="hint">Loading details…</p>
    {:else if loader.status === "error"}
      <p class="hint">Couldn't load details{loader.error ? `: ${loader.error.message}` : ""}.</p>
      <button type="button" class="retry" onclick={() => loader.load(id)}>Retry</button>
    {/if}
  </div>

  <div class="df"><a href="/issues/{encodeURIComponent(id)}">Open full view</a></div>
</aside>

<style>
  .drawer {
    background: var(--bg-raised);
    border-left: 1px solid var(--border);
    box-shadow: var(--shadow-pop);
    position: fixed;
    right: 0;
    top: calc(var(--header-h) + env(safe-area-inset-top, 0px));
    bottom: 0;
    width: 420px;
    max-width: 100vw;
    z-index: 30;
    display: flex;
    flex-direction: column;
  }
  /* Wide windows: the drawer pushes the list instead of covering it. */
  @media (min-width: 1280px) {
    .drawer {
      position: static;
      flex: none;
      min-height: 0;
      box-shadow: none;
      margin-left: var(--space-4);
    }
  }
  /* Phones: a full-screen sheet. */
  @media (max-width: 639px) {
    .drawer {
      width: 100vw;
      border-left: 0;
    }
  }
  .dh {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
    display: flex;
    gap: 8px;
    align-items: flex-start;
  }
  .ttl {
    flex: 1;
    min-width: 0;
  }
  .id {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  h2 {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .x {
    width: 28px;
    height: 28px;
    border: 0;
    background: none;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 16px;
    cursor: pointer;
  }
  .x:hover {
    background: var(--bg-hover);
  }
  .note-ro {
    padding: 8px 16px;
    font-size: var(--text-sm);
    background: var(--warning-subtle);
    color: var(--warning);
    border-bottom: 1px solid var(--border);
  }
  .db {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 4px 16px 16px;
  }
  .df {
    border-top: 1px solid var(--border);
    padding: 8px 16px;
    font-size: var(--text-sm);
  }
  .sec {
    padding: 12px 0;
    border-bottom: 1px solid var(--border);
  }
  .sec:last-child {
    border-bottom: 0;
  }
  .sec.hl {
    animation: flash 1.2s ease-out;
  }
  @keyframes flash {
    0%,
    70% {
      background: var(--bg-selected);
    }
    100% {
      background: transparent;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sec.hl {
      animation: none;
    }
  }
  h3 {
    margin: 0 0 8px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-muted);
  }
  .fields {
    display: grid;
    grid-template-columns: 90px 1fr;
    gap: 6px 8px;
    font-size: var(--text-sm);
    align-items: center;
    margin: 0;
  }
  .fields dt {
    color: var(--text-muted);
  }
  .fields dd {
    margin: 0;
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
    align-items: center;
  }
  /* The shared cells are laid out by order inside a list row; here they sit in a plain flex line. */
  .fields :global(.pp),
  .fields :global(.sg) {
    order: 0;
  }
  .faint {
    color: var(--text-faint);
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .tag {
    font-size: var(--text-xs);
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
  }
  .tag.ready {
    background: var(--success-subtle);
    color: var(--success);
  }
  .lbl {
    height: 18px;
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    color: var(--text-muted);
  }
  .badge {
    display: inline-block;
    padding: 0 8px;
    border-radius: 10px;
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 20px;
    background: var(--accent-subtle);
    color: var(--accent);
  }
  .badge[data-tone="ok"] {
    background: var(--success-subtle);
    color: var(--success);
  }
  .badge[data-tone="bad"] {
    background: var(--danger-subtle);
    color: var(--danger);
  }
  .plain {
    margin: 8px 0 0;
    padding-left: 18px;
    font-size: var(--text-sm);
  }
  .lead {
    margin: 8px 0 0;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .q {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .prog {
    display: block;
    width: 100%;
    height: 4px;
    margin-bottom: 6px;
    border-radius: 2px;
    background: var(--border);
    overflow: hidden;
  }
  .prog i {
    display: block;
    height: 100%;
    background: var(--success);
  }
  .reasons {
    margin: 4px 0 0;
    padding-left: 18px;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .retry {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .retry:hover {
    background: var(--bg-hover);
  }
  .hint {
    font-size: var(--text-sm);
    color: var(--text-muted);
    margin: 12px 0;
  }
</style>
