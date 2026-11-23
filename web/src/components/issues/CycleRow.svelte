<script lang="ts">
  import type { ListItem } from "../../lib/api/types";
  import type { CycleRow as CycleRowModel } from "../../lib/issues/tree";

  let { row, item }: { row: CycleRowModel; item: ListItem | undefined } = $props();
</script>

<div class="row" data-id={row.id}>
  <span class="indent">
    {#each row.edges as e, k (k)}<span class="g" class:gb={e === "block"}></span>{/each}
  </span>
  <span class="caret-sp"></span>
  <span class="ico" title="Blocking cycle: this issue is already shown above">↻</span>
  <span class="ref">
    <span class="t">{item?.title ?? row.id}</span>
    <span class="note">cycle</span>
  </span>
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--row-h);
    padding: 0 8px;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  .indent {
    display: flex;
    flex: none;
    align-self: stretch;
  }
  .g {
    display: block;
    width: 16px;
    border-left: 1px solid var(--border);
  }
  .g.gb {
    border-left: 1px dashed color-mix(in srgb, var(--danger) 50%, transparent);
  }
  .caret-sp {
    width: 24px;
    flex: none;
  }
  .ico {
    width: 16px;
    flex: none;
    text-align: center;
    color: var(--text-faint);
  }
  .ref {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .t {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-style: italic;
    color: var(--text-muted);
  }
  .note {
    flex: none;
    font-size: var(--text-xs);
    color: var(--text-faint);
    white-space: nowrap;
  }
</style>
