<script lang="ts">
  /** A 4px value bar. The fill width is a .pct-N class (app.css): no style attributes under the CSP. */
  let { pct }: { pct: number } = $props();
  const n = $derived(Math.min(100, Math.max(0, Math.round(pct))));
</script>

<span class="bar" aria-hidden="true"><i class="pct-{n}"></i></span>

<style>
  .bar {
    display: block;
    height: 4px;
    border-radius: 2px;
    background: var(--bg-hover);
    overflow: hidden;
    min-width: 0;
  }
  i {
    display: block;
    height: 100%;
    background: var(--accent);
    border-radius: 2px;
  }
</style>
