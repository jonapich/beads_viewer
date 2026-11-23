package api

import (
	"net/http"
	"strconv"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// Insights list sizes: ?limit=N, 1..maxInsightsLimit, default defaultInsightsLimit.
// The snapshot generates its insight lists at the node count, so an uncapped
// response would be O(N) per list.
const (
	defaultInsightsLimit = 25
	maxInsightsLimit     = 500
)

// handleTriage serves GET /api/v1/triage: the TUI's triage result, close to
// robot-triage but deliberately not identical (ADR 0001 section 3: GUI-specific
// DTOs). It is 503 analysis_pending (Retry-After: 2) while the snapshot has no
// triage yet (large and huge tiers before Phase 2), and 500 analysis_failed
// when Phase 2 failed and there is none to serve.
func (s *server) handleTriage(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	if v.Snap.Triage == nil {
		switch {
		case v.Phase2Err != "":
			writeError(w, http.StatusInternalServerError, "analysis_failed", "triage analysis failed: "+v.Phase2Err)
			return
		case !v.Phase2Ready:
			w.Header().Set("Retry-After", "2")
			writeError(w, http.StatusServiceUnavailable, "analysis_pending", "triage is still being computed")
			return
		}
		// Phase 2 is done and produced no triage: the tracker has nothing to
		// triage (WithPhase2 computes none for zero issues). Serve it empty
		// rather than keep the client polling.
	}
	writeView(w, r, v, func() any { return buildTriage(v) })
}

func orEmpty[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func buildTriage(v *View) TriageResponse {
	t := v.Snap.Triage
	if t == nil {
		t = &analysis.TriageResult{}
	}
	resp := TriageResponse{
		Phase2Ready:     v.Phase2Ready,
		Phase2Error:     v.Phase2Err,
		IssueCount:      t.Meta.IssueCount,
		QuickRef:        t.QuickRef,
		Recommendations: make([]TriageRecommendation, 0, len(t.Recommendations)),
		QuickWins:       orEmpty(t.QuickWins),
		BlockersToClear: make([]analysis.BlockerItem, 0, len(t.BlockersToClear)),
		ProjectHealth:   t.ProjectHealth,
		Alerts:          t.Alerts,
	}
	resp.QuickRef.TopPicks = make([]analysis.TopPick, 0, len(t.QuickRef.TopPicks))
	for _, p := range t.QuickRef.TopPicks {
		p.Reasons = orEmpty(p.Reasons)
		resp.QuickRef.TopPicks = append(resp.QuickRef.TopPicks, p)
	}
	for _, r := range t.Recommendations {
		// Actions (bd argv and working directories) are left out: they leak
		// local paths and the GUI has no use for them.
		breakdown := r.Breakdown
		breakdown.RiskSignals = nil
		resp.Recommendations = append(resp.Recommendations, TriageRecommendation{
			ID: r.ID, Title: r.Title, Type: r.Type, Status: r.Status, Assignee: r.Assignee,
			Priority: r.Priority, Labels: orEmpty(r.Labels), DeferUntil: r.DeferUntil,
			Score: r.Score, Breakdown: breakdown, Action: r.Action, Reasons: orEmpty(r.Reasons),
			UnblocksIDs: orEmpty(r.UnblocksIDs), BlockedBy: r.BlockedBy, Claimable: r.Claimable,
		})
	}
	for _, b := range t.BlockersToClear {
		b.UnblocksIDs = orEmpty(b.UnblocksIDs)
		resp.BlockersToClear = append(resp.BlockersToClear, b)
	}
	// Maps must not serialize as null. by_priority is map[int]int, so its keys
	// are JSON strings ("0".."4"): Record<string, number> in TypeScript.
	c := &resp.ProjectHealth.Counts
	c.ByStatus, c.ByType, c.ByPriority = orEmptyMap(c.ByStatus), orEmptyMap(c.ByType), orEmptyMap(c.ByPriority)
	return resp
}

func orEmptyMap[K comparable, V any](in map[K]V) map[K]V {
	if in == nil {
		return map[K]V{}
	}
	return in
}

// handleInsights serves GET /api/v1/insights?limit=N: the graph-metric lists the
// TUI's Insights panel shows, each capped at limit. Before Phase 2 (and on the
// huge tier, which skips it) the metric lists are empty; phase2_ready and
// phase2_skipped say which.
func (s *server) handleInsights(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	limit := defaultInsightsLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxInsightsLimit {
			writeError(w, http.StatusBadRequest, "bad_request", "limit must be an integer from 1 to "+strconv.Itoa(maxInsightsLimit))
			return
		}
		limit = n
	}
	writeViewTagged(w, r, variantETag(v.ETag, "l"+strconv.Itoa(limit)), func() any { return buildInsights(v, limit) })
}

func buildInsights(v *View, limit int) InsightsResponse {
	snap := v.Snap
	in := snap.Insights
	entries := func(items []analysis.InsightItem) []InsightEntry {
		out := make([]InsightEntry, 0, min(len(items), limit))
		for _, it := range items {
			if len(out) == limit {
				break
			}
			e := InsightEntry{ID: it.ID, Value: it.Value}
			if issue := snap.Issue(it.ID); issue != nil {
				e.Title = issue.Title
			}
			out = append(out, e)
		}
		return out
	}
	capStrings := func(in []string) []string {
		return append([]string{}, in[:min(len(in), limit)]...)
	}
	cycles := in.Cycles[:min(len(in.Cycles), limit)]
	resp := InsightsResponse{
		Phase2Ready:    v.Phase2Ready,
		Phase2Skipped:  snap.Tier == snapshot.TierHuge,
		Phase2Error:    v.Phase2Err,
		ClusterDensity: in.ClusterDensity,
		Bottlenecks:    entries(in.Bottlenecks),
		Keystones:      entries(in.Keystones),
		Influencers:    entries(in.Influencers),
		Hubs:           entries(in.Hubs),
		Authorities:    entries(in.Authorities),
		Cores:          entries(in.Cores),
		Slack:          entries(in.Slack),
		Articulation:   capStrings(in.Articulation),
		Orphans:        capStrings(in.Orphans),
		OrphanCount:    len(in.Orphans),
		Cycles:         make([][]string, 0, len(cycles)),
		CycleCount:     len(in.Cycles),
		Skipped:        map[string]string{},
	}
	// The stats are the live ones Phase 2 writes into, so their status is read
	// only once the store has swapped Phase 2 in: reading it earlier would change
	// the body under an unchanged ETag.
	if v.Phase2Ready && in.Stats != nil {
		resp.Skipped = skippedMetrics(in.Stats)
	}
	for _, c := range cycles {
		resp.Cycles = append(resp.Cycles, append([]string{}, c...))
	}
	return resp
}

// skippedMetrics maps each insight list name to the reason its metric was
// skipped or timed out, as the TUI insights panels show it
// (pkg/ui/insights.go isPanelSkipped): the runtime status first, then, for
// betweenness, HITS and cycles, the config's skip reason. The metric status is
// not sent as is: it has no JSON tags and carries elapsed time.
func skippedMetrics(stats *analysis.GraphStats) map[string]string {
	out := map[string]string{}
	status, cfg := stats.Status(), stats.Config
	add := func(lists []string, state, reason string, configOff bool, configReason string) {
		switch {
		case state == "skipped" || state == "timeout":
			for _, l := range lists {
				out[l] = reason
			}
		case configOff:
			for _, l := range lists {
				out[l] = configReason
			}
		}
	}
	add([]string{"bottlenecks"}, status.Betweenness.State, status.Betweenness.Reason, !cfg.ComputeBetweenness, cfg.BetweennessSkipReason)
	add([]string{"hubs", "authorities"}, status.HITS.State, status.HITS.Reason, !cfg.ComputeHITS, cfg.HITSSkipReason)
	add([]string{"cycles"}, status.Cycles.State, status.Cycles.Reason, !cfg.ComputeCycles, cfg.CyclesSkipReason)
	add([]string{"keystones", "slack"}, status.Critical.State, status.Critical.Reason, false, "")
	add([]string{"influencers"}, status.Eigenvector.State, status.Eigenvector.Reason, false, "")
	add([]string{"cores"}, status.KCore.State, status.KCore.Reason, false, "")
	add([]string{"articulation"}, status.Articulation.State, status.Articulation.Reason, false, "")
	return out
}
