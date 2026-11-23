package snapshot

import (
	"sort"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

const (
	// MaxBlockingTreeDepth caps how deep a blocking chain is expanded.
	MaxBlockingTreeDepth = 64
	// MaxBlockingTreeNodes caps the number of nodes in a blocking-chain forest.
	MaxBlockingTreeNodes = 10000
)

// TreeNode represents a node in the hierarchical issue tree.
type TreeNode struct {
	Issue     *model.Issue // Reference to the actual issue
	Children  []*TreeNode  // Child nodes
	Expanded  bool         // Default expansion (depth < 2); views may override
	Depth     int          // Nesting level (0 = root)
	Parent    *TreeNode    // Back-reference for navigation
	SharedRef bool         // True when attached as a non-reexpanded shared DAG target
}

// treeBuilder is the scratch state shared by the tree constructors.
type treeBuilder struct {
	byID  map[string]*TreeNode
	roots []*TreeNode
}

// ScanTreeEdges reports whether issues contain at least one parent-child edge
// and at least one blocking edge.
func ScanTreeEdges(issues []model.Issue) (hasParentChild, hasBlocking bool) {
	for i := range issues {
		for _, dep := range issues[i].Dependencies {
			if dep == nil {
				continue
			}
			if dep.Type == model.DepParentChild {
				hasParentChild = true
			}
			if dep.Type.IsBlocking() {
				hasBlocking = true
			}
			if hasParentChild && hasBlocking {
				return true, true
			}
		}
	}
	return hasParentChild, hasBlocking
}

// BuildBlockingChains constructs blocker -> dependent chains over blocking edges
// in strictly O(V + E) nodes by expanding each issue's downstream DAG once and
// attaching leaf SharedRef nodes on secondary incoming blocking edges.
func BuildBlockingChains(issues []model.Issue) ([]*TreeNode, map[string]*TreeNode, bool) {
	t := treeBuilder{
		byID: make(map[string]*TreeNode),
	}
	if len(issues) == 0 {
		return nil, t.byID, false
	}

	issueByID := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		issueByID[issues[i].ID] = &issues[i]
	}

	childrenOf := make(map[string][]*model.Issue)
	seenChildEdge := make(map[string]map[string]bool)
	hasInSetBlocker := make(map[string]bool)
	participates := make(map[string]bool)
	hasBlocking := false

	for i := range issues {
		issue := &issues[i]
		for _, dep := range issue.Dependencies {
			if dep == nil || !dep.Type.IsBlocking() {
				continue
			}
			hasBlocking = true
			participates[issue.ID] = true
			blockerID := dep.DependsOnID
			if _, ok := issueByID[blockerID]; ok {
				participates[blockerID] = true
				if seenChildEdge[blockerID] == nil {
					seenChildEdge[blockerID] = make(map[string]bool)
				}
				if !seenChildEdge[blockerID][issue.ID] {
					seenChildEdge[blockerID][issue.ID] = true
					childrenOf[blockerID] = append(childrenOf[blockerID], issue)
				}
				hasInSetBlocker[issue.ID] = true
			}
		}
	}

	if !hasBlocking {
		return nil, t.byID, false
	}

	sortBlockingIssues := func(list []*model.Issue) {
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if a == nil || b == nil {
				return a != nil
			}
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return a.ID < b.ID
		})
	}

	var rootIssues []*model.Issue
	for i := range issues {
		issue := &issues[i]
		if participates[issue.ID] && !hasInSetBlocker[issue.ID] {
			rootIssues = append(rootIssues, issue)
		}
	}
	sortBlockingIssues(rootIssues)
	for id := range childrenOf {
		sortBlockingIssues(childrenOf[id])
	}

	// O(V + E) minimum-depth BFS pre-pass so shallow roots/branches always own the
	// primary expanded subtree and shared descendants are never hidden behind a
	// deeper collapsed path.
	claimed := make(map[string]bool, len(participates))
	primaryParent := make(map[string]string, len(participates))
	bfsDepth := make(map[string]int, len(participates))
	queue := make([]string, 0, len(participates))

	drainBFS := func() {
		for head := 0; head < len(queue); head++ {
			u := queue[head]
			if bfsDepth[u] >= MaxBlockingTreeDepth {
				continue
			}
			for _, child := range childrenOf[u] {
				if child == nil || claimed[child.ID] {
					continue
				}
				claimed[child.ID] = true
				primaryParent[child.ID] = u
				bfsDepth[child.ID] = bfsDepth[u] + 1
				queue = append(queue, child.ID)
			}
		}
		queue = queue[:0]
	}

	for _, root := range rootIssues {
		if root == nil || claimed[root.ID] {
			continue
		}
		claimed[root.ID] = true
		primaryParent[root.ID] = ""
		bfsDepth[root.ID] = 0
		queue = append(queue, root.ID)
	}
	drainBFS()

	// Cycle fallback: if a connected component is a pure blocking cycle with no
	// unblocked root, deterministically walk blocking predecessors to the cycle,
	// pick the lowest-ID member as display root, and run BFS from it.
	var remaining []string
	for id := range participates {
		if !claimed[id] {
			remaining = append(remaining, id)
		}
	}
	sort.Strings(remaining)
	var cycleRoots []*model.Issue
	for _, startID := range remaining {
		if claimed[startID] {
			continue
		}
		curr := startID
		order := []string{}
		pos := make(map[string]int)
		for {
			if idx, seen := pos[curr]; seen {
				minID := order[idx]
				for _, cid := range order[idx+1:] {
					if cid < minID {
						minID = cid
					}
				}
				curr = minID
				break
			}
			pos[curr] = len(order)
			order = append(order, curr)
			nextBlocker := ""
			found := false
			for _, dep := range issueByID[curr].Dependencies {
				if dep == nil || !dep.Type.IsBlocking() || issueByID[dep.DependsOnID] == nil {
					continue
				}
				if !found || dep.DependsOnID < nextBlocker {
					nextBlocker = dep.DependsOnID
					found = true
				}
			}
			if !found {
				break
			}
			curr = nextBlocker
		}
		seedID := curr
		if claimed[seedID] {
			seedID = startID
		}
		if issueByID[seedID] != nil && !claimed[seedID] {
			claimed[seedID] = true
			primaryParent[seedID] = ""
			bfsDepth[seedID] = 0
			cycleRoots = append(cycleRoots, issueByID[seedID])
			queue = append(queue, seedID)
			drainBFS()
		}
	}
	sortBlockingIssues(cycleRoots)

	visited := make(map[string]bool)
	primaryBuilt := make(map[string]bool, len(issues))
	nodeCount := 0

	var buildDAGNode func(issue *model.Issue, depth int, parent *TreeNode) *TreeNode
	buildDAGNode = func(issue *model.Issue, depth int, parent *TreeNode) *TreeNode {
		if issue == nil || nodeCount >= MaxBlockingTreeNodes {
			return nil
		}
		parentID := ""
		if parent != nil && parent.Issue != nil {
			parentID = parent.Issue.ID
		}
		isPrimary := !primaryBuilt[issue.ID] && primaryParent[issue.ID] == parentID
		if visited[issue.ID] || !isPrimary {
			nodeCount++
			return &TreeNode{
				Issue:     issue,
				Depth:     depth,
				Parent:    parent,
				Expanded:  false,
				SharedRef: true,
			}
		}

		primaryBuilt[issue.ID] = true
		nodeCount++
		node := &TreeNode{
			Issue:    issue,
			Depth:    depth,
			Parent:   parent,
			Expanded: depth < 2,
		}
		t.byID[issue.ID] = node
		if depth >= MaxBlockingTreeDepth {
			return node
		}

		visited[issue.ID] = true
		defer func() { visited[issue.ID] = false }()

		for _, child := range childrenOf[issue.ID] {
			if nodeCount >= MaxBlockingTreeNodes {
				break
			}
			if childNode := buildDAGNode(child, depth+1, node); childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		}
		SortTreeNodes(node.Children)
		return node
	}

	for _, issue := range rootIssues {
		if nodeCount >= MaxBlockingTreeNodes {
			break
		}
		if node := buildDAGNode(issue, 0, nil); node != nil {
			t.roots = append(t.roots, node)
		}
	}
	for _, issue := range cycleRoots {
		if nodeCount >= MaxBlockingTreeNodes {
			break
		}
		if node := buildDAGNode(issue, 0, nil); node != nil {
			t.roots = append(t.roots, node)
		}
	}

	SortTreeNodes(t.roots)
	return t.roots, t.byID, true
}

// BuildHierarchy constructs the hierarchical parent/child tree and returns:
// - roots: root nodes (sorted)
// - nodeMap: issue ID -> tree node
//
// This does NOT load persisted expand/collapse state or build the visible flat
// list. Those remain view concerns, so user state can change without requiring a
// snapshot rebuild.
func BuildHierarchy(issues []model.Issue) ([]*TreeNode, map[string]*TreeNode) {
	t := treeBuilder{
		byID: make(map[string]*TreeNode),
	}
	if len(issues) == 0 {
		return nil, t.byID
	}

	// Step 1: Build parent→children index and track which issues have parents
	childrenOf := make(map[string][]*model.Issue)
	hasParent := make(map[string]bool)
	issueByID := make(map[string]*model.Issue)

	for i := range issues {
		issue := &issues[i]
		issueByID[issue.ID] = issue

		for _, dep := range issue.Dependencies {
			if dep != nil && dep.Type == model.DepParentChild {
				parentID := dep.DependsOnID
				childrenOf[parentID] = append(childrenOf[parentID], issue)
				hasParent[issue.ID] = true
			}
		}
	}

	// Step 2: Identify root nodes (issues with no parent OR whose parent doesn't exist)
	var rootIssues []*model.Issue
	for i := range issues {
		issue := &issues[i]
		if !hasParent[issue.ID] {
			rootIssues = append(rootIssues, issue)
			continue
		}

		// Issue declares a parent - verify at least one referenced parent exists
		hasValidParent := false
		for _, dep := range issue.Dependencies {
			if dep != nil && dep.Type == model.DepParentChild {
				if _, exists := issueByID[dep.DependsOnID]; exists {
					hasValidParent = true
					break
				}
			}
		}
		if !hasValidParent {
			rootIssues = append(rootIssues, issue)
		}
	}

	// Step 3: Build tree recursively with cycle detection
	visited := make(map[string]bool)
	for _, issue := range rootIssues {
		node := t.buildNode(issue, 0, childrenOf, nil, visited)
		if node != nil {
			t.roots = append(t.roots, node)
		}
	}

	// A component made entirely of parent-child cycles has no natural root.
	// Follow a deterministic parent chain from each unrepresented issue to a
	// cycle member and use that member as a display root. Starting at an
	// arbitrary descendant would unnecessarily split its hierarchy into roots.
	// The existing path guard keeps the resulting tree finite; source
	// dependencies are not changed.
	var remaining []string
	if len(t.byID) < len(issueByID) {
		for id := range issueByID {
			if t.byID[id] == nil {
				remaining = append(remaining, id)
			}
		}
	}
	sort.Strings(remaining)
	for _, id := range remaining {
		if t.byID[id] != nil {
			continue
		}
		onPath := make(map[string]bool)
		for !onPath[id] {
			onPath[id] = true
			parent := ""
			foundParent := false
			for _, dep := range issueByID[id].Dependencies {
				if dep == nil || dep.Type != model.DepParentChild || issueByID[dep.DependsOnID] == nil {
					continue
				}
				if !foundParent || dep.DependsOnID < parent {
					parent = dep.DependsOnID
					foundParent = true
				}
			}
			if !foundParent {
				break
			}
			id = parent
		}
		if node := t.buildNode(issueByID[id], 0, childrenOf, nil, visited); node != nil {
			t.roots = append(t.roots, node)
		}
	}

	// Step 4: Sort roots by priority, type, then created date
	SortTreeNodes(t.roots)

	return t.roots, t.byID
}

// buildNode recursively builds a tree node and its children.
// Uses visited map for cycle detection.
func (t *treeBuilder) buildNode(issue *model.Issue, depth int,
	childrenOf map[string][]*model.Issue,
	parent *TreeNode,
	visited map[string]bool) *TreeNode {

	if issue == nil {
		return nil
	}

	// Cycle detection - if we've already visited this node in current path
	if visited[issue.ID] {
		// Return a node marked as part of a cycle (no children to break the loop)
		return &TreeNode{
			Issue:    issue,
			Depth:    depth,
			Parent:   parent,
			Expanded: false,
			// Children intentionally left empty to break cycle
		}
	}

	// Mark as visited for cycle detection
	visited[issue.ID] = true
	defer func() { visited[issue.ID] = false }()

	node := &TreeNode{
		Issue:    issue,
		Depth:    depth,
		Parent:   parent,
		Expanded: depth < 2, // Auto-expand first 2 levels
	}

	// Store in lookup map
	t.byID[issue.ID] = node

	// Build children recursively
	children := childrenOf[issue.ID]
	for _, child := range children {
		childNode := t.buildNode(child, depth+1, childrenOf, node, visited)
		if childNode != nil {
			node.Children = append(node.Children, childNode)
		}
	}

	// Sort children
	SortTreeNodes(node.Children)

	return node
}

// SortTreeNodes sorts a slice of tree nodes by priority, issue type, then created date.
func SortTreeNodes(nodes []*TreeNode) {
	if len(nodes) <= 1 {
		return
	}

	sort.Slice(nodes, func(i, j int) bool {
		// Defensive: check for nil nodes first
		if nodes[i] == nil || nodes[j] == nil {
			return nodes[i] != nil // Non-nil nodes first
		}
		a, b := nodes[i].Issue, nodes[j].Issue
		if a == nil || b == nil {
			return a != nil // Non-nil issues first
		}

		// 1. Priority (ascending - P0 first)
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}

		// 2. IssueType order: epic → feature → task → bug → chore
		aTypeOrder := IssueTypeOrder(a.IssueType)
		bTypeOrder := IssueTypeOrder(b.IssueType)
		if aTypeOrder != bTypeOrder {
			return aTypeOrder < bTypeOrder
		}

		// 3. CreatedAt (oldest first for stable ordering), then ID tiebreaker
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
}

// IssueTypeOrder returns a numeric order for issue types.
// Lower numbers sort first: epic → feature → task → bug → chore
func IssueTypeOrder(t model.IssueType) int {
	switch t {
	case model.TypeEpic:
		return 0
	case model.TypeFeature:
		return 1
	case model.TypeTask:
		return 2
	case model.TypeBug:
		return 3
	case model.TypeChore:
		return 4
	default:
		return 5
	}
}

// CloneTree creates a deep copy of roots and byID. Tree views mutate
// node.Expanded and node.Children, which would corrupt shared snapshots if only
// pointers were aliased. Issue pointers are rebound to the provided issueMap so
// the copied tree stays detached from any aliasing of the original issues.
// Returns (nil, nil) if input is empty.
func CloneTree(roots []*TreeNode, byID map[string]*TreeNode, issueMap map[string]*model.Issue) ([]*TreeNode, map[string]*TreeNode) {
	if len(roots) == 0 && len(byID) == 0 {
		return nil, nil
	}

	// Build a mapping from old node pointers to new node pointers
	oldToNew := make(map[*TreeNode]*TreeNode, len(byID))

	// The ID map contains only one occurrence per issue. Walk the actual tree
	// too, so shared children and terminal cycle occurrences remain visible.
	// Deduplicate by pointer, not ID, and install each copy before following
	// links so Parent back-references cannot recurse indefinitely.
	var copyNode func(*TreeNode)
	copyNode = func(oldNode *TreeNode) {
		if oldNode == nil || oldToNew[oldNode] != nil {
			return
		}
		issue := oldNode.Issue
		if oldNode.Issue != nil && issueMap != nil {
			if rebound, ok := issueMap[oldNode.Issue.ID]; ok {
				issue = rebound
			}
		}
		newNode := &TreeNode{
			Issue:     issue,
			Expanded:  oldNode.Expanded,
			Depth:     oldNode.Depth,
			SharedRef: oldNode.SharedRef,
			// Children and Parent set in second pass
		}
		oldToNew[oldNode] = newNode
		for _, child := range oldNode.Children {
			copyNode(child)
		}
		copyNode(oldNode.Parent)
	}
	for _, root := range roots {
		copyNode(root)
	}
	for _, node := range byID {
		copyNode(node)
	}

	// Second pass: rebuild Children slices and Parent pointers
	for oldNode, newNode := range oldToNew {
		if len(oldNode.Children) > 0 {
			newNode.Children = make([]*TreeNode, 0, len(oldNode.Children))
			for _, oldChild := range oldNode.Children {
				if newChild, ok := oldToNew[oldChild]; ok {
					newNode.Children = append(newNode.Children, newChild)
				}
			}
		}
		if oldNode.Parent != nil {
			if newParent, ok := oldToNew[oldNode.Parent]; ok {
				newNode.Parent = newParent
			}
		}
	}

	// Build new roots slice
	var newRoots []*TreeNode
	if len(roots) > 0 {
		newRoots = make([]*TreeNode, 0, len(roots))
		for _, oldRoot := range roots {
			if newRoot, ok := oldToNew[oldRoot]; ok {
				newRoots = append(newRoots, newRoot)
			}
		}
	}

	// Build new nodeMap
	var newNodeMap map[string]*TreeNode
	if len(byID) > 0 {
		newNodeMap = make(map[string]*TreeNode, len(byID))
		for id, oldNode := range byID {
			if newNode, ok := oldToNew[oldNode]; ok {
				newNodeMap[id] = newNode
			}
		}
	}

	return newRoots, newNodeMap
}
