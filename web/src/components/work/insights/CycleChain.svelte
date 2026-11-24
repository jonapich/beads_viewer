<script lang="ts">
  import type { Chain } from "../../../lib/work/insights";

  interface Props {
    chain: Chain;
    /** The title of an id, for the chip's tooltip. */
    titleOf: (id: string) => string | undefined;
    onpeek: (id: string) => void;
  }

  let { chain, titleOf, onpeek }: Props = $props();
</script>

<div class="cyc">
  <div class="chain">
    {#each chain.ids as id, i (i)}
      {#if i > 0}<span class="ar" aria-hidden="true">→</span>{/if}
      <button type="button" class="idchip" title={titleOf(id) ?? id} onclick={() => onpeek(id)}>{id}</button>
    {/each}
  </div>
  <p class="cap">{chain.caption}</p>
</div>

<style>
  .chain {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
    padding: 4px 0;
  }
  .ar {
    color: var(--danger);
    font-size: var(--text-sm);
  }
  .cap {
    font-size: var(--text-xs);
    color: var(--text-faint);
    margin: 0 0 4px;
  }
  .idchip {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: 0 6px;
    height: 20px;
    border: 1px solid var(--border-strong);
    border-radius: 10px;
    background: var(--bg-raised);
    color: var(--text);
    cursor: pointer;
  }
  .idchip:hover {
    background: var(--bg-hover);
  }
</style>
