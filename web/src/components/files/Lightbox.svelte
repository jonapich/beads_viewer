<script lang="ts">
  import { copyText } from "../../lib/copy";
  import { closeLightbox, lightbox, stepLightbox } from "../../lib/lightbox.svelte";
  import { toasts } from "../../lib/toasts.svelte";
  import RevealButton from "./RevealButton.svelte";

  let dialog = $state<HTMLDialogElement>();
  const item = $derived(lightbox.index >= 0 ? (lightbox.items[lightbox.index] ?? null) : null);
  const many = $derived(lightbox.items.length > 1);

  // The native dialog is the modal: it traps focus, closes on Esc and dims the page.
  $effect(() => {
    if (!dialog) return;
    if (item && !dialog.open) dialog.showModal();
    else if (!item && dialog.open) dialog.close();
  });

  /** Esc (and anything else the browser closes it by): keep the state in step. */
  function onclose(): void {
    if (lightbox.index >= 0) closeLightbox();
  }

  /** A click on the backdrop (the dialog element itself, outside its content) closes it. */
  function onclick(e: MouseEvent): void {
    if (e.target === dialog) closeLightbox();
  }
</script>

<dialog bind:this={dialog} aria-label={item ? `Preview of ${item.name}` : "Preview"} {onclose} {onclick}>
  {#if item}
    <div class="media">
      {#key item.url}
        {#if item.kind === "video"}
          <video src={item.url} controls autoplay muted></video>
        {:else}
          <img src={item.url} alt={item.alt} />
        {/if}
      {/key}
    </div>
    <div class="foot">
      <button type="button" class="b" aria-label="Previous image" disabled={lightbox.index <= 0} onclick={() => stepLightbox(-1)}>‹</button>
      <button type="button" class="b" aria-label="Next image" disabled={lightbox.index >= lightbox.items.length - 1} onclick={() => stepLightbox(1)}>›</button>
      {#if many}<span class="n">{lightbox.index + 1} / {lightbox.items.length}</span>{/if}
      <span class="name" title={item.abs}>{item.name}</span>
      <button type="button" class="b txt" onclick={() => void copyText(item.abs || item.name, "Copied path", { clipboard: navigator.clipboard, toasts })}>Copy path</button>
      <RevealButton abs={item.abs} text />
      <a class="b txt" href={item.url} target="_blank" rel="noopener noreferrer">Open in new tab ↗</a>
      <button type="button" class="b" aria-label="Close preview" onclick={closeLightbox}>✕</button>
    </div>
  {/if}
</dialog>

<style>
  dialog {
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow-pop);
    max-width: 94vw;
    max-height: 94vh;
    overflow: hidden;
  }
  dialog::backdrop {
    background: rgb(0 0 0 / 0.72);
  }
  .media {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-subtle);
  }
  .media img,
  .media video {
    display: block;
    max-width: 92vw;
    max-height: 86vh;
    object-fit: contain;
  }
  .foot {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    padding: 4px var(--space-3);
    border-top: 1px solid var(--border);
    font-size: var(--text-sm);
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
  }
  .n {
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .b {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 28px;
    height: 28px;
    padding: 0 var(--space-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    color: var(--text);
    font-size: var(--text-sm);
    text-decoration: none;
    cursor: pointer;
    white-space: nowrap;
  }
  .b:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .b:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
</style>