<script lang="ts">
  import { appCaps } from "../../lib/desk/nav-counts.svelte";
  import { fileStats, requestStat } from "../../lib/files.svelte";
  import FileChip from "./FileChip.svelte";
  import FileLink from "./FileLink.svelte";

  interface Props {
    /** The path as written, the text to show, whether the text made it an explicit link, and whether it came from a code span. */
    path: string;
    text: string;
    explicit: boolean;
    code: boolean;
  }
  let { path, text, explicit, code }: Props = $props();

  // Only a server that serves files is asked; without one, a path is just text, as before.
  $effect(() => {
    if (appCaps.files) requestStat(path);
  });
  const stat = $derived(appCaps.files ? fileStats.get(path) : undefined);
</script>

{#if stat?.status === "ok" && stat.url}<FileLink {stat} {text} {code} />{:else if stat && explicit}<FileChip {stat} {text} />{:else if code}<code>{text}</code>{:else}{text}{/if}

<style>
  code {
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0 4px;
  }
</style>