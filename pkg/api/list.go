package api

import (
	"net/http"
	"sort"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/search"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// Tree edge kinds.
const (
	edgeChild = "child"
	edgeBlock = "block"
)

// listIndex is the per-View, lazily built list state: the DTO items and the
// strings /search matches against. Both are over Snap.ViewIssues (the recipe
// view, as in the TUI), in snapshot order.
type listIndex struct {
	items   []ListItem
	byID    map[string]int
	targets []string // search.FilterText per item, parallel to items
}

// list returns the View's list index, building it on first use.
func (v *View) list() *listIndex {
	v.listOnce.Do(func() { v.listIdx = buildListIndex(v.Snap) })
	return v.listIdx
}

// listTree returns the View's tree adjacency, building it on first use.
func (v *View) listTree() *ListTree {
	v.treeOnce.Do(func() { v.tree = buildListTree(v.list().items) })
	return v.tree
}

func closedLike(s model.Status) bool { return s.IsClosed() || s.IsTombstone() }

func buildListIndex(snap *snapshot.Snapshot) *listIndex {
	view := snap.ViewIssues
	idx := &listIndex{
		items:   make([]ListItem, 0, len(view)),
		byID:    make(map[string]int, len(view)),
		targets: make([]string, 0, len(view)),
	}
	for i := range view {
		idx.byID[view[i].ID] = i
	}
	var readiness *model.ReadinessIndex
	if snap.Analyzer != nil {
		readiness = snap.Analyzer.Readiness()
	}
	dependents := snapshot.ActiveDependentsCounts(snap.Issues)

	for i := range view {
		issue := view[i]
		item := ListItem{
			ID:             issue.ID,
			Title:          issue.Title,
			Status:         string(issue.Status),
			Priority:       issue.Priority,
			IssueType:      string(issue.IssueType),
			Assignee:       issue.Assignee,
			Labels:         append([]string{}, issue.Labels...),
			CreatedAt:      issue.CreatedAt,
			UpdatedAt:      issue.UpdatedAt,
			ClosedAt:       issue.ClosedAt,
			DeferUntil:     issue.DeferUntil,
			BlockedBy:      snapshot.ActiveBlockerIDs(snap.IssueMap, readiness, issue),
			DependentCount: dependents[issue.ID],
			Ready:          snap.IsReady(issue.ID),
			QuickWin:       snap.QuickWinSet[issue.ID],
			IsBlocker:      snap.BlockerSet[issue.ID],
			Signoff:        agentdesk.SignoffState(issue),
		}
		item.Blocked = !closedLike(issue.Status) && (issue.Status == model.StatusBlocked || len(item.BlockedBy) > 0)
		if score, ok := snap.TriageScores[issue.ID]; ok {
			item.TriageScore = &score
		}
		// The first parent-child dependency whose target is in the item set.
		for _, dep := range issue.Dependencies {
			if dep == nil || dep.Type != model.DepParentChild || dep.DependsOnID == "" || dep.DependsOnID == issue.ID {
				continue
			}
			if _, ok := idx.byID[dep.DependsOnID]; ok {
				item.ParentID = dep.DependsOnID
				break
			}
		}
		idx.items = append(idx.items, item)
		idx.targets = append(idx.targets, search.FilterText(issue, search.RepoKey(issue)))
	}
	for i := range idx.items {
		if p := idx.items[i].ParentID; p != "" {
			idx.items[idx.byID[p]].ChildCount++
		}
	}
	return idx
}

// buildListTree places every item in the approved list tree (mock-list
// buildTree mode "every"):
//
//  1. An OPEN item with active blockers that are in the item set goes under
//     EACH of them (edge kind "block"), not under its parent and not as a root.
//     A closed item is not blocked, whatever its dependencies say.
//  2. Otherwise an item with a parent in the set goes under it (kind "child").
//  3. Otherwise it is a root.
//
// The server decides placement; the client renders the same id under several
// parents, lazily, stopping at an id already on the current path. The payload
// stays O(V+E) however many blockers an item has.
//
// Cycles (pure blocking cycles, parent cycles) leave items unreachable from the
// roots; they are promoted to roots, smallest id first, each followed by a DFS.
func buildListTree(items []ListItem) *ListTree {
	byID := make(map[string]*ListItem, len(items))
	for i := range items {
		byID[items[i].ID] = &items[i]
	}
	children := map[string][]TreeEdge{}
	var roots []string
	for i := range items {
		it := &items[i]
		// Only an OPEN item is "blocked": a closed item whose blocker is still
		// open keeps its place under its parent (its BlockedBy stays as data).
		var blockers []*ListItem
		if !closedLike(model.Status(it.Status)) {
			for _, id := range it.BlockedBy {
				if b, ok := byID[id]; ok && b.ID != it.ID {
					blockers = append(blockers, b)
				}
			}
		}
		switch {
		case len(blockers) > 0:
			for _, b := range blockers {
				children[b.ID] = append(children[b.ID], TreeEdge{ID: it.ID, Kind: edgeBlock})
			}
		case it.ParentID != "":
			children[it.ParentID] = append(children[it.ParentID], TreeEdge{ID: it.ID, Kind: edgeChild})
		default:
			roots = append(roots, it.ID)
		}
	}

	// Deterministic order: child edges first, then block edges; within each,
	// priority ascending, then id.
	for id, edges := range children {
		sort.SliceStable(edges, func(a, b int) bool {
			if edges[a].Kind != edges[b].Kind {
				return edges[a].Kind == edgeChild
			}
			pa, pb := byID[edges[a].ID].Priority, byID[edges[b].ID].Priority
			if pa != pb {
				return pa < pb
			}
			return edges[a].ID < edges[b].ID
		})
		children[id] = edges
	}

	reached := make(map[string]bool, len(items))
	visit := func(root string) {
		stack := []string{root}
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if reached[id] {
				continue
			}
			reached[id] = true
			for _, e := range children[id] {
				if !reached[e.ID] {
					stack = append(stack, e.ID)
				}
			}
		}
	}
	for _, r := range roots {
		visit(r)
	}
	var unreached []string
	for i := range items {
		if !reached[items[i].ID] {
			unreached = append(unreached, items[i].ID)
		}
	}
	sort.Strings(unreached)
	for _, id := range unreached {
		if !reached[id] {
			roots = append(roots, id)
			visit(id)
		}
	}

	if roots == nil {
		roots = []string{}
	}
	return &ListTree{Roots: roots, Children: children}
}

// handleIssues serves GET /api/v1/issues?view=tree|flat (default tree). Both
// views carry the same items; tree adds the adjacency. Filtering is client-side
// over this one payload (advice R2), and text goes through /search.
func (s *server) handleIssues(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "tree"
	}
	if view != "tree" && view != "flat" {
		writeError(w, http.StatusBadRequest, "bad_request", `view must be "tree" or "flat"`)
		return
	}
	writeViewTagged(w, r, variantETag(v.ETag, view), func() any {
		idx := v.list()
		resp := IssuesResponse{Phase2Ready: v.Phase2Ready, Total: len(idx.items), Items: idx.items}
		if view == "tree" {
			resp.Tree = v.listTree()
		}
		return resp
	})
}
