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
