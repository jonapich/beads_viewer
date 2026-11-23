// Package ui provides the terminal user interface for beads_viewer.
// This file implements the DataSnapshot type for thread-safe UI rendering.
package ui

import (
	"context"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/drift"
	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
	"github.com/Dicklesworthstone/beads_viewer/pkg/search"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/charmbracelet/bubbles/list"
)

func isClosedLikeStatus(status model.Status) bool {
	return status == model.StatusClosed || status == model.StatusTombstone
}

type snapshotBuildConfig struct {
	PrecomputeTriage      bool
	PrecomputeTree        bool
	PrecomputeBoard       bool
	PrecomputeGraphLayout bool
	PrecomputeInsights    bool
	SkipPhase2            bool
}

func snapshotBuildConfigDefault() snapshotBuildConfig {
	return snapshotBuildConfig{
		PrecomputeTriage:      true,
		PrecomputeTree:        true,
		PrecomputeBoard:       true,
		PrecomputeGraphLayout: true,
		PrecomputeInsights:    true,
		SkipPhase2:            false,
	}
}

// core returns the UI-free part of the configuration.
func (c snapshotBuildConfig) core() snapshot.Config {
	return snapshot.Config{
		PrecomputeTriage:   c.PrecomputeTriage,
		PrecomputeTree:     c.PrecomputeTree,
		PrecomputeInsights: c.PrecomputeInsights,
		SkipPhase2:         c.SkipPhase2,
	}
}

func snapshotBuildConfigForTier(tier datasetTier) snapshotBuildConfig {
	core := snapshot.ConfigForTier(tier)
	// The UI needs the board and graph layout to install a snapshot. Preparing
	// them here keeps their O(N) construction off the event loop at 5k-20k rows;
	// only the huge tier skips them.
	precomputeUI := tier != datasetTierHuge
	return snapshotBuildConfig{
		PrecomputeTriage:      core.PrecomputeTriage,
		PrecomputeTree:        core.PrecomputeTree,
		PrecomputeBoard:       precomputeUI,
		PrecomputeGraphLayout: precomputeUI,
		PrecomputeInsights:    core.PrecomputeInsights,
		SkipPhase2:            core.SkipPhase2,
	}
}

const incrementalListMaxChangeRatio = 0.2

// IssueDiffStats summarizes the change volume between snapshots.
type IssueDiffStats struct {
	Changed int
	Total   int
	Ratio   float64
}

// pooledIssueLease gives Phase 1 and Phase 2 snapshots shared, one-shot
// ownership of pooled parser structs. A Phase 2 snapshot is a distinct object,
// so copying the raw pointer slice would otherwise let shutdown return the same
// objects to sync.Pool twice.
type pooledIssueLease struct {
	once     sync.Once
	released atomic.Bool
	refs     []*model.Issue
	release  func([]*model.Issue)
}

func newPooledIssueLease(refs []*model.Issue) *pooledIssueLease {
	if len(refs) == 0 {
		return nil
	}
	return &pooledIssueLease{
		refs:    refs,
		release: loader.ReturnIssuePtrsToPool,
	}
}

func (l *pooledIssueLease) releaseOnce() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		refs := l.refs
		l.refs = nil
		if len(refs) > 0 && l.release != nil {
			l.release(refs)
		}
		l.released.Store(true)
	})
}

func (l *pooledIssueLease) active() bool {
	return l != nil && !l.released.Load()
}

// DataSnapshot is an immutable, self-contained representation of all data
// the UI needs to render. Once created, it never changes - this is critical
// for thread safety when the background worker is building the next snapshot.
//
// The UI thread reads exclusively from its current snapshot pointer.
// When a new snapshot is ready, the UI swaps the pointer atomically.
type DataSnapshot struct {
	// Core data
	Issues   []model.Issue           // All issues (sorted)
	IssueMap map[string]*model.Issue // Lookup by ID
	// pooledIssues is shared by Phase 1/Phase 2 snapshots so parser refs are
	// returned exactly once even though both snapshot objects can outlive a swap.
	pooledIssues *pooledIssueLease
	// ViewIssues are the issues included in the current view context (e.g. recipe).
	// When empty, callers should fall back to Issues.
	ViewIssues []model.Issue

	// Graph analysis
	Analyzer *analysis.Analyzer
	Analysis *analysis.GraphStats
	insights analysis.Insights // unexported for immutability; use GetInsights()

	// Computed statistics
	CountOpen    int
	CountReady   int
	CountBlocked int
	CountClosed  int

	// Pre-computed UI data. The unexported adapter/cache fields let the UI install
	// the common unfiltered view without rebuilding interface slices, search
	// documents, alert state, or selection indexes on the event loop.
	ListItems      []IssueItem // Pre-built list items with scores
	listModelItems []list.Item
	listIndexByID  map[string]int
	listOrderHash  uint64
	semanticIDs    []string
	semanticDocs   map[string]string
	alerts         []drift.Alert
	alertsCritical int
	alertsWarning  int
	alertsInfo     int
	phase2Triage   *analysis.TriageResult
	// Detached mutable inputs are prepared before publication, so starting a
	// Phase2 command does not deep-copy every view on the event loop.
	phase2Input   *DataSnapshot
	TriageScores  map[string]float64
	TriageReasons map[string]analysis.TriageReasons
	QuickWinSet   map[string]bool
	BlockerSet    map[string]bool
	UnblocksMap   map[string][]string
	// TreeRoots and TreeNodeMap contain a pre-built parent/child tree for the Tree view.
	// These are computed off-thread by SnapshotBuilder to avoid UI-thread work when
	// entering the tree view for large datasets.
	TreeRoots   []*IssueTreeNode
	TreeNodeMap map[string]*IssueTreeNode
	// BoardState contains pre-built Kanban board columns for each swimlane mode (bv-guxz).
	BoardState *BoardState
	// graphLayout contains pre-built graph view data (blockers/dependents, sorted IDs, ranks)
	// to avoid rebuilding graph structures on the UI thread (bv-za8z).
	// Unexported for immutability; use GetGraphLayout().
	graphLayout *GraphLayout

	// Metadata
	CreatedAt         time.Time      // When this snapshot was built
	DataHash          string         // Hash of source data for cache validation
	AuthorityHash     string         // Full source identity, including hidden dependency records.
	AuthorityComplete bool           // True when source authority is complete and warning-free.
	RecipeName        string         // Active recipe name for this snapshot (bv-2h40)
	RecipeHash        string         // Fingerprint of active recipe for this snapshot (bv-4ilb)
	recipe            *recipe.Recipe // Retained to re-sort ViewIssues/ListItems when Phase 2 completes
	// DatasetTier is a tiered performance mode for large datasets (bv-9thm).
	// When unknown, normal behavior applies.
	DatasetTier datasetTier
	// SourceIssueCountHint is an approximate total issue count from the source file
	// (e.g., JSONL line count). This may be 0 if unavailable.
	SourceIssueCountHint int
	// LoadedOpenOnly indicates the snapshot intentionally excluded closed/tombstone
	// issues for performance (huge tier).
	LoadedOpenOnly bool
	// TruncatedCount is an approximate count of issues excluded by load policy.
	// This may include invalid/empty lines when computed from a line count hint.
	TruncatedCount int
	// LargeDatasetWarning is a short, user-facing warning to show in the footer.
	LargeDatasetWarning string
	// LoadWarningCount is the number of non-fatal parse warnings encountered while loading.
	// In TUI mode, warnings must not be printed to stderr during render.
	LoadWarningCount int

	// Phase 2 analysis status
	// phase2Ready is true when expensive metrics (PageRank, Betweenness, etc.) are computed.
	// UI can render immediately with Phase 1 data, then refresh when Phase 2 completes.
	// Unexported for immutability; use IsPhase2Ready().
	phase2Ready bool
	// referenceTime is the analyzer instant the core used for readiness and
	// triage. It is stored (not read from Analyzer) so adapters never touch the
	// shared analyzer from the Phase 2 goroutine.
	referenceTime time.Time

	// Incremental update metadata (bv-5mzz).
	IssueDiff      *analysis.IssueDiff
	IssueDiffStats IssueDiffStats
	// IncrementalListUsed reports whether list items were rebuilt incrementally.
	IncrementalListUsed bool

	// Error state (for graceful degradation)
	LoadError    error     // Non-nil if last load had recoverable errors
	ErrorTime    time.Time // When error occurred
	StaleWarning bool      // True if data is from previous successful load

	beadsPath          string
	projectDir         string
	explicitProjectDir string
	explicitWeights    bool
}

func (s *DataSnapshot) attachPooledIssues(refs []*model.Issue) {
	if s == nil {
		return
	}
	s.pooledIssues = newPooledIssueLease(refs)
}

func (s *DataSnapshot) releasePooledIssues() {
	if s == nil {
		return
	}
	s.pooledIssues.releaseOnce()
}

func (s *DataSnapshot) hasPooledIssues() bool {
	return s != nil && s.pooledIssues.active()
}

// IsPhase2Ready returns whether expensive Phase 2 metrics are computed.
func (s *DataSnapshot) IsPhase2Ready() bool {
	if s == nil {
		return false
	}
	return s.phase2Ready
}

// GetInsights returns the precomputed insights for this snapshot.
func (s *DataSnapshot) GetInsights() analysis.Insights {
	if s == nil {
		return analysis.Insights{}
	}
	return s.insights
}

// GetGraphLayout returns the precomputed graph layout for this snapshot.
func (s *DataSnapshot) GetGraphLayout() *GraphLayout {
	if s == nil {
		return nil
	}
	return s.graphLayout
}

// GraphLayout contains precomputed data used by the graph view.
// This intentionally focuses on the current ASCII graph view needs (relationships + ranks),
// not geometric coordinates.
type GraphLayout struct {
	// Relationships (blocks/dependents)
	Blockers   map[string][]string // What each issue depends on (blocks this issue)
	Dependents map[string][]string // What depends on each issue (this issue blocks)

	// Navigation order (all IDs in the snapshot)
	SortedIDs []string

	// One visible dependency chain, independent of project-wide metric ranks.
	// Prepared with the relationships so UI delivery only installs these maps.
	CriticalPath map[string]bool
	CriticalNext map[string]string

	// Metric ranks (1 = best, higher = worse). Missing ranks imply "unknown".
	RankPageRank     map[string]int
	RankBetweenness  map[string]int
	RankEigenvector  map[string]int
	RankHubs         map[string]int
	RankAuthorities  map[string]int
	RankCriticalPath map[string]int
	RankInDegree     map[string]int
	RankOutDegree    map[string]int
}

// BoardState contains precomputed Kanban columns for each swimlane mode.
// This lets the UI swap board data in O(1) when the full dataset is shown.
type BoardState struct {
	ByStatus     [4][]model.Issue
	ByPriority   [4][]model.Issue
	ByType       [4][]model.Issue
	BlocksIndex  map[string][]string
	ReferenceNow time.Time
}

func (s *BoardState) ColumnsForMode(mode SwimLaneMode) [4][]model.Issue {
	if s == nil {
		return [4][]model.Issue{}
	}
	switch mode {
	case SwimByPriority:
		return s.ByPriority
	case SwimByType:
		return s.ByType
	default:
		return s.ByStatus
	}
}

func (l *GraphLayout) UpdatePhase2Ranks(stats *analysis.GraphStats) {
	if l == nil || stats == nil {
		return
	}

	// Phase 2 ranks may become available later (AnalyzeAsync).
	l.RankPageRank = stats.PageRankRank()
	l.RankBetweenness = stats.BetweennessRank()
	l.RankEigenvector = stats.EigenvectorRank()
	l.RankHubs = stats.HubsRank()
	l.RankAuthorities = stats.AuthoritiesRank()
	l.RankCriticalPath = stats.CriticalPathRank()

	// Rebuild SortedIDs using the new critical-path ranking, preserving determinism.
	l.SortedIDs = orderIssueIDsByRank(l.SortedIDs, l.RankCriticalPath)
}

// SnapshotBuilder constructs DataSnapshots from raw data.
// This is used by the BackgroundWorker to build new snapshots.
//
// It decorates a snapshot.Builder: the UI-free core (sorting, analysis, counts,
// triage, recipe view, tree, insights) runs in pkg/snapshot, shared with the
// HTTP API, and this type adds the TUI-only parts (list items, board, graph
// layout, semantic documents, drift alerts).
type SnapshotBuilder struct {
	core *snapshot.Builder
	// analyzer is core.Analyzer(). Tests and Phase 2 preparation configure it
	// directly (SetNow, RestoreScoring, SetReadinessScope).
	analyzer *analysis.Analyzer
	// analysis and recipe mirror what was passed to core: the list-item builder
	// needs to know whether WithAnalysis was called and which recipe is active.
	analysis *analysis.GraphStats
	recipe   *recipe.Recipe
	// cfg is read at Build time, because callers adjust it after construction.
	cfg snapshotBuildConfig

	prevSnapshot *DataSnapshot
	diff         *analysis.IssueDiff
	diffStats    IssueDiffStats
}

// NewSnapshotBuilder creates a builder for constructing a DataSnapshot.
func NewSnapshotBuilder(issues []model.Issue, authority ...*model.ReadinessIndex) *SnapshotBuilder {
	var readiness *model.ReadinessIndex
	if len(authority) > 0 {
		readiness = authority[0]
	}
	core := snapshot.NewBuilder(issues, readiness)
	return &SnapshotBuilder{
		core:     core,
		analyzer: core.Analyzer(),
		cfg:      snapshotBuildConfigDefault(),
	}
}

// WithWeights installs feedback-adjusted factor weights on the builder's
// analyzer so triage and priority hints computed from this snapshot use them.
// nil leaves the defaults in place.
func (b *SnapshotBuilder) WithWeights(w *analysis.Weights) *SnapshotBuilder {
	b.core.WithWeights(w)
	return b
}

// WithBeadsPath associates a .beads/<file>.jsonl path with this snapshot build
// so baseline drift alerts and .beads/feedback.json weights resolve against the
// target workspace even when the process CWD is a subdirectory.
func (b *SnapshotBuilder) WithBeadsPath(beadsPath string) *SnapshotBuilder {
	b.core.WithBeadsPath(beadsPath)
	return b
}

// WithProjectDir associates a project root directory with this snapshot build.
func (b *SnapshotBuilder) WithProjectDir(projectDir string) *SnapshotBuilder {
	b.core.WithProjectDir(projectDir)
	return b
}

func (s *DataSnapshot) resolvedBeadsDir() string {
	if s == nil {
		return ""
	}
	if strings.TrimSpace(s.beadsPath) != "" {
		return filepath.Dir(s.beadsPath)
	}
	if strings.TrimSpace(s.projectDir) != "" {
		return filepath.Join(s.projectDir, ".beads")
	}
	return ""
}

// feedbackWeightsForBeadsDir loads feedback.json from beadsDir and returns the
// adjusted factor weights when enough accept/ignore samples exist to apply them;
// nil otherwise.
func feedbackWeightsForBeadsDir(beadsDir string) *analysis.Weights {
	return snapshot.FeedbackWeights(beadsDir)
}

// feedbackWeightsForBeadsPath loads .beads/feedback.json next to the issue
// file and returns the adjusted factor weights when enough accept/ignore
// samples exist to apply them (analysis.MinFeedbackSamples); nil otherwise.
// This is the TUI counterpart of the robot path's loadRobotFeedback, so the
// priority hints (p) and the actionable view rank issues the same way
// --robot-triage does.
func feedbackWeightsForBeadsPath(beadsPath string) *analysis.Weights {
	return snapshot.FeedbackWeightsForBeadsPath(beadsPath)
}

// WithContext binds Phase 2 async analysis to the caller's lifecycle context.
func (b *SnapshotBuilder) WithContext(ctx context.Context) *SnapshotBuilder {
	b.core.WithContext(ctx)
	return b
}

// WithAnalysis sets the pre-computed analysis (for when we have cached results).
func (b *SnapshotBuilder) WithAnalysis(a *analysis.GraphStats) *SnapshotBuilder {
	b.analysis = a
	b.core.WithAnalysis(a)
	return b
}

func (b *SnapshotBuilder) WithRecipe(r *recipe.Recipe) *SnapshotBuilder {
	b.recipe = r
	b.core.WithRecipe(r)
	return b
}

func (b *SnapshotBuilder) WithBuildConfig(cfg snapshotBuildConfig) *SnapshotBuilder {
	b.cfg = cfg
	return b
}

// WithPreviousSnapshot enables incremental list-item rebuilds when possible.
func (b *SnapshotBuilder) WithPreviousSnapshot(prev *DataSnapshot, diff *analysis.IssueDiff) *SnapshotBuilder {
	b.prevSnapshot = prev
	b.diff = diff
	b.diffStats = issueDiffStats(diff)
	return b
}

// Build constructs the final immutable DataSnapshot.
// This performs all necessary computations that should happen in the background.
// Uses AnalyzeAsync() so Phase 2 metrics compute in background - check Phase2Ready
// or call GetGraphStats().WaitForPhase2() if you need Phase 2 data immediately.
func (b *SnapshotBuilder) Build() *DataSnapshot {
	c := b.core.WithConfig(b.cfg.core()).Build()
	issues, viewIssues, graphStats, now := c.Issues, c.ViewIssues, c.Analysis, c.ReferenceTime

	// Build list items with graph scores (respecting recipe filtering/sorting when present).
	listItemsIncremental := false
	statsForListItems := graphStats
	// If analysis was computed asynchronously for this snapshot build, treat Phase 2 scores
	// as not-yet-available to keep list items deterministic (they will be refreshed when
	// Phase 2 completes via Phase2ReadyMsg).
	if b.analysis == nil {
		statsForListItems = nil
	}
	var listItems []IssueItem
	if shouldUseIncrementalList(b.prevSnapshot, b.diff, b.recipe, b.diffStats, viewIssues) {
		listItems = buildListItemsIncremental(viewIssues, statsForListItems, b.prevSnapshot, b.diff)
		listItemsIncremental = true
	} else {
		listItems = buildListItems(viewIssues, statsForListItems)
	}

	// Populate list items with triage insights (may be skipped for large/huge datasets; bv-9thm).
	// The core sets Triage exactly when it computed the triage maps.
	if c.Triage != nil {
		for i := range listItems {
			id := listItems[i].Issue.ID
			listItems[i].TriageScore = c.TriageScores[id]
			if reasons, exists := c.TriageReasons[id]; exists {
				listItems[i].TriageReason = reasons.Primary
				listItems[i].TriageReasons = reasons.All
			}
			listItems[i].IsQuickWin = c.QuickWinSet[id]
			listItems[i].IsBlocker = c.BlockerSet[id]
			listItems[i].UnblocksCount = len(c.UnblocksMap[id])
		}
	}

	var boardState *BoardState
	if b.cfg.PrecomputeBoard {
		boardState = buildBoardStateAt(issues, now)
	}

	var graphLayout *GraphLayout
	if b.cfg.PrecomputeGraphLayout {
		graphLayout = buildGraphLayout(issues, graphStats)
	}

	listModelItems := make([]list.Item, len(listItems))
	listIndexByID := make(map[string]int, len(listItems))
	for i := range listItems {
		item := listItems[i]
		listModelItems[i] = item
		id := item.Issue.ID
		listIndexByID[id] = i
	}
	semanticIDs, semanticDocs := buildSnapshotSearchDocuments(listItems, b.prevSnapshot, b.diff)

	alerts, alertsCritical, alertsWarning, alertsInfo := computeAlertsForDir(issues, graphStats, b.analyzer, c.ProjectDir)

	ds := dataSnapshotFromCore(c)
	ds.ListItems = listItems
	ds.listModelItems = listModelItems
	ds.listIndexByID = listIndexByID
	ds.listOrderHash = listOrderFingerprint(listItems)
	ds.semanticIDs = semanticIDs
	ds.semanticDocs = semanticDocs
	ds.alerts = alerts
	ds.alertsCritical = alertsCritical
	ds.alertsWarning = alertsWarning
	ds.alertsInfo = alertsInfo
	ds.BoardState = boardState
	ds.graphLayout = graphLayout
	ds.IssueDiff = b.diff
	ds.IssueDiffStats = IssueDiffStats{
		Changed: b.diffStats.Changed,
		Total:   b.diffStats.Total,
		Ratio:   b.diffStats.Ratio,
	}
	ds.IncrementalListUsed = listItemsIncremental
	// Sample readiness last: Phase 2 may complete while the UI-only parts above
	// are built, and the flag has always reflected the end of Build.
	ds.phase2Ready = graphStats.IsPhase2Ready()
	ds.phase2Input = ds.detachedPhase2Input()
	return ds
}

// buildSnapshotSearchDocuments owns its ID slice and map. Document strings are
// immutable and can be shared only when the complete source diff says the issue
// is unchanged. Iterating the current list keeps removed and filtered rows out.
func buildSnapshotSearchDocuments(items []IssueItem, previous *DataSnapshot, diff *analysis.IssueDiff) ([]string, map[string]string) {
	var unchanged map[string]bool
	if previous != nil && diff != nil && !diff.HasDuplicateIDs {
		unchanged = make(map[string]bool, len(diff.Unchanged))
		for _, id := range diff.Unchanged {
			unchanged[id] = true
		}
	}
	ids := make([]string, len(items))
	docs := make(map[string]string, len(items))
	for i, item := range items {
		id := item.Issue.ID
		ids[i] = id
		// Canonical fingerprints treat labels as a set; search text preserves
		// their order. Require that order too before sharing the old string.
		if unchanged[id] && previous.IssueMap[id] != nil && slices.Equal(previous.IssueMap[id].Labels, item.Issue.Labels) {
			if document, exists := previous.semanticDocs[id]; exists {
				docs[id] = document
				continue
			}
		}
		docs[id] = search.IssueDocument(item.Issue)
	}
	return ids, docs
}

func listOrderFingerprint(items []IssueItem) uint64 {
	const (
		offset64 = uint64(14695981039346656037)
		prime64  = uint64(1099511628211)
	)
	hash := offset64
	for i := range items {
		id := items[i].Issue.ID
		for j := 0; j < len(id); j++ {
			hash ^= uint64(id[j])
			hash *= prime64
		}
		hash ^= 0xff
		hash *= prime64
	}
	return hash
}

func issueDiffStats(diff *analysis.IssueDiff) IssueDiffStats {
	if diff == nil {
		return IssueDiffStats{}
	}
	changed := len(diff.Added) + len(diff.Removed) + len(diff.Modified)
	total := changed + len(diff.Unchanged)
	ratio := 0.0
	if total > 0 {
		ratio = float64(changed) / float64(total)
	}
	return IssueDiffStats{
		Changed: changed,
		Total:   total,
		Ratio:   ratio,
	}
}

func shouldUseIncrementalList(prev *DataSnapshot, diff *analysis.IssueDiff, r *recipe.Recipe, stats IssueDiffStats, currentIssues []model.Issue) bool {
	if prev == nil || diff == nil || len(prev.ListItems) == 0 {
		return false
	}
	if diff.HasDuplicateIDs {
		return false
	}
	// Topology changes can alter graph-derived scores for otherwise unchanged
	// issues, so reusing their old list items would be incorrect. Additions and
	// removals also change pagination and may shift recipe membership.
	if len(diff.Added) > 0 || len(diff.Removed) > 0 || len(diff.DependencyChanged) > 0 {
		return false
	}

	currentRecipeName := ""
	currentRecipeHash := ""
	if r != nil {
		currentRecipeName = r.Name
		currentRecipeHash = recipeFingerprint(r)
	}

	if prev.RecipeName != currentRecipeName || prev.RecipeHash != currentRecipeHash {
		return false
	}
	if len(currentIssues) != len(prev.ListItems) {
		return false
	}
	for i := range currentIssues {
		if currentIssues[i].ID != prev.ListItems[i].Issue.ID {
			return false
		}
	}
	if stats.Total == 0 {
		return false
	}
	return stats.Ratio <= incrementalListMaxChangeRatio
}

func buildListItems(issues []model.Issue, stats *analysis.GraphStats) []IssueItem {
	listItems := make([]IssueItem, len(issues))
	for i := range issues {
		resetIssueItemForSnapshot(&listItems[i], issues[i], stats)
	}
	return listItems
}

func buildListItemsIncremental(issues []model.Issue, stats *analysis.GraphStats, prev *DataSnapshot, diff *analysis.IssueDiff) []IssueItem {
	if prev == nil || len(prev.ListItems) != len(issues) || diff == nil {
		return buildListItems(issues, stats)
	}

	listItems := make([]IssueItem, len(issues))
	prevItems := prev.ListItems[:len(issues)]
	for i := range issues {
		// Fingerprints canonicalize collection order. Keep derived metrics, but
		// publish the current source order for labels, comments and dependencies.
		// New rows already have zero transient presentation state, so copying
		// the previous issue and transient fields would only overwrite them again.
		dst := &listItems[i]
		prevItem := &prevItems[i]
		dst.Issue = issues[i]
		dst.GraphScore = prevItem.GraphScore
		dst.Impact = prevItem.Impact
		dst.RepoPrefix = prevItem.RepoPrefix
	}
	for _, id := range diff.Modified {
		index, ok := prev.listIndexByID[id]
		if !ok || index < 0 || index >= len(issues) || issues[index].ID != id {
			return buildListItems(issues, stats)
		}
		listItems[index] = buildIssueItemForSnapshot(issues[index], stats)
	}
	return listItems
}

func buildIssueItemForSnapshot(issue model.Issue, stats *analysis.GraphStats) IssueItem {
	item := IssueItem{}
	resetIssueItemForSnapshot(&item, issue, stats)
	return item
}

func resetIssueItemForSnapshot(item *IssueItem, issue model.Issue, stats *analysis.GraphStats) {
	item.Issue = issue
	if stats != nil {
		item.GraphScore = stats.GetPageRankScore(issue.ID)
		item.Impact = stats.GetCriticalPathScore(issue.ID)
	} else {
		item.GraphScore = 0
		item.Impact = 0
	}
	item.RepoPrefix = issueRepoKey(issue)
	clearIssueItemEphemeral(item)
}

func clearIssueItemEphemeral(item *IssueItem) {
	item.DiffStatus = DiffStatusNone

	item.SearchScore = 0
	item.SearchTextScore = 0
	item.SearchComponents = nil
	item.SearchScoreSet = false

	item.TriageScore = 0
	item.TriageReason = ""
	item.TriageReasons = nil
	item.IsQuickWin = false
	item.IsBlocker = false
	item.UnblocksCount = 0
}

func buildGraphLayout(issues []model.Issue, stats *analysis.GraphStats) *GraphLayout {
	size := len(issues)
	ids := make([]string, 0, size)
	blockers := make(map[string][]string, size)
	dependents := make(map[string][]string, size)

	for i := range issues {
		issue := &issues[i]
		ids = append(ids, issue.ID)

		for _, dep := range issue.Dependencies {
			if dep == nil || !dep.Type.IsBlocking() {
				continue
			}
			blockers[issue.ID] = append(blockers[issue.ID], dep.DependsOnID)
			dependents[dep.DependsOnID] = append(dependents[dep.DependsOnID], issue.ID)
		}
	}

	layout := &GraphLayout{
		Blockers:   blockers,
		Dependents: dependents,
	}

	if stats != nil {
		layout.RankInDegree = stats.InDegreeRank()
		layout.RankOutDegree = stats.OutDegreeRank()
		layout.RankPageRank = stats.PageRankRank()
		layout.RankBetweenness = stats.BetweennessRank()
		layout.RankEigenvector = stats.EigenvectorRank()
		layout.RankHubs = stats.HubsRank()
		layout.RankAuthorities = stats.AuthoritiesRank()
		layout.RankCriticalPath = stats.CriticalPathRank()
	}

	layout.SortedIDs = orderIssueIDsByRank(ids, layout.RankCriticalPath)
	layout.CriticalPath, layout.CriticalNext = visibleCriticalChain(ids, blockers, dependents)
	return layout
}

func buildBoardState(issues []model.Issue) *BoardState {
	return buildBoardStateAt(issues, time.Time{})
}

func buildBoardStateAt(issues []model.Issue, now time.Time) *BoardState {
	if len(issues) == 0 {
		return nil
	}
	return &BoardState{
		ByStatus:     groupIssuesByMode(issues, SwimByStatus),
		ByPriority:   groupIssuesByMode(issues, SwimByPriority),
		ByType:       groupIssuesByMode(issues, SwimByType),
		BlocksIndex:  buildBlocksIndex(issues),
		ReferenceNow: now,
	}
}

func orderIssueIDsByRank(ids []string, ranks map[string]int) []string {
	if len(ids) == 0 {
		return nil
	}

	// If we have a rank map, rebuild in O(n) without sorting (ranks are 1..N).
	if len(ranks) > 0 {
		ordered := make([]string, len(ids))
		var missing []string

		for _, id := range ids {
			rank := ranks[id]
			if rank < 1 || rank > len(ordered) || ordered[rank-1] != "" {
				missing = append(missing, id)
				continue
			}
			ordered[rank-1] = id
		}

		// Compact any gaps then append missing IDs deterministically.
		out := make([]string, 0, len(ids))
		for _, id := range ordered {
			if id != "" {
				out = append(out, id)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			out = append(out, missing...)
		}
		if len(out) > 0 {
			return out
		}
	}

	// Fallback: stable alphabetical ordering.
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return sorted
}

// GetGraphStats returns the GraphStats pointer for Phase 2 waiting.
// Callers can use stats.WaitForPhase2() to block until Phase 2 completes.
func (s *DataSnapshot) GetGraphStats() *analysis.GraphStats {
	if s == nil {
		return nil
	}
	return s.Analysis
}

// IsEmpty returns true if the snapshot has no issues.
func (s *DataSnapshot) IsEmpty() bool {
	return s == nil || len(s.Issues) == 0
}

// GetIssue returns an issue by ID, or nil if not found.
func (s *DataSnapshot) GetIssue(id string) *model.Issue {
	if s == nil || s.IssueMap == nil {
		return nil
	}
	return s.IssueMap[id]
}

// Age returns how long ago this snapshot was created.
func (s *DataSnapshot) Age() time.Duration {
	if s == nil {
		return 0
	}
	return time.Since(s.CreatedAt)
}

// deepCopyListItems creates a deep copy of a ListItems slice.
// Each IssueItem contains mutable issue backing state plus mutable adapter
// fields (SearchComponents map, TriageReasons slice) that must be copied to
// prevent race conditions between snapshots.
func deepCopyListItems(items []IssueItem) []IssueItem {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]IssueItem, len(items))
	for i := range items {
		cloned[i] = items[i]
		cloned[i].Issue = items[i].Issue.Clone()
		// Deep copy the mutable SearchComponents map
		if len(items[i].SearchComponents) > 0 {
			cloned[i].SearchComponents = make(map[string]float64, len(items[i].SearchComponents))
			for k, v := range items[i].SearchComponents {
				cloned[i].SearchComponents[k] = v
			}
		}
		// Deep copy the mutable TriageReasons slice
		if len(items[i].TriageReasons) > 0 {
			cloned[i].TriageReasons = make([]string, len(items[i].TriageReasons))
			copy(cloned[i].TriageReasons, items[i].TriageReasons)
		}
	}
	return cloned
}

// deepCopyBoardState creates a deep copy of a BoardState.
// BoardState contains [4][]model.Issue arrays which are mutable slices
// that must be copied to prevent race conditions between snapshots.
func deepCopyBoardState(bs *BoardState) *BoardState {
	if bs == nil {
		return nil
	}
	cloned := &BoardState{
		ReferenceNow: bs.ReferenceNow,
	}
	for i := 0; i < 4; i++ {
		if len(bs.ByStatus[i]) > 0 {
			cloned.ByStatus[i] = make([]model.Issue, len(bs.ByStatus[i]))
			for j := range bs.ByStatus[i] {
				cloned.ByStatus[i][j] = bs.ByStatus[i][j].Clone()
			}
		}
		if len(bs.ByPriority[i]) > 0 {
			cloned.ByPriority[i] = make([]model.Issue, len(bs.ByPriority[i]))
			for j := range bs.ByPriority[i] {
				cloned.ByPriority[i][j] = bs.ByPriority[i][j].Clone()
			}
		}
		if len(bs.ByType[i]) > 0 {
			cloned.ByType[i] = make([]model.Issue, len(bs.ByType[i]))
			for j := range bs.ByType[i] {
				cloned.ByType[i][j] = bs.ByType[i][j].Clone()
			}
		}
	}
	if bs.BlocksIndex != nil {
		cloned.BlocksIndex = make(map[string][]string, len(bs.BlocksIndex))
		for k, v := range bs.BlocksIndex {
			if len(v) > 0 {
				cp := make([]string, len(v))
				copy(cp, v)
				cloned.BlocksIndex[k] = cp
			} else {
				cloned.BlocksIndex[k] = nil
			}
		}
	}
	return cloned
}

// graphLayoutWithRanks returns a new GraphLayout with Phase 2 ranks populated from stats.
// This is a pure function - it does not modify the input.
func graphLayoutWithRanks(old *GraphLayout, stats *analysis.GraphStats) *GraphLayout {
	if old == nil {
		return nil
	}
	if stats == nil {
		// Return a shallow copy with no rank updates
		return &GraphLayout{
			Blockers:         old.Blockers,
			Dependents:       old.Dependents,
			SortedIDs:        old.SortedIDs,
			CriticalPath:     old.CriticalPath,
			CriticalNext:     old.CriticalNext,
			RankPageRank:     old.RankPageRank,
			RankBetweenness:  old.RankBetweenness,
			RankEigenvector:  old.RankEigenvector,
			RankHubs:         old.RankHubs,
			RankAuthorities:  old.RankAuthorities,
			RankCriticalPath: old.RankCriticalPath,
			RankInDegree:     old.RankInDegree,
			RankOutDegree:    old.RankOutDegree,
		}
	}

	criticalPathRank := stats.CriticalPathRank()
	return &GraphLayout{
		Blockers:         old.Blockers,
		Dependents:       old.Dependents,
		SortedIDs:        orderIssueIDsByRank(old.SortedIDs, criticalPathRank),
		CriticalPath:     old.CriticalPath,
		CriticalNext:     old.CriticalNext,
		RankPageRank:     stats.PageRankRank(),
		RankBetweenness:  stats.BetweennessRank(),
		RankEigenvector:  stats.EigenvectorRank(),
		RankHubs:         stats.HubsRank(),
		RankAuthorities:  stats.AuthoritiesRank(),
		RankCriticalPath: criticalPathRank,
		RankInDegree:     stats.InDegreeRank(),
		RankOutDegree:    stats.OutDegreeRank(),
	}
}

// detachedPhase2Input captures the mutable issue-backed surfaces on the UI
// thread before asynchronous preparation. Layout ranks and semantic documents
// are immutable snapshot data; the preparation only reads them.
func (s *DataSnapshot) detachedPhase2Input() *DataSnapshot {
	if s == nil {
		return nil
	}
	cloned := *s
	cloned.phase2Input = nil
	if input := s.phase2Input; input != nil {
		// Keep current source metadata (the worker fills hashes/load warnings
		// after Build), while reusing private detached issue-backed inputs.
		cloned.Issues, cloned.IssueMap = input.Issues, input.IssueMap
		cloned.ViewIssues, cloned.ListItems = input.ViewIssues, input.ListItems
		cloned.TreeRoots, cloned.TreeNodeMap = input.TreeRoots, input.TreeNodeMap
		cloned.BoardState = input.BoardState
		return &cloned
	}
	cloned.Issues = cloneIssuesForAsync(s.Issues)
	cloned.IssueMap = make(map[string]*model.Issue, len(cloned.Issues))
	for i := range cloned.Issues {
		cloned.IssueMap[cloned.Issues[i].ID] = &cloned.Issues[i]
	}
	cloned.ViewIssues = cloneIssuesForAsync(s.ViewIssues)
	cloned.ListItems = deepCopyListItems(s.ListItems)
	cloned.TreeRoots, cloned.TreeNodeMap = deepCopyTree(s.TreeRoots, s.TreeNodeMap, cloned.IssueMap)
	cloned.BoardState = deepCopyBoardState(s.BoardState)
	return &cloned
}

// WithPhase2 returns a new DataSnapshot with Phase 2 analysis results populated.
// It keeps read-only Phase 1 structures where safe, but clones issue-backed and
// tree-backed state so the returned snapshot remains detached from any legacy UI
// code that may continue mutating slices, maps, or tree expansion state.
// This is the core method for immutable snapshot updates (bv-f6uz).
//
// The UI-free half (triage, tree, recipe view) runs in snapshot.Snapshot.WithPhase2;
// this method adds the list items, semantic documents, alerts, board and graph layout.
func (s *DataSnapshot) WithPhase2(stats *analysis.GraphStats, insights analysis.Insights, issues []model.Issue, analyzer *analysis.Analyzer) *DataSnapshot {
	if s == nil {
		return nil
	}

	c := s.coreView().WithPhase2(stats, insights, issues, analyzer)
	out := dataSnapshotFromCore(c)
	out.phase2Triage = c.Triage

	var listItems []IssueItem
	if c.ViewRebuilt {
		listItems = buildListItems(c.ViewIssues, stats)
	} else {
		listItems = deepCopyListItems(s.ListItems)
	}
	listModelItems := make([]list.Item, len(listItems))
	listIndexByID := make(map[string]int, len(listItems))
	for i := range listItems {
		item := &listItems[i]
		id := item.Issue.ID
		if stats != nil {
			item.GraphScore = stats.GetPageRankScore(id)
			item.Impact = stats.GetCriticalPathScore(id)
		}
		item.TriageScore = c.TriageScores[id]
		reasons := c.TriageReasons[id]
		item.TriageReason = reasons.Primary
		item.TriageReasons = reasons.All
		item.IsQuickWin = c.QuickWinSet[id]
		item.IsBlocker = c.BlockerSet[id]
		item.UnblocksCount = len(c.UnblocksMap[id])
		listModelItems[i] = listItems[i]
		listIndexByID[listItems[i].Issue.ID] = i
	}
	newListOrderHash := listOrderFingerprint(listItems)
	semanticIDs := s.semanticIDs
	semanticDocs := s.semanticDocs
	if newListOrderHash != s.listOrderHash || len(listItems) != len(s.semanticIDs) {
		semanticIDs = make([]string, len(listItems))
		semanticDocs = make(map[string]string, len(listItems))
		for i := range listItems {
			id := listItems[i].Issue.ID
			semanticIDs[i] = id
			if doc, ok := s.semanticDocs[id]; ok {
				semanticDocs[id] = doc
			} else {
				semanticDocs[id] = search.IssueDocument(listItems[i].Issue)
			}
		}
	}
	alerts, alertsCritical, alertsWarning, alertsInfo := computeAlertsForDir(c.Issues, stats, analyzer, s.projectDir)

	// Metadata the core does not track.
	out.pooledIssues = s.pooledIssues
	out.ErrorTime = s.ErrorTime
	out.StaleWarning = s.StaleWarning
	out.IssueDiff = s.IssueDiff
	out.IssueDiffStats = s.IssueDiffStats
	out.IncrementalListUsed = s.IncrementalListUsed

	// UI-only state. Deep copy: list items and the board hold mutable issue copies.
	out.ListItems = listItems
	out.listModelItems = listModelItems
	out.listIndexByID = listIndexByID
	out.listOrderHash = newListOrderHash
	out.semanticIDs = semanticIDs
	out.semanticDocs = semanticDocs
	out.alerts = alerts
	out.alertsCritical = alertsCritical
	out.alertsWarning = alertsWarning
	out.alertsInfo = alertsInfo
	out.BoardState = deepCopyBoardState(s.BoardState)
	out.graphLayout = graphLayoutWithRanks(s.graphLayout, stats)
	return out
}
