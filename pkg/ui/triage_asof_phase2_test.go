package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
)

// beads-c2o.53: a TUI session without a beads path (--as-of, workspace sources)
// runs without feedback weights, per the robot rule. Phase-2 preparation fills
// in the project directory; it must not load <projectDir>/.beads/feedback.json
// behind that decision.
func TestPreparePhase2_NoBeadsPathStaysUnweighted(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	issues := triageFixtureIssues(now, "")
	// The project has an applicable feedback.json, but the session has no beads path.
	projectBeadsPath, fbWeights := triageFixtureBeadsPath(t, now, "STALE-P3", true)
	projectDir := filepath.Dir(filepath.Dir(projectBeadsPath))

	build := func(t *testing.T) *Model {
		t.Helper()
		m := NewModel(cloneIssuesForAsync(issues), nil, "", ReadinessScope{ReferenceNow: now})
		t.Cleanup(m.Stop)
		m.SetWorkDir(projectDir)
		m.analysis.WaitForPhase2()
		return m
	}

	triageWith := func(m *Model, w *analysis.Weights) analysis.TriageResult {
		az := analysis.NewAnalyzer(cloneIssuesForAsync(issues))
		az.SetNow(now)
		if w != nil {
			az.SetWeights(*w)
		}
		return analysis.ComputeTriageFromAnalyzer(az, m.analysis, cloneIssuesForAsync(issues), analysis.TriageOptions{}, now)
	}

	// Unweighted reference on the same Phase-2 stats.
	ref := build(t)
	plainTriage := triageWith(ref, nil)
	plainScores, _, _, _, _ := extractTriageMaps(plainTriage)

	// The fixture's feedback would change the ranking if it were loaded.
	fbScores, _, _, _, _ := extractTriageMaps(triageWith(ref, &fbWeights))
	if fbScores["STALE-P3"] <= plainScores["STALE-P3"] {
		t.Fatalf("fixture does not move the ranking: weighted=%v default=%v", fbScores["STALE-P3"], plainScores["STALE-P3"])
	}

	ids := []string{"FRESH-P0", "STALE-P3"}
	apply := func(t *testing.T, m *Model) {
		t.Helper()
		cmd := m.preparePhase2Cmd()
		if cmd == nil {
			t.Fatal("preparePhase2Cmd returned no command")
		}
		msg, ok := cmd().(Phase2ReadyMsg)
		if !ok || msg.prepared == nil {
			t.Fatalf("expected a prepared Phase2ReadyMsg, got %#v", msg)
		}
		m.Update(msg)
		want := triageWith(m, nil)
		wantScores, _, _, _, _ := extractTriageMaps(want)
		assertScoresEqual(t, "Phase-2 triageScores", m.triageScores, wantScores, ids...)
		assertScoresEqual(t, "Phase-2 radar", recommendationScores(m.insightsPanel.recommendations),
			recommendationScores(want.Recommendations), ids...)
	}

	t.Run("no phase-1 snapshot (builder branch)", func(t *testing.T) {
		m := build(t)
		if m.snapshot != nil {
			t.Fatal("expected a model without a snapshot")
		}
		apply(t, m)
	})

	t.Run("with a phase-1 snapshot (input branch)", func(t *testing.T) {
		m := build(t)
		builder := NewSnapshotBuilder(cloneIssuesForAsync(issues))
		builder.cfg.SkipPhase2 = true
		builder.analyzer.SetNow(now)
		m.snapshot = builder.Build()
		apply(t, m)
	})
}
