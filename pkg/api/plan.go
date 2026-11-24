package api

import (
	"net/http"

	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// handlePlan serves GET /api/v1/plan: the TUI's execution plan. It uses only
// readiness, connected components and the unblocks map, never the graph
// metrics, so it is served at phase 1 and never answers 503. The body depends
// only on the View, so it takes the plain View ETag: triage_score appears at
// the phase 1 to phase 2 swap, which changes the ETag suffix too.
func (s *server) handlePlan(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	writeView(w, r, v, func() any { return v.plan() })
}

// plan returns the View's plan response, building it on first use.
func (v *View) plan() *PlanResponse {
	v.planOnce.Do(func() { v.planResp = buildPlan(v.Snap) })
	return v.planResp
}

// buildPlan converts analysis.GetExecutionPlan into the GUI DTO. Tracks keep the
// analysis order (component-root id order); empty lists are [], never null.
func buildPlan(snap *snapshot.Snapshot) *PlanResponse {
	resp := &PlanResponse{Tracks: []PlanTrack{}}
	if snap == nil || snap.Analyzer == nil {
		return resp
	}
	plan := snap.Analyzer.GetExecutionPlan()
	resp.TotalActionable = plan.TotalActionable
	resp.TotalBlocked = plan.TotalBlocked
	resp.TotalDeferredOrParked = plan.TotalDeferredOrParked
	resp.TotalInProgress = plan.TotalInProgress
	resp.Summary = plan.Summary
	for _, t := range plan.Tracks {
		track := PlanTrack{TrackID: t.TrackID, Reason: t.Reason, Items: make([]PlanItem, 0, len(t.Items))}
		for _, it := range t.Items {
			item := PlanItem{
				ID: it.ID, Title: it.Title, Status: it.Status, Priority: it.Priority,
				UnblocksIDs: orEmpty(it.UnblocksIDs),
			}
			if issue := snap.Issue(it.ID); issue != nil {
				item.IssueType = string(issue.IssueType)
				item.Assignee = issue.Assignee
			}
			if score, ok := snap.TriageScores[it.ID]; ok {
				item.TriageScore = &score
			}
			track.Items = append(track.Items, item)
		}
		resp.Tracks = append(resp.Tracks, track)
	}
	return resp
}
