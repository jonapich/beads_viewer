<script lang="ts">
  import { parseMarkdown } from "../../lib/markdown";
  import Inlines from "./Inlines.svelte";

  /** Markdown text of a free-text field. Rendered from an AST: no HTML from the tracker reaches the DOM. */
  let { src }: { src: string } = $props();

  const blocks = $derived(parseMarkdown(src));
</script>

<!-- data-lightbox-group: the images of this text step together in the lightbox. -->
<div class="prose" data-lightbox-group>
  {#each blocks as b, i (i)}
    {#if b.t === "p"}
      <p><Inlines nodes={b.c} thumbs /></p>
    {:else if b.t === "h"}
      <h4><Inlines nodes={b.c} /></h4>
    {:else if b.t === "ul"}
      <ul>{#each b.items as item, j (j)}<li><Inlines nodes={item} thumbs /></li>{/each}</ul>
    {:else if b.t === "ol"}
      <ol>{#each b.items as item, j (j)}<li><Inlines nodes={item} thumbs /></li>{/each}</ol>
    {:else if b.t === "pre"}
      <pre><code>{b.v}</code></pre>
    {:else if b.t === "table"}
      <div class="tw">
        <table>
          <thead>
            <tr>{#each b.head as cell, j (j)}<th><Inlines nodes={cell} thumbs /></th>{/each}</tr>
          </thead>
          <tbody>
            {#each b.rows as row, r (r)}
              <tr>{#each row as cell, j (j)}<td><Inlines nodes={cell} thumbs /></td>{/each}</tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/each}
</div>

<style>
  .prose {
    font-size: var(--text-base);
    line-height: 1.55;
    max-width: 72ch;
    overflow-wrap: anywhere;
  }
  p,
  h4 {
    margin: 0;
  }
  p + p,
  p + ul,
  ul + p,
  pre + p,
  p + pre,
  h4 + p,
  .tw + p,
  p + .tw,
  ol + p {
    margin-top: var(--space-2);
  }
  h4 {
    font-size: var(--text-base);
    font-weight: 600;
    margin-top: var(--space-3);
  }
  h4:first-child {
    margin-top: 0;
  }
  ul,
  ol {
    padding-left: 20px;
    margin: 6px 0;
  }
  li {
    margin: 2px 0;
  }
  pre {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    line-height: 1.5;
    background: var(--bg-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2) var(--space-3);
    overflow-x: auto;
    margin: var(--space-2) 0;
    white-space: pre;
  }
  .tw {
    overflow-x: auto;
    margin: var(--space-2) 0;
  }
  table {
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  th,
  td {
    border: 1px solid var(--border);
    padding: 3px var(--space-2);
    text-align: left;
    vertical-align: top;
  }
  th {
    background: var(--bg-subtle);
  }
</style>
