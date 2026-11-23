package snapshot

import (
	"path/filepath"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
)

// FeedbackWeights loads feedback.json from beadsDir and returns the adjusted
// factor weights when enough accept/ignore samples exist to apply them; nil
// otherwise.
func FeedbackWeights(beadsDir string) *analysis.Weights {
	if strings.TrimSpace(beadsDir) == "" {
		return nil
	}
	fb, err := analysis.LoadFeedback(beadsDir)
	if err != nil || fb == nil || !fb.Applies() {
		return nil
	}
	w := fb.Weights()
	return &w
}

// FeedbackWeightsForBeadsPath loads .beads/feedback.json next to the issue file
// and returns the adjusted factor weights when enough accept/ignore samples
// exist to apply them (analysis.MinFeedbackSamples); nil otherwise. This is the
// one definition the TUI and the API share, so both rank issues the same way
// --robot-triage does.
func FeedbackWeightsForBeadsPath(beadsPath string) *analysis.Weights {
	if strings.TrimSpace(beadsPath) == "" {
		return nil
	}
	return FeedbackWeights(filepath.Dir(beadsPath))
}

// ExtractTriageMaps flattens a triage result into the per-issue lookup maps the
// views consume: scores, reasons, quick-win and blocker sets, and the unblocks
// map.
func ExtractTriageMaps(triageResult analysis.TriageResult) (
	triageScores map[string]float64,
	triageReasons map[string]analysis.TriageReasons,
	quickWinSet map[string]bool,
	blockerSet map[string]bool,
	unblocksMap map[string][]string,
) {
	scoreCap := len(triageResult.AllScores)
	if scoreCap < len(triageResult.Recommendations) {
		scoreCap = len(triageResult.Recommendations)
	}
	unblockCap := len(triageResult.UnblocksMap)
	if unblockCap < len(triageResult.Recommendations) {
		unblockCap = len(triageResult.Recommendations)
	}
	triageScores = make(map[string]float64, scoreCap)
	triageReasons = make(map[string]analysis.TriageReasons, len(triageResult.Recommendations))
	quickWinSet = make(map[string]bool, len(triageResult.QuickWins))
	blockerSet = make(map[string]bool, len(triageResult.BlockersToClear))
	unblocksMap = make(map[string][]string, unblockCap)

	for _, rec := range triageResult.Recommendations {
		if len(triageResult.AllScores) == 0 {
			triageScores[rec.ID] = rec.Score
		}
		if len(rec.Reasons) > 0 {
			reasonsCopy := make([]string, len(rec.Reasons))
			copy(reasonsCopy, rec.Reasons)
			triageReasons[rec.ID] = analysis.TriageReasons{
				Primary:    reasonsCopy[0],
				All:        reasonsCopy,
				ActionHint: rec.Action,
			}
		}
		if len(triageResult.UnblocksMap) == 0 && len(rec.UnblocksIDs) > 0 {
			cp := make([]string, len(rec.UnblocksIDs))
			copy(cp, rec.UnblocksIDs)
			unblocksMap[rec.ID] = cp
		}
	}
	if len(triageResult.AllScores) > 0 {
		for id, score := range triageResult.AllScores {
			triageScores[id] = score
		}
	}
	if len(triageResult.UnblocksMap) > 0 {
		for id, ids := range triageResult.UnblocksMap {
			if len(ids) > 0 {
				cp := make([]string, len(ids))
				copy(cp, ids)
				unblocksMap[id] = cp
			}
		}
	}
	for _, qw := range triageResult.QuickWins {
		quickWinSet[qw.ID] = true
	}
	for _, bl := range triageResult.BlockersToClear {
		blockerSet[bl.ID] = true
	}
	return triageScores, triageReasons, quickWinSet, blockerSet, unblocksMap
}
