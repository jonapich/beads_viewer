package snapshot

import (
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

var testNow = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// fixtureIssues returns a small tracker with both edge kinds:
//
//	E (epic, P1, open)
//	  |- A (task, P2, open)            parent-child -> E
//	  |- B (task, P2, blocked)         parent-child -> E, blocks-on A
//	  `- C (task, P3, closed)          parent-child -> E
//	D (task, P0, open)                 standalone
func fixtureIssues() []model.Issue {
	mk := func(id string, typ model.IssueType, prio int, status model.Status, deps ...*model.Dependency) model.Issue {
		return model.Issue{
			ID: id, Title: "Issue " + id, IssueType: typ, Priority: prio, Status: status,
			CreatedAt: testNow.Add(-48 * time.Hour), UpdatedAt: testNow.Add(-48 * time.Hour),
			Dependencies: deps,
		}
	}
	child := func(id string) *model.Dependency {
		return &model.Dependency{IssueID: id, DependsOnID: "E", Type: model.DepParentChild}
	}
	return []model.Issue{
		mk("C", model.TypeTask, 3, model.StatusClosed, child("C")),
		mk("B", model.TypeTask, 2, model.StatusBlocked, child("B"),
			&model.Dependency{IssueID: "B", DependsOnID: "A", Type: model.DepBlocks}),
		mk("A", model.TypeTask, 2, model.StatusOpen, child("A")),
		mk("E", model.TypeEpic, 1, model.StatusOpen),
		mk("D", model.TypeTask, 0, model.StatusOpen),
	}
}

func newTestBuilder(cfg Config) *Builder {
	b := NewBuilder(fixtureIssues(), nil).WithConfig(cfg)
	b.Analyzer().SetNow(testNow)
	return b
}

// phase1Config is DefaultConfig without the background Phase 2 goroutines, so
// results are deterministic.
func phase1Config() Config {
	cfg := DefaultConfig()
	cfg.SkipPhase2 = true
	return cfg
}

func ids(issues []model.Issue) []string {
	out := make([]string, len(issues))
	for i := range issues {
		out[i] = issues[i].ID
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildSortsAndCounts(t *testing.T) {
	s := newTestBuilder(phase1Config()).Build()

	if got, want := ids(s.Issues), []string{"D", "E", "A", "B", "C"}; !equalStrings(got, want) {
		t.Fatalf("issue order = %v, want %v", got, want)
	}
	if len(s.IssueMap) != 5 || s.Issue("A") == nil || s.Issue("missing") != nil {
		t.Fatalf("IssueMap lookup wrong: %d entries", len(s.IssueMap))
	}
	if s.CountOpen != 4 || s.CountClosed != 1 || s.CountBlocked != 1 {
		t.Fatalf("counts open/closed/blocked = %d/%d/%d, want 4/1/1", s.CountOpen, s.CountClosed, s.CountBlocked)
	}
	// B is blocked by the open A and C is closed: D, E and A are ready.
	if s.CountReady != 3 {
		t.Fatalf("CountReady = %d, want 3", s.CountReady)
	}
	for id, want := range map[string]bool{"D": true, "E": true, "A": true, "B": false, "C": false, "missing": false} {
		if got := s.IsReady(id); got != want {
			t.Errorf("IsReady(%s) = %v, want %v", id, got, want)
		}
	}
	if !s.ReferenceTime.Equal(testNow) {
		t.Fatalf("ReferenceTime = %v, want %v", s.ReferenceTime, testNow)
	}
	if s.Phase2Ready != s.Analysis.IsPhase2Ready() || s.IsEmpty() || !s.AuthorityComplete || s.Age() < 0 {
		t.Fatalf("unexpected metadata: phase2=%v empty=%v authority=%v", s.Phase2Ready, s.IsEmpty(), s.AuthorityComplete)
	}
	if len(s.ViewIssues) != len(s.Issues) || s.Recipe != nil || s.RecipeName != "" || s.RecipeHash != "" {
		t.Fatalf("no-recipe view should mirror Issues: view=%d recipe=%v", len(s.ViewIssues), s.Recipe)
	}
}

func TestBuildDropsTombstones(t *testing.T) {
	issues := append(fixtureIssues(), model.Issue{
		ID: "X", Title: "gone", IssueType: model.TypeTask, Status: model.StatusTombstone,
		CreatedAt: testNow, UpdatedAt: testNow,
	})
	s := NewBuilder(issues, nil).WithConfig(phase1Config()).Build()
	if s.Issue("X") != nil || len(s.Issues) != 5 {
		t.Fatalf("tombstone leaked into snapshot: %v", ids(s.Issues))
	}
}

func TestBuildTreeRootsAndDepths(t *testing.T) {
	s := newTestBuilder(phase1Config()).Build()

	var rootIDs []string
	for _, r := range s.TreeRoots {
		rootIDs = append(rootIDs, r.Issue.ID)
	}
	if want := []string{"D", "E"}; !equalStrings(rootIDs, want) {
		t.Fatalf("tree roots = %v, want %v (priority order)", rootIDs, want)
	}
	epic := s.TreeNodeMap["E"]
	if epic == nil || epic.Depth != 0 || !epic.Expanded {
		t.Fatalf("epic node wrong: %+v", epic)
	}
	var childIDs []string
	for _, c := range epic.Children {
		childIDs = append(childIDs, c.Issue.ID)
		if c.Depth != 1 || c.Parent != epic {
			t.Errorf("child %s depth=%d parent ok=%v", c.Issue.ID, c.Depth, c.Parent == epic)
		}
	}
	if want := []string{"A", "B", "C"}; !equalStrings(childIDs, want) {
		t.Fatalf("epic children = %v, want %v", childIDs, want)
	}
	if node := s.TreeNodeMap["A"]; node == nil || node.Issue != s.IssueMap["A"] {
		t.Fatalf("tree node must point at the snapshot's own issue")
	}
}

func TestBuildTriageMapsByConfig(t *testing.T) {
	s := newTestBuilder(phase1Config()).Build()
	if s.Triage == nil || len(s.TriageScores) == 0 || s.TriageReasons == nil ||
		s.QuickWinSet == nil || s.BlockerSet == nil || s.UnblocksMap == nil {
		t.Fatalf("default config must precompute triage: %+v", s.Triage)
	}
	if _, ok := s.TriageScores["A"]; !ok {
		t.Fatalf("expected a triage score for A, got %v", s.TriageScores)
	}

	large := ConfigForTier(TierLarge)
	large.SkipPhase2 = true // keep the test deterministic
	l := newTestBuilder(large).Build()
	if l.Triage != nil || l.TriageScores != nil || l.TriageReasons != nil ||
		l.QuickWinSet != nil || l.BlockerSet != nil || l.UnblocksMap != nil {
		t.Fatalf("large tier must not precompute triage")
	}
	if l.TreeRoots != nil || l.TreeNodeMap != nil {
		t.Fatalf("large tier must not precompute the tree")
	}
	if l.Insights.Stats == nil {
		t.Fatalf("insights must still carry the graph stats")
	}
}

func TestTierForIssueCount(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  Tier
		name  string
	}{
		{-1, TierUnknown, "unknown"},
		{0, TierUnknown, "unknown"},
		{1, TierSmall, "small"},
		{999, TierSmall, "small"},
		{1000, TierMedium, "medium"},
		{4999, TierMedium, "medium"},
		{5000, TierLarge, "large"},
		{19999, TierLarge, "large"},
		{20000, TierHuge, "huge"},
	} {
		got := TierForIssueCount(tc.count)
		if got != tc.want || got.String() != tc.name {
			t.Errorf("TierForIssueCount(%d) = %v (%q), want %v (%q)", tc.count, got, got.String(), tc.want, tc.name)
		}
	}
}

func TestConfigForTier(t *testing.T) {
	for tier, want := range map[Tier]Config{
		TierUnknown: DefaultConfig(),
		TierSmall:   DefaultConfig(),
		TierMedium:  DefaultConfig(),
		TierLarge:   {SkipPhase2: false},
		TierHuge:    {SkipPhase2: true},
	} {
		if got := ConfigForTier(tier); got != want {
			t.Errorf("ConfigForTier(%v) = %+v, want %+v", tier, got, want)
		}
	}
}

func TestWithPhase2(t *testing.T) {
	large := ConfigForTier(TierLarge)
	large.SkipPhase2 = true
	p1 := newTestBuilder(large).Build()
	if p1.Triage != nil {
		t.Fatal("precondition: large tier skips triage")
	}

	issues := CloneIssues(p1.Issues)
	az := analysis.NewAnalyzer(issues)
	az.SetNow(testNow)
	stats := az.AnalyzeAsync(nil)
	stats.WaitForPhase2()
	insights := stats.GenerateInsights(stats.NodeCount)

	p2 := p1.WithPhase2(stats, insights, issues, az)
	if !p2.Phase2Ready || p2.Triage == nil || len(p2.TriageScores) == 0 {
		t.Fatalf("WithPhase2 must mark phase 2 ready and compute triage: ready=%v triage=%v", p2.Phase2Ready, p2.Triage)
	}
	if p2.Analysis != stats || p2.Analyzer != az || !p2.ReferenceTime.Equal(testNow) {
		t.Fatalf("WithPhase2 must adopt the supplied stats, analyzer and reference time")
	}
	if p2.CountOpen != p1.CountOpen || p2.CountReady != p1.CountReady || !p2.CreatedAt.Equal(p1.CreatedAt) {
		t.Fatalf("metadata not carried over")
	}
	if p2.ViewRebuilt {
		t.Fatalf("no recipe: ViewRebuilt must stay false")
	}
	// Issues are detached from the phase 1 snapshot.
	if p2.IssueMap["A"] == p1.IssueMap["A"] || p2.Issue("A") == nil {
		t.Fatalf("phase 2 must hold its own issue copies")
	}
	if p1.Analysis == stats || p1.Analyzer == az || p1.Triage != nil {
		t.Fatalf("phase 1 snapshot was mutated")
	}
	if nilSnap := (*Snapshot)(nil); nilSnap.WithPhase2(stats, insights, issues, az) != nil {
		t.Fatalf("nil receiver must return nil")
	}
}

func TestCloneTreeRebindsIssues(t *testing.T) {
	s := newTestBuilder(phase1Config()).Build()
	clone := CloneIssues(s.Issues)
	cloneMap := make(map[string]*model.Issue, len(clone))
	for i := range clone {
		cloneMap[clone[i].ID] = &clone[i]
	}
	roots, byID := CloneTree(s.TreeRoots, s.TreeNodeMap, cloneMap)
	if len(roots) != len(s.TreeRoots) || len(byID) != len(s.TreeNodeMap) {
		t.Fatalf("clone changed tree size")
	}
	if byID["E"] == s.TreeNodeMap["E"] || byID["E"].Issue != cloneMap["E"] {
		t.Fatalf("clone must copy nodes and rebind issues")
	}
	byID["E"].Expanded = false
	if !s.TreeNodeMap["E"].Expanded {
		t.Fatalf("mutating the clone changed the original")
	}
	if r, m := CloneTree(nil, nil, nil); r != nil || m != nil {
		t.Fatalf("empty input must clone to nil")
	}
}

func TestBuildBlockingChains(t *testing.T) {
	roots, byID, ok := BuildBlockingChains(fixtureIssues())
	if !ok || len(roots) == 0 {
		t.Fatalf("expected blocking chains, got ok=%v roots=%d", ok, len(roots))
	}
	a := byID["A"]
	if a == nil || len(a.Children) != 1 || a.Children[0].Issue.ID != "B" {
		t.Fatalf("A must block B: %+v", a)
	}
	if _, m, ok := BuildBlockingChains(nil); ok || m == nil {
		t.Fatalf("empty input: want ok=false and a non-nil map")
	}
	hasPC, hasBlock := ScanTreeEdges(fixtureIssues())
	if !hasPC || !hasBlock {
		t.Fatalf("ScanTreeEdges = %v,%v, want true,true", hasPC, hasBlock)
	}
}

func TestIssueTypeOrder(t *testing.T) {
	order := []model.IssueType{model.TypeEpic, model.TypeFeature, model.TypeTask, model.TypeBug, model.TypeChore, "custom"}
	for i, typ := range order {
		if got := IssueTypeOrder(typ); got != i {
			t.Errorf("IssueTypeOrder(%q) = %d, want %d", typ, got, i)
		}
	}
}
