package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// item builds a ListItem for tree tests.
func item(id string, prio int, parent string, blockedBy ...string) ListItem {
	if blockedBy == nil {
		blockedBy = []string{}
	}
	return ListItem{ID: id, Priority: prio, ParentID: parent, BlockedBy: blockedBy, Labels: []string{}}
}

func edgeIDs(edges []TreeEdge) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		out[i] = e.Kind + ":" + e.ID
	}
	return out
}

func TestBuildListTreePlacement(t *testing.T) {
	items := []ListItem{
		item("epic", 1, ""),
		item("a", 1, "epic"),
		item("x", 1, "epic2"), // other epic
		item("epic2", 2, ""),
		item("b", 2, "epic", "a", "x"), // blocked by two, one in another epic
		item("c", 3, "epic"),
		item("d", 1, "epic", "outside"), // blocker not in the list: stays under its parent
		item("orphan-blocked", 2, "", "ghost"),
	}
	tree := buildListTree(items)

	if want := []string{"epic", "epic2", "orphan-blocked"}; !reflect.DeepEqual(tree.Roots, want) {
		t.Fatalf("roots = %v, want %v", tree.Roots, want)
	}
	// b appears under BOTH blockers (block edges) and not under its parent.
	if got := edgeIDs(tree.Children["a"]); !reflect.DeepEqual(got, []string{"block:b"}) {
		t.Fatalf("children of a = %v", got)
	}
	if got := edgeIDs(tree.Children["x"]); !reflect.DeepEqual(got, []string{"block:b"}) {
		t.Fatalf("children of x = %v", got)
	}
	// Child edges first, then priority asc, then id. b is not a child of epic.
	// d's only blocker is outside the list, so d stays under its parent.
	if got := edgeIDs(tree.Children["epic"]); !reflect.DeepEqual(got, []string{"child:a", "child:d", "child:c"}) {
		t.Fatalf("children of epic = %v", got)
	}
}

func TestBuildListTreeOrdersChildEdgesBeforeBlockEdges(t *testing.T) {
	items := []ListItem{
		item("p", 1, ""),
		item("kid-late", 5, "p"),
		item("kid-early", 1, "p"),
		item("blocked", 1, "", "p"),
	}
	tree := buildListTree(items)
	got := edgeIDs(tree.Children["p"])
	want := []string{"child:kid-early", "child:kid-late", "block:blocked"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("children of p = %v, want %v", got, want)
	}
}

func TestBuildListTreeCyclesPromoteRoots(t *testing.T) {
	// A blocking 2-cycle, a 3-cycle behind one, a parent cycle, and a self block.
	items := []ListItem{
		item("ok", 1, ""),
		item("c1", 2, "", "c2"),
		item("c2", 2, "", "c1"),
		item("t1", 2, "", "t3"),
		item("t2", 2, "", "t1"),
		item("t3", 2, "", "t2"),
		item("tail", 2, "", "t2"), // reachable only through the 3-cycle
		item("p1", 2, "p2"),
		item("p2", 2, "p1"),
		item("self", 2, "", "self"),
	}
	tree := buildListTree(items)

	reached := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		if reached[id] {
			return
		}
		reached[id] = true
		for _, e := range tree.Children[id] {
			walk(e.ID)
		}
	}
	for _, r := range tree.Roots {
		walk(r)
	}
	for _, it := range items {
		if !reached[it.ID] {
			t.Errorf("%s is unreachable from the roots %v", it.ID, tree.Roots)
		}
	}
	// Promoted roots come after the natural ones, smallest id first, and an item
	// reached from an earlier promotion is not promoted again.
	want := []string{"ok", "self", "c1", "p1", "t1"}
	if !reflect.DeepEqual(tree.Roots, want) {
		t.Fatalf("roots = %v, want %v", tree.Roots, want)
	}
	seen := map[string]bool{}
	for _, r := range tree.Roots {
		if seen[r] {
			t.Fatalf("duplicate root %s", r)
		}
		seen[r] = true
	}
}

func TestBuildListTreeEmpty(t *testing.T) {
	tree := buildListTree(nil)
	b, _ := json.Marshal(tree)
	if string(b) != `{"roots":[],"children":{}}` {
		t.Fatalf("empty tree = %s", b)
	}
}

// --- handlers ---

type issuesBody struct {
	Phase2Ready bool       `json:"phase2_ready"`
	Total       int        `json:"total"`
	Items       []ListItem `json:"items"`
	Tree        *ListTree  `json:"tree"`
}

func goldenHandler(t *testing.T) http.Handler {
	t.Helper()
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)
	return newTestHandler(t, func(c *Config) { c.Store = store })
}

func TestIssuesTreeAndFlat(t *testing.T) {
	h := goldenHandler(t)

	rec := do(h, "GET", "/api/v1/issues", withSession()) // default view is tree
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var tree issuesBody
	if err := json.Unmarshal(rec.Body.Bytes(), &tree); err != nil {
		t.Fatal(err)
	}
	if tree.Tree == nil || tree.Total != len(goldenIssues()) || len(tree.Items) != tree.Total || !tree.Phase2Ready {
		t.Fatalf("tree body: tree=%v total=%d items=%d ready=%v", tree.Tree != nil, tree.Total, len(tree.Items), tree.Phase2Ready)
	}
	// Placement B: fx-b (blocked by fx-a and fx-x) is listed under both, not under fx-epic.
	for _, blocker := range []string{"fx-a", "fx-x"} {
		if got := edgeIDs(tree.Tree.Children[blocker]); !reflect.DeepEqual(got, []string{"block:fx-b"}) {
			t.Errorf("children of %s = %v", blocker, got)
		}
	}
	for _, e := range tree.Tree.Children["fx-epic"] {
		if e.ID == "fx-b" {
			t.Error("a blocked item must not also sit under its parent")
		}
	}
	// The blocking 2-cycle still gets a root.
	if !contains(tree.Tree.Roots, "fx-cy1") {
		t.Errorf("roots %v should include the promoted cycle root fx-cy1", tree.Tree.Roots)
	}

	byID := map[string]ListItem{}
	for _, it := range tree.Items {
		byID[it.ID] = it
		if it.Labels == nil || it.BlockedBy == nil {
			t.Errorf("%s: labels/blocked_by must serialize as [] not null", it.ID)
		}
	}
	if b := byID["fx-b"]; !b.Blocked || b.Ready || b.ParentID != "fx-epic" || !reflect.DeepEqual(b.BlockedBy, []string{"fx-a", "fx-x"}) {
		t.Errorf("fx-b = %+v", b)
	}
	if e := byID["fx-epic"]; e.ChildCount != 3 || !e.Ready {
		t.Errorf("fx-epic = %+v", e)
	}
	if d := byID["fx-done"]; d.Signoff != "pending" || d.ClosedAt == nil || d.Blocked {
		t.Errorf("fx-done = %+v", d)
	}
	if a := byID["fx-a"]; a.DependentCount != 1 || a.TriageScore == nil {
		t.Errorf("fx-a = %+v", a)
	}
	if byID["fx-later"].Ready {
		t.Error("a deferred issue is not ready")
	}

	rec = do(h, "GET", "/api/v1/issues?view=flat", withSession())
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &flat); err != nil {
		t.Fatal(err)
	}
	if _, ok := flat["tree"]; ok {
		t.Fatal("flat view must not carry a tree")
	}
	var flatBody issuesBody
	if err := json.Unmarshal(rec.Body.Bytes(), &flatBody); err != nil || !reflect.DeepEqual(flatBody.Items, tree.Items) {
		t.Fatalf("flat items differ from tree items: %v", err)
	}
}

func TestIssuesErrors(t *testing.T) {
	h := goldenHandler(t)
	rec := do(h, "GET", "/api/v1/issues?view=grid", withSession())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad view: %d", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != "bad_request" {
		t.Fatalf("code = %q", e.Code)
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatal("errors must not carry an ETag")
	}
	rec = do(h, "POST", "/api/v1/issues", withSession(), withOrigin(localOrigin))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	for _, target := range []string{"/api/v1/issues", "/api/v1/search?q=a"} {
		rec = do(newTestHandler(t, nil), "GET", target, withSession())
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s without a store: %d", target, rec.Code)
		}
	}
}

func TestIssuesAndSearchConditionalGet(t *testing.T) {
	h := goldenHandler(t)
	etagOf := func(target string) string {
		rec := do(h, "GET", target, withSession())
		if rec.Code != http.StatusOK || rec.Header().Get("ETag") == "" {
			t.Fatalf("%s: %d etag=%q", target, rec.Code, rec.Header().Get("ETag"))
		}
		return rec.Header().Get("ETag")
	}
	cond := func(target, tag string) int {
		return do(h, "GET", target, withSession(), func(r *http.Request) { r.Header.Set("If-None-Match", tag) }).Code
	}

	tree, flat := etagOf("/api/v1/issues?view=tree"), etagOf("/api/v1/issues?view=flat")
	if tree == flat {
		t.Fatal("tree and flat are different bodies and need different ETags")
	}
	if cond("/api/v1/issues?view=tree", tree) != http.StatusNotModified || cond("/api/v1/issues", tree) != http.StatusNotModified {
		t.Error("matching tag must 304 (explicit and default tree view)")
	}
	if cond("/api/v1/issues?view=flat", tree) != http.StatusOK {
		t.Error("a tree tag must not validate the flat body")
	}

	a, b := etagOf("/api/v1/search?q=cart"), etagOf("/api/v1/search?q=gift")
	if a == b {
		t.Fatal("different queries need different ETags")
	}
	if cond("/api/v1/search?q=cart", a) != http.StatusNotModified || cond("/api/v1/search?q=gift", a) != http.StatusOK {
		t.Error("search ETag must be bound to its query")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestSearchHandler(t *testing.T) {
	h := goldenHandler(t)
	get := func(q string) (SearchResponse, int) {
		rec := do(h, "GET", "/api/v1/search?q="+url.QueryEscape(q), withSession())
		var resp SearchResponse
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
		}
		return resp, rec.Code
	}

	resp, code := get("cart")
	if code != http.StatusOK || resp.Query != "cart" || resp.Total != len(resp.Hits) || resp.Total == 0 {
		t.Fatalf("cart: %d %+v", code, resp)
	}
	for i := 1; i < len(resp.Hits); i++ {
		if resp.Hits[i-1].Score < resp.Hits[i].Score {
			t.Fatalf("hits are not in rank order: %+v", resp.Hits)
		}
	}
	ids := []string{}
	for _, hit := range resp.Hits {
		ids = append(ids, hit.ID)
	}
	if !contains(ids, "fx-a") || !contains(ids, "fx-done") {
		t.Fatalf("cart should match Cart model and Ship the cart: %v", ids)
	}

	// Status, type and id are part of the matched text (search.FilterText).
	if resp, _ = get("closed"); len(resp.Hits) == 0 || resp.Hits[0].ID != "fx-done" {
		t.Errorf("status text is searchable: %+v", resp.Hits)
	}
	// No match is an empty list, not null.
	rec := do(h, "GET", "/api/v1/search?q=zzzzzqqqq", withSession())
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"hits":[]`) {
		t.Fatalf("no-match body: %d %s", rec.Code, rec.Body.String())
	}

	for name, q := range map[string]string{"empty": "", "too long": strings.Repeat("a", maxSearchQuery+1)} {
		if _, code := get(q); code != http.StatusBadRequest {
			t.Errorf("%s q: %d, want 400", name, code)
		}
	}
	if _, code := get(strings.Repeat("a", maxSearchQuery)); code != http.StatusOK {
		t.Errorf("q at the limit: %d, want 200", code)
	}
	if rec := do(h, "GET", "/api/v1/search", withSession()); rec.Code != http.StatusBadRequest {
		t.Errorf("missing q: %d", rec.Code)
	}
}

func TestBuildListTreeClosedItemIsNotBlocked(t *testing.T) {
	// A closed item whose blocker is still open stays under its parent: only an
	// open item is "blocked". Its BlockedBy is untouched data.
	closed := item("done", 2, "epic", "blk")
	closed.Status = "closed"
	items := []ListItem{
		item("epic", 1, ""),
		item("blk", 1, ""),
		closed,
		item("open-blocked", 2, "epic", "blk"),
	}
	tree := buildListTree(items)
	if got := edgeIDs(tree.Children["epic"]); !reflect.DeepEqual(got, []string{"child:done"}) {
		t.Fatalf("children of epic = %v, want the closed item as a child", got)
	}
	if got := edgeIDs(tree.Children["blk"]); !reflect.DeepEqual(got, []string{"block:open-blocked"}) {
		t.Fatalf("children of blk = %v, want only the open blocked item", got)
	}
	if !reflect.DeepEqual(items[2].BlockedBy, []string{"blk"}) {
		t.Fatal("BlockedBy must keep describing the dependency")
	}

	// A closed item with no parent becomes a root rather than a blocked child.
	orphan := item("closed-orphan", 2, "", "blk")
	orphan.Status = "tombstone"
	tree = buildListTree([]ListItem{item("blk", 1, ""), orphan})
	if !reflect.DeepEqual(tree.Roots, []string{"blk", "closed-orphan"}) || len(tree.Children) != 0 {
		t.Fatalf("roots = %v children = %v", tree.Roots, tree.Children)
	}
}

func TestIssuesDeferUntil(t *testing.T) {
	h := goldenHandler(t)
	var body issuesBody
	rec := do(h, "GET", "/api/v1/issues?view=flat", withSession())
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, it := range body.Items {
		switch it.ID {
		case "fx-later":
			if it.DeferUntil == nil || !it.DeferUntil.Equal(ts(30, 9)) || it.Ready || it.Blocked {
				t.Errorf("fx-later = %+v", it)
			}
		default:
			if it.DeferUntil != nil {
				t.Errorf("%s has an unexpected defer_until", it.ID)
			}
		}
	}
}
