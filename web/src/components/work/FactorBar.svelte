<script lang="ts">
  import { barPct } from "../../lib/work/present";
  import ValueBar from "./ValueBar.svelte";

  interface Props {
    label: string;
    /** 0..1 */
    value: number;
    /** Shown as a number at the right; omit for a bar-only line (the pick cards). */
    showValue?: boolean;
    /** Why the factor has this value, under the bar. */
    caption?: string;
  }

  let { label, value, showValue = false, caption = "" }: Props = $props();
</script>

<div class="fb" class:nov={!showValue}>
  <span class="fl">{label}</span>
  <ValueBar pct={barPct(value, 1)} />
  {#if showValue}<span class="fv">{value.toFixed(2)}</span>{/if}
  {#if caption}<span class="fc">{caption}</span>{/if}
</div>

<style>
  .fb {
    display: grid;
    grid-template-columns: 96px minmax(0, 1fr) 34px;
    gap: 2px 8px;
    align-items: center;
    margin: 3px 0;
  }
  .fb.nov {
    grid-template-columns: 96px minmax(0, 1fr);
  }
  .fl {
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .fv {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-align: right;
    color: var(--text-faint);
  }
  .fc {
    grid-column: 2 / -1;
    font-size: var(--text-xs);
    color: var(--text-faint);
    line-height: 1.3;
  }
</style>
