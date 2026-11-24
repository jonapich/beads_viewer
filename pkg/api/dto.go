package api

import (
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
)

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

	// Generation is the store generation: 1 at startup, plus one per published
	// reload. A phase 2 swap does not change it.
	Generation uint64 `json:"generation"`
	// LoadedAt is when the served snapshot was built.
	LoadedAt time.Time `json:"loaded_at"`
	// ReloadError is set while the last reload failed; the data above is then
	// the last good load.
	ReloadError   string     `json:"reload_error,omitempty"`
	ReloadErrorAt *time.Time `json:"reload_error_at,omitempty"`
}

// SnapshotEvent is the data of a GET /api/v1/events "snapshot" message. It is a
// signal: the client refetches what it shows.
type SnapshotEvent struct {
	Generation uint64 `json:"generation"`
	Hash       string `json:"hash"`
	// Phase is "phase1", "phase2" or "phase2_error".
	Phase string `json:"phase"`
	// ReloadError is set while the last reload failed.
	ReloadError string `json:"reload_error,omitempty"`
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
	// Writes is true when the write endpoints (POST/PUT/DELETE under
	// /api/v1/issues/{id}/) can reach the tracker: the first issue's origin is a
	// complete, writable tracker route. IssueDetail.read_only_reason says why one
	// issue is not.
	Writes bool `json:"writes"`
	// Events is true while the server watches the tracker and serves
	// GET /api/v1/events.
	Events bool `json:"events"`
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

// IssueRef points at another issue with just enough to render a row: id, title,
// status glyph and priority. Missing is true (and the rest absent) when the id
// is not loaded: a dangling dependency, or an issue outside the loaded set.
type IssueRef struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status,omitempty"`
	Priority  *int   `json:"priority,omitempty"` // a pointer: P0 must not be dropped
	IssueType string `json:"issue_type,omitempty"`
	Missing   bool   `json:"missing,omitempty"`
}

// DepRef is an IssueRef in a blocking relation. For blocked_by, Active is true
// while the blocker still blocks (it is not closed); for blocks, true while the
// dependent is still blocked by this issue.
type DepRef struct {
	IssueRef
	Active bool `json:"active"`
}

// RelatedRef is a non-blocking, non-parent link (related, discovered-from, ...).
// Direction "out" means this issue declared it, "in" that the other issue did.
type RelatedRef struct {
	IssueRef
	Type      string `json:"type"`
	Direction string `json:"direction"`
}

// BlockerCountsDTO counts the distinct blocking dependencies of an issue.
type BlockerCountsDTO struct {
	Active   int `json:"active"`
	Resolved int `json:"resolved"`
}

// DetailTriage is the issue's triage data; absent when triage was not computed.
type DetailTriage struct {
	Score       float64                 `json:"score"`
	Reasons     *analysis.TriageReasons `json:"reasons,omitempty"`
	QuickWin    bool                    `json:"quick_win"`
	IsBlocker   bool                    `json:"is_blocker"`
	UnblocksIDs []string                `json:"unblocks_ids"`
}

// DetailGraph is the issue's graph metrics; present only once Phase 2 is done
// (and never on the huge tier, which skips them).
type DetailGraph struct {
	PageRank    float64 `json:"pagerank"`
	Betweenness float64 `json:"betweenness"`
	Eigenvector float64 `json:"eigenvector"`
	Hub         float64 `json:"hub"`
	Authority   float64 `json:"authority"`
	// CriticalPathScore is the analysis critical-path score: higher means more
	// work is chained behind this issue.
	CriticalPathScore float64 `json:"critical_path_score"`
}

// DecisionInfo is the data behind the "Needs your decision" card of an open bead
// labelled needs-decision or needs-design (the agent-desk inbox rules).
type DecisionInfo struct {
	// Label is the awaiting label an answer removes: needs-decision when
	// present, else needs-design.
	Label string `json:"label"`
	// ReadyToJudge is true when a mock or artifact link is attached.
	ReadyToJudge bool `json:"ready_to_judge"`
	// WaitingOn names the artifact a needs-design bead still lacks.
	WaitingOn string `json:"waiting_on,omitempty"`
	// Question is the description's "Question: ...?" sentence; absent when it
	// has none (the client falls back to the title).
	Question string `json:"question,omitempty"`
	// Options are parsed from the description; empty when it offers none, in
	// which case an answer needs free text.
	Options []agentdesk.Option `json:"options"`
}

// CommentDTO is one comment, with its bd kind header split off. Text is the
// body after the header. Collapsible comments (kinded, or with a JSON object
// body) render as a one-line Summary that expands; the client keeps signoff,
// user, answer and plain comments expanded. Data is the JSON object body, if any.
type CommentDTO struct {
	ID          string         `json:"id"`
	Author      string         `json:"author"`
	CreatedAt   time.Time      `json:"created_at"`
	Kind        string         `json:"kind,omitempty"`
	Text        string         `json:"text"`
	Summary     string         `json:"summary,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
	Collapsible bool           `json:"collapsible"`
}

// IssueDetail is GET /api/v1/issues/{id}: the whole issue under the model's JSON
// names, its relations, and what the detail view's cards need.
type IssueDetail struct {
	Phase2Ready bool `json:"phase2_ready"`

	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	Design             string     `json:"design,omitempty"`
	AcceptanceCriteria string     `json:"acceptance_criteria,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	Status             string     `json:"status"`
	Priority           int        `json:"priority"`
	IssueType          string     `json:"issue_type"`
	Assignee           string     `json:"assignee,omitempty"`
	EstimatedMinutes   *int       `json:"estimated_minutes,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	CreatedBy          string     `json:"created_by,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DueDate            *time.Time `json:"due_date,omitempty"`
	DeferUntil         *time.Time `json:"defer_until,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
	CloseReason        string     `json:"close_reason,omitempty"`
	ExternalRef        string     `json:"external_ref,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	LeaseExpiresAt     *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt        *time.Time `json:"heartbeat_at,omitempty"`
	Labels             []string   `json:"labels"`
	SourceRepo         string     `json:"source_repo,omitempty"`

	// Metadata is the decoded metadata object (bd keeps the pipeline stage,
	// stage_at.<stage> and signoff_at timestamps, link.<kind> and claim data
	// there). The History timeline is built from it plus the dates above.
	Metadata map[string]any `json:"metadata,omitempty"`

	// Parent is the first loaded parent-child target. Children are the issues
	// that declare this one as their parent, in snapshot order.
	Parent   *IssueRef  `json:"parent,omitempty"`
	Children []IssueRef `json:"children"`
	// BlockedBy lists every distinct blocking dependency, active ones first.
	BlockedBy []DepRef `json:"blocked_by"`
	// Blocks lists the open issues with a blocking dependency on this one.
	Blocks  []DepRef     `json:"blocks"`
	Related []RelatedRef `json:"related"`

	BlockerCounts    BlockerCountsDTO `json:"blocker_counts"`
	ActiveDependents int              `json:"active_dependents"`
	Ready            bool             `json:"ready"`
	// Blocked is true for an open issue with status blocked or an active blocker.
	Blocked bool `json:"blocked"`

	Triage *DetailTriage `json:"triage,omitempty"`
	Graph  *DetailGraph  `json:"graph,omitempty"`

	// Signoff is present when the issue is in the sign-off flow (any state).
	Signoff  *SignoffDTO      `json:"signoff,omitempty"`
	Decision *DecisionInfo    `json:"decision,omitempty"`
	Links    []agentdesk.Link `json:"links"`
	Comments []CommentDTO     `json:"comments"`

	// ReadOnlyReason says why bv cannot write to this issue's tracker; absent
	// when it can.
	ReadOnlyReason string `json:"read_only_reason,omitempty"`
}

// SignoffCall is one "call made on your behalf": what was decided, what it was
// chosen over, and why. The sign-off card shows each as a row.
type SignoffCall struct {
	Call        string `json:"call"`
	Alternative string `json:"alternative,omitempty"`
	Why         string `json:"why,omitempty"`
}

// SignoffDTO is agentdesk.SignoffItem plus Calls. SignoffItem.CallsMade
// flattens structured calls into sorted "key: value" lines (the robot output
// depends on that shape, so pkg/agentdesk keeps it); Calls keeps the rows.
type SignoffDTO struct {
	agentdesk.SignoffItem
	Calls []SignoffCall `json:"calls"`
}

// SignoffGroupDTO is the pending sign-offs of one epic.
type SignoffGroupDTO struct {
	Epic      string       `json:"epic"`
	EpicTitle string       `json:"epic_title,omitempty"`
	Items     []SignoffDTO `json:"items"`
}

// SignoffResponse is GET /api/v1/signoff: the closed beads pending sign-off,
// grouped by epic, in agentdesk.BuildSignoffs order.
type SignoffResponse struct {
	Total  int               `json:"total"`
	Groups []SignoffGroupDTO `json:"groups"`
}

// TriageResponse is GET /api/v1/triage: close to robot-triage, but GUI-specific
// (no generated_at or compute time, which would change every response; no
// commands or per-recommendation actions, which carry local paths).
type TriageResponse struct {
	Phase2Ready bool `json:"phase2_ready"`
	// Phase2Error is set when Phase 2 failed but triage is still served from
	// phase 1 (small and medium tiers); phase2_ready then stays false.
	Phase2Error     string                 `json:"phase2_error,omitempty"`
	IssueCount      int                    `json:"issue_count"`
	QuickRef        analysis.QuickRef      `json:"quick_ref"`
	Recommendations []TriageRecommendation `json:"recommendations"`
	QuickWins       []analysis.QuickWin    `json:"quick_wins"`
	BlockersToClear []analysis.BlockerItem `json:"blockers_to_clear"`
	// ProjectHealth.counts.by_priority has integer keys, which JSON writes as
	// strings: Record<string, number> on the client.
	ProjectHealth analysis.ProjectHealth `json:"project_health"`
	Alerts        []analysis.Alert       `json:"alerts,omitempty"`
}

// TriageRecommendation is analysis.Recommendation without its Actions (bd argv
// and working directories) and without the breakdown's risk signals.
type TriageRecommendation struct {
	ID          string                  `json:"id"`
	Title       string                  `json:"title"`
	Type        string                  `json:"type"`
	Status      string                  `json:"status"`
	Assignee    string                  `json:"assignee,omitempty"`
	Priority    int                     `json:"priority"`
	Labels      []string                `json:"labels"`
	DeferUntil  *time.Time              `json:"defer_until,omitempty"`
	Score       float64                 `json:"score"`
	Breakdown   analysis.ScoreBreakdown `json:"breakdown"`
	Action      string                  `json:"action"`
	Reasons     []string                `json:"reasons"`
	UnblocksIDs []string                `json:"unblocks_ids"`
	BlockedBy   []string                `json:"blocked_by,omitempty"`
	Claimable   bool                    `json:"claimable"`
}

// InsightEntry is one row of an insight list: an issue and its metric value.
type InsightEntry struct {
	ID    string  `json:"id"`
	Title string  `json:"title,omitempty"`
	Value float64 `json:"value"`
}

// InsightsResponse is GET /api/v1/insights?limit=N. Every list is capped at N
// (default 25, at most 500). The metric lists may be empty or partial until
// phase2_ready (the small and medium tiers fill some at load) and stay empty
// on the huge tier (phase2_skipped). Skipped names the lists whose metric was
// skipped or timed out, with the reason; project velocity is in the triage
// response (project_health.velocity), not here.
type InsightsResponse struct {
	Phase2Ready    bool    `json:"phase2_ready"`
	Phase2Skipped  bool    `json:"phase2_skipped"`
	Phase2Error    string  `json:"phase2_error,omitempty"`
	ClusterDensity float64 `json:"cluster_density"`

	Bottlenecks  []InsightEntry `json:"bottlenecks"`
	Keystones    []InsightEntry `json:"keystones"`
	Influencers  []InsightEntry `json:"influencers"`
	Hubs         []InsightEntry `json:"hubs"`
	Authorities  []InsightEntry `json:"authorities"`
	Cores        []InsightEntry `json:"cores"`
	Slack        []InsightEntry `json:"slack"`
	Articulation []string       `json:"articulation"`
	Orphans      []string       `json:"orphans"`
	// OrphanCount and CycleCount are the totals before the cap.
	OrphanCount int        `json:"orphan_count"`
	Cycles      [][]string `json:"cycles"`
	CycleCount  int        `json:"cycle_count"`

	// Skipped maps a list name (bottlenecks, hubs, authorities, cycles,
	// keystones, slack, influencers, cores, articulation) to the reason its
	// metric was skipped or timed out. Always an object; filled once
	// phase2_ready.
	Skipped map[string]string `json:"skipped"`
}

// SignoffRequest is the body of POST /api/v1/issues/{id}/signoff. Approve is
// required. A dispute needs a Note (what is wrong or missing); an approval takes
// none.
type SignoffRequest struct {
	Approve *bool  `json:"approve"`
	Note    string `json:"note,omitempty"`
}

// AnswerRequest is the body of POST /api/v1/issues/{id}/answer: the option key
// picked (a, b, ...), free text, or both; at least one. The server composes the
// comment the same way the TUI does (agentdesk.DecisionComment) and removes the
// needs-decision / needs-design label.
type AnswerRequest struct {
	Option string `json:"option,omitempty"`
	Text   string `json:"text,omitempty"`
}

// PriorityRequest is the body of PUT /api/v1/issues/{id}/priority: 0 (P0) to 4
// (P4), required.
type PriorityRequest struct {
	Priority *int `json:"priority"`
}

// IssuePatchRequest is the body of PATCH /api/v1/issues/{id}: the fields of the
// TUI's O editor; an absent (or null) field stays as it is. Assignee "" clears
// the assignee. BaseUpdatedAt is the issue's updated_at as the client last read
// it, echoed verbatim (RFC 3339, nanoseconds included), and is required: a
// mismatch is a 409. The response is a WriteResponse.
type IssuePatchRequest struct {
	Title         *string `json:"title,omitempty"`
	Status        *string `json:"status,omitempty"`
	Priority      *int    `json:"priority,omitempty"`
	Assignee      *string `json:"assignee,omitempty"`
	Type          *string `json:"type,omitempty"`
	Description   *string `json:"description,omitempty"`
	BaseUpdatedAt string  `json:"base_updated_at"`
}

// WriteResponse is the 200 body of every write: PUT/DELETE
// /api/v1/issues/{id}/labels/close-candidate as well, and a no-op (the issue
// already is as asked) too. The data is already reloaded when it is sent, so the
// next GET is fresh; there is no issue DTO because the list carries derived
// state a client cannot patch in place.
type WriteResponse struct {
	ID string `json:"id"`
	// Generation is the store generation after the reload.
	Generation uint64 `json:"generation"`
	// DataHash is the data hash after the reload (as in MetaResponse).
	DataHash string `json:"data_hash"`
	// CreatedID is the follow-up bead a dispute filed.
	CreatedID string `json:"created_id,omitempty"`
	// Warning is set when the write succeeded but refreshing the export or
	// reloading failed: the list may lag until the next change. It is never an
	// error status, because the client would retry a write that took effect.
	Warning string `json:"warning,omitempty"`
}
