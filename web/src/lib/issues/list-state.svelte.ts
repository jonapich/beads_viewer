import type { ListItem, ListTree } from "../api/types";
import {
  defaultFilters,
  isTriggered,
  nextModePick,
  resolveMode,
  type ListFilters,
  type Mode,
} from "./filter";
import type { Prefs } from "./persist";
import type { PeekSection } from "./peek";
import type { SortKey } from "./sort";
import { defaultExpanded } from "./tree";
import type { UrlState } from "./url";

export interface HydrateArgs {
  tree: ListTree;
  byId: ReadonlyMap<string, ListItem>;
  /** The mount URL, parsed. */
  url: UrlState;
  /** What localStorage held for this tracker (expansion and sort), if anything. */
  prefs: Prefs | null;
}

/**
 * The Issues view's state, one instance per mount. Plain values are
 * $state.raw and replaced immutably; the API data itself lives in IssuesFeed.
 * The Tree/Flat mode is derived: the explicit pick, else flat while a query or
 * filter is set, else tree.
 */
export class IssueListState {
  /** The debounced search text; the input keeps its own draft. */
  query = $state("");
  filters = $state.raw<ListFilters>(defaultFilters());
  sortPick = $state<SortKey | null>(null);
  /** Explicit Tree/Flat choice; null = automatic. */
  modePick = $state<Mode | null>(null);
  peek = $state<string | null>(null);
  /** The drawer block to scroll to when it opens from a row chip ("Sign off", "Answer"). Not in the URL. */
  peekSection = $state<PeekSection | null>(null);
  expanded = $state.raw<Set<string>>(new Set());
  /** Tree + filters (the user forced Tree): ids the user collapsed from the all-open view. */
  pruneCollapsed = $state.raw<Set<string>>(new Set());
  /** Set when "Show in tree" left flat results: where to return to (scroll offset and the mode pick). */
  backTo = $state.raw<{ scrollTop: number; modePick: Mode | null } | null>(null);
  flashId = $state<string | null>(null);
  /** Issues a write just changed: kept in "Active" until the filters or the query change (see stateMatches). */
  sticky = $state.raw<ReadonlySet<string>>(new Set());
  /** Search hits in rank order (id -> position); null = no query. */
  rank = $state.raw<Map<string, number> | null>(null);
  /** Set once the URL and remembered prefs have been applied. */
  hydrated = $state(false);

  triggered = $derived(isTriggered(this.query, this.filters));
  mode = $derived(resolveMode(this.modePick, this.triggered));
  /** Tree shown while filtered: every matching branch is open unless collapsed. */
  pruning = $derived(this.modePick === "tree" && this.triggered);

  /**
   * First apply: the URL (filters, query, mode, peek, sort), then remembered
   * prefs (sort and expansion only), then the defaults. Filters are URL-only,
   * so a bare /issues is the default tree.
   */
  hydrate(a: HydrateArgs): void {
    this.applyUrl(a.url, a.prefs?.sort ?? null);
    this.expanded = a.prefs ? new Set(a.prefs.expanded.filter((id) => a.byId.has(id))) : defaultExpanded(a.tree);
    this.pruneCollapsed = new Set();
    this.hydrated = true;
  }

  /**
   * Adopts the query string after an in-route change (the nav link to a bare
   * /issues, Back/Forward between /issues entries). Replaces filters, query,
   * sort, peek and the explicit mode; expansion is left alone. A bare URL gives
   * the defaults (and the remembered sort).
   */
  applyUrl(u: UrlState, rememberedSort: SortKey | null = null): void {
    this.filters = u.filters;
    this.query = u.query;
    this.sortPick = u.sortPick ?? rememberedSort;
    this.modePick = nextModePick(u.modePick, this.triggered);
    this.peek = u.peek;
    this.peekSection = null;
    this.pruneCollapsed = new Set();
    this.backTo = null;
    this.sticky = new Set();
  }
  isExpanded(id: string): boolean {
    return this.pruning ? !this.pruneCollapsed.has(id) : this.expanded.has(id);
  }

  /** A write changed this issue: keep its row in the list under "Active". */
  markSticky(id: string): void {
    this.sticky = new Set(this.sticky).add(id);
  }

  setQuery(q: string): void {
    this.backTo = null;
    this.sticky = new Set();
    this.query = q;
    this.modePick = nextModePick(this.modePick, this.triggered);
  }

  setFilters(f: ListFilters): void {
    this.backTo = null;
    this.sticky = new Set();
    this.filters = f;
    this.modePick = nextModePick(this.modePick, this.triggered);
  }

  clearFilters(): void {
    this.setFilters(defaultFilters());
  }

  setMode(m: Mode): void {
    this.backTo = null;
    this.modePick = nextModePick(m, this.triggered);
  }

  setSort(k: SortKey | null): void {
    this.sortPick = k;
  }

  /** Opens or closes one node. Expansion is per issue id, so a duplicate placement follows. */
  toggle(id: string): void {
    if (this.pruning) {
      const next = new Set(this.pruneCollapsed);
      if (!next.delete(id)) next.add(id);
      this.pruneCollapsed = next;
    } else {
      const next = new Set(this.expanded);
      if (!next.delete(id)) next.add(id);
      this.expanded = next;
    }
  }

  /** `ids`: every node that has children. */
  expandAll(ids: ReadonlySet<string>): void {
    this.expanded = new Set(ids);
    this.pruneCollapsed = new Set();
  }

  collapseAll(ids: ReadonlySet<string>): void {
    this.expanded = new Set();
    if (this.pruning) this.pruneCollapsed = new Set(ids);
  }

  /**
   * First step of "Show in tree": switch to the tree, remembering where the flat
   * results were so "Back to results" can return. The caller then waits for the
   * rows to settle, finds the path with findPath and calls expandPath.
   */
  beginReveal(scrollTop: number): void {
    this.backTo = this.mode === "flat" ? { scrollTop, modePick: this.modePick } : null;
    this.modePick = nextModePick("tree", this.triggered);
  }

  /** Opens exactly these ancestors, in both expansion models (plain and the pruned view). */
  expandPath(ids: readonly string[]): void {
    const expanded = new Set(this.expanded);
    for (const id of ids) expanded.add(id);
    this.expanded = expanded;
    if (this.pruneCollapsed.size > 0) {
      const pruned = new Set(this.pruneCollapsed);
      for (const id of ids) pruned.delete(id);
      this.pruneCollapsed = pruned;
    }
  }

  /** "Back to results": restores the mode pick; returns the scroll offset to restore, or null. */
  back(): number | null {
    const b = this.backTo;
    if (!b) return null;
    this.backTo = null;
    this.modePick = b.modePick;
    return b.scrollTop;
  }

  openPeek(id: string, section: PeekSection | null = null): void {
    this.peek = id;
    this.peekSection = section;
  }

  closePeek(): void {
    this.peek = null;
    this.peekSection = null;
  }
}
