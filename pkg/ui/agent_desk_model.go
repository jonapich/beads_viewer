package ui

import (
	"fmt"
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
	}
	return cmd, true
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
