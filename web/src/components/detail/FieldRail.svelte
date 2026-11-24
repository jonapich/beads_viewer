<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { absTime, linkHost, stageOf, typedLinks } from "../../lib/issues/detail";
  import { CLOSE_CANDIDATE, READ_ONLY_TITLE } from "../../lib/issues/writes";
  import { safeHref } from "../../lib/markdown";
  import TypeIcon from "../issues/TypeIcon.svelte";
  import Field from "./Field.svelte";
  import SignoffPill from "./SignoffPill.svelte";

  interface Props {
    detail: IssueDetail;
    /** A write on this issue is in flight. */
    busy: boolean;
    /** The tracker (or this issue) takes no writes: the close-candidate chip is disabled. */
    readOnly: boolean;
    /** Mark (true) or unmark (false) the issue as a close candidate. */
    oncandidate: (on: boolean) => void;
  }

  let { detail, busy, readOnly, oncandidate }: Props = $props();

  const labels = $derived(detail.labels.filter((l) => l !== CLOSE_CANDIDATE));
  const candidate = $derived(detail.labels.includes(CLOSE_CANDIDATE));
  const stage = $derived(stageOf(detail));
  const links = $derived(typedLinks(detail));
  /** The date and time without the zone: every timestamp is UTC, said once in the title. */
  const noZone = (iso: string) => absTime(iso).replace(" UTC", "");
</script>

<!-- Rendered once; CSS lays it out as a sticky column beside the sections (wide) or a grid above them (narrow).
     Each field is one <Field> whose value is a snippet: the inline editors (beads-c2o.14) replace one value. -->
<aside class="rail" aria-label="Details">
  <Field label="Assignee" name="assignee">
    {#if detail.assignee}{detail.assignee}{:else}<span class="faint">Unassigned</span>{/if}
  </Field>

  <Field label="Type" name="type">
    <TypeIcon type={detail.issue_type} />
    <span>{detail.issue_type}</span>
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
