package snapshot

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/cass"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
)

// Builder constructs Snapshots from raw issues. The TUI and the HTTP API both
// build through it, so they share triage options, sorting and the tree.
type Builder struct {
	ctx                context.Context
	issues             []model.Issue
	analyzer           *analysis.Analyzer
	analysis           *analysis.GraphStats
	recipe             *recipe.Recipe
	cfg                Config
	beadsPath          string
	projectDir         string
	explicitProjectDir string
	explicitWeights    bool
}

// NewBuilder creates a builder for issues. authority is the readiness index of
// the full source (so hidden dependency records still count); nil derives it
// from issues. Tombstones are dropped from the visible set.
//
// NewBuilder takes ownership of issues: Build sorts the slice in place and the
// Snapshot's Issues/IssueMap alias it. Pass CloneIssues(x) if the caller keeps
// using x.
func NewBuilder(issues []model.Issue, authority *model.ReadinessIndex) *Builder {
	readiness := authority
	if readiness == nil {
		readiness = model.NewReadinessIndex(issues)
	}
	issues = IssuesWithoutTombstones(issues)
	analyzer := analysis.NewAnalyzer(issues)
	analyzer.SetReadinessScope(readiness, nil)
	return &Builder{
		issues:   issues,
		analyzer: analyzer,
		cfg:      DefaultConfig(),
	}
}

// IssuesWithoutTombstones returns issues without tombstoned rows. It returns the
// input slice itself when it holds no tombstones.
func IssuesWithoutTombstones(issues []model.Issue) []model.Issue {
	for i, issue := range issues {
		if !issue.Status.IsTombstone() {
			continue
		}
		visible := make([]model.Issue, 0, len(issues)-1)
		visible = append(visible, issues[:i]...)
		for _, remaining := range issues[i+1:] {
			if !remaining.Status.IsTombstone() {
				visible = append(visible, remaining)
			}
		}
		return visible
	}
	return issues
}

// Analyzer returns the analyzer this builder (and the snapshot it builds) uses.
// Callers may configure it (SetNow, RestoreScoring, ...) before Build.
func (b *Builder) Analyzer() *analysis.Analyzer {
	return b.analyzer
}

// WithContext binds Phase 2 async analysis to the caller's lifecycle context.
func (b *Builder) WithContext(ctx context.Context) *Builder {
	b.ctx = ctx
	return b
}

// WithWeights installs feedback-adjusted factor weights on the builder's
// analyzer so triage and priority hints computed from this snapshot use them.
// nil leaves the defaults in place.
func (b *Builder) WithWeights(w *analysis.Weights) *Builder {
	if w != nil {
		b.analyzer.SetWeights(*w)
		b.explicitWeights = true
	}
	return b
}

// WithBeadsPath associates a .beads/<file>.jsonl path with this snapshot build
// so baseline drift alerts and .beads/feedback.json weights resolve against the
// target workspace even when the process CWD is a subdirectory.
func (b *Builder) WithBeadsPath(beadsPath string) *Builder {
	b.beadsPath = beadsPath
	if b.projectDir == "" && beadsPath != "" {
		b.projectDir = cass.WorkspaceFromBeadsPath(beadsPath)
	}
	return b
}

// WithProjectDir associates a project root directory with this snapshot build.
func (b *Builder) WithProjectDir(projectDir string) *Builder {
	b.projectDir = projectDir
	b.explicitProjectDir = projectDir
	return b
}

// WithAnalysis sets the pre-computed analysis (for when we have cached results).
func (b *Builder) WithAnalysis(a *analysis.GraphStats) *Builder {
	b.analysis = a
	return b
}

// WithRecipe sets the recipe that derives Snapshot.ViewIssues.
func (b *Builder) WithRecipe(r *recipe.Recipe) *Builder {
	b.recipe = r
	return b
}

// WithConfig selects what Build precomputes.
func (b *Builder) WithConfig(cfg Config) *Builder {
	b.cfg = cfg
	return b
}

func (b *Builder) resolvedProjectDir() string {
	if strings.TrimSpace(b.projectDir) != "" {
		return b.projectDir
	}
	if strings.TrimSpace(b.beadsPath) != "" {
		return cass.WorkspaceFromBeadsPath(b.beadsPath)
	}
	return ""
}

func (b *Builder) resolvedBeadsDir() string {
	if strings.TrimSpace(b.beadsPath) != "" {
		return filepath.Dir(b.beadsPath)
	}
	if strings.TrimSpace(b.projectDir) != "" {
		return filepath.Join(b.projectDir, ".beads")
	}
	return ""
}

// Build constructs the final immutable Snapshot.
// Unless WithAnalysis was used, it calls AnalyzeAsync() so Phase 2 metrics
// compute in the background: check Phase2Ready or call
// Analysis.WaitForPhase2() when Phase 2 data is needed immediately.
func (b *Builder) Build() *Snapshot {
	issues := b.issues
	createdAt := time.Now()
	// Readiness, recipes, and scoring share the analyzer's reference instant.
	// Freshness still describes when this snapshot was actually constructed.
	now := b.analyzer.Now()

	// Apply default sorting to match the legacy reload path:
	// Open first, then priority (ascending), then created date (newest first), then ID (ascending).
	sort.Slice(issues, func(i, j int) bool {
		iClosed := isClosedLikeStatus(issues[i].Status)
		jClosed := isClosedLikeStatus(issues[j].Status)
		if iClosed != jClosed {
			return !iClosed
		}
		if issues[i].Priority != issues[j].Priority {
			return issues[i].Priority < issues[j].Priority
		}
		if !issues[i].CreatedAt.Equal(issues[j].CreatedAt) {
			return issues[i].CreatedAt.After(issues[j].CreatedAt)
		}
		return issues[i].ID < issues[j].ID
	})

	// Compute analysis if not provided
	// Use AnalyzeAsync to allow Phase 2 to run in background
	graphStats := b.analysis
	if graphStats == nil {
		analysisCtx := b.ctx
		if analysisCtx == nil {
			analysisCtx = context.Background()
		}
		if b.cfg.SkipPhase2 {
			// Still compute Phase 1 metrics, but skip expensive Phase 2 work.
			cfg := analysis.ConfigForSize(len(issues), 0)
			cfg.ComputePageRank = false
			cfg.ComputeBetweenness = false
			cfg.ComputeEigenvector = false
			cfg.ComputeHITS = false
			cfg.ComputeCriticalPath = false
			cfg.ComputeCycles = false
			cfg.ComputeKCore = false
			cfg.ComputeArticulation = false
			cfg.ComputeSlack = false
			graphStats = b.analyzer.AnalyzeAsyncWithConfig(analysisCtx, cfg)
		} else {
			graphStats = b.analyzer.AnalyzeAsync(analysisCtx)
		}
	}

	// Build lookup map
	issueMap := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	// Compute statistics
	cOpen, cReady, cBlocked, cClosed := 0, 0, 0, 0
	readiness := b.analyzer.Readiness()
	for i := range issues {
		issue := &issues[i]
		if !b.analyzer.IsCandidate(issue.ID) {
			continue
		}
		if isClosedLikeStatus(issue.Status) {
			cClosed++
			continue
		}

		cOpen++
		if issue.Status == model.StatusBlocked {
			cBlocked++
		}
		if readiness.Ready(issue.ID, now) {
			cReady++
		}
	}

	beadsDir := b.resolvedBeadsDir()
	var feedbackWeights *analysis.Weights
	if !b.explicitWeights && beadsDir != "" {
		if fw := FeedbackWeights(beadsDir); fw != nil {
			b.analyzer.SetWeights(*fw)
			feedbackWeights = fw
		}
	}

	var (
		recipeTriage  map[string]float64
		triage        *analysis.TriageResult
		triageScores  map[string]float64
		triageReasons map[string]analysis.TriageReasons
		quickWinSet   map[string]bool
		blockerSet    map[string]bool
		unblocksMap   map[string][]string
	)
	hasTriage := b.cfg.PrecomputeTriage || (b.recipe != nil && b.recipe.NeedsTriageScores())
	if hasTriage {
		opts := analysis.TriageOptions{}
		if feedbackWeights != nil {
			opts.Weights = feedbackWeights
		}
		triageResult := analysis.ComputeTriageFromAnalyzer(b.analyzer, graphStats, issues, opts, now)
		triage = &triageResult
		triageScores, triageReasons, quickWinSet, blockerSet, unblocksMap = ExtractTriageMaps(triageResult)
		recipeTriage = triageScores
	}
	viewIssues := issues
	var recipeErr error
	if b.recipe != nil {
		viewIssues, recipeErr = ApplyRecipe(issues, b.analyzer, graphStats, recipeTriage, b.recipe, now)
	}

	var (
		treeRoots   []*TreeNode
		treeNodeMap map[string]*TreeNode
	)
	if b.cfg.PrecomputeTree {
		treeRoots, treeNodeMap = BuildHierarchy(issues)
	}

	insights := analysis.Insights{Stats: graphStats, ClusterDensity: graphStats.Density}
	if b.cfg.PrecomputeInsights {
		insights = graphStats.GenerateInsights(len(issues))
	}

	return &Snapshot{
		LoadError:          recipeErr,
		Issues:             issues,
		IssueMap:           issueMap,
		ViewIssues:         viewIssues,
		Analyzer:           b.analyzer,
		Analysis:           graphStats,
		Insights:           insights,
		ReferenceTime:      now,
		CountOpen:          cOpen,
		CountReady:         cReady,
		CountBlocked:       cBlocked,
		CountClosed:        cClosed,
		Triage:             triage,
		TriageScores:       triageScores,
		TriageReasons:      triageReasons,
		QuickWinSet:        quickWinSet,
		BlockerSet:         blockerSet,
		UnblocksMap:        unblocksMap,
		TreeRoots:          treeRoots,
		TreeNodeMap:        treeNodeMap,
		CreatedAt:          createdAt,
		AuthorityComplete:  true,
		Recipe:             b.recipe,
		RecipeName:         RecipeName(b.recipe),
		RecipeHash:         RecipeFingerprint(b.recipe),
		Phase2Ready:        graphStats.IsPhase2Ready(),
		BeadsPath:          b.beadsPath,
		ProjectDir:         b.resolvedProjectDir(),
		ExplicitProjectDir: b.explicitProjectDir,
		ExplicitWeights:    b.explicitWeights,
	}
}
