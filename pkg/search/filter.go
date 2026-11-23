package search

import (
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// FilterText returns the text the TUI's default list filter matches against for
// an issue: title, ID, status, type, then assignee, labels and the repository
// prefix when present. The HTTP API runs the same fuzzy matcher over these
// strings so both surfaces find the same issues.
func FilterText(issue model.Issue, repoPrefix string) string {
	var sb strings.Builder
	sb.WriteString(issue.Title)
	sb.WriteString(" ")
	sb.WriteString(issue.ID)
	sb.WriteString(" ")
	sb.WriteString(string(issue.Status))
	sb.WriteString(" ")
	sb.WriteString(string(issue.IssueType))

	if issue.Assignee != "" {
		sb.WriteString(" ")
		sb.WriteString(issue.Assignee)
	}

	if len(issue.Labels) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(issue.Labels, " "))
	}

	// Include repo prefix for filtering
	if repoPrefix != "" {
		sb.WriteString(" ")
		sb.WriteString(repoPrefix)
	}

	return sb.String()
}
