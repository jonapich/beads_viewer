<script lang="ts">
  import type { RailRow } from "../../../lib/work/triage";
  import Tag from "../Tag.svelte";

  interface Props {
    title: string;
    /** A line under the title: what the list is. */
    sub: string;
    rows: readonly RailRow[];
    /** The first fetch is still running: three skeleton rows, no count. */
    loading: boolean;
    selected: string | null;
    onpeek: (id: string) => void;
  }

  let { title, sub, rows, loading, selected, onpeek }: Props = $props();
</script>

<section class="card" aria-label={title}>
  <h3>{title} {#if !loading}<span class="cnt">{rows.length}</span>{/if}</h3>
  <div class="sub">{sub}</div>
  {#if loading}
    {#each [0, 1, 2] as n (n)}
      <div class="sk" aria-hidden="true"><i class="w{n}"></i></div>
    {/each}
  {:else if rows.length === 0}
    <div class="none">None found</div>
  {:else}
    {#each rows as r (r.id)}
      <div class="rrow" class:sel={selected === r.id} data-row data-id={r.id}>
        <div class="r1">
          <button type="button" class="t-btn rt" title={r.title} onclick={() => onpeek(r.id)}>{r.title}</button>
          <span class="rs">{r.figure}</span>
        </div>
        <div class="r2">
          <span class="ri">{r.id}</span>
          {#if r.tag}<Tag tone={r.tag.tone}>{r.tag.text}</Tag>{/if}
          {#if r.note}<span>{r.note}</span>{/if}
        </div>
      </div>
    {/each}
  {/if}
</section>

<style>
  .card {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    min-width: 0;
  }
  h3 {
    margin: 0;
    font-size: var(--text-base);
    font-weight: 600;
  }
  .cnt {
    font-size: var(--text-xs);
    color: var(--text-faint);
    font-weight: 400;
  }
  .sub {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .none {
    font-size: var(--text-sm);
    color: var(--text-muted);
    padding: 6px 0;
  }
  .rrow {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 0;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
    cursor: pointer;
    min-width: 0;
  }
  .rrow:last-child {
    border-bottom: 0;
    padding-bottom: 0;
  }
  .rrow:hover .rt {
    color: var(--accent);
  }
  .rrow.sel .rt {
    color: var(--accent);
  }
  .r1 {
    display: flex;
    gap: 8px;
    align-items: baseline;
    min-width: 0;
  }
  .rt {
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .rs {
    flex: none;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-muted);
  }
  .r2 {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: var(--text-xs);
    color: var(--text-muted);
    flex-wrap: wrap;
  }
  .ri {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .sk {
    height: 28px;
    display: flex;
    align-items: center;
  }
  .sk i {
    display: block;
    height: 10px;
    border-radius: 5px;
    background: linear-gradient(90deg, var(--bg-subtle), var(--bg-hover), var(--bg-subtle));
    background-size: 200% 100%;
    animation: shim 1.4s linear infinite;
  }
  .w0 {
    width: 70%;
  }
  .w1 {
    width: 50%;
  }
  .w2 {
    width: 60%;
  }
  @keyframes shim {
    from {
      background-position: 200% 0;
    }
    to {
      background-position: -200% 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sk i {
      animation: none;
    }
  }
</style>
