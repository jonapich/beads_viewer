package ui

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// These tests pin beads-c2o.31: every TUI triage site ranks with the same
// feedback-weighted scoring the snapshot (and so bv serve) uses. The weights
// ride on the analyzer (SetWeights at load); ComputeTriageFromAnalyzer ignores
// TriageOptions.Weights.

const triageScoreEpsilon = 1e-9

// triageFixtureIssues returns one fresh P0 and one stale P3 open issue, the
// pair whose ranking feedback.json moves (Staleness x2, PriorityBoost x0.5).
func triageFixtureIssues(now time.Time, prefix string) []model.Issue {
	return []model.Issue{
		{
			ID:        prefix + "FRESH-P0",
			Title:     "Fresh high priority issue",
			Status:    model.StatusOpen,
			Priority:  0,
			IssueType: model.TypeTask,
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        prefix + "STALE-P3",
			Title:     "Stale lower priority issue",
			Status:    model.StatusOpen,
			Priority:  3,
			IssueType: model.TypeTask,
			CreatedAt: now.Add(-35 * 24 * time.Hour),
			UpdatedAt: now.Add(-35 * 24 * time.Hour),
		},
	}
}

// triageFixtureBeadsPath creates <tmp>/.beads/issues.jsonl and, when withFeedback
// is set, a feedback.json whose adjustments are applied. It returns the issues
// path and the weights the feedback resolves to.
func triageFixtureBeadsPath(t *testing.T, now time.Time, staleID string, withFeedback bool) (string, analysis.Weights) {
	t.Helper()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	beadsPath := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(beadsPath, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !withFeedback {
		return beadsPath, analysis.DefaultWeights()
	}
	fb := analysis.DefaultFeedbackData()
	for i := 0; i < analysis.MinFeedbackSamples; i++ {
		fb.Events = append(fb.Events, analysis.FeedbackEvent{
			IssueID:   staleID,
			Action:    "accept",
			Score:     0.6,
			Timestamp: now,
		})
	}
	fb.Adjustments = []analysis.WeightAdjustment{
		{Name: "Staleness", Adjustment: 2.0, Samples: analysis.MinFeedbackSamples, LastUpdated: now},
		{Name: "PriorityBoost", Adjustment: 0.5, Samples: analysis.MinFeedbackSamples, LastUpdated: now},
	}
	if err := fb.Save(beadsDir); err != nil {
		t.Fatalf("save feedback: %v", err)
	}
	return beadsPath, fb.Weights()
}

func recommendationScores(recs []analysis.Recommendation) map[string]float64 {
	out := make(map[string]float64, len(recs))
	for _, r := range recs {
		out[r.ID] = r.Score
	}
	return out
}

func recommendationOrder(recs []analysis.Recommendation) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

func assertScoresEqual(t *testing.T, what string, got, want map[string]float64, ids ...string) {
	t.Helper()
	for _, id := range ids {
		g, gok := got[id]
		w, wok := want[id]
		if !gok || !wok {
			t.Fatalf("%s: %s missing (got present=%v, want present=%v)", what, id, gok, wok)
		}
		if math.Abs(g-w) > triageScoreEpsilon {
			t.Fatalf("%s: %s score=%v, want %v", what, id, g, w)
		}
	}
}

// Site A: NewModel's initial triage (list rows until Phase 2 replaces them).
// Phase 2 may land mid-test, so the pin compares the STALE-P3 minus FRESH-P0
// score gap: Phase 2 metrics are identical for the two unconnected issues and
// cancel out of it.
func TestNewModel_TriageUsesFeedbackWeights(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	issues := triageFixtureIssues(now, "")
	weightedPath, fbWeights := triageFixtureBeadsPath(t, now, "STALE-P3", true)
	plainPath, _ := triageFixtureBeadsPath(t, now, "STALE-P3", false)

	mWeighted := NewModel(cloneIssuesForAsync(issues), nil, weightedPath, ReadinessScope{ReferenceNow: now})
	t.Cleanup(mWeighted.Stop)
	mPlain := NewModel(cloneIssuesForAsync(issues), nil, plainPath, ReadinessScope{ReferenceNow: now})
	t.Cleanup(mPlain.Stop)

	gap := func(scores map[string]float64) float64 {
		return scores["STALE-P3"] - scores["FRESH-P0"]
	}
	weightedGap := gap(mWeighted.triageScores)
	plainGap := gap(mPlain.triageScores)
	if weightedGap <= plainGap {
		t.Fatalf("feedback weights did not lift STALE-P3 relative to FRESH-P0 in NewModel triage: weighted gap=%v plain gap=%v", weightedGap, plainGap)
	}

	// The same triage computed by hand on an analyzer holding the feedback weights.
	az := analysis.NewAnalyzer(cloneIssuesForAsync(issues))
	az.SetNow(now)
	az.SetWeights(fbWeights)
	want := analysis.ComputeTriageFromAnalyzer(az, mWeighted.analysis, cloneIssuesForAsync(issues), analysis.TriageOptions{}, now)
	wantScores, _, _, _, _ := extractTriageMaps(want)
	if math.Abs(weightedGap-gap(wantScores)) > triageScoreEpsilon {
		t.Fatalf("NewModel triage gap=%v, want %v from a feedback-weighted analyzer", weightedGap, gap(wantScores))
	}

	// And the list rows carry those scores.
	for _, item := range mWeighted.list.Items() {
		row, ok := item.(IssueItem)
		if !ok {
			continue
		}
		if row.TriageScore != mWeighted.triageScores[row.Issue.ID] {
			t.Fatalf("list row %s TriageScore=%v, want %v", row.Issue.ID, row.TriageScore, mWeighted.triageScores[row.Issue.ID])
		}
	}
}

// Sites E/F and the prepared Phase-2 path: after Phase 2 the TUI's triage maps
// and Priority Radar equal what the snapshot builder (bv serve's source)
// produces for the same beads path.
func TestPhase2Triage_MatchesSnapshotWeights(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	issues := triageFixtureIssues(now, "")
	beadsPath, _ := triageFixtureBeadsPath(t, now, "STALE-P3", true)

	// Reference: the snapshot builder with the same beads path, Phase 2 applied.
	refBuilder := NewSnapshotBuilder(cloneIssuesForAsync(issues)).WithBeadsPath(beadsPath)
	refBuilder.cfg.SkipPhase2 = true
	refBuilder.analyzer.SetNow(now)
	refSnap := refBuilder.Build()
	refAz := analysis.NewAnalyzer(cloneIssuesForAsync(issues))
	refAz.SetNow(now)
	refStats := refAz.AnalyzeAsync(nil)
	refStats.WaitForPhase2()
	refSnap = refSnap.WithPhase2(refStats, refStats.GenerateInsights(len(issues)), cloneIssuesForAsync(issues), refAz)
	if refSnap.phase2Triage == nil {
		t.Fatal("reference snapshot has no Phase-2 triage")
	}
	ids := []string{"FRESH-P0", "STALE-P3"}

	// A default-weight reference proves the fixture actually moves the ranking.
	plainAz := analysis.NewAnalyzer(cloneIssuesForAsync(issues))
	plainAz.SetNow(now)
	plainTriage := analysis.ComputeTriageFromAnalyzer(plainAz, refStats, cloneIssuesForAsync(issues), analysis.TriageOptions{}, now)
	plainScores, _, _, _, _ := extractTriageMaps(plainTriage)
	if refSnap.TriageScores["STALE-P3"] <= plainScores["STALE-P3"] {
		t.Fatalf("fixture does not move the ranking: weighted=%v default=%v", refSnap.TriageScores["STALE-P3"], plainScores["STALE-P3"])
	}

	newModel := func(t *testing.T) *Model {
		t.Helper()
		m := NewModel(cloneIssuesForAsync(issues), nil, beadsPath, ReadinessScope{ReferenceNow: now})
		t.Cleanup(m.Stop)
		m.analysis.WaitForPhase2()
		return m
	}
	check := func(t *testing.T, m *Model) {
		t.Helper()
		assertScoresEqual(t, "m.triageScores vs snapshot", m.triageScores, refSnap.TriageScores, ids...)
		assertScoresEqual(t, "radar scores vs snapshot", recommendationScores(m.insightsPanel.recommendations),
			recommendationScores(refSnap.phase2Triage.Recommendations), ids...)
		gotOrder := recommendationOrder(m.insightsPanel.recommendations)
		wantOrder := recommendationOrder(refSnap.phase2Triage.Recommendations)
		if len(gotOrder) != len(wantOrder) {
			t.Fatalf("radar order=%v, want %v", gotOrder, wantOrder)
		}
		for i := range wantOrder {
			if gotOrder[i] != wantOrder[i] {
				t.Fatalf("radar order=%v, want %v", gotOrder, wantOrder)
			}
		}
	}

	t.Run("recompute fallback (no snapshot, nothing prepared)", func(t *testing.T) {
		m := newModel(t)
		if m.snapshot != nil {
			t.Fatal("expected a model without a snapshot")
		}
		updated, _ := m.Update(Phase2ReadyMsg{Stats: m.analysis, Insights: m.analysis.GenerateInsights(len(m.issues))})
		check(t, updated.(*Model))
	})

	t.Run("prepared Phase 2", func(t *testing.T) {
		m := newModel(t)
		cmd := m.preparePhase2Cmd()
		if cmd == nil {
			t.Fatal("preparePhase2Cmd returned no command")
		}
		msg, ok := cmd().(Phase2ReadyMsg)
		if !ok || msg.prepared == nil {
			t.Fatalf("expected a prepared Phase2ReadyMsg, got %#v", msg)
		}
		updated, _ := m.Update(msg)
		check(t, updated.(*Model))
	})
}

// Site B: refreshScopedTriageData after a snapshot arrives for a scoped
// (candidate IDs and reference time) session keeps the feedback weights.
func TestRefreshScopedTriage_KeepsFeedbackWeights(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	issues := append(triageFixtureIssues(now, ""), model.Issue{
		ID: "CONTEXT-P1", Title: "Context only", Status: model.StatusOpen, Priority: 1, IssueType: model.TypeTask,
		CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-48 * time.Hour),
	})
	candidates := map[string]bool{"FRESH-P0": true, "STALE-P3": true}
	weightedPath, fbWeights := triageFixtureBeadsPath(t, now, "STALE-P3", true)

	m := NewModel(cloneIssuesForAsync(issues), nil, weightedPath, ReadinessScope{CandidateIDs: candidates, ReferenceNow: now})
	t.Cleanup(m.Stop)

	// The worker's snapshot, built for the same beads path (Phase 1 only).
	builder := NewSnapshotBuilder(cloneIssuesForAsync(issues)).WithBeadsPath(weightedPath)
	builder.cfg.SkipPhase2 = true
	builder.analyzer.SetNow(now)
	snap := builder.Build()

	updated, _ := m.Update(SnapshotReadyMsg{Snapshot: snap, SnapshotVer: 1})
	m = updated.(*Model)
	if m.snapshot != snap {
		t.Fatal("snapshot was not applied")
	}
	if m.analyzer == snap.Analyzer {
		t.Fatal("scoped session should own its analyzer rather than share the worker's")
	}

	az := analysis.NewAnalyzer(cloneIssuesForAsync(issues))
	az.SetNow(now)
	az.SetReadinessScope(snap.Analyzer.Readiness(), candidates)
	az.SetWeights(fbWeights)
	want := analysis.ComputeTriageFromAnalyzer(az, m.analysis, m.issues, analysis.TriageOptions{}, now)
	wantScores, _, _, _, _ := extractTriageMaps(want)
	assertScoresEqual(t, "scoped triageScores", m.triageScores, wantScores, "FRESH-P0", "STALE-P3")

	// Default-weight scoring ranks STALE-P3 lower, so this is not vacuous.
	plainAz := analysis.NewAnalyzer(cloneIssuesForAsync(issues))
	plainAz.SetNow(now)
	plainAz.SetReadinessScope(snap.Analyzer.Readiness(), candidates)
	plainTriage := analysis.ComputeTriageFromAnalyzer(plainAz, m.analysis, m.issues, analysis.TriageOptions{}, now)
	plainScores, _, _, _, _ := extractTriageMaps(plainTriage)
	if m.triageScores["STALE-P3"] <= plainScores["STALE-P3"] {
		t.Fatalf("scoped triage was not feedback-weighted: got=%v default=%v", m.triageScores["STALE-P3"], plainScores["STALE-P3"])
	}
}

// Site C: the workspace repo-filtered Insights rebuild builds a fresh analyzer
// over the scoped issues. It must inherit the TUI analyzer's scoring.
func TestRebuildInsightsPanel_WorkspaceScopedInheritsAnalyzerWeights(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	issues := append(triageFixtureIssues(now, "api-"), triageFixtureIssues(now, "web-")...)

	m := NewModel(cloneIssuesForAsync(issues), nil, "", ReadinessScope{ReferenceNow: now})
	t.Cleanup(m.Stop)
	m.workspaceMode = true
	m.availableRepos = []string{"api", "web"}
	m.activeRepos = map[string]bool{"api": true}

	weights := analysis.DefaultWeights()
	weights.Staleness *= 3
	weights.PriorityBoost *= 0.2
	m.analyzer.SetWeights(weights)

	m.rebuildInsightsPanel()
	got := recommendationScores(m.insightsPanel.recommendations)

	scoped := m.repoScopedIssues()
	if len(scoped) != 2 {
		t.Fatalf("expected 2 api issues in scope, got %d", len(scoped))
	}
	candidates := map[string]bool{}
	for _, issue := range scoped {
		candidates[issue.ID] = true
	}
	reference := func(w analysis.Weights) map[string]float64 {
		az := analysis.NewAnalyzer(cloneIssuesForAsync(scoped))
		az.SetNow(now)
		az.SetWeights(w)
		az.SetReadinessScope(m.analyzer.Readiness(), candidates)
		stats := az.Analyze()
		triage := analysis.ComputeTriageFromAnalyzer(az, &stats, cloneIssuesForAsync(scoped), analysis.TriageOptions{}, now)
		return recommendationScores(triage.Recommendations)
	}
	ids := []string{"api-FRESH-P0", "api-STALE-P3"}
	assertScoresEqual(t, "scoped Priority Radar vs weighted analyzer", got, reference(weights), ids...)

	// Default weights would score differently, so the equality above is not vacuous.
	defaults := reference(analysis.DefaultWeights())
	if math.Abs(got["api-STALE-P3"]-defaults["api-STALE-P3"]) <= triageScoreEpsilon {
		t.Fatalf("test weights do not move api-STALE-P3: weighted=%v default=%v", got["api-STALE-P3"], defaults["api-STALE-P3"])
	}
}
