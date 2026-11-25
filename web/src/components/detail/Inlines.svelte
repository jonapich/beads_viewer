<script lang="ts">
  import { linkHost } from "../../lib/issues/detail";
  import type { Inline } from "../../lib/markdown";
  import FileImage from "../files/FileImage.svelte";
  import FileRef from "../files/FileRef.svelte";

  interface Props {
    /** Already-parsed inline runs (lib/markdown.ts). Text is interpolated, so it is escaped; hrefs are http(s) only. */
    nodes: readonly Inline[];
    /** Draw local images as thumbnails (long text); false (titles, one-line fields) draws them as links. */
    thumbs?: boolean;
  }

  let { nodes, thumbs = false }: Props = $props();
</script>

{#each nodes as n, i (i)}{#if n.t === "text"}{n.v}{:else if n.t === "code"}<code>{n.v}</code>{:else if n.t === "strong"}<strong>{n.v}</strong>{:else if n.t === "link"}<a href={n.href} target="_blank" rel="noopener noreferrer">{n.v}</a>{:else if n.t === "img"}{#if n.remote}<a class="rimg" href={n.src} target="_blank" rel="noopener noreferrer" title="Remote images are not loaded: open it in a new tab"><svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="3" width="12" height="10" rx="1.5" /><circle cx="6" cy="7" r="1.3" /><path d="M2.5 12l3.5-3 2.5 2 2-1.5 3 2.5" /></svg> {n.alt || linkHost(n.src)} ↗</a>{:else}<FileImage src={n.src} alt={n.alt} {thumbs} />{/if}{:else if n.t === "file"}<FileRef path={n.path} text={n.v} explicit={n.explicit} code={n.code} />{:else}<br />{/if}{/each}

<style>
  code {
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0 4px;
  }
  .rimg {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    overflow-wrap: anywhere;
  }
</style>