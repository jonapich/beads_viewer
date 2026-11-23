package ui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/search"
)

// TestFilterRanksMatchesTUIDefaultFilter pins the HTTP API's /search to the
// TUI's list filter: for the same targets (IssueItem.FilterValue) and term,
// search.FilterRanks must rank exactly as list.DefaultFilter does.
func TestFilterRanksMatchesTUIDefaultFilter(t *testing.T) {
	issues := []model.Issue{
		{ID: "api-AUTH-1", Title: "Add login flow", Status: model.StatusOpen, IssueType: model.TypeFeature, Labels: []string{"auth", "backend"}, Assignee: "ada"},
		{ID: "api-AUTH-2", Title: "Login throttling", Status: model.StatusInProgress, IssueType: model.TypeTask, Labels: []string{"security"}},
		{ID: "web-UI-7", Title: "Cart page layout", Status: model.StatusOpen, IssueType: model.TypeTask, SourceRepo: "Web-"},
		{ID: "web-UI-8", Title: "Checkout cart totals", Status: model.StatusClosed, IssueType: model.TypeBug, Assignee: "grace"},
		{ID: "lib_core:3", Title: "Refactor the cache", Status: model.StatusBlocked, IssueType: model.TypeChore},
		{ID: "plainid", Title: "No prefix at all", Status: model.StatusOpen, IssueType: model.TypeTask},
		{ID: "x-1", Title: "Authentication audit", Status: model.StatusOpen, IssueType: model.TypeEpic, Labels: []string{"auth"}},
	}
	targets := make([]string, len(issues))
	for i, issue := range issues {
		// The TUI's item text: FilterValue over the issue and its repo key.
		item := IssueItem{Issue: issue, RepoPrefix: issueRepoKey(issue)}
		targets[i] = item.FilterValue()
		if want := search.FilterText(issue, search.RepoKey(issue)); targets[i] != want {
			t.Fatalf("target %d = %q, want %q", i, targets[i], want)
		}
	}

	for _, term := range []string{"login", "cart", "auth", "api", "web", "ada", "closed", "bug", "ca", "xyz-nothing", "A", "lib", "refactor cache", "feature"} {
		var want []int
		for _, r := range list.DefaultFilter(term, targets) {
			want = append(want, r.Index)
		}
		var got []int
		for _, m := range search.FilterRanks(term, targets) {
			got = append(got, m.Index)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("term %q: FilterRanks order %v, list.DefaultFilter order %v", term, got, want)
		}
	}
}

// TestRepoKeyDelegates keeps the pkg/ui helpers and the lifted search helpers
// in step.
func TestRepoKeyDelegates(t *testing.T) {
	for _, issue := range []model.Issue{
		{ID: "api-AUTH-1"}, {ID: "plainid"}, {ID: "web-UI-7", SourceRepo: "Web-"}, {ID: "a/b-1", SourceRepo: "x/y"}, {ID: "toolongprefix-1"},
	} {
		if got, want := issueRepoKey(issue), search.RepoKey(issue); got != want {
			t.Errorf("%s: issueRepoKey %q, search.RepoKey %q", issue.ID, got, want)
		}
	}
	if got := ExtractRepoPrefix("api-AUTH-123"); got != "api" {
		t.Errorf("ExtractRepoPrefix = %q", got)
	}
}
