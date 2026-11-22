package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
)

// renderCommentMD renders one comment as a markdown quote for the detail
// pane. A structured comment (a "bd-kind:" header or a JSON object body)
// collapses to a one-line summary unless expand is set, when its fields are
// listed one per line.
func renderCommentMD(c *model.Comment, when string, expand bool) string {
	parsed := agentdesk.ParseComment(c.Text)
	if parsed.Kind == agentdesk.KindSignoff {
		expand = true // a signoff is meant to be read, never collapsed
	}
	if !parsed.Collapsible() {
		return fmt.Sprintf("> **%s** (%s)\n> \n> %s\n\n", c.Author, when, strings.ReplaceAll(c.Text, "\n", "\n> "))
	}
	if !expand {
		return fmt.Sprintf("> **%s** (%s) ▸ %s  _(z expands)_\n\n", c.Author, when, parsed.Summary())
	}
	kind := parsed.Kind
	if kind == "" {
		kind = "json"
	}
	return fmt.Sprintf("> **%s** (%s) ▾ %s\n> \n> ```\n> %s\n> ```\n\n", c.Author, when, kind, strings.ReplaceAll(parsed.Expanded(), "\n", "\n> "))
}

// renderSignoffMD renders the sign-off section that leads the detail pane of
// a closed bead pending sign-off: calls made on the user's behalf first, then
// the other parts, captures and commits. It returns "" for any other bead.
func renderSignoffMD(issue model.Issue) string {
	if !model.IsClosedLikeStatus(issue.Status) || agentdesk.SignoffState(issue) != agentdesk.SignoffPending {
		return ""
	}
	s := agentdesk.ParseSignoff(issue)
	var sb strings.Builder
	sb.WriteString("### ✍ Awaiting your sign-off (agent desk: D)\n\n")
	if !s.HasComment {
		sb.WriteString("_No `bd-kind: signoff` comment on this bead._\n\n")
	}
	for _, part := range []struct {
		title string
		lines []string
	}{
		{"Calls made on your behalf", s.CallsMade},
		{"What changed", s.WhatChanged},
		{"Removed or renamed", s.RemovedOrRenamed},
		{"Where to see it", s.WhereToSee},
		{"Captures", s.Captures},
		{"Commits", s.Commits},
	} {
		if len(part.lines) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "**%s**\n", part.title)
		for _, l := range part.lines {
			fmt.Fprintf(&sb, "- %s\n", l)
		}
		sb.WriteString("\n")
	}
	if s.Raw != "" {
		fmt.Fprintf(&sb, "%s\n\n", s.Raw)
	}
	return sb.String()
}

// renderLinksMD lists the bead's links for the detail pane (typed links
// first, then URLs found in comments), or "" when it has none. Terminals that
// support hyperlinks make the URLs clickable; the agent desk inbox opens them
// with 1-9.
func renderLinksMD(issue model.Issue) string {
	links := agentdesk.Links(issue)
	if len(links) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "### Links (%d)\n", len(links))
	for _, l := range links {
		label := l.Kind
		if l.Source == agentdesk.SourceComment {
			label += " (from a comment)"
		}
		if l.Note != "" {
			label += ": " + l.Note
		}
		fmt.Fprintf(&sb, "- **%s** %s\n", label, l.URL)
	}
	sb.WriteString("\n")
	return sb.String()
}

// deskBadgeCache holds the footer's agent-desk counts for one issue slice, so
// metadata is decoded once per load rather than once per frame.
type deskBadgeCache struct {
	valid              bool
	gen                uint64 // semanticDataGeneration: bumps on every dataset update
	first              *model.Issue
	n                  int
	awaiting, signoffs int
}

// deskBadgeCounts returns the number of open beads awaiting the user
// (needs-decision / needs-design) and of closed beads pending sign-off.
func (m *Model) deskBadgeCounts() (awaiting, signoffs int) {
	issues := m.repoScopedIssues()
	var first *model.Issue
	if len(issues) > 0 {
		first = &issues[0]
	}
	c := &m.deskBadges
	if c.valid && c.gen == m.semanticDataGeneration && c.first == first && c.n == len(issues) {
		return c.awaiting, c.signoffs
	}
	*c = deskBadgeCache{valid: true, gen: m.semanticDataGeneration, first: first, n: len(issues)}
	for i := range issues {
		issue := issues[i]
		if model.IsClosedLikeStatus(issue.Status) {
			if agentdesk.SignoffState(issue) == agentdesk.SignoffPending {
				c.signoffs++
			}
			continue
		}
		for _, l := range agentdesk.AwaitingLabels {
			if slices.Contains(issue.Labels, l) {
				c.awaiting++
				break
			}
		}
	}
	return c.awaiting, c.signoffs
}

// deskNow is the instant pipeline ages are measured from: wall-clock time,
// or the snapshot time while time-traveling.
func (m *Model) deskNow() time.Time {
	if m.timeTravelMode {
		return m.analysisReferenceTime().UTC()
	}
	return time.Now().UTC()
}

// openAgentDesk shows the agent desk on the given tab, or closes it when that
// tab is already showing.
func (m *Model) openAgentDesk(mode deskMode) {
	if m.focused == focusAgentDesk && m.agentDesk.Mode() == mode {
		m.closeAgentDesk()
		return
	}
	m.isGraphView = false
	m.isBoardView = false
	m.isActionableView = false
	m.isHistoryView = false
	m.isSprintView = false
	m.showVelocityComparison = false
	m.focused = focusAgentDesk
	m.agentDesk.SetMode(mode)
	m.refreshAgentDesk()
	m.statusMsg = m.agentDesk.StatusLine()
	m.statusIsError = false
}

func (m *Model) closeAgentDesk() {
	m.agentDesk.SetMode(m.agentDesk.Mode()) // drops a half-typed answer
	m.focused = focusList
	m.statusMsg = ""
	if m.savedViewFocus == focusAgentDesk {
		m.savedViewFocus = focusList
	}
}

// refreshAgentDesk rebuilds the desk from the current issue set.
func (m *Model) refreshAgentDesk() {
	m.agentDesk.SetData(m.repoScopedIssues(), m.deskNow())
	m.agentDesk.SetSize(m.mainContentWidth(), max(3, m.height-1))
}

// handleAgentDeskKeys routes a key to the desk and carries out the action it
// asks for. It reports whether the key was consumed.
func (m *Model) handleAgentDeskKeys(msg tea.KeyMsg) (tea.Cmd, bool) {
	before := m.agentDesk.Mode()
	action, cmd, handled := m.agentDesk.Update(msg)
	if !handled {
		return nil, false
	}
	if m.agentDesk.Mode() != before {
		m.statusMsg, m.statusIsError = m.agentDesk.StatusLine(), false
	}
	switch action.Kind {
	case deskActionClose:
		m.closeAgentDesk()
	case deskActionStatus:
		m.statusMsg, m.statusIsError = action.Status, false
	case deskActionOpenURL:
		if err := openBrowserURL(action.URL); err != nil {
			m.statusMsg, m.statusIsError = fmt.Sprintf("❌ Could not open %s: %v", action.URL, err), true
		} else {
			m.statusMsg, m.statusIsError = "🌐 Opened "+action.URL, false
		}
	case deskActionShowIssue:
		m.closeAgentDesk()
		if m.selectListIssueByID(action.IssueID) {
			m.statusMsg, m.statusIsError = "Showing "+action.IssueID, false
		} else {
			m.statusMsg, m.statusIsError = action.IssueID+" is not in the current list (check the status filter)", true
		}
	case deskActionAnswer:
		return tea.Batch(cmd, m.postDeskAnswer(action)), true
	case deskActionApprove:
		return tea.Batch(cmd, m.signoffDeskBead(action.IssueID, agentdesk.SignoffApproved, "")), true
	case deskActionDispute:
		return tea.Batch(cmd, m.signoffDeskBead(action.IssueID, agentdesk.SignoffDisputed, action.Comment)), true
	}
	return cmd, true
}

// signoffDeskBead approves or disputes a closed bead awaiting sign-off.
//
// On bd it runs `bd signoff --json --approve` or `--dispute=<note>`, which
// does the whole job in one command. A tracker without `signoff` (br, or an
// older bd) gets the equivalent sequence: approve removes needs-signoff and
// sets metadata signoff=approved; dispute files a follow-up bead
// (discovered-from the closed one) with the note as its description, then
// comments on the closed bead and marks it disputed. The closed bead is
// never reopened.
func (m *Model) signoffDeskBead(id, state, note string) tea.Cmd {
	issue := m.resolveLiveIssue(id, nil)
	if issue == nil {
		m.statusMsg, m.statusIsError = "❌ "+id+" is no longer loaded", true
		return nil
	}
	mark := []string{"update", "--json", "--remove-label=" + agentdesk.LabelNeedsSignoff, "--set-metadata=" + agentdesk.SignoffMetaKey + "=" + state}
	var ops []trackerOp
	summary := "Signed off " + issue.ID
	if state == agentdesk.SignoffDisputed {
		local := issue.ID
		if issue.Origin != nil && issue.Origin.LocalID != "" {
			local = issue.Origin.LocalID
		}
		ops = append(ops,
			trackerOp{noTarget: true, captureID: true, args: []string{"create", "--json",
				"--title=" + agentdesk.DisputeTitle(issue.Title), "--description=" + note,
				"--deps=discovered-from:" + local, "--priority=" + strconv.Itoa(issue.Priority)}},
			trackerOp{args: []string{"comments", "add"}, trailing: []string{"Disputed: " + note + " (follow-up {new})"}},
		)
		summary = "Disputed " + issue.ID + " (follow-up filed)"
		mark = append(mark, "--set-metadata=signoff_note="+note, "--set-metadata=signoff_followup={new}")
	}
	ops = append(ops, trackerOp{args: mark})
	if issue.Origin != nil && issue.Origin.Tracker == "bd" {
		native := trackerOp{args: []string{"signoff", "--json", "--approve"}, fallback: ops}
		if state == agentdesk.SignoffDisputed {
			native = trackerOp{args: []string{"signoff", "--json", "--dispute=" + note}, captureID: true, fallback: ops}
		}
		ops = []trackerOp{native}
	}
	cmd := m.runTrackerOps(issue, ops, summary)
	if cmd != nil {
		m.agentDesk.MarkSignedOff(issue.ID, state)
	}
	return cmd
}

// postDeskAnswer writes the answer comment and removes the awaiting label
// through the bead's own tracker, then reloads.
func (m *Model) postDeskAnswer(action deskAction) tea.Cmd {
	issue := m.resolveLiveIssue(action.IssueID, nil)
	if issue == nil {
		m.statusMsg, m.statusIsError = "❌ "+action.IssueID+" is no longer loaded", true
		return nil
	}
	ops := []trackerOp{
		{args: []string{"comments", "add"}, trailing: []string{action.Comment}},
		{args: []string{"update", "--json", "--remove-label=" + action.RemoveLabel}},
	}
	cmd := m.runTrackerOps(issue, ops, fmt.Sprintf("Answered %s (%s removed)", issue.ID, action.RemoveLabel))
	if cmd != nil {
		m.agentDesk.MarkAnswered(issue.ID, action.Comment)
	}
	return cmd
}
