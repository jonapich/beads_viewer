package search

import (
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func TestFilterText(t *testing.T) {
	issue := model.Issue{
		ID: "api-1", Title: "Fix login", Status: model.StatusOpen, IssueType: model.TypeBug,
		Assignee: "ada", Labels: []string{"auth", "urgent"},
	}
	if got, want := FilterText(issue, "api"), "Fix login api-1 open bug ada auth urgent api"; got != want {
		t.Fatalf("FilterText = %q, want %q", got, want)
	}
	bare := model.Issue{ID: "x", Title: "T", Status: model.StatusClosed, IssueType: model.TypeTask}
	if got, want := FilterText(bare, ""), "T x closed task"; got != want {
		t.Fatalf("FilterText without optional parts = %q, want %q", got, want)
	}
}
