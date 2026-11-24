<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { defaultCommentOpen, kindCounts } from "../../lib/issues/detail";
  import CommentItem from "./CommentItem.svelte";
  import KindBadge from "./KindBadge.svelte";
  import Section from "./Section.svelte";

  interface Props {
    detail: IssueDetail;
    now: number;
    open?: boolean;
  }

  let { detail, now, open = $bindable(true) }: Props = $props();

  // View state, kept here so a refetch (every live update) re-renders in place and loses nothing.
  // Per-comment overrides are keyed by comment id (unique across issues); the filter by issue id.
  let picked = $state({ id: "", kind: "all" });
  let expanded = $state<Record<string, boolean>>({});
  let raw = $state<Record<string, boolean>>({});

  const kinds = $derived(kindCounts(detail.comments));
  /** The pick for this issue, if its kind is still among the comments. */
  const filter = $derived(picked.id === detail.id && kinds.some((k) => k.kind === picked.kind) ? picked.kind : "all");
  const shown = $derived(filter === "all" ? detail.comments : detail.comments.filter((c) => c.kind === filter));

  const isOpen = (id: string, kind: string | undefined): boolean => expanded[id] ?? defaultCommentOpen(kind);

  function setAll(on: boolean): void {
    for (const c of detail.comments) if (c.kind) expanded[c.id] = on;
  }
</script>

{#snippet actions()}
  <!-- preventDefault: a click inside <summary> would also toggle the section. -->
  <button type="button" class="link" onclick={(e) => { e.preventDefault(); setAll(true); }}>Expand all</button>
  <span class="faint">·</span>
  <button type="button" class="link" onclick={(e) => { e.preventDefault(); setAll(false); }}>Collapse all</button>
{/snippet}

<Section name="activity" title="Activity" count={detail.comments.length} bind:open {actions}>
  <div class="chips" role="group" aria-label="Filter by kind">
    <button type="button" class="fchip" aria-pressed={filter === "all"} onclick={() => (picked = { id: detail.id, kind: "all" })}>
      All <span class="n">{detail.comments.length}</span>
    </button>
    {#each kinds as k (k.kind)}
      <button type="button" class="fchip" aria-pressed={filter === k.kind} onclick={() => (picked = { id: detail.id, kind: k.kind })}>
        <KindBadge kind={k.kind} /><span class="n">{k.n}</span>
      </button>
    {/each}
  </div>
  <div class="list">
    {#each shown as c (c.id)}
      <CommentItem
        comment={c}
        open={isOpen(c.id, c.kind)}
        raw={raw[c.id] ?? false}
        {now}
        ontoggle={() => (expanded[c.id] = !isOpen(c.id, c.kind))}
        onraw={() => (raw[c.id] = !raw[c.id])}
      />
    {/each}
  </div>
</Section>

<style>
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
  .faint {
    color: var(--text-faint);
  }
  .chips {
    display: flex;
    gap: var(--space-2);
    overflow-x: auto;
    padding: 2px 0 var(--space-2);
    margin-bottom: var(--space-1);
  }
  .fchip {
    flex: 0 0 auto;
    display: inline-flex;
    gap: 6px;
    align-items: center;
    height: 24px;
    padding: 0 10px;
    border-radius: 12px;
    font-size: var(--text-sm);
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .fchip[aria-pressed="true"] {
    background: var(--bg-selected);
    border-color: var(--accent);
    color: var(--accent);
  }
  .n {
    font-variant-numeric: tabular-nums;
    color: var(--text-faint);
  }
</style>
