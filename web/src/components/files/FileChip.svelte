<script lang="ts">
  import { copyText } from "../../lib/copy";
  import type { FileStat, FileStatus } from "../../lib/api/types";
  import { toasts } from "../../lib/toasts.svelte";
  import RevealButton from "./RevealButton.svelte";

  interface Props {
    stat: FileStat;
    /** The path as the text wrote it. */
    text: string;
  }
  let { stat, text }: Props = $props();

  // Every status but ok (a resolved file is a link, not a chip).
  const REASON: Record<FileStatus, string> = {
    ok: "can't open",
    missing: "not found",
    outside: "outside the project",
    ambiguous: "ambiguous",
    unsupported: "can't preview",
    too_large: "too large",
    invalid: "invalid path",
    unavailable: "can't check",
  };
  const reason = $derived(REASON[stat.status]);
  const target = $derived(stat.abs || stat.path);
</script>

<span class="chip" title={stat.abs ?? stat.path}>
  <span class="p">{text}</span>
  <span class="why">{reason}</span>
  <button type="button" class="cp" aria-label="Copy path" title="Copy path" onclick={(e) => { e.stopPropagation(); void copyText(target, "Copied path", { clipboard: navigator.clipboard, toasts }); }}>Copy path</button>
  <!-- A file that exists under a served directory but is not previewed can still be shown in Explorer. -->
  {#if stat.abs && (stat.status === "unsupported" || stat.status === "too_large")}<RevealButton abs={stat.abs} />{/if}
</span>

<style>
  .chip {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    max-width: 100%;
    padding: 0 6px;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-faint);
    vertical-align: baseline;
  }
  .p {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .why {
    flex: none;
    font-family: var(--font-sans);
  }
  .cp {
    flex: none;
    padding: 0;
    border: 0;
    background: none;
    font-family: var(--font-sans);
    font-size: var(--text-xs);
    color: var(--accent);
    cursor: pointer;
  }
  .cp:hover {
    text-decoration: underline;
  }
</style>