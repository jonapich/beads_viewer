<script lang="ts">
  import { sparkBars } from "../../../lib/work/insights";

  /** Closes per week, oldest first (at most 8). */
  let { counts }: { counts: readonly number[] } = $props();
  const bars = $derived(sparkBars(counts));
</script>

<!-- x, y, width and height are SVG presentation attributes, not style=, so they pass the CSP. -->
<svg width="64" height="24" viewBox="0 0 64 24" role="img" aria-label="Closed per week, last {counts.length} weeks, newest on the right">
  {#each bars as b, i (i)}
    <rect x={b.x} y={b.y} width="6" height={b.h} rx="1" fill="currentColor" opacity={b.dim ? 0.35 : 1} />
  {/each}
</svg>

<style>
  svg {
    flex: none;
    color: var(--accent);
  }
</style>
