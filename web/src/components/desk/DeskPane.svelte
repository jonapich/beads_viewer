<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { isLongText, judgeLink, type DeskTab, type Outcome, type RowKind } from "../../lib/desk/desk";
  import type { IssueActions } from "../../lib/issues/actions.svelte";
  import { actionTarget, linkHost } from "../../lib/issues/detail";
  import { awaitsSignoff, canAnswer } from "../../lib/issues/model";
  import CommentItem from "../detail/CommentItem.svelte";
  import DecisionCard from "../detail/DecisionCard.svelte";
  import InlineText from "../detail/InlineText.svelte";
  import Markdown from "../detail/Markdown.svelte";
  import ResolvedNote from "../detail/ResolvedNote.svelte";
  import SignoffCard from "../detail/SignoffCard.svelte";
  import SkeletonRows from "../work/SkeletonRows.svelte";

  interface Props {
    tab: DeskTab;
    /** The selected id; the detail is only shown while it is this one. */
    id: string;
    detail: IssueDetail | null;
    /** The detail request failed and there is nothing to show (Retry). */
    failed: string | null;
    /** The id is not in the list any more (answered elsewhere), so nothing was loaded for it. */
    vanished: boolean;
    actions: IssueActions;
    /** The tracker (or this issue) takes no writes. */
    readOnly: boolean;
    /** The row's kind in the list: the bead's labels no longer say once it was answered. */
    rowKind: RowKind | undefined;
    /** What the user did with this row this visit. */
    outcome: Outcome | undefined;
    /** A card write on this row is in flight or just landed and is not marked yet. */
    writing: boolean;
    now: number;
    /** A card write started: the view moves the selection on once it landed. */
    onwrite: (write: Promise<boolean>, kind: "answered" | "approved" | "disputed") => void;
    /** Select the next row not handled yet (also the sign-off Skip). */
    onnext: () => void;
    onretry: () => void;
  }

  let { tab, id, detail, failed, vanished, actions, readOnly, rowKind, outcome, writing, now, onwrite, onnext, onretry }: Props = $props();

  const target = $derived(detail ? actionTarget(detail) : null);
  const card = $derived.by((): "decision" | "signoff" | null => {
    if (!detail || !target || outcome) return null;
    if (tab === "inbox") return canAnswer(target) && detail.decision ? "decision" : null;
    return awaitsSignoff(target) && detail.signoff ? "signoff" : null;
  });
  // The bead has left the list's state without the user's write here: answered or signed off elsewhere.
  const gone = $derived(vanished || (detail !== null && !outcome && !writing && card === null));
  const judge = $derived(detail ? judgeLink(detail.links) : undefined);
  const designWaiting = $derived(
    !!detail?.decision && detail.decision.label === "needs-design" && !detail.decision.ready_to_judge,
  );
  const kindLabel = $derived(
    tab === "signoff" ? "Sign-off" : (rowKind ?? (detail?.labels.includes("needs-decision") ? "decision" : "design")) === "decision" ? "Decision" : "Design",
  );

  let descOpen = $state<Record<string, boolean>>({});
  let commentOpen = $state<Record<string, boolean>>({});
  let commentRaw = $state<Record<string, boolean>>({});
  const recent = $derived(detail ? detail.comments.slice(-3) : []);

  let heading = $state<HTMLElement>();
  /** Parks focus on the title once a write moved the selection here (the form that had it went away). */
  export function focusHeading(): void {
    heading?.focus({ preventScroll: true });
  }
</script>

<div class="pane" data-pane={id}>
  {#if vanished}
    <div class="msg">
      <h2 tabindex="-1" bind:this={heading}>{id}</h2>
      <p>{id} no longer {tab === "inbox" ? "awaits you" : "awaits sign-off"} (it was handled elsewhere).</p>
      <button type="button" class="btn" onclick={onnext}>Next item</button>
    </div>
  {:else if failed && !detail}
    <div class="msg">
      <h2 tabindex="-1" bind:this={heading}>{id}</h2>
      <p class="err" role="alert">Can't load {id}: {failed}</p>
      <button type="button" class="btn" onclick={onretry}>Retry</button>
    </div>
  {:else if !detail}
    <div class="sk"><SkeletonRows n={5} label="Loading {id}…" /></div>
  {:else}
    <div class="head">
      <span class="kt" data-kind={tab === "signoff" ? "signoff" : kindLabel.toLowerCase()}>{kindLabel}</span>
      <span class="id">{detail.id}</span>
      <span class="sp"></span>
      {#if tab === "signoff" && !outcome}
        <button type="button" class="btn ghost" onclick={onnext}>Skip</button>
      {/if}
      <a class="open" href="/issues/{encodeURIComponent(detail.id)}">Open issue ↗</a>
    </div>
    <h2 class="title" tabindex="-1" bind:this={heading}><InlineText src={detail.title} /></h2>

    {#if outcome}
      <ResolvedNote kind={outcome.kind} followUp={actions.followUps[detail.id]} />
    {/if}

    {#if gone}
      <div class="gone">
        <p>{detail.id} no longer {tab === "inbox" ? "awaits you" : "awaits sign-off"} (it was handled elsewhere).</p>
        <button type="button" class="btn" onclick={onnext}>Next item</button>
      </div>
    {/if}

    {#if card === "decision" && detail.decision}
      {#if judge}
        <p class="mock"><a class="btn" href={judge.url} target="_blank" rel="noopener noreferrer">Open {judge.kind} ↗</a>{#if judge.note}<span class="mnote">{judge.note}</span>{/if}</p>
      {/if}
      <DecisionCard
        id={detail.id}
        issueTitle={detail.title}
        decision={detail.decision}
        {actions}
        busy={actions.isBusy(detail.id)}
        {readOnly}
        blocked={designWaiting && !judge ? "Nothing to judge yet." : ""}
        onwrite={(write, kind) => onwrite(write, kind)}
      />
    {:else if card === "signoff" && detail.signoff}
      <SignoffCard
        id={detail.id}
        signoff={detail.signoff}
        {actions}
        busy={actions.isBusy(detail.id)}
        {readOnly}
        onwrite={(write, kind) => onwrite(write, kind)}
      />
    {/if}

    {#if detail.links.length > 0}
      <section class="sec">
        <h3>Links ({detail.links.length})</h3>
        {#each detail.links as l, i (i)}
          <a class="lnk" href={l.url} target="_blank" rel="noopener noreferrer" title={l.url}>
            <span class="lk">{l.kind}</span>
            <span class="lu">{linkHost(l.url)}{l.note ? ` · ${l.note}` : ""}</span>
          </a>
        {/each}
      </section>
    {/if}

    {#if detail.description.trim()}
      {@const long = isLongText(detail.description)}
      <section class="sec">
        <h3>Description</h3>
        <div class="desc" class:clamp={long && !descOpen[detail.id]}><Markdown src={detail.description} /></div>
        {#if long}
          <button type="button" class="more" aria-expanded={!!descOpen[detail.id]} onclick={() => (descOpen[detail.id] = !descOpen[detail.id])}>
            {descOpen[detail.id] ? "Show less" : "Show all"}
          </button>
        {/if}
      </section>
    {/if}

    {#if recent.length > 0}
      <section class="sec">
        <h3>Recent comments</h3>
        {#each recent as c (c.id)}
          {@const key = `${detail.id}:${c.id}`}
          <CommentItem
            comment={c}
            open={!!commentOpen[key]}
            raw={!!commentRaw[key]}
            {now}
            ontoggle={() => (commentOpen[key] = !commentOpen[key])}
            onraw={() => (commentRaw[key] = !commentRaw[key])}
          />
        {/each}
      </section>
    {/if}
  {/if}
</div>

<style>
  .pane {
    padding: var(--space-4) 20px;
    min-width: 0;
  }
  .sk {
    margin: calc(-1 * var(--space-4)) -20px 0;
  }
  .head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .sp {
    flex: 1;
  }
  .kt {
    padding: 0 6px;
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    line-height: 16px;
    background: var(--accent-subtle);
    color: var(--accent);
  }
  .kt[data-kind="design"] {
    background: color-mix(in srgb, var(--status-review) 14%, transparent);
    color: var(--status-review);
  }
  .kt[data-kind="signoff"] {
    background: var(--warning-subtle);
    color: var(--warning);
  }
  .id {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .open {
    font-size: var(--text-sm);
  }
  .title {
    margin: var(--space-2) 0 var(--space-3);
    font-size: var(--text-lg);
    overflow-wrap: anywhere;
  }
  .title:focus,
  .msg h2:focus {
    outline: none;
  }
  .msg {
    max-width: 420px;
    margin: 48px auto;
    text-align: center;
  }
  .msg p,
  .gone p {
    color: var(--text-muted);
  }
  .err {
    color: var(--danger);
  }
  .gone {
    padding: var(--space-3) var(--space-4);
    margin-bottom: var(--space-4);
    border: 1px solid var(--border);
    border-left: 3px solid var(--text-faint);
    border-radius: var(--radius-md);
    background: var(--bg-subtle);
  }
  .gone p {
    margin: 0 0 var(--space-2);
  }
  .mock {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: 0 0 var(--space-3);
  }
  .mnote {
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .btn {
    display: inline-flex;
    align-items: center;
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
    color: var(--text);
    text-decoration: none;
  }
  .btn:hover {
    background: var(--bg-hover);
  }
  .btn.ghost {
    border-color: transparent;
    background: transparent;
    color: var(--text-muted);
  }
  .btn.ghost:hover {
    background: var(--bg-hover);
  }
  .sec {
    margin-top: var(--space-4);
  }
  .sec h3 {
    margin: 0 0 6px;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .lnk {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 26px;
    color: var(--text);
    text-decoration: none;
    border-radius: var(--radius-sm);
  }
  .lnk:hover {
    background: var(--bg-hover);
  }
  .lk {
    flex: none;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg-subtle);
    font-size: var(--text-xs);
    line-height: 16px;
    color: var(--text-muted);
  }
  .lu {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--accent);
  }
  .desc.clamp {
    max-height: calc(1.55em * 6);
    overflow: hidden;
  }
  .more {
    margin-top: 4px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .more:hover {
    text-decoration: underline;
  }
</style>
