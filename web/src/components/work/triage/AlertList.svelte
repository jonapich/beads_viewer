<script lang="ts">
  import type { Alert } from "../../../lib/api/types";
  import { severityTone } from "../../../lib/work/present";
  import { alertIds } from "../../../lib/work/triage";

  interface Props {
    alerts: readonly Alert[];
    /** The title of an issue, for the chip's tooltip. */
    titleOf: (id: string) => string | undefined;
    onpeek: (id: string) => void;
  }

  let { alerts, titleOf, onpeek }: Props = $props();
</script>

<div class="alerts" role="list" aria-label="Alerts">
  {#each alerts as a, i (i)}
    {@const tone = severityTone(a.severity)}
    <div class="alert {tone}" role="listitem">
      <i class="dot {tone}"></i>
      <span class="am">{a.message}</span>
      <span class="ac">
        {#each alertIds(a) as id (id)}
          <button type="button" class="idchip" title={titleOf(id) ?? id} onclick={() => onpeek(id)}>{id}</button>
        {/each}
      </span>
    </div>
  {/each}
</div>

<style>
  .alerts {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin: 8px 0;
  }
  .alert {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 32px;
    padding: 4px 12px;
    border: 1px solid var(--danger);
    background: var(--danger-subtle);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    flex-wrap: wrap;
  }
  .alert.warning {
    border-color: var(--warning);
    background: var(--warning-subtle);
  }
  .alert.info {
    border-color: var(--accent);
    background: var(--accent-subtle);
  }
  .am {
    flex: 1 1 220px;
    min-width: 0;
  }
  .ac {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    display: inline-block;
    flex: none;
  }
  .dot.danger {
    background: var(--danger);
  }
  .dot.warning {
    background: var(--warning);
  }
  .dot.info {
    background: var(--accent);
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
