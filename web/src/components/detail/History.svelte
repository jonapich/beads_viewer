<script lang="ts">
  import type { IssueDetail } from "../../lib/api/types";
  import { absTime, historyEvents, relTime } from "../../lib/issues/detail";
  import Section from "./Section.svelte";

  interface Props {
    detail: IssueDetail;
    now: number;
    open?: boolean;
  }

  let { detail, now, open = $bindable(false) }: Props = $props();

  const events = $derived(historyEvents(detail));
</script>

<Section name="history" title="History" count={events.length} bind:open>
  <ol class="tl">
    {#each events as e (`${e.kind}:${e.label}:${e.at}`)}
      <li>
        <i data-ev={e.kind}></i>{e.label}
        <span class="w" title={absTime(e.at)}>{absTime(e.at)} · {relTime(e.at, now)}</span>
      </li>
    {/each}
  </ol>
</Section>

<style>
  .tl {
    list-style: none;
    padding: 0;
    margin: 0;
    position: relative;
  }
  li {
    position: relative;
    padding: 0 0 var(--space-3) 20px;
    font-size: 13px;
  }
  li::before {
    content: "";
    position: absolute;
    left: 3px;
    top: 8px;
    bottom: -4px;
    width: 1px;
    background: var(--border);
  }
  li:last-child::before {
    display: none;
  }
  i {
    position: absolute;
    left: 0;
    top: 5px;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-faint);
  }
  i[data-ev="started"] {
    background: var(--status-progress);
  }
  i[data-ev="stage"] {
    background: var(--status-review);
  }
  i[data-ev="signoff"] {
    background: var(--warning);
  }
  i[data-ev="closed"] {
    background: var(--status-closed);
  }
  .w {
    display: block;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
