package snapshot

import "github.com/Dicklesworthstone/beads_viewer/pkg/model"

// CloneIssues returns a deep copy of issues, or nil when issues is empty.
func CloneIssues(issues []model.Issue) []model.Issue {
	if len(issues) == 0 {
		return nil
	}
	clones := make([]model.Issue, len(issues))
	for i := range issues {
		clones[i] = issues[i].Clone()
	}
	return clones
}
