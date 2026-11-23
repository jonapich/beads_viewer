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

func TestRepoKey(t *testing.T) {
	for _, tc := range []struct {
		issue model.Issue
		want  string
	}{
		{model.Issue{ID: "api-AUTH-1"}, "api"},
		{model.Issue{ID: "Web:UI-2"}, "web"},
		{model.Issue{ID: "plainid"}, ""},
		{model.Issue{ID: "toolongprefix-1"}, ""},                      // prefix over 10 chars
		{model.Issue{ID: "a_b-1"}, "a"},                               // "a_b" is not alphanumeric, so the "_" separator is tried next
		{model.Issue{ID: "api-1", SourceRepo: "Billing-"}, "billing"}, // SourceRepo wins, normalized
		{model.Issue{ID: "api-1", SourceRepo: "x/y"}, "api"},          // unusable SourceRepo falls back to the id
		{model.Issue{ID: "api-1", SourceRepo: "."}, "api"},
	} {
		if got := RepoKey(tc.issue); got != tc.want {
			t.Errorf("RepoKey(%q, source %q) = %q, want %q", tc.issue.ID, tc.issue.SourceRepo, got, tc.want)
		}
	}
	if got := NormalizeRepoKey("  API-_: "); got != "api" {
		t.Errorf("NormalizeRepoKey = %q", got)
	}
}

func TestFilterRanks(t *testing.T) {
	targets := []string{"Fix login api-1 open bug", "Cart page", "Login throttling", "unrelated"}
	got := FilterRanks("login", targets)
	if len(got) != 2 {
		t.Fatalf("hits = %+v, want 2", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Score < got[i].Score {
			t.Fatalf("not in rank order: %+v", got)
		}
	}
	if got := FilterRanks("zzzz", targets); got == nil || len(got) != 0 {
		t.Fatalf("no match = %#v, want empty non-nil", got)
	}
	// Equal scores keep target order (sort.Stable).
	same := FilterRanks("a", []string{"a", "a", "a"})
	if len(same) != 3 || same[0].Index != 0 || same[1].Index != 1 || same[2].Index != 2 {
		t.Fatalf("ties = %+v", same)
	}
}
