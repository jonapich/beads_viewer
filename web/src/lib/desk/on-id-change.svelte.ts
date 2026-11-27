import { untrack } from "svelte";

/**
 * Runs `fn` when the id read by `getId` changes, never because something else the getter walks
 * through was rebuilt. The Desk selection (`sel`) is a $derived object that is a new object on
 * every feed refresh, so an effect that read `sel.id` directly re-ran on each poll or snapshot even
 * with the same row selected and, for the dispute form, closed it under the user. Reading the id
 * through a $derived of a primitive makes the effect depend on the value, which is compared by
 * equality. Call it while a component (or an effect root) is initialising.
 *
 * `fn` runs once at the start too, with the first id; it is untracked.
 */
export function onIdChange(getId: () => string | null, fn: (id: string | null) => void): void {
  const id = $derived(getId());
  $effect(() => {
    const current = id;
    untrack(() => fn(current));
  });
}
