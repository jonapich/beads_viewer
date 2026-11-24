<script lang="ts">
  import { OUTCOME_LABEL, readyCount, type DeskGroup, type DeskTab, type Outcomes } from "../../lib/desk/desk";
  import { relTime } from "../../lib/issues/detail";
  import PriorityPill from "../issues/PriorityPill.svelte";
  import StatusGlyph from "../issues/StatusGlyph.svelte";

  interface Props {
    tab: DeskTab;
    groups: readonly DeskGroup[];
    selectedId: string | null;
    outcomes: Outcomes;
    now: number;
    onselect: (id: string) => void;
  }

  let { tab, groups, selectedId, outcomes, now, onselect }: Props = $props();
</script>

<div class="list" role="group" aria-label={tab === "inbox" ? "Awaiting you" : "Pending sign-offs"}>
  {#each groups as g (g.epic)}
    {@const ready = readyCount(g.rows, outcomes)}
    <div class="gh">
      <span class="eid">{g.epic || "(no epic)"}</span>
      {#if g.title}<span class="et">{g.title}</span>{/if}
      <span class="gc">{g.rows.length}</span>
      {#if ready > 0}<span class="tag ok">{ready} ready to judge</span>{/if}
    </div>
    {#each g.rows as r (r.id)}
      {@const out = outcomes.get(r.id)}
      <button
        type="button"
        class="row"
        class:sel={r.id === selectedId}
        class:handled={out !== undefined}
        data-row={r.id}
        aria-current={r.id === selectedId ? "true" : undefined}
        onclick={() => onselect(r.id)}
      >
        <span class="l1">
          {#if r.kind === "signoff"}
            <span class="gl">
              {#if out}<span class="mark" data-outcome={out.kind} aria-hidden="true">{out.kind === "disputed" ? "!" : "✓"}</span>{:else}<StatusGlyph status="closed" />{/if}
            </span>
          {:else if out}
            <span class="mark" data-outcome={out.kind} aria-hidden="true">✓</span>
          {:else}
            <span class="kt" data-kind={r.kind}>{r.kind === "decision" ? "Decision" : "Design"}</span>
          {/if}
          <span class="id">{r.id}</span>
          <span class="ti">{r.title}</span>
          {#if out}
            <span class="tag" data-outcome={out.kind}>{OUTCOME_LABEL[out.kind]}</span>
          {:else if r.kind === "signoff"}
            {#if r.noNote}<span class="tag warn">No sign-off note</span>{/if}
          {:else if r.ready}
            <span class="tag ok">Ready to judge</span>
          {/if}
          {#if r.links > 0}<span class="lk" title="{r.links} {r.links === 1 ? 'link' : 'links'}">⧉{r.links}</span>{/if}
          <span class="pp"><PriorityPill priority={r.priority} /></span>
          <span class="age">{r.at ? relTime(r.at, now) : ""}</span>
        </span>
        {#if out?.note}
          <span class="l2">Answered: {out.note}</span>
        {:else if !out && r.kind === "design" && !r.ready && r.waitingOn}
          <span class="l2">Waiting on: {r.waitingOn}</span>
        {/if}
      </button>
    {/each}
  {/each}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
  }
  .gh {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 28px;
    padding: 0 var(--space-3);
    background: var(--bg-subtle);
    border-bottom: 1px solid var(--border);
    font-size: var(--text-sm);
  }
  .eid {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .et {
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .gc {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .row {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: 6px var(--space-3);
    border: 0;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
    background: transparent;
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .row.sel {
    background: var(--bg-selected);
    box-shadow: inset 2px 0 0 var(--accent);
  }
  .l1 {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 20px;
  }
  .l2 {
    padding-left: 64px;
    font-size: var(--text-sm);
    color: var(--text-faint);
    overflow-wrap: anywhere;
  }
  .handled .id,
  .handled .ti,
  .handled .age,
  .handled .l2 {
    color: var(--text-muted);
  }
  .handled .ti {
    opacity: 0.8;
  }
  .gl {
    flex: none;
    display: inline-flex;
    width: 56px;
    justify-content: center;
  }
  .kt {
    flex: none;
    width: 56px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    line-height: 16px;
    text-align: center;
    background: var(--accent-subtle);
    color: var(--accent);
  }
  .kt[data-kind="design"] {
    background: color-mix(in srgb, var(--status-review) 14%, transparent);
    color: var(--status-review);
  }
  .mark {
    flex: none;
    width: 56px;
    text-align: center;
    color: var(--success);
    font-weight: 600;
  }
  .mark[data-outcome="disputed"] {
    color: var(--danger);
  }
  .id {
    flex: none;
    width: 104px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ti {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tag {
    flex: none;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    line-height: 16px;
    white-space: nowrap;
    background: var(--bg-subtle);
    color: var(--text-muted);
    border: 1px solid var(--border);
  }
  .tag.ok,
  .tag[data-outcome="answered"],
  .tag[data-outcome="approved"] {
    background: var(--success-subtle);
    color: var(--success);
    border-color: transparent;
  }
  .tag.warn {
    background: var(--warning-subtle);
    color: var(--warning);
    border-color: transparent;
  }
  .tag[data-outcome="disputed"] {
    background: var(--danger-subtle);
    color: var(--danger);
    border-color: transparent;
  }
  .gh .tag {
    margin-left: auto;
  }
  .lk {
    flex: none;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .pp {
    flex: none;
    display: inline-flex;
  }
  .age {
    flex: none;
    width: 48px;
    text-align: right;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-faint);
    white-space: nowrap;
  }
</style>
