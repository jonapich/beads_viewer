package snapshot

import (
	"slices"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// IsBlockerUnsatisfied reports whether blockerID still blocks issueID. A blocker
// present in issueMap is unsatisfied while it is not closed-like. A blocker
// outside issueMap is resolved through the readiness index r when there is one;
// with no index it counts as unsatisfied. issueMap holds the visible issues and
// r the authority over the full record set.
func IsBlockerUnsatisfied(issueMap map[string]*model.Issue, r *model.ReadinessIndex, issueID, blockerID string) bool {
	if blockerID == "" {
		return false
	}
	if blocker, ok := issueMap[blockerID]; ok && blocker != nil {
		return !isClosedLikeStatus(blocker.Status) && blocker.Status != model.StatusTombstone
	}
	if r != nil {
		if iss, ok := issueMap[issueID]; ok && iss != nil && (isClosedLikeStatus(iss.Status) || iss.Status == model.StatusTombstone) {
			return !slices.Contains(r.ResolvedIDs(), blockerID)
		}
		return slices.Contains(r.Blockers(issueID), blockerID)
	}
	return true
}

// eachBlocker calls fn once per distinct blocking dependency of issue, in
// Dependencies order, with whether the blocker is still unsatisfied.
func eachBlocker(issueMap map[string]*model.Issue, r *model.ReadinessIndex, issue model.Issue, fn func(id string, active bool)) {
	seen := make(map[string]bool, len(issue.Dependencies))
	for _, dep := range issue.Dependencies {
		if dep == nil || !dep.Type.IsBlocking() || dep.DependsOnID == "" || seen[dep.DependsOnID] {
			continue
		}
		seen[dep.DependsOnID] = true
		fn(dep.DependsOnID, IsBlockerUnsatisfied(issueMap, r, issue.ID, dep.DependsOnID))
	}
}

// BlockerCounts counts the distinct blocking dependencies of issue that are
// still unsatisfied (active) and already satisfied (resolved).
func BlockerCounts(issueMap map[string]*model.Issue, r *model.ReadinessIndex, issue model.Issue) (active int, resolved int) {
	eachBlocker(issueMap, r, issue, func(_ string, isActive bool) {
		if isActive {
			active++
		} else {
			resolved++
		}
	})
	return active, resolved
}

// BlockerRef is one blocking dependency of an issue and whether it still blocks.
type BlockerRef struct {
	ID     string
	Active bool
}

// Blockers returns issue's distinct blocking dependencies in Dependencies
// order, active or not. It never returns nil. BlockerCounts and
// ActiveBlockerIDs are views of the same walk.
func Blockers(issueMap map[string]*model.Issue, r *model.ReadinessIndex, issue model.Issue) []BlockerRef {
	refs := []BlockerRef{}
	eachBlocker(issueMap, r, issue, func(id string, active bool) {
		refs = append(refs, BlockerRef{ID: id, Active: active})
	})
	return refs
}

// ActiveBlockerIDs returns the ids of issue's distinct, still unsatisfied
// blocking dependencies, in Dependencies order. It never returns nil.
func ActiveBlockerIDs(issueMap map[string]*model.Issue, r *model.ReadinessIndex, issue model.Issue) []string {
	ids := []string{}
	eachBlocker(issueMap, r, issue, func(id string, active bool) {
		if active {
			ids = append(ids, id)
		}
	})
	return ids
}

// ActiveDependentsCount counts the open issues that have a blocking dependency
// on id.
func ActiveDependentsCount(issues []model.Issue, id string) int {
	if id == "" {
		return 0
	}
	count := 0
	for i := range issues {
		iss := &issues[i]
		if isClosedLikeStatus(iss.Status) || iss.Status == model.StatusTombstone {
			continue
		}
		for _, dep := range iss.Dependencies {
			if dep != nil && dep.Type.IsBlocking() && dep.DependsOnID == id {
				count++
				break
			}
		}
	}
	return count
}

// ActiveDependentsCounts is ActiveDependentsCount for every id at once, in one
// pass over issues instead of one pass per id. Ids with no active dependents
// are absent from the map.
func ActiveDependentsCounts(issues []model.Issue) map[string]int {
	counts := make(map[string]int)
	for i := range issues {
		iss := &issues[i]
		if isClosedLikeStatus(iss.Status) || iss.Status == model.StatusTombstone {
			continue
		}
		var seen map[string]bool
		for _, dep := range iss.Dependencies {
			if dep == nil || !dep.Type.IsBlocking() || dep.DependsOnID == "" {
				continue
			}
			if seen == nil {
				seen = make(map[string]bool, len(iss.Dependencies))
			}
			if !seen[dep.DependsOnID] {
				seen[dep.DependsOnID] = true
				counts[dep.DependsOnID]++
			}
		}
	}
	return counts
}
