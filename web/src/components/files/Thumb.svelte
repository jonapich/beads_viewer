<script lang="ts">
  import type { FileStat } from "../../lib/api/types";
  import { isPlainClick, openFromElement } from "../../lib/lightbox.svelte";

  interface Props {
    /** A resolved image or video (status ok). */
    stat: FileStat;
    alt: string;
    /** The image failed to load: the caller shows the path chip instead. */
    onerror: () => void;
    /** inline: up to 320x200 in text; tile: 144x96 captures; compact: 96x64 (the drawer). */
    size?: "inline" | "tile" | "compact";
  }
  let { stat, alt, onerror, size = "inline" }: Props = $props();

  const video = $derived(stat.kind === "video");

  function click(e: MouseEvent): void {
    if (!isPlainClick(e)) return; // ctrl, shift and middle click open the file in a new tab
    e.preventDefault();
    openFromElement(e.currentTarget as HTMLElement);
  }
</script>

<a
  class="thumb {size}"
  href={stat.url}
  target="_blank"
  rel="noopener noreferrer"
  title={stat.abs}
  data-lb-url={stat.url}
  data-lb-kind={stat.kind}
  data-lb-abs={stat.abs}
  data-lb-alt={alt}
  onclick={click}
>
  {#if video}
    <video src={stat.url} preload="metadata" muted {onerror}></video>
    <span class="play" aria-hidden="true">▶</span>
  {:else}
    <img src={stat.url} {alt} loading="lazy" decoding="async" {onerror} />
  {/if}
</a>

<style>
  .thumb {
    position: relative;
    display: inline-block;
    vertical-align: top;
    margin: 4px 8px 4px 0;
    line-height: 0;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .thumb:hover {
    border-color: var(--border-strong);
  }
  /* A tile has a fixed frame so a row of captures lines up whatever the pictures measure. */
  .thumb.tile,
  .thumb.compact {
    margin: 0;
    background: var(--bg-subtle);
  }
  .thumb.tile img,
  .thumb.tile video {
    width: 144px;
    height: 96px;
    max-width: 144px;
    max-height: 96px;
  }
  .thumb.compact img,
  .thumb.compact video {
    width: 96px;
    height: 64px;
    max-width: 96px;
    max-height: 64px;
  }
  img,
  video {
    display: block;
    max-width: 320px;
    max-height: 200px;
    object-fit: contain;
  }
  .play {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 24px;
    line-height: 1;
    color: var(--on-accent);
    text-shadow: 0 1px 4px rgb(0 0 0 / 0.6);
    pointer-events: none;
  }
</style>