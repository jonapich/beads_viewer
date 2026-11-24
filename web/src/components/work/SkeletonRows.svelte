<script lang="ts">
  /** Placeholder rows while a view's first fetch is running. `label` is read out to screen readers. */
  let { n = 8, label = "Loading…" }: { n?: number; label?: string } = $props();
  const rows = $derived(Array.from({ length: n }, (_, i) => i));
</script>

<div class="sk-list" aria-hidden="true">
  {#each rows as i (i)}
    <div class="sk"><i class="a"></i><i class="b"></i><i class={i % 2 ? "w50" : "w70"}></i></div>
  {/each}
</div>
<span class="sr" role="status">{label}</span>

<style>
  .sk {
    height: 40px;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 12px;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  i {
    display: block;
    height: 10px;
    border-radius: 5px;
    background: linear-gradient(90deg, var(--bg-subtle), var(--bg-hover), var(--bg-subtle));
    background-size: 200% 100%;
    animation: shim 1.4s linear infinite;
  }
  .a {
    width: 16px;
  }
  .b {
    width: 80px;
  }
  .w50 {
    width: 50%;
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
    i {
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
