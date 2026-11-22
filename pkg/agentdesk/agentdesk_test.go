package agentdesk

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []Option
	}{
		{
			name: "inline lettered with recommended marker and trailing field",
			text: "Context.\nOptions: (a, recommended) the smash button fires an Aim even when charges are off (charging stays disabled); (b) keep the mask, the Aim fires on its timeout. Files: scenes/match/match.gd. Stats: none.",
			want: []Option{
				{Key: "a", Text: "the smash button fires an Aim even when charges are off (charging stays disabled)", Recommended: true},
				{Key: "b", Text: "keep the mask, the Aim fires on its timeout"},
			},
		},
		{
			name: "inline with built-meanwhile field",
			text: "Options: (a, recommended) yes, a clash: add ClashEnd.DASH; (b) no, keep as is. Built meanwhile: (b). Files: src/sim/match/pong_match.gd.",
			want: []Option{
				{Key: "a", Text: "yes, a clash: add ClashEnd.DASH", Recommended: true},
				{Key: "b", Text: "no, keep as is"},
			},
		},
		{
			name: "two-way or with recommended parenthetical",
			text: "Options: keep (recommended: one card everywhere, the epic's point) or set bare on podium plates. Capture: x.png. Files: a.gd.",
			want: []Option{
				{Key: "a", Text: "keep (recommended: one card everywhere, the epic's point)", Recommended: true},
				{Key: "b", Text: "set bare on podium plates"},
			},
		},
		{
			name: "block lettered list after header",
			text: "Which way?\n\nOptions:\nA) Widen first\nB) Shrink first (Recommended)\nC) Neither\n\nMore prose.",
			want: []Option{
				{Key: "a", Text: "Widen first"},
				{Key: "b", Text: "Shrink first", Recommended: true},
				{Key: "c", Text: "Neither"},
			},
		},
		{
			name: "lettered list without header, continuation line",
			text: "Pick one:\n- **A** blue glow\n  with sparks\n- **B** red glow\nDone.",
			want: []Option{
				{Key: "a", Text: "blue glow with sparks"},
				{Key: "b", Text: "red glow"},
			},
		},
		{
			name: "bullets under header",
			text: "Options:\n- ship it\n- hold it (recommended)\n",
			want: []Option{
				{Key: "a", Text: "ship it"},
				{Key: "b", Text: "hold it", Recommended: true},
			},
		},
		{
			name: "block list with letter then parenthetical",
			text: "Context.\n\nOptions:\nA (recommended, built meanwhile): keep lending the base card.\nB: lend Brick Wall+ instead.\n\nFiles: x.gd",
			want: []Option{
				{Key: "a", Text: "keep lending the base card", Recommended: true},
				{Key: "b", Text: "lend Brick Wall+ instead"},
			},
		},
		{
			name: "inline bare capitals after header",
			text: "Options: A (recommended) keep it: the line is the slot's own sign. B drop it and redraw the slot glyph. Files: a.gd.",
			want: []Option{
				{Key: "a", Text: "keep it: the line is the slot's own sign", Recommended: true},
				{Key: "b", Text: "drop it and redraw the slot glyph"},
			},
		},
		{
			name: "inline close-paren capitals with a later RECOMMENDED letter",
			text: "QUESTION: where? Options: A) on the offer; B) in the Forge; C) in the pause screen. RECOMMENDED: B first, then C. Built meanwhile: nothing.",
			want: []Option{
				{Key: "a", Text: "on the offer"},
				{Key: "b", Text: "in the Forge", Recommended: true},
				{Key: "c", Text: "in the pause screen"},
			},
		},
		{
			name: "paren letters without header",
			text: "Question: when the plate cannot fit the flair, (a) drop the flair (recommended: fewer rules), or (b) shrink the flair below its floor. Files: q.gd.",
			want: []Option{
				{Key: "a", Text: "drop the flair (recommended: fewer rules)", Recommended: true},
				{Key: "b", Text: "shrink the flair below its floor"},
			},
		},
		{
			name: "option text opening with recommended",
			text: "Options: (a) recommended: seed every trait; (b) author fields per rival.",
			want: []Option{
				{Key: "a", Text: "seed every trait", Recommended: true},
				{Key: "b", Text: "author fields per rival"},
			},
		},
		{
			name: "question with one or",
			text: "Bots wait late. Question: keep the wait (default, built) or skip it for a bump-sent shot too? Decide after playing.",
			want: []Option{
				{Key: "a", Text: "keep the wait (default, built)"},
				{Key: "b", Text: "skip it for a bump-sent shot too"},
			},
		},
		{
			name: "no options",
			text: "Goal: when a card meets its opposite (a stun or a stagger), the game says so.",
			want: nil,
		},
		{
			name: "single lettered item is not a choice",
			text: "Options: (a) only this.",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseOptions(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseOptions:\n got %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func TestDecisionComment(t *testing.T) {
	opt := &Option{Key: "b", Text: "keep the mask"}
	if got := DecisionComment(opt, ""); got != "Decision: (b) keep the mask" {
		t.Fatalf("option: %q", got)
	}
	if got := DecisionComment(nil, "  go with C  "); got != "Decision: go with C" {
		t.Fatalf("free: %q", got)
	}
	if got := DecisionComment(opt, "but log it"); got != "Decision: (b) keep the mask — but log it" {
		t.Fatalf("both: %q", got)
	}
}

func meta(t *testing.T, raw string) []byte {
	t.Helper()
	return []byte(raw)
}

func TestLinksTypedThenCommentURLs(t *testing.T) {
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	issue := model.Issue{
		ID:       "x-1",
		Metadata: meta(t, `{"link.mock":[{"url":"https://claude.ai/code/artifact/old","at":"2026-11-20T10:00:00Z"},{"url":"https://claude.ai/code/artifact/new","note":"v2","at":"2026-11-21T10:00:00Z","by":"poet"}],"link.commit":[{"url":"https://github.com/o/r/commit/abc"}],"stage":"code"}`),
		Comments: []*model.Comment{
			{Text: "see (https://example.com/a_(b)) and https://claude.ai/code/artifact/new.", Author: "me", CreatedAt: at},
			{Text: "mock published: https://claude.ai/code/artifact/m1, done", Author: "fx", CreatedAt: at.Add(time.Hour)},
		},
	}
	got := Links(issue)
	var urls, kinds []string
	for _, l := range got {
		urls = append(urls, l.URL)
		kinds = append(kinds, l.Kind+"/"+l.Source)
	}
	wantURLs := []string{
		"https://github.com/o/r/commit/abc",
		"https://claude.ai/code/artifact/new",
		"https://claude.ai/code/artifact/old",
		"https://claude.ai/code/artifact/m1",
		"https://example.com/a_(b)",
	}
	wantKinds := []string{"commit/typed", "mock/typed", "mock/typed", "mock/comment", "url/comment"}
	if !reflect.DeepEqual(urls, wantURLs) || !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("Links:\n urls %v\nkinds %v", urls, kinds)
	}
	if got[1].Note != "v2" || got[1].By != "poet" {
		t.Fatalf("typed link fields lost: %+v", got[1])
	}
}

func TestParseCommentHandoff(t *testing.T) {
	p := ParseComment("bd-kind: handoff\n{\"from_stage\":\"code\",\"to_stage\":\"test\",\"summary\":\"wired the beat\",\"next\":\"run full suite\"}")
	if p.Kind != KindHandoff || p.Data == nil || !p.Collapsible() {
		t.Fatalf("parse: %+v", p)
	}
	if got := p.Summary(); got != "handoff [code→test] wired the beat · next: run full suite" {
		t.Fatalf("summary: %q", got)
	}
	if !strings.Contains(p.Expanded(), "next: run full suite") {
		t.Fatalf("expanded: %q", p.Expanded())
	}

	keysOnly := ParseComment("bd-kind: handoff\n{\"files\":[\"a\"],\"beads\":2}")
	if got := keysOnly.Summary(); got != "handoff {beads, files}" {
		t.Fatalf("keys summary: %q", got)
	}

	plain := ParseComment("User 2026-11-21: plain words {not json}")
	if plain.Collapsible() || plain.Kind != "" {
		t.Fatalf("plain comment collapsed: %+v", plain)
	}
}

func TestEpicIndex(t *testing.T) {
	issues := []model.Issue{
		{ID: "e", IssueType: model.TypeEpic},
		{ID: "e.1", IssueType: model.TypeFeature, Dependencies: []*model.Dependency{{IssueID: "e.1", DependsOnID: "e", Type: model.DepParentChild}}},
		{ID: "e.1.1", Dependencies: []*model.Dependency{{IssueID: "e.1.1", DependsOnID: "e.1", Type: model.DepParentChild}}},
		{ID: "root"},
		{ID: "root.1", Dependencies: []*model.Dependency{{DependsOnID: "root", Type: model.DepParentChild}}},
		{ID: "loose"},
		{ID: "c1", Dependencies: []*model.Dependency{{DependsOnID: "c2", Type: model.DepParentChild}}},
		{ID: "c2", Dependencies: []*model.Dependency{{DependsOnID: "c1", Type: model.DepParentChild}}},
	}
	x := NewEpicIndex(issues)
	for id, want := range map[string]string{"e.1.1": "e", "e": "e", "root.1": "root", "loose": NoEpic, "root": NoEpic} {
		if got := x.EpicOf(id); got != want {
			t.Errorf("EpicOf(%s) = %q, want %q", id, got, want)
		}
	}
	_ = x.EpicOf("c1") // a parent cycle must terminate
}

func TestBuildInbox(t *testing.T) {
	issues := []model.Issue{
		{ID: "e", Title: "Epic", IssueType: model.TypeEpic, Status: model.StatusOpen},
		{ID: "e.1", Title: "decide", Status: model.StatusOpen, Priority: 2, Labels: []string{"needs-decision"},
			Description: "Options: (a) yes; (b, recommended) no.", Dependencies: []*model.Dependency{{DependsOnID: "e", Type: model.DepParentChild}}},
		{ID: "e.2", Title: "design ready", Status: model.StatusOpen, Priority: 3, Labels: []string{"needs-design"},
			Metadata:     []byte(`{"link.artifact":[{"url":"https://claude.ai/code/artifact/z"}]}`),
			Dependencies: []*model.Dependency{{DependsOnID: "e", Type: model.DepParentChild}}},
		{ID: "e.3", Title: "design waiting", Status: model.StatusOpen, Priority: 1, Labels: []string{"needs-design"},
			Description:  "Open: stuff.\nNext artifact: poet direction, then an fx-coder HTML slider mock; the user picks.\nFiles: a.gd",
			Dependencies: []*model.Dependency{{DependsOnID: "e", Type: model.DepParentChild}}},
		{ID: "e.4", Title: "closed", Status: model.StatusClosed, Labels: []string{"needs-decision"}},
		{ID: "z", Title: "loose", Status: model.StatusInProgress, Labels: []string{"needs-decision", "needs-design"}},
		{ID: "n", Title: "not awaiting", Status: model.StatusOpen, Labels: []string{"slice"}},
	}
	in := BuildInbox(issues, nil)
	if in.Total != 4 || in.Ready != 1 || len(in.Groups) != 2 {
		t.Fatalf("totals: %+v", in)
	}
	g := in.Groups[0]
	if g.Epic != "e" || g.EpicTitle != "Epic" || g.Ready != 1 {
		t.Fatalf("first group: %+v", g)
	}
	var ids []string
	for _, it := range g.Items {
		ids = append(ids, it.ID)
	}
	if !reflect.DeepEqual(ids, []string{"e.2", "e.3", "e.1"}) {
		t.Fatalf("order: %v", ids)
	}
	if !g.Items[0].ReadyToJudge || g.Items[0].LinkCount() != 1 || g.Items[0].WaitingOn != "" {
		t.Fatalf("ready item: %+v", g.Items[0])
	}
	if g.Items[1].WaitingOn != "poet direction, then an fx-coder HTML slider mock" {
		t.Fatalf("waiting on: %q", g.Items[1].WaitingOn)
	}
	if opts := g.Items[2].Options; len(opts) != 2 || Recommended(opts) != 1 {
		t.Fatalf("options: %+v", opts)
	}
	loose := in.Groups[1]
	if loose.Epic != NoEpic || loose.Items[0].Label != LabelNeedsDecision || loose.Items[0].AnswerLabel() != LabelNeedsDecision {
		t.Fatalf("loose group: %+v", loose)
	}
	if got := len(in.Items()); got != 4 {
		t.Fatalf("Items() = %d", got)
	}
}

func TestWaitingOnFallbacks(t *testing.T) {
	if got := WaitingOn("We need a capture of the podium. Then decide."); got != "We need a capture of the podium" {
		t.Fatalf("prose: %q", got)
	}
	long := "Next artifact: " + strings.Repeat("a long mock clause ", 12)
	if got := WaitingOn(long); len(got) > maxWaitingOn+len("…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("long waiting-on not capped: %q", got)
	}
	if got := WaitingOn("Nothing named."); got != "an artifact" {
		t.Fatalf("none: %q", got)
	}
}

func TestBuildPipeline(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	in := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	issues := []model.Issue{
		{ID: "live", Status: model.StatusInProgress, Assignee: "lead", UpdatedAt: now.Add(-5 * time.Hour), LeaseExpiresAt: in(10 * time.Minute),
			Metadata: []byte(`{"stage":"test","stage_changed_at":"2026-11-22T11:15:00Z","claim.session":"s-1"}`)},
		{ID: "expired", Status: model.StatusInProgress, UpdatedAt: now, LeaseExpiresAt: ago(20 * time.Minute),
			Metadata: []byte(`{"stage":"code"}`), StartedAt: ago(time.Hour)},
		{ID: "quiet", Status: model.StatusInProgress, UpdatedAt: now.Add(-3 * time.Hour), StartedAt: ago(4 * time.Hour)},
		{ID: "scouting", Status: model.StatusInProgress, UpdatedAt: now.Add(-10 * time.Minute), Metadata: []byte(`{"stage":"scout","claim.at":"2026-11-22T11:50:00Z"}`)},
		{ID: "open", Status: model.StatusOpen},
	}
	p := BuildPipeline(issues, now, 0)
	var ids []string
	for _, r := range p.Rows {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []string{"expired", "quiet", "scouting", "live"}) {
		t.Fatalf("order: %v", ids)
	}
	if p.Total != 4 || p.Stale != 2 || p.ByStage["test"] != 1 || p.ByStage["unstaged"] != 1 {
		t.Fatalf("summary: %+v", p)
	}
	byID := map[string]PipelineRow{}
	for _, r := range p.Rows {
		byID[r.ID] = r
	}
	if r := byID["expired"]; r.StaleReason != "lease expired 20m ago" || r.StageAge != "1h" {
		t.Fatalf("expired: %+v", r)
	}
	if r := byID["quiet"]; r.StaleReason != "no activity for 3h" {
		t.Fatalf("quiet: %+v", r)
	}
	if r := byID["live"]; r.Stale || r.Session != "s-1" || r.Holder != "lead" || r.StageAge != "45m" {
		t.Fatalf("live: %+v", r)
	}
	if r := byID["scouting"]; r.Stale || r.StageAge != "10m" {
		t.Fatalf("scouting: %+v", r)
	}
}

func TestBuildSliceReport(t *testing.T) {
	parent := func(id string) []*model.Dependency {
		return []*model.Dependency{{DependsOnID: id, Type: model.DepParentChild}}
	}
	issues := []model.Issue{
		{ID: "e", Title: "Epic", IssueType: model.TypeEpic},
		{ID: "e.1", Dependencies: parent("e"), Metadata: []byte(`{"slice_outcome":"committed","review_rounds":1,"test_runs":4,"test_red_runs":1,"slice_wall_minutes":30,"coder_tokens":1000,"reviewer_tokens":500,"other_agent_tokens":"250"}`)},
		{ID: "e.2", Dependencies: parent("e"), Metadata: []byte(`{"slice_outcome":"stopped","review_rounds":3,"test_runs":2,"slice_wall_minutes":90,"coder_tokens":2000}`)},
		{ID: "f.1", Metadata: []byte(`{"slice_outcome":"committed","review_rounds":2}`)},
		{ID: "bug1", Metadata: []byte(`{"caused_by":"e.1"}`)},
		{ID: "bug2", Metadata: []byte(`{"caused_by":"gone"}`)},
		{ID: "plain"},
	}
	r := BuildSliceReport(issues)
	if len(r.Epics) != 2 {
		t.Fatalf("epics: %+v", r.Epics)
	}
	e := r.Epics[0]
	if e.Epic != "e" || e.EpicTitle != "Epic" || e.Slices != 2 || e.Committed != 1 || e.Stopped != 1 ||
		e.ReviewRounds != 4 || e.FirstPass != 1 || e.TestRuns != 6 || e.TestRedRuns != 1 || e.WallMinutes != 120 ||
		e.CoderTokens != 3000 || e.ReviewerTokens != 500 || e.OtherTokens != 250 || e.TokensUnknown != 1 || e.EscapedBugs != 1 {
		t.Fatalf("epic e: %+v", e)
	}
	if e.AvgReviewRounds() != 2 || e.AvgWallMinutes() != 60 || e.TotalTokens() != 3750 {
		t.Fatalf("derived: %v %v %v", e.AvgReviewRounds(), e.AvgWallMinutes(), e.TotalTokens())
	}
	if r.Total.Slices != 3 || r.Total.EscapedBugs != 1 {
		t.Fatalf("total: %+v", r.Total)
	}
	if !reflect.DeepEqual(r.UnmatchedEscapes, []EscapedBug{{Bug: "bug2", Cause: "gone"}}) {
		t.Fatalf("unmatched: %+v", r.UnmatchedEscapes)
	}
}

func TestFormatAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "30s", 12 * time.Minute: "12m", 3*time.Hour + 5*time.Minute: "3h05m",
		2 * time.Hour: "2h", 52 * time.Hour: "2d4h", 48 * time.Hour: "2d",
	} {
		if got := FormatAge(d); got != want {
			t.Errorf("FormatAge(%v) = %q, want %q", d, got, want)
		}
	}
}
