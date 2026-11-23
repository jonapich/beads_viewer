import { describe, expect, it } from "vitest";
import fixture from "../api/__fixtures__/issues-tree.json";
import type { IssuesResponse, ListItem } from "../api/types";
import { defaultFilters, passesFilters, type ListFilters } from "./filter";
import { buildIndex, type IssueIndex } from "./model";
import { comparator } from "./sort";
import { defaultExpanded, findPath, flattenTree, idsWithKids, makeTreeOrder, subtreeInfo, type TreeRow } from "./tree";
import { mkItem, mkResponse } from "./test-helpers";

const fx: IssuesResponse = fixture;
const compare = comparator("default");

function run(
  ix: IssueIndex,
  opts: { filters?: ListFilters; expanded?: ReadonlySet<string> | "all"; maxRows?: number; cmp?: typeof compare } = {},
) {
  const filters = opts.filters ?? defaultFilters();
  const matches = new Set(ix.items.filter((i) => passesFilters(i, filters)).map((i) => i.id));
  const expanded = opts.expanded ?? "all";
  return flattenTree({
    roots: ix.tree.roots,
    children: ix.tree.children,
    byId: ix.byId,
    matches,
    isExpanded: (id) => expanded === "all" || expanded.has(id),
    compare: opts.cmp ?? compare,
    maxRows: opts.maxRows,
  });
}

const shape = (rows: TreeRow[]) => rows.map((r) => (r.kind === "cycle" ? `~${r.id}` : r.id));

describe("flattenTree on the server fixture", () => {
  const ix = buildIndex(fx);

  it("walks the placement in display order, duplicating a blocked issue under each blocker", () => {
    const { rows, truncated } = run(ix);
    expect(truncated).toBe(false);
    expect(shape(rows)).toEqual([
      "fx-epic",
      "fx-a",
      "fx-b", // under fx-a (block)
      "fx-c",
      "fx-decide",
      "fx-done",
      "fx-epic2",
      "fx-x",
      "fx-b", // under fx-x (block)
      "fx-found",
      "fx-cy1",
      "fx-cy2",
      "~fx-cy1", // the cycle back-edge
      "fx-later",
    ]);
    const keys = rows.map((r) => r.key);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it("records depth, edge kinds and the blocker", () => {
    const { rows } = run(ix);
    const bUnderA = rows[2] as Extract<TreeRow, { kind: "issue" }>;
    expect(bUnderA).toMatchObject({ id: "fx-b", depth: 2, edges: ["child", "block"], viaId: "fx-a", key: "fx-epic>fx-a>fx-b" });
    const cycle = rows[12] as Extract<TreeRow, { kind: "cycle" }>;
    expect(cycle).toMatchObject({ kind: "cycle", depth: 2, edges: ["block", "block"] });
  });

  it("collapsed roots show their subtree count and no children", () => {
    const { rows } = run(ix, { expanded: new Set() });
    expect(shape(rows)).toEqual(["fx-epic", "fx-decide", "fx-done", "fx-epic2", "fx-found", "fx-cy1", "fx-later"]);
    const epic = rows[0] as Extract<TreeRow, { kind: "issue" }>;
    // fx-a, fx-b (under a), fx-c = 3 rows when fully expanded
    expect(epic).toMatchObject({ hasKids: true, expanded: false, descCount: 3, attention: "danger" });
    const cy1 = rows[5] as Extract<TreeRow, { kind: "issue" }>;
    expect(cy1).toMatchObject({ hasKids: true, descCount: 1 });
  });

  it("opens the default expansion (roots with children) one level", () => {
    const { rows } = run(ix, { expanded: defaultExpanded(ix.tree) });
    expect(shape(rows)).toEqual([
      "fx-epic",
      "fx-a",
      "fx-c",
      "fx-decide",
      "fx-done",
      "fx-epic2",
      "fx-x",
      "fx-found",
      "fx-cy1",
      "fx-cy2",
      "fx-later",
    ]);
    expect(idsWithKids(ix.tree)).toEqual(new Set(["fx-a", "fx-cy1", "fx-cy2", "fx-epic", "fx-epic2", "fx-x"]));
  });

  it("hides closed issues under Active unless they await sign-off", () => {
    const resp: IssuesResponse = { ...fx, items: fx.items.map((i) => (i.id === "fx-done" ? { ...i, signoff: "approved" } : i)) };
    const { rows } = run(buildIndex(resp));
    expect(shape(rows)).not.toContain("fx-done");
    expect(shape(run(ix).rows)).toContain("fx-done");
  });
});

describe("placement and context rows", () => {
  it("shows a closed epic with open children as a context row under Active", () => {
    const resp = mkResponse(
      [mkItem("e", { status: "closed", issue_type: "epic" }), mkItem("c", { parent_id: "e" }), mkItem("done", { status: "closed" })],
      ["e", "done"],
      { e: ["c"] },
    );
    const { rows } = run(buildIndex(resp));
    expect(shape(rows)).toEqual(["e", "c"]);
    expect(rows[0]).toMatchObject({ context: true, expanded: true });
    expect(rows[1]).toMatchObject({ context: false });
  });

  it("drops a branch with no match at all", () => {
    const resp = mkResponse([mkItem("e"), mkItem("c", { priority: 0 }), mkItem("d", { priority: 3 })], ["e"], { e: ["c", "d"] });
    const { rows } = run(buildIndex(resp), { filters: { ...defaultFilters(), priority: [0] } });
    expect(shape(rows)).toEqual(["e", "c"]);
    expect(rows[0]).toMatchObject({ context: true, descCount: 1 });
  });

  it("orders child edges before block edges, each by the comparator", () => {
    const resp = mkResponse(
      [mkItem("p"), mkItem("c1", { priority: 3 }), mkItem("c2", { priority: 1 }), mkItem("b1", { priority: 0 }), mkItem("b2", { priority: 2 })],
      ["p"],
      { p: [["b2", "block"], ["c1", "child"], ["b1", "block"], ["c2", "child"]] },
    );
    expect(shape(run(buildIndex(resp)).rows)).toEqual(["p", "c2", "c1", "b1", "b2"]);
  });

  it("lists an issue once per parent even when its edge repeats", () => {
    const resp = mkResponse([mkItem("p"), mkItem("c")], ["p"], { p: [["c", "child"], ["c", "block"]] });
    expect(shape(run(buildIndex(resp)).rows)).toEqual(["p", "c"]);
  });

  it("keeps a multi-blocker duplicate under each blocker, with the full subtree each time", () => {
    const resp = mkResponse(
      [mkItem("a"), mkItem("x"), mkItem("b", { blocked: true }), mkItem("leaf", { parent_id: "b" })],
      ["a", "x"],
      { a: [["b", "block"]], x: [["b", "block"]], b: ["leaf"] },
    );
    const { rows } = run(buildIndex(resp));
    expect(shape(rows)).toEqual(["a", "b", "leaf", "x", "b", "leaf"]);
    expect(rows.filter((r) => r.kind === "issue" && r.id === "b").map((r) => r.key)).toEqual(["a>b", "x>b"]);
    expect((rows[0] as Extract<TreeRow, { kind: "issue" }>).descCount).toBe(2);
  });
});

describe("cycle guard", () => {
  it("renders a 2-cycle once, then a cycle row", () => {
    const resp = mkResponse([mkItem("p"), mkItem("q")], ["p"], { p: [["q", "block"]], q: [["p", "block"]] });
    const { rows, truncated } = run(buildIndex(resp));
    expect(truncated).toBe(false);
    expect(shape(rows)).toEqual(["p", "q", "~p"]);
    expect(rows[1]).toMatchObject({ kind: "issue", expanded: true, descCount: 0 });
  });

  it("renders a self block as a cycle row", () => {
    const resp = mkResponse([mkItem("s")], ["s"], { s: [["s", "block"]] });
    const { rows } = run(buildIndex(resp));
    expect(shape(rows)).toEqual(["s", "~s"]);
    expect(rows[0]).toMatchObject({ hasKids: true });
  });

  it("terminates on a parent cycle", () => {
    const resp = mkResponse([mkItem("a", { parent_id: "b" }), mkItem("b", { parent_id: "a" })], ["a"], { a: ["b"], b: ["a"] });
    expect(shape(run(buildIndex(resp)).rows)).toEqual(["a", "b", "~a"]);
  });

  it("subtreeInfo is exact on a cycle where only one member matches", () => {
    const resp = mkResponse([mkItem("p"), mkItem("q", { priority: 0 })], ["p"], { p: [["q", "block"]], q: [["p", "block"]] });
    const ix = buildIndex(resp);
    const info = subtreeInfo(ix.tree.children, ix.byId, new Set(["q"]));
    expect(info("p").hasMatch).toBe(true);
    expect(info("q").hasMatch).toBe(true);
    expect(info("p").descCount).toBe(1);
  });
});

describe("attention", () => {
  it("flags a blocked descendant as danger and a decision as warning, danger winning", () => {
    const items = [
      mkItem("r1"),
      mkItem("blk", { blocked: true, ready: false }),
      mkItem("r2"),
      mkItem("ask", { labels: ["needs-decision"] }),
      mkItem("r3"),
      mkItem("both1", { blocked: true }),
      mkItem("both2", { labels: ["needs-design"] }),
    ];
    const resp = mkResponse(items, ["r1", "r2", "r3"], { r1: ["blk"], r2: ["ask"], r3: ["both1", "both2"] });
    const rows = run(buildIndex(resp), { expanded: new Set() }).rows as Array<Extract<TreeRow, { kind: "issue" }>>;
    expect(rows.map((r) => r.attention)).toEqual(["danger", "warning", "danger"]);
  });

  it("ignores the node's own flags", () => {
    const resp = mkResponse([mkItem("r", { blocked: true }), mkItem("c")], ["r"], { r: ["c"] });
    const rows = run(buildIndex(resp), { expanded: new Set() }).rows as Array<Extract<TreeRow, { kind: "issue" }>>;
    expect(rows[0]?.attention).toBeNull();
  });
});

describe("limits and caching", () => {
  it("truncates at maxRows", () => {
    const kids = Array.from({ length: 20 }, (_, i) => mkItem(`k${String(i).padStart(2, "0")}`));
    const resp = mkResponse([mkItem("p"), ...kids], ["p"], { p: kids.map((k) => k.id) });
    const { rows, truncated } = run(buildIndex(resp), { maxRows: 5 });
    expect(rows).toHaveLength(5);
    expect(truncated).toBe(true);
    expect(run(buildIndex(resp), { maxRows: 21 }).truncated).toBe(false);
  });

  it("guards a heavily duplicated DAG", () => {
    // 12 layers of two nodes, each feeding both nodes of the next layer: 2^12 paths.
    const items: ListItem[] = [];
    const children: Record<string, Array<[string, "block"]>> = {};
    const L = 12;
    for (let l = 0; l < L; l++) for (const s of ["a", "b"]) items.push(mkItem(`${s}${l}`));
    for (let l = 0; l < L - 1; l++) for (const s of ["a", "b"]) children[`${s}${l}`] = [[`a${l + 1}`, "block"], [`b${l + 1}`, "block"]];
    const resp = mkResponse(items, ["a0", "b0"], children);
    const { rows, truncated } = run(buildIndex(resp), { maxRows: 1000 });
    expect(truncated).toBe(true);
    expect(rows).toHaveLength(1000);
  });

  it("reuses a TreeOrder without changing the result", () => {
    const ix = buildIndex(fx);
    const order = makeTreeOrder(ix.tree, ix.byId, compare);
    expect(order.kids("fx-epic")).toBe(order.kids("fx-epic"));
    expect(order.roots()).toBe(order.roots());
    const matches = new Set(ix.items.map((i) => i.id));
    const info = subtreeInfo(ix.tree.children, ix.byId, matches);
    const base = { roots: ix.tree.roots, children: ix.tree.children, byId: ix.byId, matches, compare, order, info };
    const none = flattenTree({ ...base, isExpanded: () => false });
    const all = flattenTree({ ...base, isExpanded: () => true });
    expect(none.rows).toHaveLength(7);
    expect(all.rows).toHaveLength(14);
  });

  it("treats a prototype-named id as an ordinary leaf", () => {
    const resp = mkResponse([mkItem("constructor"), mkItem("toString")], ["constructor", "toString"]);
    expect(shape(run(buildIndex(resp)).rows)).toEqual(["constructor", "toString"]);
  });
});

describe("findPath", () => {
  const setup = (resp: IssuesResponse, filters = defaultFilters()) => {
    const ix = buildIndex(resp);
    const matches = new Set(ix.items.filter((i) => passesFilters(i, filters)).map((i) => i.id));
    return { ix, order: makeTreeOrder(ix.tree, ix.byId, compare), info: subtreeInfo(ix.tree.children, ix.byId, matches) };
  };

  it("returns the ancestors of the first placement in display order", () => {
    const { order, info } = setup(fx);
    expect(findPath(order, info, "fx-epic")).toEqual([]);
    expect(findPath(order, info, "fx-a")).toEqual(["fx-epic"]);
    // fx-b sits under fx-a (inside fx-epic, which sorts first) and under fx-x
    expect(findPath(order, info, "fx-b")).toEqual(["fx-epic", "fx-a"]);
    expect(findPath(order, info, "fx-x")).toEqual(["fx-epic2"]);
    expect(findPath(order, info, "fx-cy2")).toEqual(["fx-cy1"]);
  });

  it("expanding the path makes the row render", () => {
    const { ix, order, info } = setup(fx);
    const path = new Set(findPath(order, info, "fx-b"));
    const matches = new Set(ix.items.map((i) => i.id));
    const { rows } = flattenTree({
      roots: ix.tree.roots,
      children: ix.tree.children,
      byId: ix.byId,
      matches,
      isExpanded: (id) => path.has(id),
      compare,
      order,
      info,
    });
    expect(shape(rows)).toContain("fx-b");
  });

  it("is null for an id that is not in the visible tree", () => {
    const { order, info } = setup(fx, { ...defaultFilters(), type: ["bug"] });
    expect(findPath(order, info, "fx-a")).toBeNull();
    expect(findPath(order, info, "nope")).toBeNull();
    expect(findPath(order, info, "fx-found")).toEqual([]);
  });

  it("terminates on cycles", () => {
    const resp = mkResponse([mkItem("p"), mkItem("q"), mkItem("r")], ["p"], { p: [["q", "block"]], q: [["p", "block"], ["r", "child"]] });
    const { order, info } = setup(resp);
    expect(findPath(order, info, "r")).toEqual(["p", "q"]);
  });
});
