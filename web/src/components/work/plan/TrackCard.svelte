<script lang="ts">
  import type { PlanItem, PlanTrack } from "../../../lib/api/types";
  import { displayStatus } from "../../../lib/issues/model";
  import { plural } from "../../../lib/work/present";
  import { capItems, trackLetter } from "../../../lib/work/plan";
  import type { PeekHost } from "../../../lib/work/peek-host.svelte";
  import Tag from "../Tag.svelte";
  import WorkRow from "../WorkRow.svelte";

  interface Props {
    track: PlanTrack;
    /** The item the banner names ("" when there is none): tagged "Start here". */
    startId: string;
    now: number;
    /** The tracker takes writes. */
    writes: boolean;
    host: PeekHost;
  }

  let { track, startId, now, writes, host }: Props = $props();

  let all = $state(false);
  const split = $derived(capItems(track.items));
  const items = $derived(all ? track.items : split.shown);
</script>

<section class="card tcard" aria-label="Track {trackLetter(track.track_id)}">
  <div class="th">
    <span class="tpill">Track {trackLetter(track.track_id)}</span>
    <span class="why">{track.reason}</span>
    <span class="ni">{plural(track.items.length, "item", "items")}</span>
  </div>
  <div class="list" role="list">
    {#each items as it (it.id)}
      {@render item(it)}
    {/each}
  </div>
  {#if split.hidden.length > 0}
    <button type="button" class="more" aria-expanded={all} onclick={() => (all = !all)}>
      {all ? "Show fewer" : `Show ${split.hidden.length} more`}
    </button>
  {/if}
</section>

{#snippet item(it: PlanItem)}
  {#snippet tags()}
    {#if it.id === startId}<Tag tone="start">Start here</Tag>{/if}
    {#if it.unblocks_ids.length > 0}<Tag tone="unb" title="Unblocks {it.unblocks_ids.join(', ')}">→{it.unblocks_ids.length}</Tag>{/if}
  {/snippet}
  <div class="item" role="listitem">
    <WorkRow
      id={it.id}
      title={it.title}
      issue_type={it.issue_type}
      status={displayStatus({ status: it.status, blocked: false }, now)}
      priority={it.priority}
      assignee={it.assignee}
      triage_score={it.triage_score}
      selected={host.peek === it.id}
      {writes}
      busy={host.actions.isBusy(it.id)}
      prioOpen={host.rowPopOpen(it.id)}
      onpeek={(id) => host.open(id)}
      onprio={(id, el) => host.togglePop(id, el, "row")}
      {tags}
    />
    {#if host.peek === it.id && it.unblocks_ids.length > 0}
      <div class="unbl">
        Unblocks:
        {#each it.unblocks_ids as id, i (id)}{#if i > 0}, {/if}<button type="button" class="idlink" onclick={() => host.open(id)}>{id}</button>{/each}
      </div>
    {/if}
  </div>
{/snippet}

<style>
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    min-width: 0;
  }
  .th {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 16px;
    border-bottom: 1px solid var(--border);
    flex-wrap: wrap;
  }
  .tpill {
    font-size: var(--text-xs);
    font-weight: 600;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0 8px;
    line-height: 18px;
    white-space: nowrap;
  }
  .why {
    color: var(--text-muted);
    font-size: var(--text-sm);
    flex: 1 1 140px;
    min-width: 0;
  }
  .ni {
    font-size: var(--text-xs);
    color: var(--text-faint);
    margin-left: auto;
  }
  .item {
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  .item:last-child {
    border-bottom: 0;
  }
  .item :global(.row) {
    border-bottom: 0;
  }
  .unbl {
    padding: 0 8px 8px 64px;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .idlink {
    padding: 0;
    border: 0;
    background: none;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--accent);
    cursor: pointer;
  }
  .idlink:hover {
    text-decoration: underline;
  }
  .more {
    display: block;
    width: 100%;
    padding: 6px 16px;
    border: 0;
    border-top: 1px solid var(--border);
    background: none;
    text-align: left;
    font-size: var(--text-sm);
    color: var(--accent);
    cursor: pointer;
  }
  .more:hover {
    background: var(--bg-hover);
  }
</style>
