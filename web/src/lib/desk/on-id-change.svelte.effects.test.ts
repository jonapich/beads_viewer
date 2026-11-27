import { flushSync } from "svelte";
import { describe, expect, it } from "vitest";
import { IssueActions } from "../issues/actions.svelte";
import { onIdChange } from "./on-id-change.svelte";

type Sel = { id: string | null; vanished: boolean };

/** What Desk.svelte does: `sel` is a $derived object rebuilt by every feed refresh. */
function setup() {
  let groupsVersion = $state(0);
  let wanted = $state<string | null>("a-1");
  const sel = $derived<Sel>(((_v: number) => ({ id: wanted, vanished: false }))(groupsVersion));
  const actions = new IssueActions({
    client: {} as never,
    toasts: { push: () => undefined },
    writable: () => true,
    refresh: async () => {},
    sticky: () => {},
  });
  const calls: (string | null)[] = [];
  const stop = $effect.root(() => {
    onIdChange(
      () => sel.id,
      (id) => {
        calls.push(id);
        actions.closeDispute();
      },
    );
  });
  flushSync();
  return {
    actions,
    calls,
    stop,
    refresh: () => {
      groupsVersion++;
      flushSync();
    },
    select: (id: string | null) => {
      wanted = id;
      flushSync();
    },
  };
}

describe("onIdChange (the Desk dispute form)", () => {
  it("keeps the dispute form open across a feed refresh with the same selected row", () => {
    const t = setup();
    t.actions.openDispute("a-1");
    expect(t.actions.disputeOpen).toBe("a-1");
    t.refresh();
    t.refresh();
    expect(t.actions.disputeOpen).toBe("a-1");
    expect(t.calls).toEqual(["a-1"]);
    t.stop();
  });

  it("still closes the dispute form when another row is selected", () => {
    const t = setup();
    t.actions.openDispute("a-1");
    t.select("a-2");
    expect(t.actions.disputeOpen).toBeNull();
    expect(t.calls).toEqual(["a-1", "a-2"]);
    t.stop();
  });
});
