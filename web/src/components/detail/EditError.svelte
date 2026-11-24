<script lang="ts">
  import { clip, displayValue, type EditError, type EditField } from "../../lib/issues/edits.svelte";

  interface Props {
    error: EditError;
    /** The field the panel belongs to: a status is shown by its label, not its stored value. */
    field: EditField;
    /** What the user typed or picked (shown as "Yours" on a conflict). */
    draft: string;
    /** A save is in flight. */
    busy: boolean;
    /** Why Retry / Save mine can't go ahead right now ("" when they can): shown in the panel. */
    reason?: string;
    /** Retry (after an error) or Save mine (after a conflict). */
    onretry: () => void;
    /** Dismiss (an error) or Discard my edit (a conflict). */
    ondismiss: () => void;
  }

  let { error, field, draft, busy, reason = "", onretry, ondismiss }: Props = $props();

  const uid = $props.id();
  const shown = (v: string): string => (v.trim() === "" ? "(empty)" : clip(displayValue(field, v)));
  const firstLine = $derived(error.message.split("\n")[0] ?? "");
  const hasMore = $derived(error.message.trim() !== firstLine.trim());
  // aria-disabled, not disabled, so the button a keyboard user pressed keeps its focus and can say why.
  const held = $derived(reason !== "");
</script>

{#snippet why()}
  {#if held}<div class="line why" id="{uid}-why">{reason}</div>{/if}
{/snippet}

{#if error.kind === "conflict"}
  <div class="panel warn" role="alert">
    <div class="msg">This issue changed since you opened it.</div>
    <div class="line"><span class="k">Now:</span> {shown(error.now ?? "")}</div>
    <div class="line"><span class="k">Yours:</span> {shown(draft)}</div>
    {#if error.sameField}<div class="line">Something else on the issue changed.</div>{/if}
    {@render why()}
    <div class="btns">
      <button
        type="button"
        class="b primary"
        disabled={busy}
        aria-disabled={held ? "true" : undefined}
        aria-describedby={held ? `${uid}-why` : undefined}
        onclick={() => {
          if (!held) onretry();
        }}>Save mine</button
      >
      <button type="button" class="b" disabled={busy} onclick={ondismiss}>Discard my edit</button>
    </div>
  </div>
{:else}
  <div class="panel err" role="alert">
    <div class="msg">Couldn't save: {clip(firstLine, 200)}</div>
    {#if hasMore}
      <details>
        <summary>Details</summary>
        <pre>{error.message}</pre>
      </details>
    {/if}
    {@render why()}
    <div class="btns">
      <button
        type="button"
        class="b primary"
        disabled={busy}
        aria-disabled={held ? "true" : undefined}
        aria-describedby={held ? `${uid}-why` : undefined}
        onclick={() => {
          if (!held) onretry();
        }}>Retry</button
      >
      <button type="button" class="b" disabled={busy} onclick={ondismiss}>Dismiss</button>
    </div>
  </div>
{/if}
<style>
  .panel {
    margin-top: 8px;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    font-weight: 400;
    line-height: 1.4;
    text-align: left;
    width: 100%;
    min-width: 0;
  }
  .warn {
    background: var(--warning-subtle);
    border-color: color-mix(in srgb, var(--warning) 40%, transparent);
  }
  .err {
    background: var(--danger-subtle);
    border-color: color-mix(in srgb, var(--danger) 40%, transparent);
  }
  .msg {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .line {
    margin-top: 2px;
    overflow-wrap: anywhere;
  }
  .k {
    color: var(--text-muted);
  }
  details {
    margin-top: 4px;
  }
  summary {
    cursor: pointer;
    color: var(--text-muted);
  }
  pre {
    margin: 4px 0 0;
    max-height: 160px;
    overflow: auto;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .btns {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 8px;
  }
  .b {
    height: 24px;
    padding: 0 10px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .why {
    color: var(--text-muted);
  }
  .b:hover:not(:disabled):not([aria-disabled="true"]) {
    background: var(--bg-hover);
  }
  .b:disabled,
  .b[aria-disabled="true"] {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .b.primary {
    background: var(--accent-strong);
    border-color: var(--accent-strong);
    color: var(--on-accent);
  }
</style>
