package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
)

func deskKey(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func deskFixture() []model.Issue {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	parent := []*model.Dependency{{DependsOnID: "e", Type: model.DepParentChild}}
	started := now.Add(-3 * time.Hour)
	return []model.Issue{
		{ID: "e", Title: "Clash epic", IssueType: model.TypeEpic, Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now},
		{ID: "e.1", Title: "Pick a beat", Status: model.StatusOpen, Priority: 1, IssueType: model.TypeTask, Labels: []string{"needs-decision"},
			Description: "Options: (a) shards; (b, recommended) sparks. Files: x.gd.", Dependencies: parent, CreatedAt: now, UpdatedAt: now},
		{ID: "e.2", Title: "Judge the mock", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, Labels: []string{"needs-design"},
			Dependencies: parent, CreatedAt: now, UpdatedAt: now,
			Comments: []*model.Comment{{Text: "mock published: https://claude.ai/code/artifact/abc", CreatedAt: now}}},
		{ID: "e.3", Title: "Free answer", Status: model.StatusOpen, Priority: 3, IssueType: model.TypeTask, Labels: []string{"needs-decision"},
			Description: "What colour should it be?", Dependencies: parent, CreatedAt: now, UpdatedAt: now},
		{ID: "e.4", Title: "Coding the beat", Status: model.StatusInProgress, IssueType: model.TypeTask, Dependencies: parent,
			CreatedAt: now, UpdatedAt: started, StartedAt: &started,
			Metadata: []byte(`{"stage":"code","claim.session":"sess-9","slice_outcome":"committed","review_rounds":2}`)},
	}
}

func newDesk(t *testing.T) AgentDeskModel {
	t.Helper()
	d := NewAgentDeskModel(DefaultTheme(nil))
	d.SetData(deskFixture(), time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	d.SetSize(120, 40)
	return d
}

func TestAgentDeskInboxOrderAndRender(t *testing.T) {
	d := newDesk(t)
	if d.inbox.Total != 3 || d.inbox.Ready != 1 {
		t.Fatalf("inbox totals: %+v", d.inbox)
	}
	if got := d.SelectedIssueID(); got != "e.2" {
		t.Fatalf("ready-to-judge bead should be first, got %s", got)
	}
	view := d.View()
	for _, want := range []string{"Inbox (3)", "Clash epic", "[ready to judge]", "Judge the mock", "[1] mock (comment)", "claude.ai/code/artifact/abc"} {
		if !strings.Contains(view, want) {
			t.Fatalf("inbox view missing %q:\n%s", want, view)
		}
	}
}

func TestAgentDeskAnswerRecommendedWithTwoKeys(t *testing.T) {
	d := newDesk(t)
	d.Update(deskKey("j")) // e.1
	if d.SelectedIssueID() != "e.1" {
		t.Fatalf("cursor on %s", d.SelectedIssueID())
	}
	if action, _, _ := d.Update(deskKey("enter")); action.Kind != deskActionNone || !d.Answering() {
		t.Fatalf("enter should open the answer panel: %+v", action)
	}
	if !strings.Contains(d.View(), "▸ (b) sparks  ★ recommended") {
		t.Fatalf("recommended option not preselected:\n%s", d.View())
	}
	action, _, _ := d.Update(deskKey("enter"))
	if action.Kind != deskActionAnswer || action.IssueID != "e.1" || action.Comment != "Decision: (b) sparks" || action.RemoveLabel != "needs-decision" {
		t.Fatalf("answer action: %+v", action)
	}
	if d.Answering() || d.SelectedIssueID() != "e.3" {
		t.Fatalf("after posting the cursor should move on: answering=%v selected=%s", d.Answering(), d.SelectedIssueID())
	}
}

func TestAgentDeskAnswerByLetterNoteAndFreeText(t *testing.T) {
	d := newDesk(t)
	d.Update(deskKey("j"))
	d.Update(deskKey("enter"))
	if action, _, _ := d.Update(deskKey("a")); action.Comment != "Decision: (a) shards" {
		t.Fatalf("letter answer: %+v", action)
	}

	d = newDesk(t)
	d.Update(deskKey("j"))
	d.Update(deskKey("enter"))
	d.Update(deskKey("n")) // note on the recommended option
	if !d.Typing() {
		t.Fatal("n should start typing a note")
	}
	for _, r := range "log it" {
		d.Update(deskKey(string(r)))
	}
	if action, _, _ := d.Update(deskKey("enter")); action.Comment != "Decision: (b) sparks — log it" {
		t.Fatalf("note answer: %+v", action)
	}

	// A bead without options goes straight to typing; q is text, not close.
	d = newDesk(t)
	d.Update(deskKey("G"))
	if d.SelectedIssueID() != "e.3" {
		t.Fatalf("G landed on %s", d.SelectedIssueID())
	}
	d.Update(deskKey("enter"))
	if !d.Typing() {
		t.Fatal("no options: enter should start typing")
	}
	if action, _, _ := d.Update(deskKey("enter")); action.Kind != deskActionStatus {
		t.Fatalf("empty answer must not post: %+v", action)
	}
	for _, r := range "teal, quietly" {
		d.Update(deskKey(string(r)))
	}
	action, _, _ := d.Update(deskKey("enter"))
	if action.Kind != deskActionAnswer || action.Comment != "Decision: teal, quietly" {
		t.Fatalf("free answer: %+v", action)
	}

	// esc while typing a free answer to an option-less bead cancels.
	d = newDesk(t)
	d.Update(deskKey("G"))
	d.Update(deskKey("enter"))
	d.Update(deskKey("esc"))
	if d.Answering() {
		t.Fatal("esc should cancel")
	}
}

func TestAgentDeskOpenLinkAndTabs(t *testing.T) {
	d := newDesk(t)
	if action, _, _ := d.Update(deskKey("1")); action.Kind != deskActionOpenURL || action.URL != "https://claude.ai/code/artifact/abc" {
		t.Fatalf("open link 1: %+v", action)
	}
	if action, _, _ := d.Update(deskKey("o")); action.URL != "https://claude.ai/code/artifact/abc" {
		t.Fatalf("o opens the mock: %+v", action)
	}
	if action, _, _ := d.Update(deskKey("2")); action.Kind != deskActionStatus {
		t.Fatalf("missing link 2 should report: %+v", action)
	}
	d.Update(deskKey("tab"))
	if d.Mode() != deskPipeline || d.SelectedIssueID() != "e.4" {
		t.Fatalf("tab → pipeline: mode=%v sel=%s", d.Mode(), d.SelectedIssueID())
	}
	view := d.View()
	for _, want := range []string{"Pipeline (1, 1 stale)", "code", "sess-9", "⚠ no activity for 3h"} {
		if !strings.Contains(view, want) {
			t.Fatalf("pipeline view missing %q:\n%s", want, view)
		}
	}
	if action, _, _ := d.Update(deskKey("enter")); action.Kind != deskActionShowIssue || action.IssueID != "e.4" {
		t.Fatalf("enter in pipeline: %+v", action)
	}
	d.Update(deskKey("M"))
	view = d.View()
	for _, want := range []string{"Slices", "Clash epic", "ALL"} {
		if !strings.Contains(view, want) {
			t.Fatalf("metrics view missing %q:\n%s", want, view)
		}
	}
	if _, _, handled := d.Update(deskKey("x")); handled {
		t.Fatal("unknown keys must fall through")
	}
	if action, _, _ := d.Update(deskKey("q")); action.Kind != deskActionClose {
		t.Fatalf("q closes: %+v", action)
	}
}

func TestModelOpensAndClosesAgentDesk(t *testing.T) {
	m := NewModel(deskFixture(), nil, "")
	defer m.Stop()
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(deskKey("I"))
	if m.focused != focusAgentDesk || m.agentDesk.Mode() != deskInbox {
		t.Fatalf("I should open the inbox: focus=%v", m.focused)
	}
	if !strings.Contains(m.View(), "Inbox (3)") {
		t.Fatalf("inbox not rendered:\n%s", m.View())
	}
	if iss, _ := m.selectedIssueForActionWithReason(); iss == nil || iss.ID != "e.2" {
		t.Fatalf("desk selection should drive issue actions, got %+v", iss)
	}
	m.Update(deskKey("W"))
	if m.focused != focusAgentDesk || m.agentDesk.Mode() != deskPipeline {
		t.Fatalf("W inside the desk switches tabs: focus=%v mode=%v", m.focused, m.agentDesk.Mode())
	}
	m.Update(deskKey("q"))
	if m.focused != focusList {
		t.Fatalf("q should close the desk, focus=%v", m.focused)
	}
	m.Update(deskKey("M"))
	if m.focused != focusAgentDesk || m.agentDesk.Mode() != deskMetrics {
		t.Fatalf("M should open metrics: focus=%v", m.focused)
	}
	// Typing an answer must not trigger global keys such as ? (help).
	m.Update(deskKey("I"))
	m.Update(deskKey("G"))
	m.Update(deskKey("enter"))
	m.Update(deskKey("?"))
	if m.showHelp || !m.agentDesk.Typing() || !m.isTextInputActive() {
		t.Fatalf("typing should capture ?: help=%v typing=%v", m.showHelp, m.agentDesk.Typing())
	}
}

func TestModelAnswerWritesCommentAndRemovesLabel(t *testing.T) {
	issue, path, capture := installEditorTracker(t, "bd")
	issue.Labels = []string{"needs-decision", "keep-this-label"}
	issue.Description = "Options: (a) yes (recommended); (b) no."
	data, _ := json.Marshal(issue)
	for _, target := range []string{os.Getenv("BV_TEST_TRACKER_STATE"), path} {
		if err := os.WriteFile(target, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	issues := []model.Issue{issue}
	loader.AttachIssueOrigins(issues, path, true)

	m := NewModel(issues, nil, path)
	defer m.Stop()
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(deskKey("I"))
	m.Update(deskKey("enter"))
	_, cmd := m.Update(deskKey("enter"))
	if cmd == nil {
		t.Fatalf("answer produced no tracker command: %q", m.statusMsg)
	}
	result := findQuickEditResult(t, cmd)
	if result.err != nil {
		t.Fatalf("answer failed: %v (%s)", result.err, result.output)
	}
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
		ops = append(ops, args)
	}
	if len(ops) < 2 {
		t.Fatalf("expected comment + update calls, got %q", ops)
	}
	wantComment := []string{"comments", "add", "--kind", "answer", "--", issue.Origin.LocalID, "Decision: (a) yes"}
	wantUpdate := []string{"update", "--json", "--remove-label=needs-decision", "--", issue.Origin.LocalID}
	if !slices.Equal(ops[0], wantComment) || !slices.Equal(ops[1], wantUpdate) {
		t.Fatalf("tracker calls:\n got %q\nwant %q then %q", ops, wantComment, wantUpdate)
	}
	state, err := os.ReadFile(os.Getenv("BV_TEST_TRACKER_STATE"))
	if err != nil {
		t.Fatal(err)
	}
	var saved model.Issue
	if err := json.Unmarshal(state, &saved); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(saved.Labels, "needs-decision") || !slices.Contains(saved.Labels, "keep-this-label") ||
		len(saved.Comments) != 1 || saved.Comments[0].Text != "bd-kind: answer\nDecision: (a) yes" {
		t.Fatalf("tracker state after answer: labels=%v comments=%+v", saved.Labels, saved.Comments)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}

func TestModelAnswerFailureKeepsLabelAndClearsMarker(t *testing.T) {
	issue, path, capture := installEditorTracker(t, "bd")
	t.Setenv("BV_TEST_TRACKER_MODE", "comment-fail")
	issue.Labels = []string{"needs-decision"}
	data, _ := json.Marshal(issue)
	for _, target := range []string{os.Getenv("BV_TEST_TRACKER_STATE"), path} {
		if err := os.WriteFile(target, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	issues := []model.Issue{issue}
	loader.AttachIssueOrigins(issues, path, true)
	m := NewModel(issues, nil, path)
	defer m.Stop()
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(deskKey("I"))
	m.Update(deskKey("enter")) // no options: straight to typing
	for _, r := range "go" {
		m.Update(deskKey(string(r)))
	}
	_, cmd := m.Update(deskKey("enter"))
	if m.agentDesk.answered[issue.ID] == "" {
		t.Fatal("answer should be marked while in flight")
	}
	result := findQuickEditResult(t, cmd)
	if result.err == nil {
		t.Fatal("comment failure not reported")
	}
	m.Update(result)
	if !m.statusIsError || m.agentDesk.answered[issue.ID] != "" {
		t.Fatalf("failure: status=%q marker=%q", m.statusMsg, m.agentDesk.answered[issue.ID])
	}
	for _, call := range readEditorTrackerCalls(t, capture) {
		if slices.Contains(call.Args, "update") {
			t.Fatalf("label removed although the comment failed: %q", call.Args)
		}
	}
}

// findQuickEditResult runs cmd (unwrapping batches) until it yields the
// tracker result.
func findQuickEditResult(t *testing.T, cmd tea.Cmd) quickEditResultMsg {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case quickEditResultMsg:
			return msg
		case tea.BatchMsg:
			queue = append(queue, msg...)
		}
	}
	t.Fatal("no quickEditResultMsg produced")
	return quickEditResultMsg{}
}

func TestRenderCommentMDCollapsesHandoffs(t *testing.T) {
	c := &model.Comment{Author: "coder", Text: "bd-kind: handoff\n{\"from_stage\":\"code\",\"to_stage\":\"test\",\"summary\":\"wired it\"}"}
	collapsed := renderCommentMD(c, "1h ago", false)
	if !strings.Contains(collapsed, "▸ handoff [code→test] wired it") || strings.Contains(collapsed, "summary:") {
		t.Fatalf("collapsed: %q", collapsed)
	}
	expanded := renderCommentMD(c, "1h ago", true)
	if !strings.Contains(expanded, "summary: wired it") || !strings.Contains(expanded, "to_stage: test") {
		t.Fatalf("expanded: %q", expanded)
	}
	plain := renderCommentMD(&model.Comment{Author: "me", Text: "line one\nline two"}, "now", false)
	if plain != "> **me** (now)\n> \n> line one\n> line two\n\n" {
		t.Fatalf("plain comment changed: %q", plain)
	}
}

func TestRenderLinksMD(t *testing.T) {
	if got := renderLinksMD(model.Issue{}); got != "" {
		t.Fatalf("no links: %q", got)
	}
	got := renderLinksMD(model.Issue{Metadata: []byte(`{"link.mock":[{"url":"https://claude.ai/code/artifact/m","note":"v2"}]}`)})
	if !strings.Contains(got, "### Links (1)") || !strings.Contains(got, "**mock: v2** https://claude.ai/code/artifact/m") {
		t.Fatalf("links md: %q", got)
	}
}

func TestModelToggleHandoffExpansion(t *testing.T) {
	m := NewModel(deskFixture(), nil, "")
	defer m.Stop()
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(deskKey("z"))
	if !m.expandHandoffs {
		t.Fatal("z should expand handoff comments")
	}
	m.Update(deskKey("z"))
	if m.expandHandoffs {
		t.Fatal("z should collapse handoff comments again")
	}
}
