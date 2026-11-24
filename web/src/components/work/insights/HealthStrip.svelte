<script lang="ts">
  import type { HealthState, HealthTiles } from "../../../lib/work/insights";
  import Sparkline from "./Sparkline.svelte";

  interface Props {
    state: HealthState;
    /** unavailable: why. */
    message: string;
    /** null until the triage is in. */
    tiles: HealthTiles | null;
    /** The orphans card is open. */
    orphansOpen: boolean;
    onorphans: () => void;
  }

  let { state, message, tiles, orphansOpen, onorphans }: Props = $props();
</script>

{#if state === "unavailable"}
  <p class="muted">Project health unavailable: {message}</p>
{:else if state === "skeleton" || !tiles}
  <div class="tiles" aria-hidden="true">
    {#each [0, 1, 2, 3, 4] as n (n)}
      <div class="tile sk0"><i class="w40"></i><i class="w70"></i></div>
    {/each}
  </div>
  <span class="sr" role="status">Loading project health…</span>
{:else}
  <div class="tiles" role="list" aria-label="Project health">
    <div class="tile" role="listitem"><div class="v">{tiles.ready}</div><div class="l">Ready</div></div>
    <div class="tile" role="listitem"><div class="v">{tiles.blocked}</div><div class="l">Blocked</div></div>
    <div class="tile" role="listitem"><div class="v">{tiles.inProgress}</div><div class="l">In progress</div></div>
    {#if tiles.closed}
      <div class="tile" role="listitem">
        <div class="tv">
          <div>
            <div class="v">{tiles.closed.last30}</div>
            <div class="l">Closed in 30d{#if tiles.closed.avgDays !== null} · avg {tiles.closed.avgDays}d to close{/if}</div>
          </div>
          <Sparkline counts={tiles.closed.weeks} />
        </div>
      </div>
    {/if}
    <div class="tile" role="listitem">
      <div class="v">{tiles.graph.nodes} <small>nodes</small> · {tiles.graph.edges} <small>edges</small></div>
      <div class="l">
        {#if tiles.graph.density !== null}density {tiles.graph.density}{/if}
        {#if tiles.graph.orphans !== null}
          {#if tiles.graph.density !== null}&nbsp;·&nbsp;{/if}<button type="button" aria-expanded={orphansOpen} onclick={onorphans}>orphans {tiles.graph.orphans}</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
    margin: 12px 0 4px;
  }
  .tile {
    min-height: 64px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 8px 12px;
    display: flex;
    flex-direction: column;
    justify-content: center;
    min-width: 0;
  }
  .v {
    font-size: var(--text-xl);
    font-weight: 600;
    line-height: 1.25;
  }
  .v small {
    font-size: var(--text-xs);
    font-weight: 400;
    color: var(--text-muted);
  }
  .l {
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .l button {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    font-size: inherit;
    cursor: pointer;
  }
  .l button:hover {
    text-decoration: underline;
  }
  .tv {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 8px;
  }
  .muted {
    margin: 12px 0 4px;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .sk0 {
    gap: 6px;
  }
  .sk0 i {
    display: block;
    height: 10px;
    border-radius: 5px;
    background: linear-gradient(90deg, var(--bg-subtle), var(--bg-hover), var(--bg-subtle));
    background-size: 200% 100%;
    animation: shim 1.4s linear infinite;
  }
  .w40 {
    width: 40%;
  }
  .w70 {
    width: 70%;
  }
  @keyframes shim {
    from {
      background-position: 200% 0;
    }
    to {
      background-position: -200% 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sk0 i {
      animation: none;
    }
  }
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
