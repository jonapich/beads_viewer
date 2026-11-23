import type { ListItem, ListTree, TreeEdge } from "../api/types";
import { isBlockedAny, needsAnswer } from "./model";

export type EdgeKind = "child" | "block";
export type Attention = "danger" | "warning";

export interface IssueRow {
  kind: "issue";
  /** Unique per placement: the parent row's key + ">" + id. */
  key: string;
  id: string;
  depth: number;
  /** The edge kind of each step from the root, so edges.length === depth. */
  edges: EdgeKind[];
  /** The id of the parent row (the blocker when the last edge is "block"). */
  viaId: string | null;
  hasKids: boolean;
  expanded: boolean;
  /** Shown only because a descendant matches. */
  context: boolean;
  /** Rows this subtree renders when fully expanded, duplicates included. */
  descCount: number;
  /** A descendant is blocked ("danger") or waits on a decision ("warning"). */
  attention: Attention | null;
}

/** An id that is already on the current path: shown as a leaf instead of descending. */
export interface CycleRow {
  kind: "cycle";
  key: string;
  id: string;
  depth: number;
  edges: EdgeKind[];
}

export type TreeRow = IssueRow | CycleRow;

export interface OrderedEdge {
  id: string;
  kind: EdgeKind;
}

export interface SubtreeInfo {
  hasMatch: boolean;
  descCount: number;
  attention: Attention | null;
}

export const DEFAULT_MAX_ROWS = 50000;

/** children[id] without the prototype chain (an id such as "constructor" is not a node). */
function kidsOf(children: Record<string, TreeEdge[]>, id: string): readonly TreeEdge[] {
  return Object.hasOwn(children, id) ? (children[id] ?? []) : [];
}

const worse = (a: Attention | null, b: Attention | null): Attention | null =>
  a === "danger" || b === "danger" ? "danger" : (a ?? b);

function ownAttention(item: ListItem): Attention | null {
  if (isBlockedAny(item)) return "danger";
  return needsAnswer(item) ? "warning" : null;
}

/**
 * Per-node facts for the tree, computed lazily and memoised: whether the node
 * or any descendant matches, how many rows its subtree would render, and the
 * attention flag of its descendants. Reachability is exact (a reverse walk from
 * the matches), so it is cycle-safe; the DFS for the counts treats an id that
 * is in progress as contributing nothing, so a cycle never counts twice. Cost
 * O(V+E) per (children, matches).
 *
 * `hasMatch` is exact. `descCount` and `attention` are approximate inside
 * cycles: they are memoised per id on first visit, so a node first visited while
 * a cycle-mate is in progress caches a count that leaves out that mate's subtree,
 * and a later placement can show a smaller "(n)". Visibility is unaffected.
 * Cycles are rare (the server promotes them to roots), so this is accepted; do
 * not "fix" the cycle expectations in tree.test.ts without redoing the memoisation.
 */
export function subtreeInfo(
  children: Record<string, TreeEdge[]>,
  byId: ReadonlyMap<string, ListItem>,
  matches: ReadonlySet<string>,
): (id: string) => SubtreeInfo {
  const parents = new Map<string, string[]>();
  for (const [p, edges] of Object.entries(children)) {
    for (const e of edges) {
      const list = parents.get(e.id);
      if (list) list.push(p);
      else parents.set(e.id, [p]);
    }
  }
  const has = new Set<string>();
  const queue: string[] = [];
  for (const id of matches) {
    has.add(id);
    queue.push(id);
  }
  for (let i = 0; i < queue.length; i++) {
    for (const p of parents.get(queue[i] as string) ?? []) {
      if (!has.has(p)) {
        has.add(p);
        queue.push(p);
      }
    }
  }

  const memo = new Map<string, SubtreeInfo>();
  const active = new Set<string>();
  const visit = (id: string): SubtreeInfo => {
    const done = memo.get(id);
    if (done) return done;
    active.add(id);
    let descCount = 0;
    let attention: Attention | null = null;
    const seen = new Set<string>();
    for (const e of kidsOf(children, id)) {
      const kid = byId.get(e.id);
      if (!kid || seen.has(e.id) || active.has(e.id) || !has.has(e.id)) continue;
      seen.add(e.id);
      const sub = visit(e.id);
      descCount += 1 + sub.descCount;
      attention = worse(attention, worse(sub.attention, ownAttention(kid)));
    }
    active.delete(id);
    const info: SubtreeInfo = { hasMatch: has.has(id), descCount, attention };
    memo.set(id, info);
    return info;
  };
  return visit;
}

export interface TreeOrder {
  roots(): string[];
  /** Child edges first, then block edges, each sorted by the comparator; one entry per id. */
  kids(id: string): OrderedEdge[];
}

/**
 * Sorted roots and child lists for one (tree, comparator). Lists are built on
 * first use and cached, so toggling a node does not re-sort anything.
 */
export function makeTreeOrder(
  tree: ListTree,
  byId: ReadonlyMap<string, ListItem>,
  compare: (a: ListItem, b: ListItem) => number,
): TreeOrder {
  let roots: string[] | null = null;
  const cache = new Map<string, OrderedEdge[]>();
  const pick = (edges: readonly TreeEdge[], kind: EdgeKind): OrderedEdge[] =>
    edges
      .filter((e) => (e.kind === "block" ? "block" : "child") === kind && byId.has(e.id))
      .map((e) => byId.get(e.id) as ListItem)
      .sort(compare)
      .map((i) => ({ id: i.id, kind }));
  return {
    roots() {
      roots ??= tree.roots
        .filter((id) => byId.has(id))
        .map((id) => byId.get(id) as ListItem)
        .sort(compare)
        .map((i) => i.id);
      return roots;
    },
    kids(id) {
      let list = cache.get(id);
      if (!list) {
        const edges = kidsOf(tree.children, id);
        const seen = new Set<string>();
        list = [...pick(edges, "child"), ...pick(edges, "block")].filter((e) => !seen.has(e.id) && !!seen.add(e.id));
        cache.set(id, list);
      }
      return list;
    },
  };
}

export interface FlattenArgs {
  roots: string[];
  children: Record<string, TreeEdge[]>;
  byId: ReadonlyMap<string, ListItem>;
  matches: ReadonlySet<string>;
  isExpanded: (id: string) => boolean;
  compare: (a: ListItem, b: ListItem) => number;
  /** Stop after this many rows and report `truncated`. */
  maxRows?: number;
  /** Reuse these across calls that share the data and sort (a toggle changes neither). */
  order?: TreeOrder;
  info?: (id: string) => SubtreeInfo;
}

/**
 * Walks the server's tree placement into display rows. The server already puts
 * every issue (a blocked one under each of its blockers, else under its
 * parent); the client only sorts and walks. An issue shows when it or a
 * descendant matches; a non-matching one is a context row. An id that is
 * already on the path becomes a cycle row, so any graph terminates.
 */
export function flattenTree(a: FlattenArgs): { rows: TreeRow[]; truncated: boolean } {
  const maxRows = a.maxRows ?? DEFAULT_MAX_ROWS;
  const order = a.order ?? makeTreeOrder({ roots: a.roots, children: a.children }, a.byId, a.compare);
  const info = a.info ?? subtreeInfo(a.children, a.byId, a.matches);
  const rows: TreeRow[] = [];
  const onPath = new Set<string>();
  let truncated = false;

  const push = (row: TreeRow): boolean => {
    if (rows.length >= maxRows) {
      truncated = true;
      return false;
    }
    rows.push(row);
    return true;
  };

  const walk = (id: string, depth: number, edges: EdgeKind[], key: string, viaId: string | null): void => {
    if (truncated || !a.byId.has(id)) return;
    const inf = info(id);
    if (!inf.hasMatch) return;
    onPath.add(id);
    const kids = order.kids(id).filter((k) => onPath.has(k.id) || info(k.id).hasMatch);
    const hasKids = kids.length > 0;
    const expanded = hasKids && a.isExpanded(id);
    const shown = push({
      kind: "issue",
      key,
      id,
      depth,
      edges,
      viaId,
      hasKids,
      expanded,
      context: !a.matches.has(id),
      descCount: inf.descCount,
      attention: inf.attention,
    });
    if (shown && expanded) {
      for (const k of kids) {
        const kEdges = [...edges, k.kind];
        const kKey = `${key}>${k.id}`;
        if (onPath.has(k.id)) {
          if (!push({ kind: "cycle", key: `${kKey}#cycle`, id: k.id, depth: depth + 1, edges: kEdges })) break;
        } else {
          walk(k.id, depth + 1, kEdges, kKey, id);
        }
        if (truncated) break;
      }
    }
    onPath.delete(id);
  };

  for (const id of order.roots()) {
    walk(id, 0, [], id, null);
    if (truncated) break;
  }
  return { rows, truncated };
}

/** The roots that have children: what is open when nothing has been stored. */
export function defaultExpanded(tree: ListTree): Set<string> {
  return new Set(tree.roots.filter((id) => kidsOf(tree.children, id).length > 0));
}

/** Every node that has children, for Expand all. */
export function idsWithKids(tree: ListTree): Set<string> {
  return new Set(Object.keys(tree.children).filter((id) => (tree.children[id]?.length ?? 0) > 0));
}

/**
 * The ancestors (outermost first) of the first row where `target` appears in
 * display order, or null when it is not visible in the tree. Expanding exactly
 * these makes that row render. Follows the same order, visibility and cycle
 * guard as flattenTree; subtrees already searched without success are skipped,
 * so it costs O(V+E).
 */
export function findPath(order: TreeOrder, info: (id: string) => SubtreeInfo, target: string): string[] | null {
  const onPath = new Set<string>();
  const dead = new Set<string>();
  const path: string[] = [];
  const dfs = (id: string): boolean => {
    if (id === target) return true;
    if (dead.has(id) || onPath.has(id)) return false;
    onPath.add(id);
    path.push(id);
    for (const k of order.kids(id)) {
      if (!onPath.has(k.id) && info(k.id).hasMatch && dfs(k.id)) return true;
    }
    onPath.delete(id);
    path.pop();
    dead.add(id);
    return false;
  };
  for (const r of order.roots()) {
    if (info(r).hasMatch && dfs(r)) return [...path];
  }
  return null;
}
