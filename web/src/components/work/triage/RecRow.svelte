<script lang="ts">
  import { FACTORS, factorNote, factorValue, recStatus, type RankedRec } from "../../../lib/work/triage";
  import FactorBar from "../FactorBar.svelte";
  import Tag from "../Tag.svelte";
  import WorkRow from "../WorkRow.svelte";

  interface Props {
    ranked: RankedRec;
    now: number;
    /** The tracker takes writes. */
    writes: boolean;
    busy: boolean;
    prioOpen: boolean;
    selected: boolean;
    /** Reasons and the score breakdown are showing. */
    open: boolean;
    ontoggle: (id: string) => void;
    onpeek: (id: string) => void;
    onprio: (id: string, anchor: HTMLElement) => void;
  }

  let { ranked, now, writes, busy, prioOpen, selected, open, ontoggle, onpeek, onprio }: Props = $props();

  const rec = $derived(ranked.rec);
  const blockedBy = $derived(rec.blocked_by ?? []);
</script>

<div class="item" role="listitem">
  <WorkRow
    id={rec.id}
    title={rec.title}
    issue_type={rec.type}
    status={recStatus(rec, now)}
    priority={rec.priority}
    assignee={rec.assignee}
    triage_score={rec.score}
    {selected}
    {writes}
    {busy}
    {prioOpen}
    {onpeek}
    {onprio}
    {lead}
    {tags}
  />
  {#if open}
    <div class="exp" id="exp-{rec.id}">
      <div class="two">
        <div>
          <h5>Reasons</h5>
          {#if rec.reasons.length > 0}
            <ul>
              {#each rec.reasons as reason (reason)}<li>{reason}</li>{/each}
            </ul>
          {:else}
            <p class="faint">No reasons recorded.</p>
          {/if}
        </div>
        <div>
          <h5>Score breakdown</h5>
          {#each FACTORS as f (f.norm)}
            <FactorBar label={f.label} value={factorValue(rec.breakdown, f)} showValue caption={factorNote(rec.breakdown, f)} />
          {/each}
        </div>
      </div>
    </div>
  {/if}
</div>

{#snippet lead()}
  <span class="rk" title="Rank by triage score">{ranked.rank}</span>
  <button
    type="button"
    class="caret"
    aria-expanded={open}
    aria-controls={open ? `exp-${rec.id}` : undefined}
    aria-label="{open ? 'Collapse' : 'Expand'} reasons for {rec.id}"
    onclick={() => ontoggle(rec.id)}>{open ? "▾" : "▸"}</button
  >
{/snippet}

{#snippet tags()}
  {#if blockedBy.length > 0}<Tag tone="blk" title="Blocked by {blockedBy.join(', ')}">blocked by {blockedBy.join(", ")}</Tag>{/if}
  {#if rec.defer_until}<Tag tone="muted">deferred until {rec.defer_until.slice(0, 10)}</Tag>{/if}
  {#if rec.unblocks_ids.length > 0}<Tag tone="unb" title="Unblocks {rec.unblocks_ids.join(', ')}">→{rec.unblocks_ids.length}</Tag>{/if}
  {#if rec.type === "epic" && !rec.claimable}<Tag tone="muted" title="Epic: work its children">container</Tag>{/if}
  <span class="act" title={rec.action}>{rec.action}</span>
{/snippet}

<style>
  .item {
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  .item :global(.row) {
    border-bottom: 0;
  }
  .rk {
    order: 1;
    width: 24px;
    flex: none;
    font-size: var(--text-xs);
    color: var(--text-faint);
    text-align: right;
  }
  .caret {
    order: 2;
    width: 24px;
    height: 24px;
    flex: none;
    padding: 0;
    border: 0;
    background: none;
    text-align: center;
    line-height: 24px;
    color: var(--text-muted);
    font-size: 12px;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .caret:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .act {
    flex: 1 1 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-faint);
    font-size: var(--text-sm);
  }
  .exp {
    padding: 4px 8px 14px 64px;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .exp h5 {
    margin: 8px 0 4px;
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--text-faint);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .two {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 8px 24px;
  }
  ul {
    margin: 0;
    padding-left: 18px;
  }
  li {
    margin: 1px 0;
  }
  .faint {
    margin: 0;
    color: var(--text-faint);
  }
  @container view (max-width: 759px) {
    .two {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
