package agentdesk

import (
	"slices"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// BadgeCounts returns the number of open beads awaiting the user (carrying any
// AwaitingLabels label, the inbox's total) and of closed beads pending sign-off
// (the sign-off list's total). The TUI footer and GET /api/v1/meta share it, so
// the two badges always agree with BuildInbox and BuildSignoffs.
func BadgeCounts(issues []model.Issue) (awaiting, signoffs int) {
	for i := range issues {
		issue := issues[i]
		if !isOpen(&issue) {
			if SignoffState(issue) == SignoffPending {
				signoffs++
			}
			continue
		}
		for _, l := range AwaitingLabels {
			if slices.Contains(issue.Labels, l) {
				awaiting++
				break
			}
		}
	}
	return awaiting, signoffs
}
