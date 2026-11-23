package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/search"
)

// normalizeRepoKey delegates to search.NormalizeRepoKey.
func normalizeRepoKey(raw string) string { return search.NormalizeRepoKey(raw) }

// normalizeRepoPrefixes normalizes workspace repo prefixes (e.g., "api-" -> "api")
// for display and interactive filtering.
func normalizeRepoPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(prefixes))
	var out []string
	for _, raw := range prefixes {
		p := normalizeRepoKey(raw)
		if p == "" {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// issueRepoKey delegates to search.RepoKey.
func issueRepoKey(issue model.Issue) string { return search.RepoKey(issue) }

func sortedRepoKeys(selected map[string]bool) []string {
	if len(selected) == 0 {
		return nil
	}
	out := make([]string, 0, len(selected))
	for k, v := range selected {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// formatRepoList formats a sorted list of repo keys, truncating after maxNames.
// Example: ["api","web","lib"] with maxNames=2 -> "api,web+1".
func formatRepoList(repos []string, maxNames int) string {
	if len(repos) == 0 {
		return ""
	}
	if maxNames <= 0 {
		return fmt.Sprintf("%d repos", len(repos))
	}
	if len(repos) <= maxNames {
		return strings.Join(repos, ",")
	}
	head := strings.Join(repos[:maxNames], ",")
	return fmt.Sprintf("%s+%d", head, len(repos)-maxNames)
}
