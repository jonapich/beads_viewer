package snapshot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
)

const fixtureJSONL = `{"id":"E","title":"Epic","status":"open","priority":1,"issue_type":"epic"}
{"id":"A","title":"A","status":"open","priority":2,"issue_type":"task","dependencies":[{"issue_id":"A","depends_on_id":"E","type":"parent-child"}]}
{"id":"B","title":"B","status":"blocked","priority":2,"issue_type":"task","dependencies":[{"issue_id":"B","depends_on_id":"A","type":"blocks"}]}
{"id":"C","title":"C","status":"closed","priority":3,"issue_type":"task"}
`

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "beads.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeIssues(t *testing.T, n int, closedEvery int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "beads.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	for i := 0; i < n; i++ {
		status := "open"
		if closedEvery > 0 && i%closedEvery == 0 {
			status = "closed"
		}
		fmt.Fprintf(w, `{"id":"issue-%d","title":"Issue %d","status":"%s","priority":1,"issue_type":"task"}`+"\n", i, i, status)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCountSourceJSONL(t *testing.T) {
	path := writeFixture(t, fixtureJSONL)
	for name, count := range map[string]func() (int, error){
		"CountSource":     func() (int, error) { return CountSource(context.Background(), path) },
		"CountSourceSync": func() (int, error) { return CountSourceSync(path) },
		"CountJSONLLines": func() (int, error) { return CountJSONLLines(path) },
	} {
		if n, err := count(); err != nil || n != 4 {
			t.Errorf("%s = %d, %v; want 4", name, n, err)
		}
	}
	// A last line without a trailing newline still counts.
	noNL := writeFixture(t, `{"id":"A"}`+"\n"+`{"id":"B"}`)
	if n, err := CountJSONLLines(noNL); err != nil || n != 2 {
		t.Errorf("no trailing newline: %d, %v; want 2", n, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CountSource(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled CountSource err = %v", err)
	}
}

func TestLoadSource(t *testing.T) {
	path := writeFixture(t, fixtureJSONL)
	src, err := LoadSource(context.Background(), path, loader.ParseOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Issues) != 4 || src.Authority == nil || src.AuthorityHash == "" || !src.AuthorityComplete {
		t.Fatalf("source = %d issues, authority=%v hash=%q complete=%v", len(src.Issues), src.Authority != nil, src.AuthorityHash, src.AuthorityComplete)
	}

	degraded, err := LoadSource(context.Background(), path, loader.ParseOptions{}, true)
	if err != nil || degraded.AuthorityComplete {
		t.Fatalf("degraded source must report incomplete authority: %v complete=%v", err, degraded.AuthorityComplete)
	}

	// The filter hides rows from Issues but not from the authority index.
	openOnly, err := LoadSource(context.Background(), path, loader.ParseOptions{IssueFilter: func(i *model.Issue) bool { return i.ID != "C" }}, false)
	if err != nil || len(openOnly.Issues) != 3 || openOnly.AuthorityHash != src.AuthorityHash {
		t.Fatalf("filtered source: %v issues=%d authorityHash same=%v", err, len(openOnly.Issues), openOnly.AuthorityHash == src.AuthorityHash)
	}

	sync, err := LoadSourceSync(path, loader.ParseOptions{}, false)
	if err != nil || len(sync.Issues) != 4 || sync.AuthorityHash != src.AuthorityHash {
		t.Fatalf("sync source differs: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := LoadSource(ctx, path, loader.ParseOptions{}, false); !errors.Is(err, context.Canceled) || got.Issues != nil {
		t.Fatalf("canceled LoadSource = %v, %v", got.Issues, err)
	}
	if _, err := LoadSource(context.Background(), filepath.Join(t.TempDir(), "missing.jsonl"), loader.ParseOptions{}, false); err == nil {
		t.Fatal("missing file must fail")
	}
}

func TestIncludesClosedStatuses(t *testing.T) {
	if IncludesClosedStatuses(nil) {
		t.Fatal("nil recipe")
	}
	r := &recipe.Recipe{}
	if IncludesClosedStatuses(r) {
		t.Fatal("empty recipe")
	}
	r.Filters.Status = []string{"open", " Closed "}
	if !IncludesClosedStatuses(r) {
		t.Fatal("closed status must be detected case-insensitively")
	}
	r.Filters.Status = []string{"tombstone"}
	if !IncludesClosedStatuses(r) {
		t.Fatal("tombstone must be detected")
	}
}

func TestLargeDatasetWarning(t *testing.T) {
	for _, tc := range []struct {
		tier         Tier
		hint, loaded int
		openOnly     bool
		want         string
	}{
		{TierSmall, 10, 10, false, ""},
		{TierMedium, 2000, 2000, false, ""},
		{TierLarge, 0, 5200, false, "⚠ large 5k issues"},
		{TierLarge, 6000, 5200, false, "⚠ large 6k issues"},
		{TierHuge, 25000, 12000, true, "⚠ huge open-only 12k/25k"},
		{TierHuge, 25000, 25000, false, "⚠ huge 25k issues"},
		{TierHuge, 0, 1_200_000, true, "⚠ huge 1m issues"},
	} {
		if got := LargeDatasetWarning(tc.tier, tc.hint, tc.loaded, tc.openOnly); got != tc.want {
			t.Errorf("LargeDatasetWarning(%v,%d,%d,%v) = %q, want %q", tc.tier, tc.hint, tc.loaded, tc.openOnly, got, tc.want)
		}
	}
}

func TestLoadSmallDataset(t *testing.T) {
	path := writeFixture(t, fixtureJSONL)
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Tier != TierSmall || s.SourceIssueCountHint != 4 || s.LoadedOpenOnly || s.TruncatedCount != 0 || s.LargeDatasetWarning != "" {
		t.Fatalf("tier metadata wrong: %v hint=%d openOnly=%v truncated=%d warn=%q", s.Tier, s.SourceIssueCountHint, s.LoadedOpenOnly, s.TruncatedCount, s.LargeDatasetWarning)
	}
	if len(s.Issues) != 4 || s.DataHash == "" || s.AuthorityHash == "" || !s.AuthorityComplete || s.LoadWarningCount != 0 {
		t.Fatalf("load metadata wrong: issues=%d hash=%q authority=%q complete=%v warnings=%d", len(s.Issues), s.DataHash, s.AuthorityHash, s.AuthorityComplete, s.LoadWarningCount)
	}
	if s.BeadsPath != path || s.CountClosed != 1 || s.CountOpen != 3 {
		t.Fatalf("snapshot content wrong: path=%q open=%d closed=%d", s.BeadsPath, s.CountOpen, s.CountClosed)
	}
	if s.Triage == nil || len(s.TreeRoots) == 0 {
		t.Fatal("small tier must precompute triage and the tree")
	}

	degraded, err := Load(context.Background(), path, LoadOptions{Degraded: true})
	if err != nil || degraded.AuthorityComplete {
		t.Fatalf("degraded load: %v complete=%v", err, degraded.AuthorityComplete)
	}
	if _, err := Load(context.Background(), filepath.Join(t.TempDir(), "missing.jsonl"), LoadOptions{}); err == nil {
		t.Fatal("missing file must fail")
	}
}

func TestLoadHugeDatasetIsOpenOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 20000-line file")
	}
	const total = 20000
	path := writeIssues(t, total, 2) // every second issue is closed
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Tier != TierHuge || !s.LoadedOpenOnly || s.SourceIssueCountHint != total {
		t.Fatalf("tier=%v openOnly=%v hint=%d", s.Tier, s.LoadedOpenOnly, s.SourceIssueCountHint)
	}
	if len(s.Issues) != total/2 || s.TruncatedCount != total/2 || s.CountClosed != 0 {
		t.Fatalf("issues=%d truncated=%d closed=%d", len(s.Issues), s.TruncatedCount, s.CountClosed)
	}
	if s.LargeDatasetWarning != "⚠ huge open-only 10k/20k" {
		t.Fatalf("warning = %q", s.LargeDatasetWarning)
	}
	if s.Triage != nil || s.TreeRoots != nil {
		t.Fatal("huge tier must not precompute triage or the tree")
	}

	// A recipe that selects closed issues turns the open-only load off.
	r := &recipe.Recipe{Name: "all"}
	r.Filters.Status = []string{"open", "closed"}
	full, err := Load(context.Background(), path, LoadOptions{Recipe: r})
	if err != nil || full.LoadedOpenOnly || len(full.Issues) != total {
		t.Fatalf("recipe with closed must load everything: %v openOnly=%v issues=%d", err, full.LoadedOpenOnly, len(full.Issues))
	}
}

// TestLoadAnalysisContext: a canceled AnalysisContext reaches the builder, and
// CompletePhase2 still returns. Whether it returns a snapshot or an error
// depends on how far the canceled analysis got, so only the return is asserted.
func TestLoadAnalysisContext(t *testing.T) {
	path := writeFixture(t, fixtureJSONL)
	analysisCtx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := Load(context.Background(), path, LoadOptions{AnalysisContext: analysisCtx})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = s.CompletePhase2(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("CompletePhase2 did not return after the analysis context was canceled")
	}

	// nil AnalysisContext = never cancelled: Phase 2 completes normally.
	live, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p2, err := live.CompletePhase2(context.Background()); err != nil || !p2.Phase2Ready {
		t.Fatalf("default analysis context must complete phase 2: %v", err)
	}
}

func TestCompletePhase2(t *testing.T) {
	path := writeFixture(t, fixtureJSONL)
	p1, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := p1.CompletePhase2(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !p2.Phase2Ready || p2.Triage == nil || len(p2.TriageScores) == 0 || p2.Insights.Stats == nil {
		t.Fatalf("phase 2 incomplete: ready=%v triage=%v", p2.Phase2Ready, p2.Triage)
	}
	if p2.Analyzer == p1.Analyzer || p2.IssueMap["A"] == p1.IssueMap["A"] {
		t.Fatal("CompletePhase2 must use a fresh analyzer and detached issues")
	}
	if !p2.ReferenceTime.Equal(p1.ReferenceTime) {
		t.Fatalf("reference time changed: %v -> %v", p1.ReferenceTime, p2.ReferenceTime)
	}
	if p2.DataHash != p1.DataHash || p2.Tier != p1.Tier || p2.CountOpen != p1.CountOpen || p2.AuthorityHash != p1.AuthorityHash {
		t.Fatal("load metadata must carry over")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p1.CompletePhase2(ctx); err == nil {
		t.Fatal("canceled context must fail")
	}
	if got, err := (*Snapshot)(nil).CompletePhase2(context.Background()); got != nil || err != nil {
		t.Fatalf("nil receiver = %v, %v", got, err)
	}
}

func TestLoadNowOverridesReferenceTime(t *testing.T) {
	path := writeFixture(t, fixtureJSONL)
	fixed := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	p1, err := Load(context.Background(), path, LoadOptions{Now: fixed})
	if err != nil {
		t.Fatal(err)
	}
	if !p1.ReferenceTime.Equal(fixed) || !p1.Analyzer.Now().Equal(fixed) {
		t.Fatalf("phase 1 reference time = %v, analyzer now = %v, want %v", p1.ReferenceTime, p1.Analyzer.Now(), fixed)
	}
	p2, err := p1.CompletePhase2(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !p2.ReferenceTime.Equal(fixed) {
		t.Fatalf("phase 2 reference time = %v, want %v", p2.ReferenceTime, fixed)
	}

	// Zero keeps the wall clock.
	wall, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if wall.ReferenceTime.Equal(fixed) || wall.ReferenceTime.IsZero() {
		t.Fatalf("zero Now must use the wall clock, got %v", wall.ReferenceTime)
	}
}
