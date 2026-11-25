<script lang="ts">
  import type { FileStat } from "../../lib/api/types";
  import { isPlainClick, openFromElement } from "../../lib/lightbox.svelte";
  import RevealButton from "./RevealButton.svelte";

  interface Props {
    /** A resolved file (status ok). */
    stat: FileStat;
    text: string;
    /** From a code span: keeps the code look. */
    code?: boolean;
  }
  let { stat, text, code = false }: Props = $props();

  const media = $derived(stat.kind === "image" || stat.kind === "video");

  function click(e: MouseEvent): void {
    // Images and video open in the lightbox on a plain click; ctrl, shift and middle click, pdf and text
    // follow the link into a new tab served by bv.
    if (!media || !isPlainClick(e)) return;
    e.preventDefault();
    openFromElement(e.currentTarget as HTMLElement);
  }
</script>

<a
  class="flink"
  class:code
  href={stat.url}
  target="_blank"
  rel="noopener noreferrer"
  title={stat.abs}
  data-lb-url={media ? stat.url : undefined}
  data-lb-kind={media ? stat.kind : undefined}
  data-lb-abs={media ? stat.abs : undefined}
  data-lb-alt={media ? text : undefined}
  onclick={click}>{text}</a
>{#if stat.abs}<RevealButton abs={stat.abs} />{/if}

<style>
  .flink {
    overflow-wrap: anywhere;
  }
  .flink.code {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0 4px;
    text-decoration: none;
  }
  .flink.code:hover {
    text-decoration: underline;
  }
</style>