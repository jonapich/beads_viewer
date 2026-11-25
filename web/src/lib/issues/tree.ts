import type { ListItem, ListTree, TreeEdge } from "../api/types";
import { isBlockedAny, needsAnswer } from "./model";

export type EdgeKind = "child" | "block";
export type Attention = "danger" | "warning";

export interface IssueRow {
  kind: "issue";
  /**
   * Unique per placement and stable across rebuilds of the same tree: the id, plus
   * (below a root) a hash of the path from the root. Its length does not grow with
   * the depth, so hashing and comparing keys is O(1) in the depth.
   */
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

/** The edge kinds from the root down to a row, as a linked list shared by all rows below it. */
interface EdgePath {
  kind: EdgeKind;
  up: EdgePath | null;
}

/**
 * A row's `edges` array, built on first read. Copying the whole path into every
 * row is O(depth) each, O(depth^2) for a deep chain (hundreds of millions of
 * entries at 20k deep); the list view only reads the rows it renders.
 */
function lazyEdges(path: EdgePath | null, depth: number): () => EdgeKind[] {
  let built: EdgeKind[] | undefined;
  return () => {
    if (!built) {
      built = new Array<EdgeKind>(depth);
      for (let n = path, i = depth - 1; n; n = n.up, i--) built[i] = n.kind;
    }
    return built;
  };
}

/** A running 2 x 32-bit hash of the ids on the path from a root. */
interface PathHash {
  a: number;
  b: number;
}

const ROOT_HASH: PathHash = { a: 0x9e3779b9, b: 0x85ebca6b };

/**
 * The hash of `p` followed by one more id: O(id length), whatever the depth. It
 * is order-sensitive and deterministic, so a placement keeps its key from one
 * rebuild to the next. Two paths colliding needs a 64-bit coincidence among the
 * (at most maxRows) rows of one list.
 */
function extendHash(p: PathHash, id: string): PathHash {
  let a = p.a ^ 0x7f4a7c15;
  let b = p.b ^ 0x165667b1;
  for (let i = 0; i < id.length; i++) {
    const c = id.charCodeAt(i);
    a = Math.imul(a ^ c, 2654435761);
    b = Math.imul(b ^ c, 1597334677);
  }
  a = Math.imul(a ^ (a >>> 16), 2246822507) ^ Math.imul(b ^ (b >>> 13), 3266489909);
  b = Math.imul(b ^ (b >>> 16), 2246822507) ^ Math.imul(a ^ (a >>> 13), 3266489909);
  return { a: a >>> 0, b: b >>> 0 };
}

/** The key of a placement: the id, and below a root the hash of its path (which includes the root). */
const placementKey = (id: string, h: PathHash, depth: number): string =>
  depth === 0 ? id : `${id}~${h.a.toString(36)}.${h.b.toString(36)}`;

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

  // The DFS runs on an explicit stack: a chain thousands deep must not overflow
  // the JS call stack. A frame is one node being counted.
  interface Frame {
    id: string;
    /** The item of the edge that led here: its own flags feed the parent's attention. */
    via: ListItem | null;
    edges: readonly TreeEdge[];
    next: number;
    seen: Set<string>;
    descCount: number;
    attention: Attention | null;
  }
  const memo = new Map<string, SubtreeInfo>();
  const active = new Set<string>();
  const open = (id: string, via: ListItem | null): Frame => {
    active.add(id);
    return { id, via, edges: kidsOf(children, id), next: 0, seen: new Set(), descCount: 0, attention: null };
  };
  const visit = (id: string): SubtreeInfo => {
    const done = memo.get(id);
    if (done) return done;
    const stack: Frame[] = [open(id, null)];
    while (stack.length > 0) {
      const f = stack[stack.length - 1] as Frame;
      if (f.next < f.edges.length) {
        const e = f.edges[f.next++] as TreeEdge;
        const kid = byId.get(e.id);
        if (!kid || f.seen.has(e.id) || active.has(e.id) || !has.has(e.id)) continue;
        f.seen.add(e.id);
        const sub = memo.get(e.id);
        if (sub) {
          f.descCount += 1 + sub.descCount;
          f.attention = worse(f.attention, worse(sub.attention, ownAttention(kid)));
        } else {
          stack.push(open(e.id, kid));
        }
        continue;
      }
      active.delete(f.id);
      memo.set(f.id, { hasMatch: has.has(f.id), descCount: f.descCount, attention: f.attention });
      stack.pop();
      const parent = stack[stack.length - 1];
      if (parent && f.via) {
        const sub = memo.get(f.id) as SubtreeInfo;
        parent.descCount += 1 + sub.descCount;
        parent.attention = worse(parent.attention, worse(sub.attention, ownAttention(f.via)));
      }
    }
    return memo.get(id) as SubtreeInfo;
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

  // The walk runs on an explicit stack: a chain thousands deep must not overflow
  // the JS call stack. A frame is an expanded row whose kids are still being visited.
  interface Frame {
    id: string;
    depth: number;
    path: EdgePath | null;
    hash: PathHash;
    kids: OrderedEdge[];
    next: number;
  }
  const stack: Frame[] = [];

  /** Emits the row for `id` and returns its frame when its kids are to be walked. */
  const enter = (id: string, depth: number, path: EdgePath | null, hash: PathHash, viaId: string | null): Frame | null => {
    if (truncated || !a.byId.has(id)) return null;
    const inf = info(id);
    if (!inf.hasMatch) return null;
    onPath.add(id);
    const kids = order.kids(id).filter((k) => onPath.has(k.id) || info(k.id).hasMatch);
    const hasKids = kids.length > 0;
    const expanded = hasKids && a.isExpanded(id);
    const edges = lazyEdges(path, depth);
    const shown = push({
      kind: "issue",
      key: placementKey(id, hash, depth),
      id,
      depth,
      get edges() {
        return edges();
      },
      viaId,
      hasKids,
      expanded,
      context: !a.matches.has(id),
      descCount: inf.descCount,
      attention: inf.attention,
    });
    if (shown && expanded) return { id, depth, path, hash, kids, next: 0 };
    onPath.delete(id);
    return null;
  };

  for (const id of order.roots()) {
    const first = enter(id, 0, null, extendHash(ROOT_HASH, id), null);
    if (first) stack.push(first);
    while (stack.length > 0) {
      const f = stack[stack.length - 1] as Frame;
      // Once truncated, the remaining frames only unwind (clearing onPath).
      if (truncated || f.next >= f.kids.length) {
        onPath.delete(f.id);
        stack.pop();
        continue;
      }
      const k = f.kids[f.next++] as OrderedEdge;
      const kPath: EdgePath = { kind: k.kind, up: f.path };
      const kHash = extendHash(f.hash, k.id);
      if (onPath.has(k.id)) {
        const edges = lazyEdges(kPath, f.depth + 1);
        push({
          kind: "cycle",
          key: `${placementKey(k.id, kHash, f.depth + 1)}#cycle`,
          id: k.id,
          depth: f.depth + 1,
          get edges() {
            return edges();
          },
        });
      } else {
        const child = enter(k.id, f.depth + 1, kPath, kHash, f.id);
        if (child) stack.push(child);
      }
    }
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
  // Iterative (explicit stack) so a very deep chain cannot overflow the call stack.
  interface Frame {
    id: string;
    kids: OrderedEdge[];
    next: number;
  }
  const open = (id: string): Frame => {
    onPath.add(id);
    path.push(id);
    return { id, kids: order.kids(id), next: 0 };
  };
  const dfs = (start: string): boolean => {
    if (start === target) return true;
    if (dead.has(start) || onPath.has(start)) return false;
    const stack: Frame[] = [open(start)];
    while (stack.length > 0) {
      const f = stack[stack.length - 1] as Frame;
      if (f.next >= f.kids.length) {
        onPath.delete(f.id);
        path.pop();
        dead.add(f.id);
        stack.pop();
        continue;
      }
      const k = f.kids[f.next++] as OrderedEdge;
      if (onPath.has(k.id) || !info(k.id).hasMatch) continue;
      if (k.id === target) return true;
      if (dead.has(k.id)) continue;
      stack.push(open(k.id));
    }
    return false;
  };
  for (const r of order.roots()) {
    if (info(r).hasMatch && dfs(r)) return [...path];
  }
  return null;
}
