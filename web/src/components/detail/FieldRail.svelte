<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { absTime, linkHost, stageOf, typedLinks } from "../../lib/issues/detail";
  import { EDIT_TYPES, editProblem, withCurrent, type IssueEdits } from "../../lib/issues/edits.svelte";
  import { CLOSE_CANDIDATE, READ_ONLY_TITLE } from "../../lib/issues/writes";
  import { safeHref } from "../../lib/markdown";
  import TypeIcon from "../issues/TypeIcon.svelte";
  import EditButtons from "./EditButtons.svelte";
  import EditError from "./EditError.svelte";
  import Field from "./Field.svelte";
  import { autofocus, refocus } from "./focus";
  import SignoffPill from "./SignoffPill.svelte";

  interface Props {
    /** The inline editors' state (assignee and type are edited here). */
    edits: IssueEdits;
    detail: IssueDetail;
    /** A write on this issue is in flight. */
    busy: boolean;
    /** The tracker (or this issue) takes no writes: the close-candidate chip is disabled. */
    readOnly: boolean;
    /** Mark (true) or unmark (false) the issue as a close candidate. */
    oncandidate: (on: boolean) => void;
  }

  let { edits, detail, busy, readOnly, oncandidate }: Props = $props();

  // ---- assignee and type: the value is a button that opens an editor in its place.
  const canEdit = $derived(!readOnly && edits.canEdit(detail.id));
  const assigneeEdit = $derived(readOnly ? undefined : edits.session(detail.id, "assignee"));
  const assigneeError = $derived(edits.error(detail.id, "assignee"));
  const assigneeSaving = $derived(edits.isSaving(detail.id, "assignee"));
  const assigneeProblem = $derived(assigneeEdit ? editProblem("assignee", assigneeEdit.draft) : "");
  const typeEdit = $derived(readOnly ? undefined : edits.session(detail.id, "type"));
  const typeError = $derived(edits.error(detail.id, "type"));
  const typeSaving = $derived(edits.isSaving(detail.id, "type"));
  let assigneeBtn = $state<HTMLButtonElement>();
  let assigneeInput = $state<HTMLInputElement>();
  let typeBtn = $state<HTMLButtonElement>();
  let typeSelect = $state<HTMLSelectElement>();

  type Mode = "save" | "mine" | "retry";

  /**
   * Save (Enter, the button), Retry, or Save mine: one path per editor, so focus always ends up
   * somewhere sensible. A success hands it to the value button; a failure leaves the editor open
   * with the focused control gone (the Retry button, or a select that was disabled while saving),
   * so it goes back to the input. refocus() acts only when focus has fallen to the page.
   */
  async function commitAssignee(mode: Mode = "save"): Promise<void> {
    if (mode === "save" && !edits.canSave(detail.id, "assignee")) return;
    const ok = await (mode === "mine" ? edits.saveMine(detail.id, "assignee") : edits.save(detail.id, "assignee"));
    void refocus(() => (ok ? assigneeBtn : assigneeInput));
  }

  function cancelAssignee(): void {
    edits.cancel(detail.id, "assignee");
    void refocus(() => assigneeBtn);
  }

  /** Dismiss on the error panel removes the button that had focus: back to the input. */
  function dismissAssigneeError(): void {
    edits.dismissError(detail.id, "assignee");
    void refocus(() => assigneeInput);
  }

  /** The type is a draft like any other: choosing a value in the select writes nothing, Save does. */
  async function commitType(mode: Mode = "save"): Promise<void> {
    if (mode === "save" && !edits.canSave(detail.id, "type")) return;
    const ok = await (mode === "mine" ? edits.saveMine(detail.id, "type") : edits.save(detail.id, "type"));
    void refocus(() => (ok ? typeBtn : typeSelect));
  }

  function cancelType(): void {
    edits.cancel(detail.id, "type");
    void refocus(() => typeBtn);
  }

  function dismissTypeError(): void {
    edits.dismissError(detail.id, "type");
    void refocus(() => typeSelect);
  }

  const labels = $derived(detail.labels.filter((l) => l !== CLOSE_CANDIDATE));
  const candidate = $derived(detail.labels.includes(CLOSE_CANDIDATE));
  const stage = $derived(stageOf(detail));
  const links = $derived(typedLinks(detail));
  /** The date and time without the zone: every timestamp is UTC, said once in the title. */
  const noZone = (iso: string) => absTime(iso).replace(" UTC", "");
</script>

<!-- Rendered once; CSS lays it out as a sticky column beside the sections (wide) or a grid above them (narrow).
     Each field is one <Field> whose value is a snippet: the inline editors (beads-c2o.14) replace one value. -->
{#snippet pencil()}
  <svg class="pen" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 13.5l.7-3L11 2.7a1.4 1.4 0 012 2L5.2 12.8z" /><path d="M9.8 3.9l2.3 2.3" /></svg>
{/snippet}

<aside class="rail" aria-label="Details">
  <Field label="Assignee" name="assignee">
    {#if assigneeEdit}
      <form
        class="ed"
        onsubmit={(e) => {
          e.preventDefault();
          void commitAssignee();
        }}
      >
        <!-- readonly, not disabled, while a write runs: a disabled input drops the keyboard focus. -->
        <input
          class="inp"
          type="text"
          aria-label="Assignee"
          placeholder="Unassigned"
          autocomplete="off"
          value={assigneeEdit.draft}
          readonly={busy}
          aria-invalid={assigneeProblem ? "true" : undefined}
          aria-describedby={assigneeProblem ? `assignee-hint-${detail.id}` : undefined}
          aria-busy={assigneeSaving ? "true" : undefined}
          bind:this={assigneeInput}
          use:autofocus
          oninput={(e) => edits.setDraft(detail.id, "assignee", e.currentTarget.value)}
          onkeydown={(e) => {
            if (e.key === "Escape" && !busy) {
              e.preventDefault();
              cancelAssignee();
            }
          }}
        />
        <EditButtons saving={assigneeSaving} disabled={!edits.canSave(detail.id, "assignee")} {busy} reason={assigneeError ? "" : edits.lockNotice(detail.id)} oncancel={cancelAssignee} />
        {#if assigneeProblem}<p class="hint" id="assignee-hint-{detail.id}">{assigneeProblem}</p>{/if}
        {#if assigneeError}
          <EditError
            error={assigneeError}
            field="assignee"
            draft={assigneeEdit.draft}
            {busy}
            reason={edits.lockNotice(detail.id)}
            onretry={() => void commitAssignee(assigneeError.kind === "conflict" ? "mine" : "retry")}
            ondismiss={() => (assigneeError.kind === "conflict" ? cancelAssignee() : dismissAssigneeError())}
          />
        {/if}
      </form>
    {:else if readOnly}
      {#if detail.assignee}{detail.assignee}{:else}<span class="faint">Unassigned</span>{/if}
    {:else}
      <button
        type="button"
        class="valbtn"
        bind:this={assigneeBtn}
        title="Edit assignee"
        aria-label="Assignee: {detail.assignee || 'Unassigned'}, edit"
        aria-disabled={canEdit ? undefined : "true"}
        onclick={() => {
          if (canEdit) edits.open(detail.id, "assignee", detail);
        }}
      >
        {#if detail.assignee}{detail.assignee}{:else}<span class="faint">Unassigned</span>{/if}
        {@render pencil()}
      </button>
    {/if}
  </Field>

  <Field label="Type" name="type">
    {#if typeEdit}
      <!-- A draft with Save and Cancel: ArrowUp/ArrowDown on a closed select changes it and fires
           `change` at once, so nothing may be written from the change event (WCAG 3.2.2). -->
      <form
        class="ed"
        onsubmit={(e) => {
          e.preventDefault();
          void commitType();
        }}
      >
        <select
          class="sel"
          aria-label="Type"
          value={typeEdit.draft}
          disabled={busy}
          bind:this={typeSelect}
          use:autofocus
          onchange={(e) => edits.setDraft(detail.id, "type", e.currentTarget.value)}
          onkeydown={(e) => {
            if (e.key === "Escape" && !busy) {
              e.preventDefault();
              cancelType();
            }
          }}
        >
          {#each withCurrent(EDIT_TYPES, detail.issue_type) as t (t)}<option value={t}>{t}</option>{/each}
        </select>
        <EditButtons saving={typeSaving} disabled={!edits.canSave(detail.id, "type")} {busy} reason={typeError ? "" : edits.lockNotice(detail.id)} oncancel={cancelType} />
        {#if typeError}
          <EditError
            error={typeError}
            field="type"
            draft={typeEdit.draft}
            {busy}
            reason={edits.lockNotice(detail.id)}
            onretry={() => void commitType(typeError.kind === "conflict" ? "mine" : "retry")}
            ondismiss={() => (typeError.kind === "conflict" ? cancelType() : dismissTypeError())}
          />
        {/if}
      </form>
    {:else if readOnly}
      <TypeIcon type={detail.issue_type} />
      <span>{detail.issue_type}</span>
    {:else}
      <button
        type="button"
        class="valbtn"
        bind:this={typeBtn}
        title="Edit type"
        aria-label="Type: {detail.issue_type}, edit"
        aria-disabled={canEdit ? undefined : "true"}
        onclick={() => {
          if (canEdit) edits.open(detail.id, "type", detail);
        }}
      >
        <TypeIcon type={detail.issue_type} />
        <span>{detail.issue_type}</span>
        {@render pencil()}
      </button>
    {/if}
  </Field>

  <Field label="Labels" name="labels">
    {#each labels as l (l)}<span class="chip">{l}</span>{/each}
    <!-- One button for both states, so it survives the toggle and keeps keyboard focus; busy is
         aria-disabled (not disabled) for the same reason. -->
    <button
      type="button"
      class="chip"
      class:warn={candidate}
      class:dash={!candidate}
      class:busy
      aria-label={CLOSE_CANDIDATE}
      aria-pressed={candidate}
      aria-busy={busy ? "true" : undefined}
      aria-disabled={busy ? "true" : undefined}
      disabled={readOnly}
      title={readOnly ? READ_ONLY_TITLE : candidate ? `Remove ${CLOSE_CANDIDATE}` : `Mark as ${CLOSE_CANDIDATE}`}
      onclick={() => {
        if (!busy) oncandidate(!candidate);
      }}
    >
      {#if candidate}{CLOSE_CANDIDATE} {busy ? "…" : "×"}{:else}{busy ? "…" : "+"} {CLOSE_CANDIDATE}{/if}
    </button>
  </Field>

  {#if detail.signoff}
    <Field label="Sign-off" name="signoff">
      <SignoffPill state={detail.signoff.state} />
    </Field>
  {/if}

  {#if stage}
    <Field label="Stage" name="stage">
      <span class="mono">{stage}</span>
    </Field>
  {/if}

  {#if links.length > 0}
    <Field label="Links" name="links">
      {#each links as l, i (i)}
        {@const href = safeHref(l.url)}
        {#if href}
          <a class="chip" {href} target="_blank" rel="noopener noreferrer"><span class="muted">{l.kind}</span> {linkHost(l.url)}</a>
        {:else}
          <span class="chip"><span class="muted">{l.kind}</span> {linkHost(l.url)}</span>
        {/if}
      {/each}
    </Field>
  {/if}

  <Field label="Created" name="created">
    <span class="dt" title={absTime(detail.created_at)}>{noZone(detail.created_at)}</span>
  </Field>
  {#if detail.started_at}
    <Field label="Started" name="started">
      <span class="dt" title={absTime(detail.started_at)}>{noZone(detail.started_at)}</span>
    </Field>
  {/if}
  <Field label="Updated" name="updated">
    <span class="dt" title={absTime(detail.updated_at)}>{noZone(detail.updated_at)}</span>
  </Field>
  {#if detail.closed_at}
    <Field label="Closed" name="closed">
      <span class="dt" title={absTime(detail.closed_at)}>{noZone(detail.closed_at)}</span>
    </Field>
  {/if}
</aside>

<style>
  /* Narrow: a two-column grid under the header (one column on a phone). */
  .rail {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4) var(--space-5);
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }
  @media (max-width: 479px) {
    .rail {
      grid-template-columns: 1fr;
    }
  }
  /* Wide: a sticky column. IssueDetail puts it beside the sections. */
  @media (min-width: 1100px) {
    .rail {
      display: flex;
      flex-direction: column;
      gap: var(--space-4);
      margin-top: 0;
      padding-top: 0;
      border-top: 0;
      position: sticky;
      top: var(--space-4);
      max-height: calc(100dvh - var(--header-h) - var(--space-6));
      overflow: auto;
    }
  }
  .faint {
    color: var(--text-faint);
  }
  /* The value as a button: transparent until hovered, the pencil shown on hover or focus. */
  .valbtn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0 4px;
    margin: 0 -4px;
    min-height: 22px;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    background: transparent;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
    overflow-wrap: anywhere;
  }
  .valbtn:hover,
  .valbtn:focus-visible {
    background: var(--bg-hover);
    border-color: var(--border);
  }
  .valbtn[aria-disabled="true"] {
    cursor: not-allowed;
    opacity: 0.7;
  }
  .pen {
    flex: none;
    color: var(--text-muted);
    opacity: 0;
  }
  .valbtn:hover .pen,
  .valbtn:focus-visible .pen {
    opacity: 1;
  }
  @media (hover: none) {
    .pen {
      opacity: 1;
    }
  }
  .ed {
    flex: 1 1 100%;
    min-width: 0;
  }
  .inp,
  .sel {
    display: block;
    width: 100%;
    padding: 4px 8px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg);
    color: inherit;
    font: inherit;
    line-height: 1.45;
  }
  .inp:focus-visible,
  .sel:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--accent) 35%, transparent);
    outline-offset: 0;
    border-color: var(--accent);
  }
  .inp:disabled,
  .sel:disabled {
    opacity: 0.7;
  }
  .hint {
    margin: 4px 0 0;
    font-size: var(--text-xs);
    color: var(--danger);
  }
  .muted {
    color: var(--text-muted);
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
  }
  .dt {
    font-size: var(--text-sm);
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    height: 22px;
    padding: 0 var(--space-2);
    border-radius: 11px;
    font-size: var(--text-sm);
    background: var(--bg-hover);
    color: var(--text);
    border: 1px solid var(--border);
    text-decoration: none;
  }
  a.chip:hover {
    border-color: var(--border-strong);
  }
  button.chip {
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  button.chip:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }
  button.chip.busy {
    opacity: 0.55;
    cursor: progress;
  }
  .chip.dash {
    background: transparent;
    border: 1px dashed var(--border-strong);
    color: var(--text-muted);
  }
  button.chip.dash:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .chip.warn {
    background: color-mix(in srgb, var(--warning) 16%, transparent);
    border-color: color-mix(in srgb, var(--warning) 40%, transparent);
    color: var(--warning);
  }
</style>
