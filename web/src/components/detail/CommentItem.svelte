<script lang="ts">
  import type { IssueComment } from "../../lib/api/types";
  import { absTime, commentSummary, relTime } from "../../lib/issues/detail";
  import JsonView from "./JsonView.svelte";
  import KindBadge from "./KindBadge.svelte";
  import Markdown from "./Markdown.svelte";

  interface Props {
    comment: IssueComment;
    /** Kinded comments only: the body is showing (a plain comment always shows it). */
    open: boolean;
    /** Show a JSON body as the raw text instead of the structured view. */
    raw: boolean;
    now: number;
    ontoggle: () => void;
    onraw: () => void;
  }

  let { comment: c, open, raw, now, ontoggle, onraw }: Props = $props();

  const summary = $derived(commentSummary(c));
</script>

{#snippet body()}
  {#if c.data}
    <div class="cbh"><button type="button" class="link" onclick={onraw}>{raw ? "Structured" : "Raw"}</button></div>
    {#if raw}
      <pre class="raw">{JSON.stringify(c.data, null, 2)}</pre>
    {:else}
      <JsonView value={c.data} />
    {/if}
  {:else}
    <Markdown src={c.text} />
  {/if}
{/snippet}

{#if c.kind}
  <div class="cmt" data-cid={c.id}>
    <button type="button" class="row" aria-expanded={open} onclick={ontoggle}>
      <svg class="chev" viewBox="0 0 10 10" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 1.5l3.5 3.5L3 8.5" /></svg>
      <KindBadge kind={c.kind} />
      <span class="au">{c.author}</span>
      <span class="wh" title={absTime(c.created_at)}>{relTime(c.created_at, now)}</span>
      {#if !open}<span class="sm">{summary}</span>{/if}
    </button>
    {#if open}
      <div class="cbody" data-kind={c.kind}>{@render body()}</div>
    {/if}
  </div>
{:else}
  <div class="cmt plain" data-cid={c.id}>
    <div class="hd">
      <span class="au">{c.author}</span>
      <span class="wh" title={absTime(c.created_at)}>{relTime(c.created_at, now)}</span>
    </div>
    {@render body()}
  </div>
{/if}

<style>
  .cmt {
    border-top: 1px solid var(--border);
  }
  .cmt:first-child {
    border-top: 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    min-height: 32px;
    padding: 0 var(--space-1);
    border: 0;
    background: transparent;
    text-align: left;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .chev {
    flex: none;
    color: var(--text-faint);
    transition: transform 0.12s;
  }
  .row[aria-expanded="true"] .chev {
    transform: rotate(90deg);
  }
  .au {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .wh {
    font-size: var(--text-sm);
    color: var(--text-faint);
    white-space: nowrap;
  }
  .sm {
    flex: 1 1 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-muted);
  }
  .cbody {
    --kc: var(--border-strong);
    margin: var(--space-1) 0 var(--space-3) var(--space-3);
    padding: var(--space-2) 0 var(--space-2) var(--space-3);
    border-left: 2px solid var(--kc);
    min-width: 0;
  }
  .cbody[data-kind="user"],
  .cbody[data-kind="answer"] {
    --kc: var(--kind-user);
  }
  .cbody[data-kind="brief"] {
    --kc: var(--kind-brief);
  }
  .cbody[data-kind="advice"] {
    --kc: var(--kind-advice);
  }
  .cbody[data-kind="coder"] {
    --kc: var(--kind-coder);
  }
  .cbody[data-kind="review"] {
    --kc: var(--kind-review);
  }
  .cbody[data-kind="verify"] {
    --kc: var(--kind-verify);
  }
  .cbody[data-kind="signoff"] {
    --kc: var(--kind-signoff);
  }
  .plain {
    padding: 6px var(--space-1) var(--space-3);
  }
  .hd {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
    margin-bottom: var(--space-1);
  }
  .cbh {
    display: flex;
    justify-content: flex-end;
    margin-bottom: var(--space-1);
  }
  .link {
    border: 0;
    background: none;
    padding: 0;
    color: var(--accent);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .link:hover {
    text-decoration: underline;
  }
  .raw {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2) var(--space-3);
    overflow-x: auto;
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  @media (prefers-reduced-motion: reduce) {
    .chev {
      transition: none;
    }
  }
</style>
