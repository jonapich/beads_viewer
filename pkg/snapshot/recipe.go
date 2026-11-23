package snapshot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/recipe"
)

// RecipeName returns the recipe's name, or "" for a nil recipe.
func RecipeName(r *recipe.Recipe) string {
	if r == nil {
		return ""
	}
	return r.Name
}

// RecipeFingerprint returns a stable fingerprint of the recipe's content, or ""
// for a nil recipe.
func RecipeFingerprint(r *recipe.Recipe) string {
	if r == nil {
		return ""
	}

	b, err := json.Marshal(r)
	if err != nil {
		// Fall back to name-only to preserve determinism.
		return r.Name
	}

	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])
}

// ApplyRecipe filters and sorts issues through recipe r using the analyzer,
// graph stats and triage scores as metrics. A nil recipe returns a copy of
// issues.
func ApplyRecipe(issues []model.Issue, analyzer *analysis.Analyzer, stats *analysis.GraphStats, triage map[string]float64, r *recipe.Recipe, now time.Time) ([]model.Issue, error) {
	if r == nil {
		return append([]model.Issue(nil), issues...), nil
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	metrics := recipe.Metrics{Triage: triage}
	if stats != nil {
		metrics.Graph = stats
	}
	if analyzer != nil {
		metrics.Readiness = analyzer.Readiness()
	}
	candidates := issues
	if analyzer != nil && analyzer.HasCandidateScope() {
		candidates = make([]model.Issue, 0, len(issues))
		for _, issue := range issues {
			if analyzer.IsCandidate(issue.ID) {
				candidates = append(candidates, issue)
			}
		}
	}
	return recipe.Apply(candidates, metrics, r, now)
}
