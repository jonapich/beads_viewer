<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import Activity from "../components/detail/Activity.svelte";
  import DecisionCard from "../components/detail/DecisionCard.svelte";
  import DetailHeader from "../components/detail/DetailHeader.svelte";
  import DetailSkeleton from "../components/detail/DetailSkeleton.svelte";
  import FieldRail from "../components/detail/FieldRail.svelte";
  import History from "../components/detail/History.svelte";
  import InlineText from "../components/detail/InlineText.svelte";
  import Markdown from "../components/detail/Markdown.svelte";
  import Relationships from "../components/detail/Relationships.svelte";
  import ResolvedNote from "../components/detail/ResolvedNote.svelte";
  import Section from "../components/detail/Section.svelte";
  import SignoffCard from "../components/detail/SignoffCard.svelte";
  import RowActionsPopover from "../components/issues/RowActionsPopover.svelte";
  import DescriptionEditor from "../components/detail/DescriptionEditor.svelte";
  import { refocus as refocusOn } from "../components/detail/focus";
  import { patchIssue, postAnswer, postSignoff, putPriority, setCloseCandidate } from "../lib/api/client";
  import { IssueActions } from "../lib/issues/actions.svelte";
  import { actionTarget, designOpenByDefault, isReadOnly, noteHolds, readOnlyNotice, relationshipCount } from "../lib/issues/detail";
  import { DetailFeed } from "../lib/issues/detail-feed.svelte";
  import type { IssueDetail as IssueDetailT } from "../lib/api/types";
  import { IssueEdits } from "../lib/issues/edits.svelte";
  import { awaitsSignoff, canAnswer } from "../lib/issues/model";
  import { route } from "../lib/router/router.svelte";
  import { toasts } from "../lib/toasts.svelte";

  const feed = new DetailFeed();

  // /issues/a to /issues/b keeps this component, so everything follows the id.
  const id = $derived(route.params["id"] ?? "");
  $effect(() => {
    const current = id;
    untrack(() => feed.setId(current));
  });
  onMount(() => feed.start());

  /** The feed can still hold the previous issue for a frame after the id changed. */
  const detail = $derived(feed.detail && feed.detail.id === id ? feed.detail : null);
  // The relative ages are computed once per refetch, not per render.
  const now = $derived.by(() => {
    void feed.detail;
    return Date.now();
  });
  const notice = $derived(readOnlyNotice(feed.meta, detail));

  // ---- writes: the beads-c2o.19 quick actions, the same ones the peek drawer uses.
  // Writes need a writable tracker, and an issue whose own route is writable (the detail says why not).
  const writes = $derived(feed.meta?.capabilities?.writes ?? false);
  const readOnly = $derived(isReadOnly(writes, detail));
  const actions = new IssueActions({
    client: { postSignoff, postAnswer, putPriority, setCloseCandidate },
    toasts,
    writable: () => !readOnly,
    // After our own write lands, the open field editors move to the new updated_at (when their
    // field did not change), so a priority click does not make a description Save conflict.
    refresh: async () => {
      const at = id;
      await feed.refresh();
      edits.rebaseOpen(at);
    },
    sticky: () => {},
  });
  // The inline field editors (beads-c2o.14): title, status, assignee, type and description, one
  // PATCH each. Writes on one issue run one at a time, so the quick actions and the editors
  // share the busy state.
  const edits = new IssueEdits({
    client: { patchIssue },
    toasts,
    writable: () => !readOnly,
    refresh: () => feed.refresh(),
    current: (issue) => (feed.detail && feed.detail.id === issue ? feed.detail : null),
    blocked: (issue) => actions.isBusy(issue),
  });
  const busy = $derived(actions.isBusy(id) || edits.isBusy(id));

  // The same gates as a list row, fed by the detail.
  const target = $derived(detail ? actionTarget(detail) : null);
  // "Approved just now" and the like, shown where the card was: view state, not API data.
  type Done = "approved" | "disputed" | "answered";
  let resolved = $state<Record<string, { kind: Done; at: string }>>({});
  const done = $derived(detail ? resolved[detail.id] : undefined);

  // The cards stay hidden behind the note while the detail is the same snapshot the write started
  // from: after "saved, but reloading failed" the refetch changes nothing, and the action would be
  // offered again on stale data (a second click posts a second sign-off).
  const stale = $derived(!!detail && noteHolds(done?.at, detail.updated_at));
  const showSignoff = $derived(!!target && !stale && awaitsSignoff(target) && !!detail?.signoff);
  const showDecision = $derived(!!target && !stale && canAnswer(target) && !!detail?.decision);

  /**
   * A card write finished: note it, and when the control that had focus went away with the card
   * (the dispute form, the answer form) park focus on the title, so a keyboard user stays on the page.
   */
  async function written(issue: string, startedAt: string, write: Promise<boolean>, kind: Done): Promise<void> {
    if (!(await write)) return;
    resolved[issue] = { kind, at: startedAt };
    await tick();
    if (!document.activeElement || document.activeElement === document.body) {
      document.querySelector<HTMLElement>("h1.title")?.focus({ preventScroll: true });
    }
  }

  // The description editor replaces the rendered Markdown; its Edit button sits beside the section header.
  const descEditing = $derived(!!detail && !readOnly && edits.session(detail.id, "description") !== undefined);
  let descBtn = $state<HTMLButtonElement>();

  function openDescription(d: IssueDetailT): void {
    if (!edits.canEdit(d.id)) return;
    setOpen("description", true);
    edits.open(d.id, "description", d);
  }

  // The priority popover: one instance, opened from the header pill.
  type Pop = { anchor: DOMRect; el: HTMLElement };
  let pop = $state.raw<Pop | null>(null);

  // The popover goes with its issue (another one opened, or it is gone) and with write access.
  $effect(() => {
    void id;
    if (pop && (readOnly || !detail)) pop = null;
  });
  $effect(() => {
    void id;
    untrack(() => (pop = null));
  });

  function togglePop(el: HTMLElement): void {
    // The same pill again closes it.
    if (pop && pop.el === el) {
      pop = null;
      return;
    }
    pop = { anchor: el.getBoundingClientRect(), el };
  }

  async function closePop(restoreFocus: boolean): Promise<void> {
    const p = pop;
    pop = null;
    if (!restoreFocus || !p) return;
    await tick();
    (p.el.isConnected ? p.el : document.querySelector<HTMLElement>('[data-pop-anchor="prio"]'))?.focus({ preventScroll: true });
  }

  /** After the write has landed, put focus back on the pill unless the user moved it elsewhere. */
  async function refocus(write: Promise<boolean>, p: Pop | null): Promise<void> {
    await write;
    await tick();
    if (!p || (document.activeElement && document.activeElement !== document.body)) return;
    (p.el.isConnected ? p.el : document.querySelector<HTMLElement>('[data-pop-anchor="prio"]'))?.focus({ preventScroll: true });
  }

  // Which sections the user opened or closed, keyed by issue id and section. A refetch (every live
  // update) re-renders in place, so nothing is reset; another issue starts from the defaults.
  let toggled = $state<Record<string, boolean>>({});
  const isOpen = (name: string, fallback: boolean): boolean => toggled[`${id}:${name}`] ?? fallback;
  const setOpen = (name: string, open: boolean): void => {
    toggled[`${id}:${name}`] = open;
  };
</script>

<div class="page">
  {#if notice}
    <div class="banner" role="note">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true"><circle cx="8" cy="8" r="6.2" /><path d="M8 7.2V11M8 5v.1" /></svg>
      <div>{notice}</div>
    </div>
  {/if}

  {#if detail}
    <!-- DOM order: header, rail, sections. The rail is rendered once; CSS moves it beside the sections on wide screens. -->
    <div class="grid">
      <div class="head">
        <DetailHeader {edits} {detail} {now} {busy} {readOnly} prioOpen={pop !== null} onpickpriority={togglePop} />
      </div>
      <div class="railcell">
        <FieldRail {edits} {detail} {busy} {readOnly} oncandidate={(on) => void actions.setCloseCandidate(detail.id, on)} />
      </div>
      <div class="main">
        {#if showSignoff && detail.signoff}
          <SignoffCard
            id={detail.id}
            signoff={detail.signoff}
            {actions}
            {busy}
            {readOnly}
            onwrite={(write, kind) => void written(detail.id, detail.updated_at, write, kind)}
          />
        {:else if showDecision && detail.decision}
          <DecisionCard
            id={detail.id}
            issueTitle={detail.title}
            decision={detail.decision}
            {actions}
            {busy}
            {readOnly}
            onwrite={(write, kind) => void written(detail.id, detail.updated_at, write, kind)}
          />
        {:else if done}
          <ResolvedNote kind={done.kind} followUp={actions.followUps[detail.id]} />
        {/if}


        {#if detail.status === "closed" && detail.close_reason}
          <Section name="resolution" title="Resolution" bind:open={() => isOpen("resolution", true), (v) => setOpen("resolution", v)}>
            <p class="resolution"><InlineText src={detail.close_reason} /></p>
          </Section>
        {/if}

        <Section name="description" title="Description" bind:open={() => isOpen("description", true), (v) => setOpen("description", v)}>
          {#snippet actions()}
            {#if !readOnly && !descEditing}
              <button
                type="button"
                class="editbtn"
                bind:this={descBtn}
                aria-disabled={edits.canEdit(detail.id) ? undefined : "true"}
                title={edits.canEdit(detail.id) ? "Edit description" : "Wait for the current save to finish"}
                onclick={() => openDescription(detail)}
              >
                <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 13.5l.7-3L11 2.7a1.4 1.4 0 012 2L5.2 12.8z" /><path d="M9.8 3.9l2.3 2.3" /></svg>
                Edit
              </button>
            {/if}
          {/snippet}
          {#if descEditing}
            <DescriptionEditor {edits} {detail} {busy} onfinish={() => void refocusOn(() => descBtn)} />
          {:else if detail.description.trim()}<Markdown src={detail.description} />{:else}<p class="empty">No description.</p>{/if}
        </Section>

        {#if detail.acceptance_criteria?.trim()}
          <Section name="acceptance" title="Acceptance" bind:open={() => isOpen("acceptance", true), (v) => setOpen("acceptance", v)}>
            <Markdown src={detail.acceptance_criteria} />
          </Section>
        {/if}

        {#if relationshipCount(detail) > 0}
          <Relationships {detail} bind:open={() => isOpen("relationships", true), (v) => setOpen("relationships", v)} />
        {/if}

        {#if detail.design?.trim()}
          <Section
            name="design"
            title="Design"
            bind:open={() => isOpen("design", designOpenByDefault(detail.design)), (v) => setOpen("design", v)}
          >
            <Markdown src={detail.design} />
          </Section>
        {/if}

        {#if detail.notes?.trim()}
          <Section name="notes" title="Notes" bind:open={() => isOpen("notes", true), (v) => setOpen("notes", v)}>
            <Markdown src={detail.notes} />
          </Section>
        {/if}

        {#if detail.comments.length > 0}
          <Activity {detail} {now} bind:open={() => isOpen("activity", true), (v) => setOpen("activity", v)} />
        {/if}

        <History {detail} {now} bind:open={() => isOpen("history", false), (v) => setOpen("history", v)} />
      </div>
    </div>
  {:else if feed.status === "missing"}
    <div class="nf">
      <h2>No issue {id}</h2>
      <p class="muted">It may have been deleted, or the ID is wrong.</p>
      <p class="back"><a href="/issues">Back to issues</a></p>
    </div>
  {:else if feed.status === "error"}
    <div class="nf">
      <h2>Couldn't load {id}</h2>
      <p class="muted">{feed.error?.message ?? "Something went wrong."}</p>
      <p class="back"><button type="button" class="retry" onclick={() => feed.reload()}>Retry</button></p>
    </div>
  {:else}
    <DetailSkeleton />
  {/if}

  {#if pop && detail && !readOnly}
    {#key detail.id}
      <RowActionsPopover
        kind="prio"
        item={actionTarget(detail)}
        anchor={pop.anchor}
        writes={!readOnly}
        onclose={(restore) => void closePop(restore)}
        onpriority={(p) => {
          const issue = detail.id;
          const from = pop;
          void closePop(true);
          void refocus(actions.setPriority(issue, p), from);
        }}
        oncandidate={() => {}}
        onapprove={() => {}}
        ondispute={() => {}}
        onanswer={() => {}}
      />
    {/key}
  {/if}
</div>

<style>
  .banner {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    margin-bottom: var(--space-4);
    font-size: 13px;
  }
  .banner svg {
    flex: none;
    margin-top: 1px;
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .head,
  .main {
    min-width: 0;
  }
  .main {
    margin-top: var(--space-4);
  }
  /* Wide: sections on the left, the field rail sticky on the right, the header across both. */
  @media (min-width: 1100px) {
    .grid {
      grid-template-columns: minmax(0, 760px) 280px;
      column-gap: var(--space-6);
    }
    .head {
      grid-column: 1 / -1;
      grid-row: 1;
    }
    .main {
      grid-column: 1;
      grid-row: 2;
    }
    .railcell {
      grid-column: 2;
      grid-row: 2;
      margin-top: var(--space-4);
    }
  }
  .editbtn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 10px;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    background: transparent;
    font-size: var(--text-sm);
    color: var(--text-muted);
    cursor: pointer;
  }
  .editbtn:hover:not([aria-disabled="true"]) {
    background: var(--bg-hover);
    border-color: var(--border);
  }
  .editbtn[aria-disabled="true"] {
    cursor: not-allowed;
    opacity: 0.6;
  }
  .resolution {
    margin: 0;
    font-size: var(--text-base);
    line-height: 1.55;
    max-width: 72ch;
    overflow-wrap: anywhere;
  }
  .empty {
    margin: 0;
    color: var(--text-faint);
    font-style: italic;
  }
  .muted {
    color: var(--text-muted);
  }
  .nf {
    max-width: 420px;
    margin: 48px auto;
    text-align: center;
  }
  .nf h2 {
    font-size: var(--text-lg);
    margin-bottom: var(--space-2);
    overflow-wrap: anywhere;
  }
  .nf p {
    margin: 0;
  }
  .nf .back {
    margin-top: var(--space-3);
  }
  .retry {
    height: 28px;
    padding: 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .retry:hover {
    background: var(--bg-hover);
  }
</style>
