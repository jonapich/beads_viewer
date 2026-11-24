<script lang="ts">
  import { editProblem, type IssueEdits } from "../../lib/issues/edits.svelte";
  import type { IssueDetail } from "../../lib/api/types";
  import EditButtons from "./EditButtons.svelte";
  import EditError from "./EditError.svelte";
  import { autofocus, refocus } from "./focus";

  interface Props {
    edits: IssueEdits;
    detail: IssueDetail;
    /** Another write on this issue is in flight. */
    busy: boolean;
    /** The editor closed (saved or cancelled): the page puts focus back on the Edit button. */
    onfinish: () => void;
  }

  let { edits, detail, busy, onfinish }: Props = $props();

  const session = $derived(edits.session(detail.id, "description"));
  const error = $derived(edits.error(detail.id, "description"));
  const saving = $derived(edits.isSaving(detail.id, "description"));
  const problem = $derived(session ? editProblem("description", session.draft) : "");

  let textarea = $state<HTMLTextAreaElement>();

  /**
   * Save (the button), Retry, or Save mine: one path, so focus always ends up somewhere sensible.
   * A success closes the editor (the page focuses the Edit button); a failure leaves it open with
   * the focused control (Retry, Save mine) gone, so focus goes back to the textarea.
   */
  async function commit(mode: "save" | "mine" | "retry" = "save"): Promise<void> {
    if (mode === "save" && !edits.canSave(detail.id, "description")) return;
    const ok = await (mode === "mine" ? edits.saveMine(detail.id, "description") : edits.save(detail.id, "description"));
    if (ok) onfinish();
    else void refocus(() => textarea);
  }

  function cancel(): void {
    edits.cancel(detail.id, "description");
    onfinish();
  }

  /** Dismiss on the error panel removes the button that had focus: back to the textarea. */
  function dismissError(): void {
    edits.dismissError(detail.id, "description");
    void refocus(() => textarea);
  }
</script>

{#if session}
  <form
    class="de"
    onsubmit={(e) => {
      e.preventDefault();
      void commit();
    }}
  >
    <textarea
      class="ta"
      aria-label="Description"
      aria-describedby={problem ? `desc-help-${detail.id} desc-hint-${detail.id}` : `desc-help-${detail.id}`}
      aria-invalid={problem ? "true" : undefined}
      spellcheck="true"
      value={session.draft}
      readonly={busy}
      aria-busy={saving ? "true" : undefined}
      bind:this={textarea}
      use:autofocus
      oninput={(e) => edits.setDraft(detail.id, "description", e.currentTarget.value)}
      onkeydown={(e) => {
        if (e.key === "Escape" && !busy) {
          e.preventDefault();
          cancel();
        }
      }}
    ></textarea>
    <p class="help" id="desc-help-{detail.id}">Markdown. Leading blank lines and trailing whitespace are dropped when saved.</p>
    {#if problem}<p class="hint" id="desc-hint-{detail.id}">{problem}</p>{/if}
    <EditButtons {saving} disabled={!edits.canSave(detail.id, "description")} {busy} reason={error ? "" : edits.lockNotice(detail.id)} oncancel={cancel} />
    {#if error}
      <EditError
        {error}
        field="description"
        draft={session.draft}
        {busy}
        reason={edits.lockNotice(detail.id)}
        onretry={() => void commit(error.kind === "conflict" ? "mine" : "retry")}
        ondismiss={() => (error.kind === "conflict" ? cancel() : dismissError())}
      />
    {/if}
  </form>
{/if}

<style>
  .de {
    max-width: 72ch;
  }
  .ta {
    display: block;
    width: 100%;
    min-height: 200px;
    max-height: 70vh;
    padding: 6px 8px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg);
    color: inherit;
    font: inherit;
    line-height: 1.45;
    resize: vertical;
  }
  .ta:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--accent) 35%, transparent);
    outline-offset: 0;
    border-color: var(--accent);
  }
  .ta:disabled {
    opacity: 0.7;
  }
  .help {
    margin: 4px 0 0;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .hint {
    margin: 4px 0 0;
    font-size: var(--text-xs);
    color: var(--danger);
  }
</style>
