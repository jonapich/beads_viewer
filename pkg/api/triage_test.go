package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

func TestTriageResponse(t *testing.T) {
	h := goldenHandler(t)
	rec := do(h, "GET", "/api/v1/triage", withSession())
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == "" {
		t.Fatalf("triage: %d etag=%q", rec.Code, rec.Header().Get("ETag"))
	}
	body := rec.Body.String()
	var resp TriageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Phase2Ready || resp.IssueCount == 0 || len(resp.Recommendations) == 0 || len(resp.QuickRef.TopPicks) == 0 {
		t.Fatalf("triage = %+v", resp)
	}
	// Local paths and volatile fields stay out of the GUI payload.
	for _, banned := range []string{`"actions"`, `"commands"`, `"argv"`, `"working_directory"`, `"generated_at"`, `"compute_time_ms"`, `"risk_signals"`} {
		if strings.Contains(body, banned) {
			t.Errorf("triage must not contain %s", banned)
		}
	}
	// Arrays and maps are never null.
	for _, field := range []string{`"quick_wins":[`, `"blockers_to_clear":[`, `"by_status":{`, `"by_type":{`, `"by_priority":{`} {
		if !strings.Contains(body, field) {
			t.Errorf("triage lacks %s", field)
		}
	}
	for _, r := range resp.Recommendations {
		if r.Labels == nil || r.Reasons == nil || r.UnblocksIDs == nil {
			t.Errorf("%s: labels/reasons/unblocks_ids must be []", r.ID)
		}
	}
	for _, b := range resp.BlockersToClear {
		if b.UnblocksIDs == nil {
			t.Errorf("blocker %s: unblocks_ids must be []", b.ID)
		}
	}
	// by_priority has JSON-string integer keys.
	var raw struct {
		ProjectHealth struct {
			Counts struct {
				ByPriority map[string]int `json:"by_priority"`
			} `json:"counts"`
		} `json:"project_health"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil || len(raw.ProjectHealth.Counts.ByPriority) == 0 {
		t.Fatalf("by_priority = %v, %v", raw.ProjectHealth.Counts.ByPriority, err)
	}

	etag := rec.Header().Get("ETag")
	cond := do(h, "GET", "/api/v1/triage", withSession(), func(r *http.Request) { r.Header.Set("If-None-Match", etag) })
	if cond.Code != http.StatusNotModified {
		t.Errorf("matching ETag: %d", cond.Code)
	}
	if do(h, "POST", "/api/v1/triage", withSession(), withOrigin(localOrigin)).Code != http.StatusMethodNotAllowed {
		t.Error("POST must be 405")
	}
	if do(newTestHandler(t, nil), "GET", "/api/v1/triage", withSession()).Code != http.StatusServiceUnavailable {
		t.Error("no store must be 503")
	}
}

// withoutTriage loads like snapshot.Load but drops the precomputed triage, the
// state of a large or huge tier snapshot before Phase 2.
func withoutTriage(ctx context.Context, path string, opts snapshot.LoadOptions) (*snapshot.Snapshot, error) {
	s, err := snapshot.Load(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	c := *s
	c.Triage = nil
	return &c, nil
}

func TestTriagePendingThenReady(t *testing.T) {
	gate := make(chan struct{})
	store, err := newSeamStore(t, withoutTriage, func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return realComplete(ctx, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(t, func(c *Config) { c.Store = store })

	rec := do(h, "GET", "/api/v1/triage", withSession())
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("pending: %d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if e := decodeError(t, rec); e.Code != "analysis_pending" {
		t.Fatalf("code = %q", e.Code)
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatal("an error must carry no ETag")
	}

	close(gate)
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	rec = do(h, "GET", "/api/v1/triage", withSession())
	var resp TriageResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || !resp.Phase2Ready {
		t.Fatalf("after phase 2: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTriageFailedWhenPhase2FailsWithoutTriage(t *testing.T) {
	store, err := newSeamStore(t, withoutTriage, func(context.Context, *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		return nil, errors.New("analysis blew up")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	rec := do(h, "GET", "/api/v1/triage", withSession())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != "analysis_failed" || !strings.Contains(e.Message, "analysis blew up") {
		t.Fatalf("error = %+v", e)
	}
}

func TestTriageServedAtPhase1WhenPrecomputed(t *testing.T) {
	// Small tier: triage is precomputed, so it is served before Phase 2 with
	// phase2_ready false; the client re-reads it when the ETag changes.
	gate := make(chan struct{})
	store := newGatedStore(t, writeGoldenTracker(t), gate)
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	rec := do(h, "GET", "/api/v1/triage", withSession())
	var resp TriageResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || resp.Phase2Ready {
		t.Fatalf("phase 1 triage: %d ready=%v", rec.Code, resp.Phase2Ready)
	}
	p1 := rec.Header().Get("ETag")
	close(gate)
	waitPhase2(t, store)
	rec = do(h, "GET", "/api/v1/triage", withSession())
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == p1 {
		t.Fatalf("phase 2 must change the ETag: %d %q", rec.Code, rec.Header().Get("ETag"))
	}
}

func getInsights(t *testing.T, h http.Handler, query string) (InsightsResponse, *http.Response) {
	t.Helper()
	rec := do(h, "GET", "/api/v1/insights"+query, withSession())
	var resp InsightsResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
	}
	return resp, rec.Result()
}

func TestInsightsLimitAndShape(t *testing.T) {
	h := goldenHandler(t)

	full, res := getInsights(t, h, "")
	if res.StatusCode != http.StatusOK || !full.Phase2Ready || full.Phase2Skipped {
		t.Fatalf("default: %d %+v", res.StatusCode, full)
	}
	if len(full.Authorities) != len(goldenIssues()) || full.OrphanCount != len(full.Orphans) {
		t.Fatalf("default limit 25 keeps all 12 rows: %d authorities, %d/%d orphans", len(full.Authorities), len(full.Orphans), full.OrphanCount)
	}
	if full.Authorities[0].Title == "" || full.CycleCount != 1 || len(full.Cycles) != 1 {
		t.Fatalf("titles and cycles: %+v", full)
	}

	capped, _ := getInsights(t, h, "?limit=3")
	for name, n := range map[string]int{
		"bottlenecks": len(capped.Bottlenecks), "keystones": len(capped.Keystones), "influencers": len(capped.Influencers),
		"hubs": len(capped.Hubs), "authorities": len(capped.Authorities), "cores": len(capped.Cores), "slack": len(capped.Slack),
		"articulation": len(capped.Articulation), "orphans": len(capped.Orphans), "cycles": len(capped.Cycles),
	} {
		if n > 3 {
			t.Errorf("%s has %d entries, want at most 3", name, n)
		}
	}
	if len(capped.Orphans) != 3 || capped.OrphanCount != full.OrphanCount || capped.CycleCount != full.CycleCount {
		t.Errorf("totals must survive the cap: orphans %d count %d cycle_count %d", len(capped.Orphans), capped.OrphanCount, capped.CycleCount)
	}
	// The capped list is the head of the full one.
	for i, e := range capped.Authorities {
		if e != full.Authorities[i] {
			t.Errorf("authorities[%d] = %+v, want %+v", i, e, full.Authorities[i])
		}
	}

	// Empty lists are [] not null.
	rec := do(h, "GET", "/api/v1/insights?limit=1", withSession())
	for _, field := range []string{`"bottlenecks":[`, `"keystones":[`, `"slack":[`, `"cycles":[`, `"articulation":[`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("insights lacks %s", field)
		}
	}
	if _, res := getInsights(t, h, "?limit=500"); res.StatusCode != http.StatusOK {
		t.Errorf("limit=500: %d", res.StatusCode)
	}
}

func TestInsightsLimitValidation(t *testing.T) {
	h := goldenHandler(t)
	for _, q := range []string{"?limit=0", "?limit=-1", "?limit=501", "?limit=abc", "?limit=1.5", "?limit=" + strconv.Itoa(1<<40)} {
		rec := do(h, "GET", "/api/v1/insights"+q, withSession())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, rec.Code)
			continue
		}
		if e := decodeError(t, rec); e.Code != "bad_request" {
			t.Errorf("%s: code %q", q, e.Code)
		}
		if rec.Header().Get("ETag") != "" {
			t.Errorf("%s: an error must carry no ETag", q)
		}
	}
	if do(h, "POST", "/api/v1/insights", withSession(), withOrigin(localOrigin)).Code != http.StatusMethodNotAllowed {
		t.Error("POST must be 405")
	}
	if do(newTestHandler(t, nil), "GET", "/api/v1/insights", withSession()).Code != http.StatusServiceUnavailable {
		t.Error("no store must be 503")
	}
}

func TestInsightsETagBoundToLimitAndPhase(t *testing.T) {
	gate := make(chan struct{})
	store := newGatedStore(t, writeGoldenTracker(t), gate)
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	tag := func(q string) string {
		rec := do(h, "GET", "/api/v1/insights"+q, withSession())
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", q, rec.Code)
		}
		return rec.Header().Get("ETag")
	}
	status := func(q, etag string) int {
		return do(h, "GET", "/api/v1/insights"+q, withSession(), func(r *http.Request) { r.Header.Set("If-None-Match", etag) }).Code
	}

	p1, p1five := tag(""), tag("?limit=5")
	if p1 == p1five {
		t.Fatal("different limits need different ETags")
	}
	if status("", p1) != http.StatusNotModified || status("?limit=5", p1) != http.StatusOK {
		t.Error("ETag must be bound to its limit")
	}
	// Phase 1 body says so, and the swap changes the tag.
	early, _ := getInsights(t, h, "")
	if early.Phase2Ready {
		t.Error("phase 1 insights must not claim phase2_ready")
	}
	close(gate)
	waitPhase2(t, store)
	if status("", p1) != http.StatusOK {
		t.Error("a phase 1 tag must not validate the phase 2 body")
	}
	late, _ := getInsights(t, h, "")
	if !late.Phase2Ready || len(late.Authorities) == 0 {
		t.Errorf("phase 2 insights = %+v", late)
	}
}

func TestInsightsPhase2SkippedFollowsTier(t *testing.T) {
	huge := func(ctx context.Context, path string, opts snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		s, err := snapshot.Load(ctx, path, opts)
		if err != nil {
			return nil, err
		}
		c := *s
		c.Tier = snapshot.TierHuge
		return &c, nil
	}
	store, err := newSeamStore(t, huge, realComplete)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	resp, res := getInsights(t, h, "")
	if res.StatusCode != http.StatusOK || !resp.Phase2Skipped {
		t.Fatalf("huge tier: %d skipped=%v", res.StatusCode, resp.Phase2Skipped)
	}
}

func TestTriageEmptyTrackerAfterPhase2(t *testing.T) {
	// Zero issues: phase 1 computes triage but WithPhase2 computes none, so the
	// phase 2 view has no triage. That is "nothing to triage", not "pending".
	h := customHandler(t, nil)
	rec := do(h, "GET", "/api/v1/triage", withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("empty tracker triage: %d %s", rec.Code, rec.Body.String())
	}
	var resp TriageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Phase2Ready || resp.IssueCount != 0 {
		t.Fatalf("empty triage = %+v", resp)
	}
	for _, field := range []string{`"recommendations":[]`, `"top_picks":[]`, `"quick_wins":[]`, `"blockers_to_clear":[]`, `"by_status":{}`, `"by_type":{}`, `"by_priority":{}`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("empty triage lacks %s: %s", field, rec.Body.String())
		}
	}
	// Insights on the same tracker are an empty, well-formed body.
	rec = do(h, "GET", "/api/v1/insights", withSession())
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"skipped":{`) {
		t.Fatalf("empty insights: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTriageCarriesPhase2ErrorWhenServedFromPhase1(t *testing.T) {
	store, err := newSeamStore(t, snapshot.Load, func(context.Context, *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		return nil, errors.New("analysis failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	rec := do(h, "GET", "/api/v1/triage", withSession())
	var resp TriageResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || resp.Phase2Ready || resp.Phase2Error != "analysis failed" {
		t.Fatalf("phase 1 triage after a failed phase 2: %d %s", rec.Code, rec.Body.String())
	}
}

func TestInsightsSkippedReasons(t *testing.T) {
	gate := make(chan struct{})
	store, err := newSeamStore(t, snapshot.Load, func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		next, err := realComplete(ctx, s)
		if err == nil {
			// A metric the size policy would skip on a big graph.
			next.Insights.Stats.Config.ComputeBetweenness = false
			next.Insights.Stats.Config.BetweennessSkipReason = "graph too large for betweenness"
			next.Insights.Stats.Config.ComputeHITS = false
			next.Insights.Stats.Config.HITSSkipReason = "graph too large for HITS"
		}
		return next, err
	})
	if err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(t, func(c *Config) { c.Store = store })

	// Phase 1: the live stats are still being written, so nothing is read from them.
	early, _ := getInsights(t, h, "")
	if early.Phase2Ready || early.Skipped == nil || len(early.Skipped) != 0 {
		t.Fatalf("phase 1 skipped = %#v (ready=%v)", early.Skipped, early.Phase2Ready)
	}
	close(gate)
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	late, _ := getInsights(t, h, "")
	want := map[string]string{
		"bottlenecks": "graph too large for betweenness",
		"hubs":        "graph too large for HITS",
		"authorities": "graph too large for HITS",
	}
	// The fixture tracker also has a blocking cycle, which makes the runtime
	// status skip the critical-path metrics: keystones and slack.
	if !late.Phase2Ready || len(late.Skipped) != len(want)+2 {
		t.Fatalf("phase 2 skipped = %#v", late.Skipped)
	}
	for k, v := range want {
		if late.Skipped[k] != v {
			t.Errorf("skipped[%s] = %q, want %q", k, late.Skipped[k], v)
		}
	}
	for _, k := range []string{"keystones", "slack"} {
		if !strings.Contains(late.Skipped[k], "cycle") {
			t.Errorf("skipped[%s] = %q, want the cycle reason from the metric status", k, late.Skipped[k])
		}
	}
	// An acyclic tracker skips nothing, and the field is still an object.
	acyclic, _ := getInsights(t, customHandler(t, []model.Issue{
		{ID: "a-1", Title: "A", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9)},
	}), "")
	if acyclic.Skipped == nil || len(acyclic.Skipped) != 0 {
		t.Fatalf("acyclic skipped = %#v", acyclic.Skipped)
	}
}

func TestTriageAppliesFeedbackWeights(t *testing.T) {
	// feedback.json next to the tracker tunes the scores at phase 1 and after
	// phase 2 (Load installs the weights; CompletePhase2 carries them over).
	scoreOf := func(h http.Handler, id string) float64 {
		rec := do(h, "GET", "/api/v1/triage", withSession())
		var resp TriageResponse
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil {
			t.Fatalf("triage: %d %s", rec.Code, rec.Body.String())
		}
		for _, r := range resp.Recommendations {
			if r.ID == id {
				return r.Score
			}
		}
		t.Fatalf("%s not among the recommendations", id)
		return 0
	}
	open := func(path string, gate <-chan struct{}) (*Store, http.Handler) {
		ctx, cancel := context.WithCancel(context.Background())
		complete := func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
			select {
			case <-gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return realComplete(ctx, s)
		}
		store, err := newStore(ctx, path, snapshot.LoadOptions{Now: goldenNow}, snapshot.Load, complete)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close(); cancel() })
		return store, newTestHandler(t, func(c *Config) { c.Store = store })
	}

	plainPath := writeGoldenTracker(t)
	tunedPath := writeGoldenTracker(t)
	fb := analysis.DefaultFeedbackData()
	for i := 0; i < analysis.MinFeedbackSamples; i++ {
		fb.Events = append(fb.Events, analysis.FeedbackEvent{IssueID: "fx-x", Action: "accept", Score: 0.6, Timestamp: goldenNow})
	}
	fb.Adjustments = []analysis.WeightAdjustment{
		{Name: "Staleness", Adjustment: 2.0, Samples: analysis.MinFeedbackSamples, LastUpdated: goldenNow},
		{Name: "PriorityBoost", Adjustment: 0.5, Samples: analysis.MinFeedbackSamples, LastUpdated: goldenNow},
	}
	if err := fb.Save(filepath.Dir(tunedPath)); err != nil {
		t.Fatal(err)
	}

	gate := make(chan struct{})
	plainStore, plain := open(plainPath, gate)
	tunedStore, tuned := open(tunedPath, gate)
	if p, q := scoreOf(plain, "fx-x"), scoreOf(tuned, "fx-x"); p == q {
		t.Fatalf("phase 1: feedback weights must move the score (plain %v, tuned %v)", p, q)
	}
	close(gate)
	for _, s := range []*Store{plainStore, tunedStore} {
		if err := s.waitPhase2(testCtx(t)); err != nil {
			t.Fatal(err)
		}
	}
	if p, q := scoreOf(plain, "fx-x"), scoreOf(tuned, "fx-x"); p == q {
		t.Fatalf("phase 2: feedback weights must survive CompletePhase2 (plain %v, tuned %v)", p, q)
	}
}
