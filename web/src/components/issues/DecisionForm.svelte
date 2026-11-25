<script lang="ts">
  import type { DecisionInfo } from "../../lib/api/types";
  import { decisionComment } from "../../lib/issues/decision";
  import { MAX_TEXT, READ_ONLY_TITLE, tooLong } from "../../lib/issues/writes";

  interface Props {
    issueId: string;
    decision: DecisionInfo;
    busy: boolean;
    /** The tracker (or this issue) takes no writes. */
    disabled: boolean;
    /** The picked option's key, "" when the user has not picked (nothing is pre-selected, the recommended option included). */
    option: string;
    /** The unsent note or answer; the owner keeps both, so they survive a re-render. */
    text: string;
    onoption: (key: string) => void;
    ontext: (text: string) => void;
    onanswer: (answer: { option?: string; text?: string }) => void;
  }

  let { issueId, decision, busy, disabled, option, text, onoption, ontext, onanswer }: Props = $props();

  const options = $derived(decision.options);
  // Nothing is pre-selected, not even the recommended option (it only carries a tag): a pre-selected
  // answer reads as a decision already taken. Post stays off until an option is picked or a note typed.
  const chosenKey = $derived(options.some((o) => o.key === option) ? option : "");
  const chosen = $derived(options.find((o) => o.key === chosenKey) ?? null);
  // The one Tab stop of the radio group: the checked option, else the first.
  const tabKey = $derived(chosen ? chosen.key : (options[0]?.key ?? ""));
  const hasText = $derived(text.trim() !== "");
  const long = $derived(tooLong(text));
  const ready = $derived((chosen !== null || hasText) && !long);
  const inert = $derived(busy || disabled);
  const noteWord = $derived(options.length > 0 ? "note" : "answer");

  function post(): void {
    onanswer({ ...(chosen ? { option: chosen.key } : {}), ...(hasText ? { text } : {}) });
  }

  // The ARIA radio pattern: the checked option is the one Tab stop, and the arrow keys move the
  // choice (and focus) to the next or previous option, wrapping around; Home and End jump to the ends.
  function onRadioKey(e: KeyboardEvent): void {
    const radios = [...(e.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('[role="radio"]:not(:disabled)')];
    const at = radios.findIndex((r) => r === document.activeElement);
    if (at < 0 || radios.length === 0) return;
    let to: number;
    switch (e.key) {
      case "ArrowDown":
      case "ArrowRight":
        to = (at + 1) % radios.length;
        break;
      case "ArrowUp":
      case "ArrowLeft":
        to = (at - 1 + radios.length) % radios.length;
        break;
      case "Home":
        to = 0;
        break;
      case "End":
        to = radios.length - 1;
        break;
      default:
        return;
    }
    e.preventDefault();
    const next = radios[to];
    const key = next?.dataset["key"];
    if (!next || key === undefined) return;
    onoption(key);
    next.focus({ preventScroll: true });
  }
</script>

<div class="df" data-issue={issueId}>
  {#if options.length > 0}
    <!-- The group itself is not a Tab stop: focus sits on its radios (roving tabindex), and the keys bubble up here. -->
    <!-- svelte-ignore a11y_interactive_supports_focus -->
    <div class="opts" role="radiogroup" aria-label="Options" onkeydown={onRadioKey}>
      {#each options as o (o.key)}
        <button
          type="button"
          class="o"
          class:on={o.key === chosenKey}
          role="radio"
          aria-checked={o.key === chosenKey}
          tabindex={o.key === tabKey ? 0 : -1}
          data-key={o.key}
          disabled={inert}
          onclick={() => onoption(o.key)}
        >
          <span class="k">{o.key}</span>
          <span class="t">{o.text}</span>
          {#if o.recommended}<span class="tag">Recommended</span>{/if}
        </button>
      {/each}
    </div>
  {:else}
    <p class="faint">The description offers no options; an answer needs free text.</p>
  {/if}
  <label class="lab" for="answer-text-{issueId}">{options.length > 0 ? "Add a note (optional)" : "Your answer"}</label>
  <textarea
    id="answer-text-{issueId}"
    class="ta"
    rows="3"
    maxlength={MAX_TEXT}
    disabled={inert}
    aria-invalid={long ? "true" : undefined}
    aria-describedby={long ? `answer-long-${issueId}` : undefined}
    value={text}
    oninput={(e) => ontext(e.currentTarget.value)}
  ></textarea>
  {#if long}<p class="warn" id="answer-long-{issueId}" role="alert">Too long: the {noteWord} can be at most {MAX_TEXT} bytes.</p>{/if}
  {#if ready}
    <div class="prev" data-preview>
      <span class="pl">Will post when you click Post answer:</span>
      <code>{decisionComment(chosen, text)}</code> · removes <code>{decision.label}</code>
    </div>
  {/if}
  <button type="button" class="btn primary" disabled={inert || !ready} title={disabled ? READ_ONLY_TITLE : ""} onclick={post}>
    {busy ? "Saving…" : "Post answer"}
  </button>
</div>

<style>
  .df {
    margin-top: 8px;
  }
  .opts {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 8px;
  }
  .o {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  .o:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .o.on {
    border-color: var(--accent);
    background: var(--accent-subtle);
  }
  .o:disabled {
    opacity: 0.7;
    cursor: not-allowed;
  }
  .k {
    flex: none;
    font-family: var(--font-mono);
    color: var(--text-muted);
  }
  .t {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .tag {
    flex: none;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    background: var(--success-subtle);
    color: var(--success);
    font-size: var(--text-xs);
    line-height: 16px;
  }
  .faint {
    margin: 0 0 8px;
    font-size: var(--text-sm);
    color: var(--text-faint);
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
    min-height: 56px;
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
  /* A preview of what the button will do, not a record of anything: muted, in a dashed box. */
  .prev {
    margin: 8px 0;
    padding: 6px 8px;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    color: var(--text-muted);
    overflow-wrap: anywhere;
  }
  .pl {
    display: block;
    margin-bottom: 2px;
    color: var(--text-faint);
  }
  .btn {
    height: 28px;
    margin-top: 4px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
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
</style>
