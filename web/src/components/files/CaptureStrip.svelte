<script lang="ts">
  import { appCaps } from "../../lib/desk/nav-counts.svelte";
  import { fileStats, requestStat } from "../../lib/files.svelte";
  import FileChip from "./FileChip.svelte";
  import FileLink from "./FileLink.svelte";
  import RevealButton from "./RevealButton.svelte";
  import Thumb from "./Thumb.svelte";

  interface Props {
    /** The capture paths of a sign-off, as written. */
    captures: readonly string[];
    /** tile 144x96 (the sign-off card), compact 96x64 (the drawer). */
    size?: "tile" | "compact";
  }
  let { captures, size = "tile" }: Props = $props();

  $effect(() => {
    if (!appCaps.files) return;
    for (const c of captures) requestStat(c);
  });

  /** A capture that failed to load as a picture: drawn as a chip instead. */
  let broken = $state<Record<string, boolean>>({});

  const baseName = (p: string): string => p.split(/[\\/]/).filter(Boolean).pop() ?? p;
</script>

<!-- One lightbox group: the captures of this list step together. -->
<div class="strip" data-lightbox-group>
  {#each captures as c, i (i)}
    {@const stat = appCaps.files ? fileStats.get(c) : undefined}
    {@const framed = !appCaps.files || !stat || (stat.status === "ok" && !!stat.url && !broken[c] && (stat.kind === "image" || stat.kind === "video"))}
    <div class="cell {size}" class:wide={!framed}>
      {#if !appCaps.files}
        <span class="plain" title={c}>{baseName(c)}</span>
      {:else if !stat}
        <span class="ph {size}" aria-hidden="true"></span>
        <span class="nm">{baseName(c)}</span>
      {:else if stat.status === "ok" && stat.url && !broken[c] && (stat.kind === "image" || stat.kind === "video")}
        <Thumb {stat} alt={baseName(c)} {size} onerror={() => (broken[c] = true)} />
        <span class="nm" title={stat.abs}>{baseName(c)}<RevealButton abs={stat.abs ?? ""} /></span>
      {:else if stat.status === "ok" && stat.url && !broken[c]}
        <FileLink {stat} text={baseName(c)} />
      {:else}
        <FileChip stat={broken[c] ? { ...stat, status: "unsupported" } : stat} text={c} />
      {/if}
    </div>
  {/each}
</div>

<style>
  .strip {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 2px;
  }
  .cell {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .cell.tile {
    max-width: 144px;
  }
  .cell.compact {
    max-width: 96px;
  }
  /* A chip or a file link is as wide as its text, not as wide as a tile. */
  .cell.wide {
    max-width: 100%;
  }
  .nm {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .plain {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .ph {
    display: block;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }
  .ph.tile {
    width: 144px;
    height: 96px;
  }
  .ph.compact {
    width: 96px;
    height: 64px;
  }
</style>