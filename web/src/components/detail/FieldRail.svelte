<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { absTime, linkHost, signoffPill, stageOf, typedLinks } from "../../lib/issues/detail";
  import { CLOSE_CANDIDATE } from "../../lib/issues/writes";
  import { safeHref } from "../../lib/markdown";
  import TypeIcon from "../issues/TypeIcon.svelte";
  import Field from "./Field.svelte";

  let { detail }: { detail: IssueDetail } = $props();

  const labels = $derived(detail.labels.filter((l) => l !== CLOSE_CANDIDATE));
  const candidate = $derived(detail.labels.includes(CLOSE_CANDIDATE));
  const signoff = $derived(detail.signoff ? signoffPill(detail.signoff.state) : null);
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
    {#if candidate}<span class="chip warn">{CLOSE_CANDIDATE}</span>{/if}
    {#if labels.length === 0 && !candidate}<span class="faint">None</span>{/if}
  </Field>

  {#if detail.signoff && signoff}
    <Field label="Sign-off" name="signoff">
      <span class="pill" data-state={detail.signoff.state}>
        {#if signoff.glyph}<span aria-hidden="true">{signoff.glyph}</span>{/if}{signoff.label}
      </span>
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
  .chip.warn {
    background: color-mix(in srgb, var(--warning) 16%, transparent);
    border-color: color-mix(in srgb, var(--warning) 40%, transparent);
    color: var(--warning);
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
  .pill[data-state="pending"] {
    --c: var(--warning);
  }
  .pill[data-state="approved"] {
    --c: var(--success);
  }
  .pill[data-state="disputed"] {
    --c: var(--danger);
  }
  .pill[data-state="waived"] {
    --c: var(--status-closed);
  }
</style>
