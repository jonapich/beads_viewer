<script lang="ts">
  let { title }: { title: string } = $props();
</script>

<h1>{title}</h1>
<p class="muted">This view is not built yet.</p>

<style>
  .muted {
    color: var(--text-muted);
  }
</style>