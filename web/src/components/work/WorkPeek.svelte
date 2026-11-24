<script lang="ts">
  import { untrack } from "svelte";
  import type { PeekHost } from "../../lib/work/peek-host.svelte";
  import PeekDrawer from "../issues/PeekDrawer.svelte";
  import RowActionsPopover from "../issues/RowActionsPopover.svelte";

  interface Props {
    host: PeekHost;
    /** The tracker takes writes (meta.capabilities.writes). */
    writes: boolean;
    /** Relative ages are computed from this. */
    now: number;
    /** Changes when the view's data is refreshed: the drawer's detail is fetched again. */
    reloadKey: unknown;
  }

  let { host, writes, now, reloadKey }: Props = $props();

  // The popover goes with its issue (it left the view) and with write access.
  $effect(() => {
    void host.pop;
    void host.popItem;
    void writes;
    untrack(() => host.dropStalePop());
  });

  // A live update can move rows while the popover is open: measure its anchor again.
  $effect(() => {
    void reloadKey;
    untrack(() => void host.remeasure());
  });

  const noop = (): void => {};
</script>

{#if host.peek}
  <PeekDrawer
    id={host.peek}
    item={undefined}
    section={null}
    {now}
    {reloadKey}
    onclose={() => host.close()}
    onopen={(id) => host.open(id)}
    onmissing={(id) => host.peekMissing(id)}
    actions={host.actions}
    {writes}
    onpickpriority={(id, el, priority) => host.togglePop(id, el, "drawer", priority)}
    prioOpen={host.pop?.from === "drawer" && host.pop.id === host.peek}
  />
{/if}
{#if host.pop && host.popItem && writes}
  {#key `${host.pop.id}:${host.pop.from}`}
    <RowActionsPopover
      kind="prio"
      item={host.popItem}
      anchor={host.pop.anchor}
      {writes}
      onclose={(restore) => void host.closePop(restore)}
      onpriority={(p) => host.pickPriority(host.popItem?.id ?? "", p)}
      oncandidate={noop}
      onapprove={noop}
      ondispute={noop}
      onanswer={noop}
    />
  {/key}
{/if}
