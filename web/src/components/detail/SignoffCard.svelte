<script lang="ts">
  import type { SignoffItem } from "../../lib/api/types";
  import { noteKey, type IssueActions } from "../../lib/issues/actions.svelte";
  import { absTime, commitHref, isNothing, signoffHasParts } from "../../lib/issues/detail";
  import CaptureStrip from "../files/CaptureStrip.svelte";
  import SignoffActions from "../issues/SignoffActions.svelte";
  import InlineText from "./InlineText.svelte";
  import Markdown from "./Markdown.svelte";
  import NeedsCard from "./NeedsCard.svelte";
  import SignoffPill from "./SignoffPill.svelte";

  interface Props {
    id: string;
    /** The sign-off data of a closed issue awaiting it. */
    signoff: SignoffItem;
    actions: IssueActions;
    /** Any write on this issue is in flight; defaults to the quick actions' own state. */
    busy?: boolean;
    /** The tracker (or this issue) takes no writes. */
    readOnly: boolean;
    /** Hands a started write to the page, which keeps focus and shows the "done" note once it landed. */
    onwrite: (write: Promise<boolean>, kind: "approved" | "disputed") => void;
  }

  let { id, signoff: so, actions, readOnly, busy: busyProp, onwrite }: Props = $props();

  // The page passes its combined busy state (a field edit in flight counts too).
  const busy = $derived(busyProp ?? actions.isBusy(id));
  const hasParts = $derived(signoffHasParts(so));
  // The structured calls; older comments only have the flattened "key: value" lines.
  const callLines = $derived(so.calls.length === 0 ? (so.calls_made ?? []) : []);
</script>

<NeedsCard tone="warning" label="Sign-off" sub={so.closed_at ? `Closed ${absTime(so.closed_at)}` : undefined}>
  {#snippet title()}Awaiting your sign-off{/snippet}
  {#snippet aside()}<SignoffPill state="pending" />{/snippet}

  {#if !so.has_signoff_comment}
    <div class="part muted">No sign-off comment on this bead.</div>
  {:else if hasParts}
    {#if so.calls.length > 0 || callLines.length > 0}
      <div class="part">
        <h4>Calls made on your behalf</h4>
        {#each so.calls as c, i (i)}
          <div class="call">
            <div><InlineText src={c.call} /></div>
            {#if c.alternative}<div class="sm">Instead of: <InlineText src={c.alternative} /></div>{/if}
            {#if c.why}<div class="sm">Why: <InlineText src={c.why} /></div>{/if}
          </div>
        {/each}
        {#each callLines as line, i (i)}<div class="call"><InlineText src={line} /></div>{/each}
      </div>
    {/if}
    {#if so.what_changed?.length}
      <div class="part">
        <h4>What changed</h4>
        {#each so.what_changed as l, i (i)}<div class="line"><InlineText src={l} /></div>{/each}
      </div>
    {/if}
    {#if so.removed_or_renamed?.length}
      <div class="part">
        <h4>Removed or renamed</h4>
        {#if isNothing(so.removed_or_renamed)}
          <span class="empty">Nothing.</span>
        {:else}
          {#each so.removed_or_renamed as l, i (i)}<div class="line"><InlineText src={l} /></div>{/each}
        {/if}
      </div>
    {/if}
    {#if so.where_to_see?.length}
      <div class="part">
        <h4>Where to see it</h4>
        {#each so.where_to_see as l, i (i)}<div class="line"><InlineText src={l} /></div>{/each}
      </div>
    {/if}
    {#if so.captures?.length}
      <div class="part">
        <h4>Captures</h4>
        <CaptureStrip captures={so.captures} />
      </div>
    {/if}
    {#if so.commits?.length}
      <div class="part">
        <h4>Commits</h4>
        <div>
          {#each so.commits as l, i (i)}
            {@const href = commitHref(l)}
            {#if href}
              <a class="commit" {href} target="_blank" rel="noopener noreferrer">{l}</a>
            {:else}
              <span class="commit">{l}</span>
            {/if}
          {/each}
        </div>
      </div>
    {/if}
  {:else if so.raw}
    <div class="part"><Markdown src={so.raw} /></div>
  {/if}

  <!-- The shared sign-off controls (beads-c2o.19): the same component and wiring as the peek drawer. -->
  <SignoffActions
    issueId={id}
    {busy}
    disabled={readOnly}
    open={actions.disputeOpen === id}
    note={actions.getDraft(noteKey(id))}
    onapprove={() => onwrite(actions.approve(id), "approved")}
    onopen={() => actions.openDispute(id)}
    oncancel={() => actions.closeDispute()}
    onnote={(v) => actions.setDraft(noteKey(id), v)}
    ondispute={(note) => onwrite(actions.dispute(id, note), "disputed")}
  />
</NeedsCard>

<style>
  .part {
    margin-top: var(--space-3);
  }
  .part h4 {
    margin: 0 0 6px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-muted);
  }
  .muted {
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .call {
    padding: 6px 0;
    border-top: 1px solid var(--border);
  }
  .call:first-of-type {
    border-top: 0;
  }
  .sm {
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .line {
    font-size: var(--text-base);
    line-height: 1.55;
    overflow-wrap: anywhere;
  }
  .empty {
    color: var(--text-faint);
    font-style: italic;
  }
  .commit {
    display: inline-block;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 1px 6px;
    margin: 0 6px 4px 0;
  }
</style>
