package agentdesk

import (
	"reflect"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func TestBuildSignoffs(t *testing.T) {
	at := func(h int) *time.Time { v := time.Date(2026, 11, 22, h, 0, 0, 0, time.UTC); return &v }
	parent := func(id string) []*model.Dependency {
		return []*model.Dependency{{DependsOnID: id, Type: model.DepParentChild}}
	}
	issues := []model.Issue{
		{ID: "e", Title: "Epic", IssueType: model.TypeEpic, Status: model.StatusOpen},
		{ID: "e.1", Status: model.StatusClosed, ClosedAt: at(8), Labels: []string{"needs-signoff"}, Dependencies: parent("e")},
		{ID: "e.2", Status: model.StatusClosed, ClosedAt: at(10), Labels: []string{"needs-signoff"}, Dependencies: parent("e")},
		{ID: "e.3", Status: model.StatusClosed, ClosedAt: at(11), Labels: []string{"needs-signoff"}, Dependencies: parent("e"),
			Metadata: []byte(`{"signoff":"approved"}`)},
		{ID: "e.4", Status: model.StatusOpen, Labels: []string{"needs-signoff"}, Dependencies: parent("e")},
		{ID: "z", Status: model.StatusClosed, ClosedAt: at(12), Metadata: []byte(`{"signoff":"pending"}`)},
		{ID: "w", Status: model.StatusClosed, ClosedAt: at(9), Labels: []string{"needs-signoff"}},
	}
	s := BuildSignoffs(issues)
	var ids []string
	for _, it := range s.Items() {
		ids = append(ids, it.ID)
	}
	if s.Total != 4 || !reflect.DeepEqual(ids, []string{"e.2", "e.1", "z", "w"}) {
		t.Fatalf("signoffs: total=%d ids=%v", s.Total, ids)
	}
	if s.Groups[0].EpicTitle != "Epic" || s.Groups[1].Epic != NoEpic {
		t.Fatalf("groups: %+v", s.Groups)
	}
}

func TestParseSignoffJSON(t *testing.T) {
	issue := model.Issue{ID: "x", Status: model.StatusClosed, Labels: []string{"needs-signoff"}, Comments: []*model.Comment{
		{Text: "bd-kind: signoff\n" + `{"what_changed":"clash beat built","calls_made":["kept the stun look","beat at CENTRE"],` +
			`"removed_or_renamed":[],"where_to_see":"tools/previews/x.tscn, %TEMP%/cap/beat_01.png","commit":"a1b2c3d","extra":"note"}`},
	}}
	got := ParseSignoff(issue)
	if !got.HasComment || got.State != SignoffPending {
		t.Fatalf("state: %+v", got)
	}
	if !reflect.DeepEqual(got.CallsMade, []string{"kept the stun look", "beat at CENTRE"}) ||
		!reflect.DeepEqual(got.WhatChanged, []string{"clash beat built"}) ||
		!reflect.DeepEqual(got.Commits, []string{"a1b2c3d"}) ||
		!reflect.DeepEqual(got.Captures, []string{"%TEMP%/cap/beat_01.png"}) || got.Raw != "extra: note" {
		t.Fatalf("parsed: %+v", got)
	}
}

func TestParseSignoffText(t *testing.T) {
	body := "bd-kind: signoff\nWhat changed: the beat\n\n**Calls made on your behalf:**\n- shards thrown goal-side\n- 4 shards, not 6\n" +
		"## Removed or renamed\nnone\nWhere to see it: run the preview, commit 9f8e7d6c\nThanks."
	issue := model.Issue{ID: "x", Status: model.StatusClosed, Comments: []*model.Comment{
		{Text: "bd-kind: signoff\nold one"}, {Text: body},
	}}
	got := ParseSignoff(issue)
	if !reflect.DeepEqual(got.CallsMade, []string{"shards thrown goal-side", "4 shards, not 6"}) ||
		!reflect.DeepEqual(got.WhatChanged, []string{"the beat"}) ||
		!reflect.DeepEqual(got.RemovedOrRenamed, []string{"none"}) ||
		!reflect.DeepEqual(got.WhereToSee, []string{"run the preview, commit 9f8e7d6c", "Thanks."}) ||
		!reflect.DeepEqual(got.Commits, []string{"9f8e7d6c"}) {
		t.Fatalf("parsed: %+v", got)
	}
}

func TestSignoffStateAndDisputeTitle(t *testing.T) {
	if SignoffState(model.Issue{}) != "" {
		t.Fatal("no signoff expected")
	}
	if SignoffState(model.Issue{Labels: []string{"needs-signoff"}, Metadata: []byte(`{"signoff":"disputed"}`)}) != SignoffDisputed {
		t.Fatal("metadata state wins over the label")
	}
	if DisputeTitle("Clash beat") != "Dispute: Clash beat" {
		t.Fatal(DisputeTitle("Clash beat"))
	}
}
