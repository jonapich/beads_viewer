<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { blockedByMeta, blocksMeta, relatedTag, relationshipCount } from "../../lib/issues/detail";
  import { childProgress } from "../../lib/issues/peek";
  import { percent } from "../../lib/issues/present";
  import RefRow from "../issues/RefRow.svelte";
  import Section from "./Section.svelte";

  interface Props {
    detail: IssueDetail;
    open?: boolean;
  }

  let { detail, open = $bindable(true) }: Props = $props();

  const progress = $derived(childProgress(detail.children));
  // The server computed these (snapshot.BlockerCounts / ActiveDependentsCount): not counted here.
  const blockedBy = $derived(blockedByMeta(detail.blocker_counts));
  const blocks = $derived(blocksMeta(detail.active_dependents));
</script>

<Section name="relationships" title="Relationships" count={relationshipCount(detail)} bind:open>
  {#if detail.parent}
    <div class="group">
      <h3>Parent <span class="cnt">1</span></h3>
      <RefRow ref={detail.parent} showPriority />
    </div>
  {/if}

  {#if detail.children.length > 0}
    <div class="group">
      <h3>Children <span class="cnt">{detail.children.length}</span></h3>
      <div class="dt">{progress.done} of {progress.total} closed</div>
      <div class="bar" role="progressbar" aria-label="Children closed" aria-valuemin="0" aria-valuemax={progress.total} aria-valuenow={progress.done}>
        <i class="pct-{percent(progress.done, progress.total)}"></i>
      </div>
      {#each detail.children as r (r.id)}<RefRow ref={r} muted={r.status === "closed"} showPriority />{/each}
    </div>
  {/if}

  {#if detail.blocked_by.length > 0}
    <div class="group">
      <h3>Blocked by <span class="cnt">{detail.blocked_by.length}</span>{#if blockedBy}<span class="meta">{blockedBy}</span>{/if}</h3>
      {#each detail.blocked_by as r (r.id)}<RefRow ref={r} muted={!r.active} showPriority />{/each}
    </div>
  {/if}

  {#if detail.blocks.length > 0}
    <div class="group">
      <h3>Blocks <span class="cnt">{detail.blocks.length}</span>{#if blocks}<span class="meta">{blocks}</span>{/if}</h3>
      {#each detail.blocks as r (r.id)}<RefRow ref={r} muted={!r.active} showPriority />{/each}
    </div>
  {/if}

  {#if detail.related.length > 0}
    <div class="group">
      <h3>Related / discovered from <span class="cnt">{detail.related.length}</span></h3>
      {#each detail.related as r (`${r.direction}:${r.type}:${r.id}`)}<RefRow ref={r} tag={relatedTag(r)} showPriority />{/each}
    </div>
  {/if}
</Section>

<style>
  .group {
    margin-bottom: var(--space-3);
  }
  h3 {
    display: flex;
    gap: 6px;
    align-items: center;
    margin: 0 0 2px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-muted);
  }
  .cnt {
    font-size: var(--text-xs);
    font-weight: 500;
    background: var(--bg-hover);
    border-radius: 10px;
    padding: 0 6px;
    line-height: 18px;
    color: var(--text-muted);
  }
  .meta {
    font-weight: 400;
    color: var(--text-faint);
  }
  .dt {
    font-size: var(--text-sm);
    color: var(--text-muted);
    margin-top: 2px;
  }
  .bar {
    height: 4px;
    border-radius: 2px;
    background: var(--bg-hover);
    overflow: hidden;
    margin: var(--space-1) 0 var(--space-2);
    max-width: 360px;
  }
  .bar i {
    display: block;
    height: 100%;
    background: var(--success);
  }
</style>
