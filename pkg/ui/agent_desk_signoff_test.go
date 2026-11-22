package ui

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
)

const signoffComment = "bd-kind: signoff\n" +
	`{"what_changed":"clash beat built","calls_made":["kept the stun look","beat at CENTRE"],"where_to_see":"tools/previews/x.tscn","commit":"a1b2c3d"}`

func signoffFixture() []model.Issue {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	older, newer := now.Add(-3*time.Hour), now.Add(-time.Hour)
	parent := []*model.Dependency{{DependsOnID: "e", Type: model.DepParentChild}}
	return []model.Issue{
		{ID: "e", Title: "Clash epic", IssueType: model.TypeEpic, Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now},
		{ID: "e.1", Title: "Old slice", Status: model.StatusClosed, ClosedAt: &older, IssueType: model.TypeTask,
			Labels: []string{"needs-signoff"}, Dependencies: parent, CreatedAt: now, UpdatedAt: now},
		{ID: "e.2", Title: "New slice", Status: model.StatusClosed, ClosedAt: &newer, IssueType: model.TypeTask,
			Labels: []string{"needs-signoff"}, Dependencies: parent, CreatedAt: now, UpdatedAt: now,
			Comments: []*model.Comment{{Author: "lead", Text: signoffComment, CreatedAt: newer}}},
		{ID: "e.3", Title: "Decide", Status: model.StatusOpen, IssueType: model.TypeTask, Labels: []string{"needs-decision"},
			Dependencies: parent, CreatedAt: now, UpdatedAt: now},
	}
}

func newSignoffDesk(t *testing.T) AgentDeskModel {
	t.Helper()
	d := NewAgentDeskModel(DefaultTheme(nil))
	d.SetData(signoffFixture(), time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	d.SetSize(120, 40)
	d.Update(deskKey("D"))
	return d
}

func TestAgentDeskSignoffListAndPreview(t *testing.T) {
	d := newSignoffDesk(t)
	if d.Mode() != deskSignoff || d.PendingSignoffs() != 2 || d.SelectedIssueID() != "e.2" {
		t.Fatalf("mode=%v pending=%d selected=%s", d.Mode(), d.PendingSignoffs(), d.SelectedIssueID())
	}
	view := d.View()
	calls := strings.Index(view, "Calls made on your behalf")
	changed := strings.Index(view, "What changed")
	for _, want := range []string{"Sign-off (2)", "Clash epic", "1h ago", "kept the stun look", "beat at CENTRE", "a1b2c3d", "tools/previews/x.tscn"} {
		if !strings.Contains(view, want) {
			t.Fatalf("sign-off view missing %q:\n%s", want, view)
		}
	}
	if calls < 0 || changed < 0 || calls > changed {
		t.Fatalf("calls made must lead the preview:\n%s", view)
	}
}

func TestAgentDeskSignoffPreviewWrapsLongTitle(t *testing.T) {
	issues := signoffFixture()
	issues[2].Title = strings.Repeat("long words in a title ", 8) + "TAILWORD"
	d := NewAgentDeskModel(DefaultTheme(nil))
	d.SetData(issues, time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	d.SetSize(60, 40)
	d.Update(deskKey("D"))
	_, preview, _ := strings.Cut(d.View(), strings.Repeat("─", 60))
	if !strings.Contains(preview, "TAILWORD") {
		t.Fatalf("preview title truncated instead of wrapped:\n%s", preview)
	}
}

func TestAgentDeskSignoffKeys(t *testing.T) {
	d := newSignoffDesk(t)
	action, _, _ := d.Update(deskKey("a"))
	if action.Kind != deskActionApprove || action.IssueID != "e.2" || d.SelectedIssueID() != "e.1" {
		t.Fatalf("approve: %+v selected=%s", action, d.SelectedIssueID())
	}

	d = newSignoffDesk(t)
	d.Update(deskKey("s"))
	if d.SelectedIssueID() != "e.1" {
		t.Fatal("s should skip to the next bead")
	}

	d = newSignoffDesk(t)
	d.Update(deskKey("d"))
	if !d.Typing() {
		t.Fatal("d should start typing a dispute note")
	}
	if action, _, _ := d.Update(deskKey("enter")); action.Kind != deskActionStatus {
		t.Fatalf("empty dispute must not post: %+v", action)
	}
	for _, r := range "beat too loud, q" {
		d.Update(deskKey(string(r)))
	}
	action, _, _ = d.Update(deskKey("enter"))
	if action.Kind != deskActionDispute || action.IssueID != "e.2" || action.Comment != "beat too loud, q" || d.Typing() {
		t.Fatalf("dispute: %+v typing=%v", action, d.Typing())
	}

	d = newSignoffDesk(t)
	d.Update(deskKey("d"))
	d.Update(deskKey("esc"))
	if d.Typing() || d.SelectedIssueID() != "e.2" {
		t.Fatal("esc should cancel the dispute and stay put")
	}
}

func signoffTrackerModel(t *testing.T) (*Model, model.Issue, string) {
	t.Helper()
	issue, path, capture := installEditorTracker(t, "bd")
	closed := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	issue.Status, issue.ClosedAt = model.StatusClosed, &closed
	issue.Labels = []string{"needs-signoff", "keep-this-label"}
	issue.Comments = []*model.Comment{{Text: signoffComment}}
	data, _ := json.Marshal(issue)
	for _, target := range []string{os.Getenv("BV_TEST_TRACKER_STATE"), path} {
		if err := os.WriteFile(target, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	issues := []model.Issue{issue}
	loader.AttachIssueOrigins(issues, path, true)
	m := NewModel(issues, nil, path)
	t.Cleanup(m.Stop)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(deskKey("D"))
	if m.focused != focusAgentDesk || m.agentDesk.SelectedIssueID() != issue.ID {
		t.Fatalf("D should open the sign-off tab on %s: focus=%v sel=%s", issue.ID, m.focused, m.agentDesk.SelectedIssueID())
	}
	return m, issues[0], capture
}

func trackerOps(t *testing.T, capture string) [][]string {
	t.Helper()
	var ops [][]string
	for _, call := range readEditorTrackerCalls(t, capture) {
		args := call.Args
		for len(args) > 0 && strings.HasPrefix(args[0], "--") {
			if args[0] == "--db" {
				args = args[2:]
			} else {
				args = args[1:]
			}
		}
		if len(args) > 0 && (args[0] == "export" || args[0] == "sync" || args[0] == "show") {
			continue
		}
		ops = append(ops, args)
	}
	return ops
}

func readTrackerState(t *testing.T) model.Issue {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("BV_TEST_TRACKER_STATE"))
	if err != nil {
		t.Fatal(err)
	}
	var issue model.Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		t.Fatal(err)
	}
	return issue
}

func TestModelSignoffApproveNative(t *testing.T) {
	m, issue, capture := signoffTrackerModel(t)
	_, cmd := m.Update(deskKey("a"))
	if result := findQuickEditResult(t, cmd); result.err != nil {
		t.Fatalf("approve failed: %v (%s)", result.err, result.output)
	}
	want := []string{"signoff", "--json", "--approve", "--", issue.Origin.LocalID}
	if ops := trackerOps(t, capture); len(ops) != 1 || !slices.Equal(ops[0], want) {
		t.Fatalf("approve calls: %q", ops)
	}
	saved := readTrackerState(t)
	if slices.Contains(saved.Labels, "needs-signoff") || !strings.Contains(string(saved.Metadata), `"signoff":"approved"`) {
		t.Fatalf("state after approve: labels=%v meta=%s", saved.Labels, saved.Metadata)
	}
}

func TestModelSignoffDisputeNative(t *testing.T) {
	m, issue, capture := signoffTrackerModel(t)
	m.Update(deskKey("d"))
	for _, r := range "-q beat too loud" {
		m.Update(deskKey(string(r)))
	}
	_, cmd := m.Update(deskKey("enter"))
	result := findQuickEditResult(t, cmd)
	if result.err != nil || result.createdID != "local-new" {
		t.Fatalf("dispute failed: %v (%s) created=%q", result.err, result.output, result.createdID)
	}
	want := []string{"signoff", "--json", "--dispute=-q beat too loud", "--", issue.Origin.LocalID}
	if ops := trackerOps(t, capture); len(ops) != 1 || !slices.Equal(ops[0], want) {
		t.Fatalf("dispute calls: %q", ops)
	}
	if saved := readTrackerState(t); saved.Status != model.StatusClosed || !strings.Contains(string(saved.Metadata), `"signoff":"disputed"`) {
		t.Fatalf("state after dispute: status=%s meta=%s", saved.Status, saved.Metadata)
	}
}

func TestModelSignoffApproveFallback(t *testing.T) {
	m, issue, capture := signoffTrackerModel(t)
	t.Setenv("BV_TEST_TRACKER_MODE", "no-signoff")
	_, cmd := m.Update(deskKey("a"))
	if result := findQuickEditResult(t, cmd); result.err != nil {
		t.Fatalf("approve failed: %v (%s)", result.err, result.output)
	}
	want := []string{"update", "--json", "--remove-label=needs-signoff", "--set-metadata=signoff=approved", "--", issue.Origin.LocalID}
	if ops := trackerOps(t, capture); len(ops) != 2 || ops[0][0] != "signoff" || !slices.Equal(ops[1], want) {
		t.Fatalf("approve calls: %q", ops)
	}
	saved := readTrackerState(t)
	if slices.Contains(saved.Labels, "needs-signoff") || saved.Status != model.StatusClosed || !strings.Contains(string(saved.Metadata), `"signoff":"approved"`) {
		t.Fatalf("state after approve: labels=%v status=%s meta=%s", saved.Labels, saved.Status, saved.Metadata)
	}
}

func TestModelSignoffDisputeFallbackFilesFollowUp(t *testing.T) {
	m, issue, capture := signoffTrackerModel(t)
	t.Setenv("BV_TEST_TRACKER_MODE", "no-signoff")
	m.Update(deskKey("d"))
	for _, r := range "beat too loud" {
		m.Update(deskKey(string(r)))
	}
	_, cmd := m.Update(deskKey("enter"))
	result := findQuickEditResult(t, cmd)
	if result.err != nil || result.createdID != "local-new" {
		t.Fatalf("dispute failed: %v (%s) created=%q", result.err, result.output, result.createdID)
	}
	local := issue.Origin.LocalID
	ops := trackerOps(t, capture)
	if len(ops) > 0 && ops[0][0] == "signoff" {
		ops = ops[1:] // the older bd refused signoff; the fallback follows
	}
	want := [][]string{
		{"create", "--json", "--title=Dispute: " + issue.Title, "--description=beat too loud", "--deps=discovered-from:" + local, "--priority=2"},
		{"comments", "add", "--", local, "Disputed: beat too loud (follow-up local-new)"},
		{"update", "--json", "--remove-label=needs-signoff", "--set-metadata=signoff=disputed",
			"--set-metadata=signoff_note=beat too loud", "--set-metadata=signoff_followup=local-new", "--", local},
	}
	if len(ops) != len(want) {
		t.Fatalf("dispute calls: %q", ops)
	}
	for i := range want {
		if !slices.Equal(ops[i], want[i]) {
			t.Fatalf("dispute call %d:\n got %q\nwant %q", i, ops[i], want[i])
		}
	}
	saved := readTrackerState(t)
	if saved.Status != model.StatusClosed || !strings.Contains(string(saved.Metadata), `"signoff":"disputed"`) ||
		!strings.Contains(string(saved.Metadata), `"signoff_followup":"local-new"`) {
		t.Fatalf("closed bead must stay closed and be disputed: status=%s meta=%s", saved.Status, saved.Metadata)
	}
}

func TestModelSignoffDisputeCreateFailureWritesNothingElse(t *testing.T) {
	m, _, capture := signoffTrackerModel(t)
	t.Setenv("BV_TEST_TRACKER_MODE", "create-fail")
	m.Update(deskKey("d"))
	m.Update(deskKey("x"))
	_, cmd := m.Update(deskKey("enter"))
	result := findQuickEditResult(t, cmd)
	if result.err == nil {
		t.Fatal("create failure not reported")
	}
	m.Update(result)
	if !m.statusIsError || m.agentDesk.signDone[result.issueID] != "" {
		t.Fatalf("failure: status=%q marker=%q", m.statusMsg, m.agentDesk.signDone[result.issueID])
	}
	if ops := trackerOps(t, capture); len(ops) != 2 || ops[0][0] != "signoff" || ops[1][0] != "create" {
		t.Fatalf("nothing but the refused signoff and the failed create may run: %q", ops)
	}
}

func TestRenderSignoffMDAndBadges(t *testing.T) {
	issues := signoffFixture()
	md := renderSignoffMD(issues[2])
	if !strings.HasPrefix(md, "### ✍ Awaiting your sign-off") || strings.Index(md, "Calls made") > strings.Index(md, "What changed") ||
		!strings.Contains(md, "- kept the stun look") || !strings.Contains(md, "- a1b2c3d") {
		t.Fatalf("signoff md: %q", md)
	}
	if renderSignoffMD(issues[3]) != "" || renderSignoffMD(issues[0]) != "" {
		t.Fatal("only closed beads pending sign-off get the section")
	}
	if got := renderCommentMD(issues[2].Comments[0], "1h ago", false); !strings.Contains(got, "calls_made:\n>   - kept the stun look") {
		t.Fatalf("signoff comments are never collapsed: %q", got)
	}
	m := NewModel(issues, nil, "")
	defer m.Stop()
	if awaiting, signoffs := m.deskBadgeCounts(); awaiting != 1 || signoffs != 2 {
		t.Fatalf("badges: awaiting=%d signoffs=%d", awaiting, signoffs)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	if footer := m.renderFooter(); !strings.Contains(footer, "✋1") || !strings.Contains(footer, "✍2") {
		t.Fatalf("footer badges missing: %q", footer)
	}
}
