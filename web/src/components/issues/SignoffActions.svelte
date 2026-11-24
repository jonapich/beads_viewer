<script lang="ts">
  import { MAX_TEXT, READ_ONLY_TITLE, tooLong } from "../../lib/issues/writes";

  interface Props {
    issueId: string;
    /** A write on this issue is in flight. */
    busy: boolean;
    /** The tracker (or this issue) takes no writes. */
    disabled: boolean;
    /** The dispute note form is showing. */
    open: boolean;
    /** The unsent note; the owner keeps it, so it survives a re-render. */
    note: string;
    onapprove: () => void;
    /** Show the note form. */
    onopen: () => void;
    /** Hide the note form (the note is kept). */
    oncancel: () => void;
    onnote: (note: string) => void;
    ondispute: (note: string) => void;
  }

  let { issueId, busy, disabled, open, note, onapprove, onopen, oncancel, onnote, ondispute }: Props = $props();

  const blank = $derived(note.trim() === "");
  const long = $derived(tooLong(note));
  const inert = $derived(busy || disabled);
</script>

<div class="so" data-issue={issueId}>
  <p class="hint">This closed issue is waiting for your sign-off.</p>
  {#if !open}
    <div class="acts">
      <button type="button" class="btn primary" disabled={inert} title={disabled ? READ_ONLY_TITLE : ""} onclick={onapprove}>
        {busy ? "Saving…" : "Approve"}
      </button>
      <button type="button" class="btn danger" disabled={inert} title={disabled ? READ_ONLY_TITLE : ""} onclick={onopen}>
        Dispute…
      </button>
    </div>
  {:else}
    <label class="lab" for="dispute-note-{issueId}">Dispute note (required)</label>
    <textarea
      id="dispute-note-{issueId}"
      class="ta"
      rows="4"
      maxlength={MAX_TEXT}
      placeholder="What is wrong or missing?"
      disabled={inert}
      aria-invalid={long ? "true" : undefined}
      aria-describedby={long ? `dispute-long-${issueId}` : undefined}
      value={note}
      oninput={(e) => onnote(e.currentTarget.value)}
    ></textarea>
    {#if long}<p class="warn" id="dispute-long-{issueId}" role="alert">Too long: the note can be at most {MAX_TEXT} bytes.</p>{/if}
    <div class="acts">
      <button type="button" class="btn danger" disabled={inert || blank || long} onclick={() => ondispute(note)}>
        {busy ? "Saving…" : "Send dispute"}
      </button>
      <button type="button" class="btn" disabled={busy} onclick={oncancel}>Cancel</button>
    </div>
  {/if}
</div>

<style>
  .so {
    margin-top: 8px;
  }
  .hint {
    margin: 0 0 8px;
    font-size: var(--text-sm);
    color: var(--text-muted);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 8px;
  }
  .lab {
    display: block;
    margin-bottom: 4px;
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .ta {
    display: block;
    width: 100%;
    min-height: 72px;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: inherit;
    font: inherit;
    font-size: var(--text-sm);
    resize: vertical;
  }
  .warn {
    margin: 4px 0 0;
    font-size: var(--text-xs);
    color: var(--danger);
  }
  .btn {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .btn:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .btn.primary {
    background: var(--accent-strong);
    border-color: var(--accent-strong);
    color: var(--on-accent);
  }
  .btn.primary:hover:not(:disabled) {
    background: var(--accent-strong);
    filter: brightness(1.1);
  }
  .btn.danger {
    border-color: var(--danger);
    color: var(--danger);
  }
  .btn.danger:hover:not(:disabled) {
    background: var(--danger-subtle);
  }
</style>
