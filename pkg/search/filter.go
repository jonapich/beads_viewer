package search

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

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

// RepoKey is the lower-cased repository key the TUI shows and filters by: the
// issue's SourceRepo, else the prefix of its ID ("api-AUTH-1" -> "api"). It is
// the repoPrefix FilterText expects, and is part of the matched text even in a
// single-repo tracker.
func RepoKey(issue model.Issue) string {
	if key := NormalizeRepoKey(issue.SourceRepo); key != "" {
		return key
	}
	return NormalizeRepoKey(ExtractRepoPrefix(issue.ID))
}

// NormalizeRepoKey trims, strips trailing "-:_" and lower-cases a repo key
// ("API-" -> "api"). Keys that are empty, ".", or contain a path separator
// normalize to "".
func NormalizeRepoKey(raw string) string {
	key := strings.TrimSpace(raw)
	key = strings.TrimRight(key, "-:_")
	key = strings.ToLower(key)
	if key == "" || key == "." || strings.ContainsAny(key, `/\`) {
		return ""
	}
	return key
}

// ExtractRepoPrefix extracts the repository prefix from a namespaced issue ID.
// For example, "api-AUTH-123" returns "api", "web-UI-1" returns "web".
// If no prefix is detected (no separator), returns empty string.
func ExtractRepoPrefix(id string) string {
	// Try common separators: -, :, _
	for _, sep := range []string{"-", ":", "_"} {
		if idx := strings.Index(id, sep); idx > 0 {
			// Check if what's before the separator looks like a short prefix (<=10 chars)
			prefix := id[:idx]
			if len(prefix) <= 10 && isAlphanumeric(prefix) {
				return prefix
			}
		}
	}
	return ""
}

func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return len(s) > 0
}

// Match is one fuzzy hit: the index into the searched targets and its score.
type Match struct {
	Index int
	Score int
}

// FilterRanks is the TUI's default list filter: sahilm/fuzzy over targets,
// stable-sorted by score (the bubbles list.DefaultFilter). Hits come back in
// rank order. An empty term matches nothing here (list.DefaultFilter is not
// called with one).
func FilterRanks(term string, targets []string) []Match {
	ranks := fuzzy.Find(term, targets)
	sort.Stable(ranks)
	out := make([]Match, len(ranks))
	for i, r := range ranks {
		out[i] = Match{Index: r.Index, Score: r.Score}
	}
	return out
}
