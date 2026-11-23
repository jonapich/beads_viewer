package snapshot

import (
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func blockerFixture() ([]model.Issue, map[string]*model.Issue) {
	blocks := func(id, on string) *model.Dependency {
		return &model.Dependency{IssueID: id, DependsOnID: on, Type: model.DepBlocks}
	}
	issues := []model.Issue{
		{ID: "open-blk", Status: model.StatusOpen},
		{ID: "closed-blk", Status: model.StatusClosed},
		{ID: "work", Status: model.StatusOpen, Dependencies: []*model.Dependency{
			blocks("work", "open-blk"),
			blocks("work", "closed-blk"),
			blocks("work", "open-blk"), // duplicate edge counts once
			{IssueID: "work", DependsOnID: "E", Type: model.DepParentChild},
			nil,
			blocks("work", ""),
		}},
		{ID: "done", Status: model.StatusClosed, Dependencies: []*model.Dependency{blocks("done", "open-blk")}},
		{ID: "tomb-dep", Status: model.StatusTombstone, Dependencies: []*model.Dependency{blocks("tomb-dep", "open-blk")}},
	}
	m := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		m[issues[i].ID] = &issues[i]
	}
	return issues, m
}

func TestIsBlockerUnsatisfied(t *testing.T) {
	issues, m := blockerFixture()
	for _, tc := range []struct {
		issue, blocker string
		want           bool
	}{
		{"work", "", false},
		{"work", "open-blk", true},
		{"work", "closed-blk", false},
		// A blocker outside the visible map and no readiness index: assume unsatisfied.
		{"work", "hidden", true},
	} {
		if got := IsBlockerUnsatisfied(m, nil, tc.issue, tc.blocker); got != tc.want {
			t.Errorf("IsBlockerUnsatisfied(%s,%s) no index = %v, want %v", tc.issue, tc.blocker, got, tc.want)
		}
	}

	// With an index, a hidden blocker is resolved through it. "hidden" is a
	// tombstoned record: visible issues never list it, the authority does.
	all := append(append([]model.Issue(nil), issues...), model.Issue{ID: "hidden", Status: model.StatusTombstone})
	all[2].Dependencies = append(all[2].Dependencies, &model.Dependency{IssueID: "work", DependsOnID: "hidden", Type: model.DepBlocks})
	r := model.NewReadinessIndex(all)
	if IsBlockerUnsatisfied(m, r, "work", "hidden") {
		t.Error("tombstoned hidden blocker must be resolved through the readiness index")
	}
	if !IsBlockerUnsatisfied(m, r, "work", "open-blk") {
		t.Error("visible open blocker must stay unsatisfied")
	}
}

func TestBlockerCounts(t *testing.T) {
	issues, m := blockerFixture()
	active, resolved := BlockerCounts(m, nil, issues[2])
	if active != 1 || resolved != 1 {
		t.Fatalf("work: active=%d resolved=%d, want 1/1 (duplicates, parent-child, nil and empty edges ignored)", active, resolved)
	}
	// A closed dependent still counts an open blocker as active.
	active, resolved = BlockerCounts(m, nil, issues[3])
	if active != 1 || resolved != 0 {
		t.Fatalf("done: active=%d resolved=%d, want 1/0", active, resolved)
	}
	if a, r := BlockerCounts(m, nil, model.Issue{ID: "none"}); a != 0 || r != 0 {
		t.Fatalf("no deps: %d/%d", a, r)
	}
}

func TestActiveDependentsCount(t *testing.T) {
	issues, _ := blockerFixture()
	// "work" is open and blocked by open-blk; "done" and "tomb-dep" are closed-like.
	if got := ActiveDependentsCount(issues, "open-blk"); got != 1 {
		t.Fatalf("open-blk dependents = %d, want 1", got)
	}
	if got := ActiveDependentsCount(issues, "closed-blk"); got != 1 {
		t.Fatalf("closed-blk dependents = %d, want 1", got)
	}
	if got := ActiveDependentsCount(issues, "E"); got != 0 {
		t.Fatalf("parent-child edges are not blocking: %d", got)
	}
	if got := ActiveDependentsCount(issues, ""); got != 0 {
		t.Fatalf("empty id = %d", got)
	}
}

func TestActiveBlockerIDs(t *testing.T) {
	issues, m := blockerFixture()
	got := ActiveBlockerIDs(m, nil, issues[2])
	if len(got) != 1 || got[0] != "open-blk" {
		t.Fatalf("ActiveBlockerIDs(work) = %v, want [open-blk]", got)
	}
	// Never nil, so API arrays serialize as [].
	if got := ActiveBlockerIDs(m, nil, issues[0]); got == nil || len(got) != 0 {
		t.Fatalf("no blockers = %#v, want empty non-nil", got)
	}
	// Agrees with BlockerCounts for every fixture issue.
	for _, iss := range issues {
		active, _ := BlockerCounts(m, nil, iss)
		if n := len(ActiveBlockerIDs(m, nil, iss)); n != active {
			t.Errorf("%s: %d active ids, BlockerCounts says %d", iss.ID, n, active)
		}
	}
}

func TestActiveDependentsCountsMatchesPerID(t *testing.T) {
	issues, _ := blockerFixture()
	counts := ActiveDependentsCounts(issues)
	for _, iss := range issues {
		if got, want := counts[iss.ID], ActiveDependentsCount(issues, iss.ID); got != want {
			t.Errorf("%s: batch %d, per-id %d", iss.ID, got, want)
		}
	}
	if counts["open-blk"] != 1 || counts[""] != 0 {
		t.Fatalf("counts = %v", counts)
	}
}
