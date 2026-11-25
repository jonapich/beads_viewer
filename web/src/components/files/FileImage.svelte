<script lang="ts">
  import { appCaps } from "../../lib/desk/nav-counts.svelte";
  import { fileStats, requestStat } from "../../lib/files.svelte";
  import FileChip from "./FileChip.svelte";
  import FileLink from "./FileLink.svelte";
  import Thumb from "./Thumb.svelte";

  interface Props {
    /** The local path as written in ![alt](path). */
    src: string;
    alt: string;
    /** Draw a thumbnail; false (titles, one-line fields) draws a link with the alt text. */
    thumbs: boolean;
  }
  let { src, alt, thumbs }: Props = $props();

  $effect(() => {
    if (appCaps.files) requestStat(src);
  });
  const stat = $derived(appCaps.files ? fileStats.get(src) : undefined);
  const label = $derived(alt || src);
  let broken = $state(false);
</script>

{#if !appCaps.files}
  {label}
{:else if !stat}
  {#if thumbs}<span class="ph" aria-hidden="true"></span>{:else}{label}{/if}
{:else if stat.status !== "ok" || broken}
  <FileChip stat={broken ? { ...stat, status: "unsupported" } : stat} text={label} />
{:else if thumbs && (stat.kind === "image" || stat.kind === "video")}
  <Thumb {stat} {alt} onerror={() => (broken = true)} />
{:else}
  <FileLink {stat} text={label} />
{/if}

<style>
  /* The thumbnail's frame while the path is being looked up: no layout jump when it resolves. */
  .ph {
    display: inline-block;
    vertical-align: top;
    width: 96px;
    height: 64px;
    margin: 4px 8px 4px 0;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }
</style>