<script lang="ts">
  const rows = Array.from({ length: 10 }, (_, i) => i);
</script>

<div class="sk-list" aria-hidden="true">
  {#each rows as n (n)}
    <div class="sk">
      <i class="a"></i><i class="b"></i><i class="w{n % 4}"></i>
    </div>
  {/each}
</div>
<span class="sr" role="status">Loading issues…</span>

<style>
  .sk-list {
    border-top: 1px solid var(--border);
  }
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
  .w0 {
    width: 60%;
  }
  .w1 {
    width: 40%;
  }
  .w2 {
    width: 70%;
  }
  .w3 {
    width: 50%;
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
