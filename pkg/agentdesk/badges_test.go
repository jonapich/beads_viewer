package agentdesk

import (
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// BadgeCounts must equal the totals of BuildInbox and BuildSignoffs: the nav
// and footer badges are only honest while the lists they announce agree.
func TestBadgeCountsMatchTheLists(t *testing.T) {
	issues := []model.Issue{
		{ID: "dec", Status: model.StatusOpen, Labels: []string{LabelNeedsDecision}},
		{ID: "both", Status: model.StatusInProgress, Labels: []string{LabelNeedsDecision, LabelNeedsDesign}},
		{ID: "design", Status: model.StatusBlocked, Labels: []string{LabelNeedsDesign}},
		{ID: "closed-dec", Status: model.StatusClosed, Labels: []string{LabelNeedsDecision}},
		{ID: "open-so", Status: model.StatusOpen, Labels: []string{LabelNeedsSignoff}},
		{ID: "so", Status: model.StatusClosed, Labels: []string{LabelNeedsSignoff}},
		{ID: "approved", Status: model.StatusClosed, Labels: []string{LabelNeedsSignoff}, Metadata: []byte(`{"signoff":"approved"}`)},
		{ID: "meta-pending", Status: model.StatusClosed, Metadata: []byte(`{"signoff":"pending"}`)},
		{ID: "plain", Status: model.StatusOpen},
	}
	awaiting, signoffs := BadgeCounts(issues)
	if awaiting != 3 || signoffs != 2 {
		t.Fatalf("BadgeCounts = %d, %d; want 3, 2", awaiting, signoffs)
	}
	if got := BuildInbox(issues, nil).Total; got != awaiting {
		t.Errorf("inbox total %d != awaiting %d", got, awaiting)
	}
	if got := BuildSignoffs(issues).Total; got != signoffs {
		t.Errorf("signoff total %d != signoffs %d", got, signoffs)
	}
	if a, s := BadgeCounts(nil); a != 0 || s != 0 {
		t.Errorf("no issues: %d, %d", a, s)
	}
}
