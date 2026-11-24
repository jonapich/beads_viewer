<script lang="ts">
  import { commitHref, humanKey, isScalar } from "../../lib/issues/detail";
  import InlineText from "./InlineText.svelte";
  import JsonView from "./JsonView.svelte";

  interface Props {
    /** A parsed JSON value: a structured comment body. */
    value: unknown;
    /** The key it sits under; "commits" draws its strings as links to GitHub. */
    keyName?: string;
    depth?: number;
  }

  /** Past this the value is shown as JSON text: bounded recursion for hostile or odd data. */
  const MAX_DEPTH = 8;

  let { value, keyName = "", depth = 0 }: Props = $props();

  const list = $derived(Array.isArray(value) ? (value as unknown[]) : null);
  const entries = $derived(value !== null && typeof value === "object" && !Array.isArray(value) ? Object.entries(value) : null);

  /** A scalar as text for InlineText; null reads as an empty value. */
  const text = (v: unknown): string => (v === null || v === undefined ? "" : String(v));
</script>

{#if depth > MAX_DEPTH}
  <pre class="deep">{JSON.stringify(value) ?? ""}</pre>
{:else if list}
  {#if list.length === 0}
    <span class="empty">None</span>
  {:else if keyName === "commits"}
    <div>
      {#each list as x, i (i)}
        {@const href = typeof x === "string" ? commitHref(x) : null}
        {#if href}
          <a class="commit" {href} target="_blank" rel="noopener noreferrer">{x}</a>
        {:else}
          <span class="commit">{text(x)}</span>
        {/if}
      {/each}
    </div>
  {:else if list.every(isScalar)}
    <ul>{#each list as x, i (i)}<li><InlineText src={text(x)} /></li>{/each}</ul>
  {:else}
    {#each list as x, i (i)}
      <div class="jcard"><JsonView value={x} depth={depth + 1} /></div>
    {/each}
  {/if}
{:else if entries}
  <dl class="jv">
    {#each entries as [k, v] (k)}
      <dt>{humanKey(k)}</dt>
      <dd><JsonView value={v} keyName={k} depth={depth + 1} /></dd>
    {/each}
  </dl>
{:else}
  <span><InlineText src={text(value)} /></span>
{/if}

<style>
  dl {
    display: block;
    margin: 0;
  }
  dt {
    font-size: var(--text-sm);
    color: var(--text-muted);
    margin-top: var(--space-2);
  }
  dt:first-child {
    margin-top: 0;
  }
  dd {
    margin: 0;
  }
  ul {
    padding-left: 20px;
    margin: var(--space-1) 0;
  }
  .empty {
    color: var(--text-faint);
    font-style: italic;
  }
  .jcard {
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: var(--space-2) 10px;
    margin: 6px 0;
  }
  .commit {
    display: inline-block;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 1px 6px;
    margin: 0 6px 4px 0;
  }
  .deep {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
