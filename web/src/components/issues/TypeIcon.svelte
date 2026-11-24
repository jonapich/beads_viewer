<script lang="ts">
  /** The issue type as a small line icon, coloured by the --type-* tokens through [data-type]. */
  let { type }: { type: string } = $props();
</script>

<svg
  class="ticon"
  data-type={type}
  viewBox="0 0 16 16"
  width="16"
  height="16"
  fill="none"
  stroke="currentColor"
  stroke-width="1.5"
  stroke-linecap="round"
  stroke-linejoin="round"
  role="img"
  aria-label={type}
>
  {#if type === "epic"}
    <path d="M9 1.5L3.5 9h4l-1 5.5L12.5 7h-4z" />
  {:else if type === "feature"}
    <path d="M8 1.8l1.9 4 4.3.5-3.2 3 .9 4.3L8 11.4l-3.9 2.2.9-4.3-3.2-3 4.3-.5z" />
  {:else if type === "bug"}
    <ellipse cx="8" cy="9.2" rx="3.2" ry="4.3" />
    <path d="M5.6 5.2L4 3.4M10.4 5.2L12 3.4M4.8 9H1.8M11.2 9h3M5.4 12.6l-1.8 1.6M10.6 12.6l1.8 1.6" />
  {:else if type === "task"}
    <rect x="2.2" y="2.2" width="11.6" height="11.6" rx="2.2" />
    <path d="M5.2 8.2l2 2 3.6-4" />
  {:else if type === "chore"}
    <circle cx="8" cy="8" r="2.4" />
    <path d="M8 1.5v2.3M8 12.2v2.3M1.5 8h2.3M12.2 8h2.3M3.4 3.4l1.6 1.6M11 11l1.6 1.6M12.6 3.4L11 5M5 11l-1.6 1.6" />
  {:else}
    <circle cx="8" cy="8" r="5.5" />
  {/if}
</svg>

<style>
  .ticon {
    flex: none;
    color: var(--text-muted);
  }
  .ticon[data-type="epic"] {
    color: var(--type-epic);
  }
  .ticon[data-type="feature"] {
    color: var(--type-feature);
  }
  .ticon[data-type="bug"] {
    color: var(--type-bug);
  }
  .ticon[data-type="task"] {
    color: var(--type-task);
  }
  .ticon[data-type="chore"] {
    color: var(--type-chore);
  }
</style>
