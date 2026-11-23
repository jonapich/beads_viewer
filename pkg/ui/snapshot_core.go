package ui

import (
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// This file is the seam between the TUI and pkg/snapshot, the UI-free data
// pipeline shared with the HTTP API. DataSnapshot keeps its flat shape (tests
// and the background worker build and mutate it directly); the adapters below
// translate to and from snapshot.Snapshot, and the aliases and one-line
// wrappers keep the old unexported names pointing at the moved code.

// Dataset tiers (bv-9thm) live in pkg/snapshot.
type datasetTier = snapshot.Tier

const (
	datasetTierUnknown = snapshot.TierUnknown
	datasetTierSmall   = snapshot.TierSmall
	datasetTierMedium  = snapshot.TierMedium
	datasetTierLarge   = snapshot.TierLarge
	datasetTierHuge    = snapshot.TierHuge
)

func datasetTierForIssueCount(total int) datasetTier {
	return snapshot.TierForIssueCount(total)
}

// IssueTreeNode is a node in the parent/child (or blocking) issue tree.
type IssueTreeNode = snapshot.TreeNode

func scanTreeEdgePresence(issues []model.Issue) (hasParentChild, hasBlocking bool) {
	return snapshot.ScanTreeEdges(issues)
}

// buildBlockingTreeNodes constructs blocker -> dependent chains over blocking edges.
func buildBlockingTreeNodes(issues []model.Issue) ([]*IssueTreeNode, map[string]*IssueTreeNode, bool) {
	return snapshot.BuildBlockingChains(issues)
}

// buildIssueTreeNodes constructs the hierarchical parent/child tree. It does not
// load persisted expand/collapse state or build the visible flat list; those stay
// view concerns handled by TreeModel.
func buildIssueTreeNodes(issues []model.Issue) ([]*IssueTreeNode, map[string]*IssueTreeNode) {
	return snapshot.BuildHierarchy(issues)
}

func issueTypeOrder(t model.IssueType) int {
	return snapshot.IssueTypeOrder(t)
}

// deepCopyTree creates a deep copy of TreeRoots and TreeNodeMap. The tree view
// mutates node.Expanded and node.Children, which would corrupt shared snapshots
// if we only did pointer aliasing. Issue pointers are rebound to issueMap.
// Returns (nil, nil) if input is empty.
func deepCopyTree(roots []*IssueTreeNode, nodeMap map[string]*IssueTreeNode, issueMap map[string]*model.Issue) ([]*IssueTreeNode, map[string]*IssueTreeNode) {
	return snapshot.CloneTree(roots, nodeMap, issueMap)
}

func cloneIssuesForAsync(issues []model.Issue) []model.Issue {
	return snapshot.CloneIssues(issues)
}

func issuesWithoutTombstones(issues []model.Issue) []model.Issue {
	return snapshot.IssuesWithoutTombstones(issues)
}

func recipeName(r *recipe.Recipe) string {
	return snapshot.RecipeName(r)
}

func recipeFingerprint(r *recipe.Recipe) string {
	return snapshot.RecipeFingerprint(r)
}

func applyRecipeToIssues(issues []model.Issue, analyzer *analysis.Analyzer, stats *analysis.GraphStats, triage map[string]float64, r *recipe.Recipe, now time.Time) ([]model.Issue, error) {
	return snapshot.ApplyRecipe(issues, analyzer, stats, triage, r, now)
}

func extractTriageMaps(triageResult analysis.TriageResult) (
	triageScores map[string]float64,
	triageReasons map[string]analysis.TriageReasons,
	quickWinSet map[string]bool,
	blockerSet map[string]bool,
	unblocksMap map[string][]string,
) {
	return snapshot.ExtractTriageMaps(triageResult)
}

// dataSnapshotFromCore copies the core fields of c into a new DataSnapshot by
// reference (no cloning). It leaves every TUI-only field, and phase2Triage,
// unset: callers fill those in.
func dataSnapshotFromCore(c *snapshot.Snapshot) *DataSnapshot {
	return &DataSnapshot{
		Issues:               c.Issues,
		IssueMap:             c.IssueMap,
		ViewIssues:           c.ViewIssues,
		Analyzer:             c.Analyzer,
		Analysis:             c.Analysis,
		insights:             c.Insights,
		CountOpen:            c.CountOpen,
		CountReady:           c.CountReady,
		CountBlocked:         c.CountBlocked,
		CountClosed:          c.CountClosed,
		TriageScores:         c.TriageScores,
		TriageReasons:        c.TriageReasons,
		QuickWinSet:          c.QuickWinSet,
		BlockerSet:           c.BlockerSet,
		UnblocksMap:          c.UnblocksMap,
		TreeRoots:            c.TreeRoots,
		TreeNodeMap:          c.TreeNodeMap,
		CreatedAt:            c.CreatedAt,
		DataHash:             c.DataHash,
		AuthorityHash:        c.AuthorityHash,
		AuthorityComplete:    c.AuthorityComplete,
		RecipeName:           c.RecipeName,
		RecipeHash:           c.RecipeHash,
		recipe:               c.Recipe,
		DatasetTier:          c.Tier,
		SourceIssueCountHint: c.SourceIssueCountHint,
		LoadedOpenOnly:       c.LoadedOpenOnly,
		TruncatedCount:       c.TruncatedCount,
		LargeDatasetWarning:  c.LargeDatasetWarning,
		LoadWarningCount:     c.LoadWarningCount,
		phase2Ready:          c.Phase2Ready,
		referenceTime:        c.ReferenceTime,
		LoadError:            c.LoadError,
		beadsPath:            c.BeadsPath,
		projectDir:           c.ProjectDir,
		explicitProjectDir:   c.ExplicitProjectDir,
		explicitWeights:      c.ExplicitWeights,
	}
}

// coreView is the reverse of dataSnapshotFromCore: it copies the core fields of
// s into a snapshot.Snapshot by reference. Triage comes from phase2Triage, which
// is only set by WithPhase2.
func (s *DataSnapshot) coreView() *snapshot.Snapshot {
	c := &snapshot.Snapshot{
		Issues:               s.Issues,
		IssueMap:             s.IssueMap,
		ViewIssues:           s.ViewIssues,
		Analyzer:             s.Analyzer,
		Analysis:             s.Analysis,
		Insights:             s.insights,
		Phase2Ready:          s.phase2Ready,
		ReferenceTime:        s.referenceTime,
		CountOpen:            s.CountOpen,
		CountReady:           s.CountReady,
		CountBlocked:         s.CountBlocked,
		CountClosed:          s.CountClosed,
		Triage:               s.phase2Triage,
		TriageScores:         s.TriageScores,
		TriageReasons:        s.TriageReasons,
		QuickWinSet:          s.QuickWinSet,
		BlockerSet:           s.BlockerSet,
		UnblocksMap:          s.UnblocksMap,
		TreeRoots:            s.TreeRoots,
		TreeNodeMap:          s.TreeNodeMap,
		CreatedAt:            s.CreatedAt,
		DataHash:             s.DataHash,
		AuthorityHash:        s.AuthorityHash,
		AuthorityComplete:    s.AuthorityComplete,
		Recipe:               s.recipe,
		RecipeName:           s.RecipeName,
		RecipeHash:           s.RecipeHash,
		Tier:                 s.DatasetTier,
		SourceIssueCountHint: s.SourceIssueCountHint,
		LoadedOpenOnly:       s.LoadedOpenOnly,
		TruncatedCount:       s.TruncatedCount,
		LargeDatasetWarning:  s.LargeDatasetWarning,
		LoadWarningCount:     s.LoadWarningCount,
		LoadError:            s.LoadError,
		BeadsPath:            s.beadsPath,
		ProjectDir:           s.projectDir,
		ExplicitProjectDir:   s.explicitProjectDir,
		ExplicitWeights:      s.explicitWeights,
	}
	return c
}
