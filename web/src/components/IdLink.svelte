<script lang="ts">
  /**
   * An issue id that opens the full issue page, like a Jira key. A real link: a plain click
   * navigates in the app (the router handles it), ctrl or middle click opens a new tab.
   */
  let { id }: { id: string } = $props();
</script>

<a class="idlink" href="/issues/{encodeURIComponent(id)}" title="Open full view">{id}</a>

<style>
  .idlink {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--accent);
    text-decoration: none;
    border-radius: var(--radius-sm);
    overflow-wrap: anywhere;
  }
  .idlink:hover {
    text-decoration: underline;
  }
</style>