package snapshot

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
)

// Snapshot is an immutable, self-contained view of the issue data a UI renders.
// Fields are exported for reading only; see the package comment.
type Snapshot struct {
	Issues     []model.Issue           // Sorted: open first, priority asc, created desc, ID
	IssueMap   map[string]*model.Issue // Lookup by ID
	ViewIssues []model.Issue           // Recipe-filtered view; same content as Issues when no recipe
	// ViewRebuilt is set by WithPhase2 when ViewIssues was re-derived from a
	// metric-dependent recipe.
	ViewRebuilt bool

	Analyzer      *analysis.Analyzer
	Analysis      *analysis.GraphStats
	Insights      analysis.Insights
	Phase2Ready   bool      // True when Phase 2 has finished or was skipped (Config.SkipPhase2); when skipped, the Phase 2 metric maps are empty
	ReferenceTime time.Time // Analyzer.Now() used for readiness, triage and recipes

	CountOpen    int
	CountReady   int
	CountBlocked int
	CountClosed  int

	Triage        *analysis.TriageResult // Nil when triage was not computed (large/huge tiers)
	TriageScores  map[string]float64
	TriageReasons map[string]analysis.TriageReasons
	QuickWinSet   map[string]bool
	BlockerSet    map[string]bool
	UnblocksMap   map[string][]string

	TreeRoots   []*TreeNode // Parent/child hierarchy; nil when not precomputed
	TreeNodeMap map[string]*TreeNode

	CreatedAt            time.Time // When this snapshot was built
	DataHash             string    // Hash of source data for cache validation
	AuthorityHash        string    // Full source identity, including hidden dependency records
	AuthorityComplete    bool      // True when source authority is complete and warning-free
	Recipe               *recipe.Recipe
	RecipeName           string // Active recipe name
	RecipeHash           string // Fingerprint of the active recipe
	Tier                 Tier
	SourceIssueCountHint int    // Approximate total issue count from the source file; 0 if unavailable
	LoadedOpenOnly       bool   // True when closed/tombstone issues were excluded by load policy
	TruncatedCount       int    // Approximate count of issues excluded by load policy
	LargeDatasetWarning  string // Short user-facing warning for large datasets
	LoadWarningCount     int    // Non-fatal parse warnings encountered while loading
	LoadError            error  // Recipe validation error from Build

	BeadsPath          string
	ProjectDir         string
	ExplicitProjectDir string
	ExplicitWeights    bool
}

// Issue returns an issue by ID, or nil if not found.
func (s *Snapshot) Issue(id string) *model.Issue {
	if s == nil || s.IssueMap == nil {
		return nil
	}
	return s.IssueMap[id]
}

// IsEmpty returns true if the snapshot has no issues.
func (s *Snapshot) IsEmpty() bool {
	return s == nil || len(s.Issues) == 0
}

// Age returns how long ago this snapshot was created.
func (s *Snapshot) Age() time.Duration {
	if s == nil {
		return 0
	}
	return time.Since(s.CreatedAt)
}

// IsReady reports whether the issue is open work that nothing unresolved
// blocks. It applies the same rule as CountReady.
func (s *Snapshot) IsReady(id string) bool {
	if s == nil || s.Analyzer == nil {
		return false
	}
	issue := s.Issue(id)
	if issue == nil || !s.Analyzer.IsCandidate(id) || isClosedLikeStatus(issue.Status) {
		return false
	}
	return s.Analyzer.Readiness().Ready(id, s.ReferenceTime)
}

func (s *Snapshot) resolvedBeadsDir() string {
	if s == nil {
		return ""
	}
	if strings.TrimSpace(s.BeadsPath) != "" {
		return filepath.Dir(s.BeadsPath)
	}
	if strings.TrimSpace(s.ProjectDir) != "" {
		return filepath.Join(s.ProjectDir, ".beads")
	}
	return ""
}

func isClosedLikeStatus(status model.Status) bool {
	return status == model.StatusClosed || status == model.StatusTombstone
}
