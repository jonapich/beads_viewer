package snapshot

import (
	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// WithPhase2 returns a new Snapshot with Phase 2 analysis results populated.
// It keeps read-only Phase 1 structures where safe, but clones issue-backed and
// tree-backed state so the returned snapshot remains detached from any caller
// that may continue mutating slices, maps, or tree expansion state.
//
// Triage is recomputed from the Phase 2 stats and the supplied analyzer. When
// the snapshot's recipe depends on graph metrics or triage scores, ViewIssues is
// re-derived and ViewRebuilt is set.
func (s *Snapshot) WithPhase2(stats *analysis.GraphStats, insights analysis.Insights, issues []model.Issue, analyzer *analysis.Analyzer) *Snapshot {
	if s == nil {
		return nil
	}

	issuesClone := CloneIssues(s.Issues)
	clonedIssueMap := make(map[string]*model.Issue, len(issuesClone))
	for i := range issuesClone {
		clonedIssueMap[issuesClone[i].ID] = &issuesClone[i]
	}

	// Compute triage data from Phase 2 analysis
	var triageScores map[string]float64
	var triageReasons map[string]analysis.TriageReasons
	var quickWinSet map[string]bool
	var blockerSet map[string]bool
	var unblocksMap map[string][]string
	var completedTriage *analysis.TriageResult

	if stats != nil && analyzer != nil && len(issues) > 0 {
		opts := analysis.TriageOptions{}
		if !s.ExplicitWeights && analyzer.Weights() == analysis.DefaultWeights() {
			if beadsDir := s.resolvedBeadsDir(); beadsDir != "" {
				if fw := FeedbackWeights(beadsDir); fw != nil {
					analyzer.SetWeights(*fw)
					opts.Weights = fw
				}
			}
		}
		triageResult := analysis.ComputeTriageFromAnalyzer(analyzer, stats, issues, opts, analyzer.Now())
		completedTriage = &triageResult
		triageScores, triageReasons, quickWinSet, blockerSet, unblocksMap = ExtractTriageMaps(triageResult)
	}

	// Deep copy tree structures since tree views mutate node.Expanded and node.Children.
	// Rebind tree nodes to cloned issues so the new snapshot stays detached from
	// the caller's own issue slice and pointer churn.
	treeRoots, treeNodeMap := CloneTree(s.TreeRoots, s.TreeNodeMap, clonedIssueMap)
	viewIssues := CloneIssues(s.ViewIssues)
	viewRebuilt := false
	if s.Recipe != nil && (s.Recipe.NeedsGraphMetrics() || s.Recipe.NeedsTriageScores()) && analyzer != nil && stats != nil {
		if applied, err := ApplyRecipe(issuesClone, analyzer, stats, triageScores, s.Recipe, analyzer.Now()); err == nil {
			viewIssues = applied
			viewRebuilt = true
		}
	}

	referenceTime := s.ReferenceTime
	if analyzer != nil {
		referenceTime = analyzer.Now()
	}

	return &Snapshot{
		// Clone mutable Phase 1 data so the new snapshot stays immutable even if
		// the caller continues mutating its own slices or maps.
		Issues:      issuesClone,
		IssueMap:    clonedIssueMap,
		ViewIssues:  viewIssues,
		ViewRebuilt: viewRebuilt,
		TreeRoots:   treeRoots,
		TreeNodeMap: treeNodeMap,

		// Updated with Phase 2 data
		Analyzer:      analyzer,
		Analysis:      stats,
		Insights:      insights,
		Phase2Ready:   true,
		ReferenceTime: referenceTime,
		Triage:        completedTriage,
		TriageScores:  triageScores,
		TriageReasons: triageReasons,
		QuickWinSet:   quickWinSet,
		BlockerSet:    blockerSet,
		UnblocksMap:   unblocksMap,

		// Copied metadata
		CountOpen:            s.CountOpen,
		CountReady:           s.CountReady,
		CountBlocked:         s.CountBlocked,
		CountClosed:          s.CountClosed,
		CreatedAt:            s.CreatedAt,
		DataHash:             s.DataHash,
		AuthorityHash:        s.AuthorityHash,
		AuthorityComplete:    s.AuthorityComplete,
		Recipe:               s.Recipe,
		RecipeName:           s.RecipeName,
		RecipeHash:           s.RecipeHash,
		Tier:                 s.Tier,
		SourceIssueCountHint: s.SourceIssueCountHint,
		LoadedOpenOnly:       s.LoadedOpenOnly,
		TruncatedCount:       s.TruncatedCount,
		LargeDatasetWarning:  s.LargeDatasetWarning,
		LoadWarningCount:     s.LoadWarningCount,
		LoadError:            s.LoadError,
		BeadsPath:            s.BeadsPath,
		ProjectDir:           s.ProjectDir,
		ExplicitProjectDir:   s.ExplicitProjectDir,
		ExplicitWeights:      s.ExplicitWeights,
	}
}
