// Package snapshot is the UI-free data pipeline shared by the TUI (pkg/ui) and
// the HTTP API (pkg/api): issues, ID map, counts, graph analysis, insights,
// triage scores and reasons, quick-win and blocker sets, the unblocks map, the
// parent/child tree, data hashes and two-phase readiness.
//
// A Snapshot is immutable after Builder.Build or Snapshot.WithPhase2: callers
// must not mutate its slices, maps or tree nodes, and must not call mutating
// Analyzer methods (SetWeights, SetNow, SetReadinessScope) on a published
// snapshot. Views that need to mutate derived structures deep-copy them first
// (CloneIssues, CloneTree).
//
// The package must stay free of terminal-UI dependencies: it never imports
// bubbletea, bubbles, lipgloss or pkg/ui, directly or transitively. A test
// enforces that.
package snapshot
