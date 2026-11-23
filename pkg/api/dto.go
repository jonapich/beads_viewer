package api

import "time"

// GUI-specific response types (ADR 0001 section 3). Rules, mirrored by
// web/src/lib/api/types.ts:
//   - arrays are never null: initialise them to empty;
//   - omitempty fields are optional in TypeScript;
//   - names reuse the model and analysis JSON names where a field already exists.

// MetaResponse is GET /api/v1/meta: what was loaded and how far analysis is.
type MetaResponse struct {
	Version string `json:"version"`
	// DataHash identifies the source data. It does NOT change when Phase 2
	// completes; the ETag does (see Store).
	DataHash string `json:"data_hash"`
	// Phase2Ready is true once the store has swapped in the CompletePhase2
	// result. On the huge tier Snapshot.Phase2Ready is already true at phase 1,
	// so this is deliberately a store flag.
	Phase2Ready bool `json:"phase2_ready"`
	// Phase2Skipped is true on the huge tier, where graph metrics are not
	// computed (only triage is added at Phase 2).
	Phase2Skipped bool   `json:"phase2_skipped"`
	Phase2Error   string `json:"phase2_error,omitempty"`

	Tier                string `json:"tier"`
	IssueCount          int    `json:"issue_count"`
	ViewCount           int    `json:"view_count"`
	SourceIssueCount    int    `json:"source_issue_count"`
	LoadedOpenOnly      bool   `json:"loaded_open_only"`
	TruncatedCount      int    `json:"truncated_count"`
	LargeDatasetWarning string `json:"large_dataset_warning,omitempty"`
	LoadWarningCount    int    `json:"load_warning_count"`
	AuthorityComplete   bool   `json:"authority_complete"`

	ReferenceTime time.Time  `json:"reference_time"`
	Source        MetaSource `json:"source"`
	Recipe        string     `json:"recipe,omitempty"`
	Counts        MetaCounts `json:"counts"`
	Capabilities  MetaCaps   `json:"capabilities"`
}

// MetaSource says where the data came from.
type MetaSource struct {
	Path string `json:"path"`
	// Kind is "jsonl" or "sqlite".
	Kind string `json:"kind"`
	// Tracker is "bd", "br", or "" when unknown.
	Tracker    string `json:"tracker"`
	ProjectDir string `json:"project_dir"`
}

// MetaCounts are the snapshot's status counts (the TUI header numbers).
type MetaCounts struct {
	Open    int `json:"open"`
	Ready   int `json:"ready"`
	Blocked int `json:"blocked"`
	Closed  int `json:"closed"`
}

// MetaCaps tells the client what the server can do.
type MetaCaps struct {
	// Writes is false until the write endpoints land (beads-c2o.19).
	Writes bool `json:"writes"`
}

// IssuesResponse is GET /api/v1/issues?view=tree|flat: every issue of the
// current view as a flat list, plus (view=tree) the tree adjacency. Filtering
// is client-side over this one payload.
type IssuesResponse struct {
	Phase2Ready bool       `json:"phase2_ready"`
	Total       int        `json:"total"`
	Items       []ListItem `json:"items"`
	Tree        *ListTree  `json:"tree,omitempty"`
}

// ListItem is one row of the issue list. Items are in snapshot order.
type ListItem struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Priority  int        `json:"priority"`
	IssueType string     `json:"issue_type"`
	Assignee  string     `json:"assignee,omitempty"`
	Labels    []string   `json:"labels"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	// DeferUntil is when a deferred issue becomes actionable. While it is in the
	// future the issue is open, unblocked and not ready; show it as deferred.
	DeferUntil *time.Time `json:"defer_until,omitempty"`

	// ParentID is the first parent-child dependency whose target is in the list.
	ParentID string `json:"parent_id,omitempty"`
	// ChildCount is the number of list items whose ParentID is this id
	// (parent-child only, for progress or labels). It is NOT the tree's child
	// count: an open blocked child sits under its blockers instead. The
	// expander caret uses ListTree.Children.
	ChildCount int `json:"child_count"`
	// BlockedBy are the active (unsatisfied) blocker ids: deduplicated, in
	// dependency order. Blockers outside the list are included.
	BlockedBy []string `json:"blocked_by"`
	// DependentCount is the number of open issues with a blocking dependency on
	// this one.
	DependentCount int  `json:"dependent_count"`
	Ready          bool `json:"ready"`
	// Blocked is true for an open issue with status blocked or an active blocker.
	Blocked bool `json:"blocked"`
	// TriageScore is absent until triage is computed (large tier before phase 2).
	TriageScore *float64 `json:"triage_score,omitempty"`
	QuickWin    bool     `json:"quick_win,omitempty"`
	IsBlocker   bool     `json:"is_blocker,omitempty"`
	// Signoff is "pending", "approved", "disputed" or "waived" for beads in the
	// sign-off flow. The client's "Active" state is: not closed, or sign-off
	// pending or disputed.
	Signoff string `json:"signoff,omitempty"`
}

// ListTree is the list's tree as adjacency, so the payload stays O(items+edges).
// An OPEN blocked issue is listed as a child of EACH of its in-list active
// blockers (kind "block"), not under its parent (a closed issue is not blocked
// and stays under its parent); the client renders the same id under
// several parents, lazily, and stops at an id already on the current path.
type ListTree struct {
	Roots    []string              `json:"roots"`
	Children map[string][]TreeEdge `json:"children"`
}

// TreeEdge is one child of a tree node. Kind is "child" (parent-child) or
// "block" (the node blocks the child).
type TreeEdge struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// SearchResponse is GET /api/v1/search?q=. Hits are every match in rank order.
type SearchResponse struct {
	Query string      `json:"query"`
	Total int         `json:"total"`
	Hits  []SearchHit `json:"hits"`
}

// SearchHit is one match: the issue id and its fuzzy score (higher is better).
type SearchHit struct {
	ID    string `json:"id"`
	Score int    `json:"score"`
}
