<script lang="ts">
  import PriorityPill from "../../issues/PriorityPill.svelte";
  import { CARD_FACTORS, factorValue, type PickView } from "../../../lib/work/triage";
  import FactorBar from "../FactorBar.svelte";
  import Tag from "../Tag.svelte";

  interface Props {
    view: PickView;
    /** The tracker takes writes. */
    writes: boolean;
    busy: boolean;
    prioOpen: boolean;
    selected: boolean;
    onpeek: (id: string) => void;
    onprio: (id: string, anchor: HTMLElement) => void;
  }

  let { view, writes, busy, prioOpen, selected, onpeek, onprio }: Props = $props();

  const pick = $derived(view.pick);
  const rec = $derived(view.rec);
</script>

<article class="card pick" class:top={view.rank === 1} class:sel={selected} data-row data-id={pick.id}>
  <div class="ph">
    <span class="rankb">#{view.rank}</span>
    <span class="scoreb" title="Triage score">{pick.score.toFixed(3)}</span>
  </div>
  <div class="pid">{pick.id}</div>
  <h3 class="pt" title={pick.title}>{pick.title}</h3>
  {#if rec}
    <div class="pm">
      {#if rec.type !== "task"}<Tag tone={rec.type === "bug" ? "bug" : "type"}>{rec.type}</Tag>{/if}
      <PriorityPill priority={rec.priority} {busy} expanded={prioOpen} disabled={!writes} onpick={(el) => onprio(pick.id, el)} />
    </div>
    <p class="pa">{rec.action}</p>
    {#each CARD_FACTORS as f (f.norm)}
      <FactorBar label={f.label} value={factorValue(rec.breakdown, f)} />
    {/each}
  {/if}
  {#if pick.unblocks > 0}<div class="un">Unblocks {pick.unblocks}</div>{/if}
  <button type="button" class="btn" onclick={() => onpeek(pick.id)}>Open</button>
</article>

<style>
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    cursor: pointer;
  }
  .card.top {
    border-color: var(--accent);
  }
  .card.sel {
    background: var(--bg-selected);
  }
  .ph {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .rankb {
    font-weight: 600;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .scoreb {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border-radius: var(--radius-sm);
    padding: 0 6px;
  }
  .pid {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .pt {
    margin: 0;
    font-size: var(--text-base);
    font-weight: 600;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    line-height: 1.35;
  }
  .pm {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .pa {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .un {
    font-size: var(--text-sm);
    color: var(--success);
    font-weight: 600;
  }
  .btn {
    align-self: flex-start;
    margin-top: 4px;
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .btn:hover {
    background: var(--bg-hover);
  }
</style>
