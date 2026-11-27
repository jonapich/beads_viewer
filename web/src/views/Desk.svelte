<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import DeskList from "../components/desk/DeskList.svelte";
  import DeskPane from "../components/desk/DeskPane.svelte";
  import MetricsTable from "../components/desk/MetricsTable.svelte";
  import PipelineTable from "../components/desk/PipelineTable.svelte";
  import Notice from "../components/work/Notice.svelte";
  import SkeletonRows from "../components/work/SkeletonRows.svelte";
  import WorkPeek from "../components/work/WorkPeek.svelte";
  import {
    getInboxIfChanged,
    getPipelineIfChanged,
    getSignoffIfChanged,
    getSliceMetricsIfChanged,
    postAnswer,
    postSignoff,
    putPriority,
    setCloseCandidate,
  } from "../lib/api/client";
  import type { InboxResponse, PipelineResponse, SignoffResponse, SliceMetricsResponse } from "../lib/api/types";
  import {
    DESK_TABS,
    TAB_LABEL,
    LINGER_MS,
    answerSummary,
    defaultTab,
    departedIds,
    firstUnhandled,
    flatRows,
    inboxGroups,
    isListTab,
    nextUnhandled,
    pipelineSummary,
    parseTab,
    readyCount,
    reconcile,
    removeRow,
    selectionFor,
    signoffGroups,
    summaryParts,
    unhandledCount,
    type DeskGroup,
    type DeskTab,
    type Outcome,
    type OutcomeKind,
    type Outcomes,
  } from "../lib/desk/desk";
  import { refreshNavCounts } from "../lib/desk/nav-counts.svelte";
  import { onIdChange } from "../lib/desk/on-id-change.svelte";
  import { Resource, ViewFeed } from "../lib/feeds/view-feed.svelte";
  import { onLive } from "../lib/live.svelte";
  import { rowClicks } from "../lib/work/row-click";
  import { PeekHost } from "../lib/work/peek-host.svelte";
  import { IssueActions, answerKey, optionKey } from "../lib/issues/actions.svelte";
  import { isReadOnly, readOnlyNotice } from "../lib/issues/detail";
  import { DetailFeed } from "../lib/issues/detail-feed.svelte";
  import { replaceQuery, route } from "../lib/router/router.svelte";
  import { session } from "../lib/session.svelte";
  import { toasts } from "../lib/toasts.svelte";

  const inbox = new Resource<InboxResponse>("/api/v1/inbox", getInboxIfChanged);
  const signoff = new Resource<SignoffResponse>("/api/v1/signoff", getSignoffIfChanged);
  const metrics = new Resource<SliceMetricsResponse>("/api/v1/slice-metrics", getSliceMetricsIfChanged);
  const feed = new ViewFeed([inbox, signoff, metrics]);
  // The pipeline is on its own clock: its ETag moves every minute (staleness depends on the clock),
  // so it is not refetched with every snapshot event, only while its tab shows (see below).
  const pipeline = new Resource<PipelineResponse>("/api/v1/pipeline", getPipelineIfChanged);
  const detailFeed = new DetailFeed();
  const ac = new AbortController();
  onMount(() => {
    const stopFeed = feed.start();
    const stopDetail = detailFeed.start();
    // Once at the start, for the summary line and the tab count.
    void pipeline.load(ac.signal);
    return () => {
      stopFeed();
      stopDetail();
      ac.abort();
      for (const t of timers.values()) clearTimeout(t);
      timers.clear();
    };
  });

  /** How often the Pipeline tab refetches while it shows: the server cuts its clock to the minute. */
  const PIPELINE_POLL_MS = 60_000;
  $effect(() => {
    if (tab !== "pipeline") return;
    const run = (): void => {
      if (document.visibilityState === "visible") void pipeline.load(ac.signal);
    };
    run();
    const timer = setInterval(run, PIPELINE_POLL_MS);
    document.addEventListener("visibilitychange", run);
    const offLive = onLive(run);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", run);
      offLive();
    };
  });

  // ---- the lists: the server's order, plus the rows handled this visit, which stay in place,
  // muted, for LINGER_MS and then leave, whoever handled them. A row is pinned (kept in the list)
  // when its write is sent, not when it is answered: the server reloads and announces the change
  // before it answers the request, so a refetch without the row can arrive first.
  let pinned = $state.raw<ReadonlySet<string>>(new Set());
  let outcomes = $state.raw<Outcomes>(new Map());
  /** Rows whose write is in flight, and rows whose write failed with the reason. */
  let saving = $state.raw<ReadonlySet<string>>(new Set());
  let unsaved = $state.raw<ReadonlyMap<string, string>>(new Map());
  let shownInbox = $state.raw<DeskGroup[] | null>(null);
  let shownSignoff = $state.raw<DeskGroup[] | null>(null);
  /** The pending unpin of each lingering row. */
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  /** The message of each failed write, from IssueActions, for the row and the pane. */
  const failMessages = new Map<string, string>();

  function schedule(id: string, ms: number): void {
    clearTimeout(timers.get(id));
    timers.set(id, setTimeout(() => unpin(id), Math.max(0, ms)));
  }

  /** The linger is over: the row leaves the list (and the selection moves on first). */
  function unpin(id: string): void {
    timers.delete(id);
    if (isListTab(tab) && sel.id === id) {
      const to = groups ? nextUnhandled(groups, id, outcomes) : null;
      setParams({ id: to });
    }
    pinned = new Set([...pinned].filter((p) => p !== id));
    // Taken out here, not left to the next reconcile, which would take it for a row that left on its own.
    if (shownInbox) shownInbox = removeRow(shownInbox, id);
    if (shownSignoff) shownSignoff = removeRow(shownSignoff, id);
  }

  /** Rows that left the list without a write from this desk: marked, and lingering like the rest. */
  function markElsewhere(ids: string[]): void {
    pinned = new Set([...pinned, ...ids]);
    const next = new Map(outcomes);
    for (const id of ids) {
      if (!next.has(id)) next.set(id, { kind: "elsewhere" });
      schedule(id, LINGER_MS);
    }
    outcomes = next;
  }

  $effect(() => {
    const data = inbox.data;
    const p = pinned;
    if (!data) return;
    untrack(() => {
      const prev = shownInbox ?? [];
      const incoming = inboxGroups(data);
      const gone = departedIds(prev, incoming, p);
      if (gone.length > 0) markElsewhere(gone);
      shownInbox = reconcile(prev, incoming, gone.length > 0 ? new Set([...p, ...gone]) : p);
    });
  });
  $effect(() => {
    const data = signoff.data;
    const p = pinned;
    if (!data) return;
    untrack(() => {
      const prev = shownSignoff ?? [];
      const incoming = signoffGroups(data);
      const gone = departedIds(prev, incoming, p);
      if (gone.length > 0) markElsewhere(gone);
      shownSignoff = reconcile(prev, incoming, gone.length > 0 ? new Set([...p, ...gone]) : p);
    });
  });
  // ---- the tab and the selection live in the URL (?tab=&id=) and are rewritten in place.
  function setParams(patch: Record<string, string | null>): void {
    const p = new URLSearchParams(route.query);
    for (const [k, v] of Object.entries(patch)) {
      if (v === null) p.delete(k);
      else p.set(k, v);
    }
    replaceQuery(p);
  }

  const urlTab = $derived(parseTab(route.query.get("tab")));
  const tab: DeskTab = $derived(urlTab ?? defaultTab(inbox.data?.total ?? null, signoff.data?.total ?? null));
  // With no tab in the URL the desk opens on the default and pins it, so answering the last
  // question does not flip the page to another tab under the user.
  $effect(() => {
    if (route.path === "/desk" && urlTab === null && inbox.data && signoff.data) {
      untrack(() => setParams({ tab: defaultTab(inbox.data?.total ?? null, signoff.data?.total ?? null) }));
    }
  });

  function selectTab(next: DeskTab): void {
    if (next === tab) return;
    host.close();
    setParams({ tab: next, id: null });
  }

  // The list the Inbox and Sign-off tabs show (the other two tabs are tables).
  const listTab = $derived<"inbox" | "signoff">(tab === "signoff" ? "signoff" : "inbox");
  const groups = $derived(listTab === "inbox" ? shownInbox : shownSignoff);
  const wanted = $derived(route.query.get("id"));
  const sel = $derived(groups ? selectionFor(groups, outcomes, wanted) : { id: null, vanished: false });
  $effect(() => {
    if (route.path === "/desk" && isListTab(tab) && urlTab !== null && !wanted && sel.id) untrack(() => setParams({ id: sel.id }));
  });

  function select(id: string): void {
    if (id !== sel.id) setParams({ id });
  }

  // ---- the pane
  // sel is a new object on every feed refresh: the effects below depend on its id (a primitive,
  // compared by value) so a refresh with the same row selected does not re-run them.
  const selId = $derived(sel.id);
  $effect(() => {
    const id = selId;
    if (id && !sel.vanished && isListTab(tab)) untrack(() => detailFeed.setId(id));
  });
  // Another row: its half-written dispute form is closed (the note is kept in the drafts).
  onIdChange(
    () => sel.id,
    () => actions.closeDispute(),
  );

  const detail = $derived(detailFeed.detail && detailFeed.detail.id === sel.id ? detailFeed.detail : null);
  const gone = $derived(sel.vanished || detailFeed.status === "missing");
  const failed = $derived(detailFeed.status === "error" ? (detailFeed.error?.message ?? "Something went wrong.") : null);
  const writes = $derived(feed.writes);
  const readOnly = $derived(isReadOnly(writes, detail));
  const notice = $derived(readOnlyNotice(feed.meta, detail));
  const outcome = $derived<Outcome | undefined>(sel.id ? outcomes.get(sel.id) : undefined);

  const actions = new IssueActions({
    client: { postSignoff, postAnswer, putPriority, setCloseCandidate },
    toasts,
    writable: () => !readOnly,
    // The lists, the pane and the nav badge all follow the write before it counts as done.
    refresh: async () => {
      await Promise.all([feed.refresh(), detailFeed.refresh(), refreshNavCounts()]);
    },
    // Rows are pinned when the write is sent (onwrite), which is earlier than this hook is called.
    sticky: () => {},
    onFailed: (id, message) => failMessages.set(id, message),
  });

  let pane = $state<DeskPane>();

  /**
   * A card write was just started on the selected row. The row is marked (Answered, Approved,
   * Disputed) and pinned at once, before the request has an answer, and the selection moves on: the
   * server takes a couple of seconds (bd runs a process per step) and nothing waits for it. When the
   * write is confirmed the row lingers LINGER_MS from the click and leaves; when it fails the mark is
   * reverted, the row says "Not saved" and the pane shows why.
   */
  function onwrite(write: Promise<boolean>, kind: Exclude<OutcomeKind, "elsewhere">): void {
    const id = sel.id;
    // IssueActions marks the issue busy before its first await: not busy means nothing was sent (read-only, blank note).
    if (!id || !actions.isBusy(id)) return;
    // The drafts are dropped when the write succeeds, so read what was posted now.
    const note =
      kind === "answered"
        ? answerSummary(detail?.decision?.options ?? [], actions.getDraft(optionKey(id)), actions.getDraft(answerKey(id)))
        : "";
    const markedAt = Date.now();
    failMessages.delete(id);
    unsaved = new Map([...unsaved].filter(([k]) => k !== id));
    pinned = new Set(pinned).add(id);
    outcomes = new Map(outcomes).set(id, { kind, ...(note ? { note } : {}) });
    saving = new Set(saving).add(id);

    // Advance now, not when the server answers.
    const to = groups ? nextUnhandled(groups, id, outcomes) : null;
    if (to) setParams({ id: to });
    void tick().then(() => pane?.focusHeading());

    void write.then((ok) => {
      saving = new Set([...saving].filter((s) => s !== id));
      if (ok) {
        // Two seconds from the click, but never before the server has confirmed it.
        schedule(id, markedAt + LINGER_MS - Date.now());
        return;
      }
      const next = new Map(outcomes);
      next.delete(id);
      outcomes = next;
      pinned = new Set([...pinned].filter((p) => p !== id));
      unsaved = new Map(unsaved).set(id, failMessages.get(id) ?? "The change was not saved.");
      failMessages.delete(id);
    });
  }
  /** The next row not handled yet (Skip, and "Next item" on a row that left the list). */
  function next(): void {
    if (!groups) return;
    const to = sel.id ? nextUnhandled(groups, sel.id, outcomes) : firstUnhandled(groups, outcomes);
    if (to) setParams({ id: to });
    else toasts.push("warning", listTab === "inbox" ? "Nothing else awaits you." : "No other sign-off is pending.");
  }

  // ---- what the header says
  const inboxLeft = $derived(shownInbox ? unhandledCount(shownInbox, outcomes) : (feed.meta?.counts.awaiting ?? 0));
  const signoffLeft = $derived(shownSignoff ? unhandledCount(shownSignoff, outcomes) : (feed.meta?.counts.signoff_pending ?? 0));
  const readyLeft = $derived(shownInbox ? readyCount(flatRows(shownInbox), outcomes) : 0);
  const parts = $derived(
    summaryParts({
      inbox: inboxLeft,
      ready: readyLeft,
      signoff: signoffLeft,
      pipeline: pipeline.data ? { total: pipeline.data.total, stale: pipeline.data.stale } : null,
    }),
  );
  const tabCount = (t: DeskTab): number | null =>
    t === "inbox" ? inboxLeft : t === "signoff" ? signoffLeft : t === "pipeline" ? (pipeline.data?.total ?? null) : null;

  const rows = $derived(groups ? flatRows(groups) : []);
  const allHandled = $derived(rows.length > 0 && rows.every((r) => outcomes.has(r.id)));
  const listError = $derived(listTab === "inbox" ? inbox.error : signoff.error);
  const other = $derived<"inbox" | "signoff">(listTab === "inbox" ? "signoff" : "inbox");
  const otherLeft = $derived(other === "inbox" ? inboxLeft : signoffLeft);

  // Relative ages are computed once per data refresh, not per render.
  const now = $derived.by(() => {
    void inbox.data;
    void signoff.data;
    void pipeline.data;
    return Date.now();
  });

  // ---- the Pipeline tab: a row opens the peek drawer (no pane of its own)
  const host = new PeekHost({
    writable: () => feed.writes,
    refresh: () => feed.refresh(),
    lookup: () => undefined,
  });
  const reloadKey = $derived([feed.meta, pipeline.data] as const);
  const pipeSummary = $derived(pipeline.data ? pipelineSummary(pipeline.data) : null);
</script>

<section class="view" class:offline={session.status === "offline"} aria-label="Agent desk">
  <div class="col">
    <div class="top">
      <h1>Agent desk</h1>
      <div class="summary" aria-live="polite">
        {#each parts as p (p.text)}
          <button type="button" class="plink" onclick={() => selectTab(p.tab)}><b>{p.n}</b> {p.text}</button>
        {/each}
      </div>
      <div class="seg" role="group" aria-label="Desk sections">
        {#each DESK_TABS as t (t)}
          {@const n = tabCount(t)}
          <button type="button" class:on={t === tab} aria-pressed={t === tab} onclick={() => selectTab(t)}>
            {TAB_LABEL[t]}{#if n !== null} <span class="n">{n}</span>{/if}
          </button>
        {/each}
      </div>
      {#if notice && isListTab(tab)}<Notice tone="info" text={notice} />{/if}
    </div>

    {#if tab === "pipeline"}
      {#if pipeline.error && !pipeline.data}
        <Notice tone="danger" text="Can't load the pipeline: {pipeline.error.message}" action={{ label: "Retry", run: () => void pipeline.load(ac.signal) }} />
      {:else if !pipeline.data}
        <div class="card grow"><SkeletonRows n={4} label="Loading the pipeline…" /></div>
      {:else}
        {#if host.missing}
          <Notice tone="warning" text={host.missing} action={{ label: "Dismiss", run: () => (host.missing = null) }} />
        {/if}
        {#if pipeline.error}
          <Notice tone="warning" text="Showing the last loaded pipeline: {pipeline.error.message}" action={{ label: "Retry", run: () => void pipeline.load(ac.signal) }} />
        {/if}
        {#if pipeSummary}
          <div class="psum">
            {#each pipeSummary.counts as c (c)}<span>{c}</span>{/each}
            <span class="faint">{pipeSummary.hint}</span>
          </div>
        {/if}
        {#if pipeline.data.rows.length === 0}
          <div class="empty">
            <h3>Nothing in progress</h3>
            <p>No bead is claimed right now.</p>
          </div>
        {:else}
          <div class="card grow" data-work-scroll tabindex="-1" use:rowClicks={(id) => host.open(id)}>
            <PipelineTable pipeline={pipeline.data} selectedId={host.peek} {now} onpeek={(id) => host.open(id)} />
          </div>
        {/if}
      {/if}
    {:else if tab === "metrics"}
      {#if metrics.error && !metrics.data}
        <Notice tone="danger" text="Can't load the slice metrics: {metrics.error.message}" action={{ label: "Retry", run: () => feed.reload() }} />
      {:else if !metrics.data}
        <div class="card grow"><SkeletonRows n={4} label="Loading the slice metrics…" /></div>
      {:else if metrics.data.total.slices === 0}
        <div class="empty">
          <h3>No slice metrics</h3>
          <p>No bead carries slice_outcome metadata.</p>
        </div>
      {:else}
        <div class="card grow"><MetricsTable report={metrics.data} /></div>
        {#if metrics.data.unmatched_escapes?.length}
          <p class="faint note">
            {metrics.data.unmatched_escapes.length} escaped {metrics.data.unmatched_escapes.length === 1 ? "bug names" : "bugs name"} a cause that is not a logged slice:
            {metrics.data.unmatched_escapes.map((e) => `${e.bug} (caused by ${e.cause})`).join(", ")}.
          </p>
        {/if}
      {/if}
    {:else if listError}
      <Notice tone="danger" text="Can't load the desk: {listError.message}" action={{ label: "Retry", run: () => feed.reload() }} />
    {:else if groups === null}
      <div class="body">
        <div class="card"><SkeletonRows n={6} label="Loading the desk…" /></div>
        <div class="card"></div>
      </div>
    {:else if rows.length === 0}
      <div class="empty">
        {#if listTab === "inbox"}
          <h3>Nothing awaits you</h3>
          <p>No open bead carries needs-decision or needs-design.</p>
        {:else}
          <h3>No sign-offs pending</h3>
          <p>No closed bead is waiting for your sign-off.</p>
        {/if}
        {#if otherLeft > 0}
          <p><button type="button" class="plink" onclick={() => selectTab(other)}>{otherLeft} {other === "inbox" ? (otherLeft === 1 ? "question" : "questions") : otherLeft === 1 ? "sign-off pending" : "sign-offs pending"} →</button></p>
        {/if}
      </div>
    {:else}
      {#if allHandled}
        <div class="alert" role="status">All caught up.</div>
      {/if}
      <div class="body">
        <div class="card" data-desk-list>
          <DeskList tab={listTab} {groups} selectedId={sel.id} {outcomes} {saving} failed={unsaved} {now} onselect={select} />
        </div>
        <div class="card" data-desk-pane>
          {#if sel.id}
            <DeskPane
              bind:this={pane}
              tab={listTab}
              id={sel.id}
              {detail}
              {failed}
              vanished={gone}
              {actions}
              {readOnly}
              rowKind={rows.find((r) => r.id === sel.id)?.kind}
              {outcome}
              saving={saving.has(sel.id)}
              notSaved={unsaved.get(sel.id) ?? null}
              {now}
              {onwrite}
              onnext={next}
              onretry={() => detailFeed.reload()}
            />
          {/if}
        </div>
      </div>
    {/if}
  </div>
  <WorkPeek {host} writes={feed.writes} {now} {reloadKey} />
</section>
<style>
  .view {
    flex: 1;
    min-height: 0;
    display: flex;
  }
  /* The drawer (Pipeline tab) sits beside this column on wide windows, as in Triage and Plan. */
  .col {
    flex: 1;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .view.offline .body,
  .view.offline .grow {
    opacity: 0.6;
  }
  .top {
    flex: none;
  }
  h1 {
    margin: 0;
  }
  .summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin: 4px 0 var(--space-3);
    min-height: 18px;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .plink {
    padding: 0;
    border: 0;
    background: none;
    font-size: var(--text-sm);
    color: var(--text-muted);
    cursor: pointer;
  }
  .plink:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .plink b {
    font-weight: 600;
    color: var(--text);
  }
  .seg {
    display: inline-flex;
    height: 32px;
    margin-bottom: var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--bg-raised);
  }
  .seg button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0 var(--space-4);
    border: 0;
    border-right: 1px solid var(--border);
    background: transparent;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .seg button:last-child {
    border-right: 0;
  }
  .seg button:hover {
    background: var(--bg-hover);
  }
  .seg button.on {
    background: var(--bg-selected);
    color: var(--accent);
    font-weight: 600;
  }
  .n {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .on .n {
    color: var(--accent);
  }
  .alert {
    flex: none;
    margin-bottom: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--accent);
    border-radius: var(--radius-md);
    background: var(--accent-subtle);
    font-size: var(--text-sm);
  }
  .body {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(360px, 2fr) minmax(480px, 3fr);
    gap: var(--space-4);
  }
  /* The narrowest supported width is 1100px; below it the list column stops growing. */
  @media (max-width: 1099px) {
    .body {
      grid-template-columns: 360px minmax(0, 1fr);
    }
  }
  .card {
    min-width: 0;
    min-height: 0;
    overflow: auto;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
  .grow {
    flex: 1;
  }
  .psum {
    flex: none;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .faint {
    color: var(--text-faint);
  }
  .note {
    flex: none;
    margin: var(--space-2) 0 0;
    font-size: var(--text-sm);
  }
  .empty {
    padding: 40px var(--space-4);
    text-align: center;
    color: var(--text-muted);
  }
  .empty h3 {
    margin: 0 0 4px;
    font-size: var(--text-lg);
    color: var(--text);
  }
  .empty p {
    margin: 4px 0;
  }
</style>
