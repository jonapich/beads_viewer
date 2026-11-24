package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
)

// planTrackerHandler serves a tracker made of issues, with phase 2 done.
func planTrackerHandler(t *testing.T, issues []model.Issue) http.Handler {
	t.Helper()
	dir := testutil.TempBeadsDir(t)
	store := goldenStore(t, testutil.WriteBeadsFile(t, dir, issues))
	waitPhase2(t, store)
	return newTestHandler(t, func(c *Config) { c.Store = store })
}

func getPlan(t *testing.T, h http.Handler) PlanResponse {
	t.Helper()
	rec := do(h, http.MethodGet, "/api/v1/plan", withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body.String())
	}
	var resp PlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestGoldenPlan(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)
	got, code := getGolden(t, store, "/api/v1/plan")
	if code != http.StatusOK {
		t.Fatalf("plan: %d", code)
	}
	testutil.NewGoldenFile(t, goldenDir, "plan.json").AssertJSON(got)
}

func TestPlanServedAtPhase1(t *testing.T) {
	gate := make(chan struct{})
	store := newGatedStore(t, writeGoldenTracker(t), gate)
	h := newTestHandler(t, func(c *Config) { c.Store = store })

	rec := do(h, http.MethodGet, "/api/v1/plan", withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("phase 1 plan must be served, got %d", rec.Code)
	}
	p1 := rec.Header().Get("ETag")
	if !strings.HasSuffix(p1, `-p1"`) {
		t.Fatalf("phase 1 ETag = %q, want a -p1 suffix", p1)
	}

	close(gate)
	waitPhase2(t, store)
	rec = do(h, http.MethodGet, "/api/v1/plan", withSession())
	p2 := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || p2 == p1 || !strings.HasSuffix(p2, `-p2"`) {
		t.Fatalf("phase 2: %d etag %q (phase 1 was %q)", rec.Code, p2, p1)
	}
	// triage_score is present once triage exists.
	var resp PlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	scored := 0
	for _, tr := range resp.Tracks {
		for _, it := range tr.Items {
			if it.TriageScore != nil {
				scored++
			}
		}
	}
	if scored == 0 {
		t.Errorf("phase 2 plan has no triage_score: %s", rec.Body.String())
	}
}

func TestPlanShape(t *testing.T) {
	mk := func(id, title string, typ model.IssueType) model.Issue {
		return model.Issue{
			ID: id, Title: title, Status: model.StatusOpen, Priority: 2, IssueType: typ,
			CreatedAt: ts(1, 10), UpdatedAt: ts(2, 10),
		}
	}
	a := mk("pl-a", "Blocker first", model.TypeBug)
	a.Assignee = "ada"
	b := mk("pl-b", "Waits for A", model.TypeTask)
	b.Dependencies = []*model.Dependency{dep("pl-b", "pl-a", model.DepBlocks)}
	s1 := mk("pl-s1", "Single one", model.TypeFeature)
	s2 := mk("pl-s2", "Single two", model.TypeTask)
	h := planTrackerHandler(t, []model.Issue{a, b, s1, s2})

	rec := do(h, http.MethodGet, "/api/v1/plan", withSession())
	body := rec.Body.String()
	var resp PlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// B is blocked, so only A and the two singles are actionable.
	if resp.TotalActionable != 3 || resp.TotalBlocked != 1 {
		t.Errorf("totals = %d actionable, %d blocked, want 3 and 1", resp.TotalActionable, resp.TotalBlocked)
	}
	if resp.Summary.HighestImpact != "pl-a" || resp.Summary.UnblocksCount != 1 {
		t.Errorf("summary = %+v, want pl-a unblocking 1", resp.Summary)
	}
	if len(resp.Tracks) != 3 {
		t.Fatalf("tracks = %d, want 3 (A's component plus two singles): %s", len(resp.Tracks), body)
	}
	byID := map[string]PlanItem{}
	for _, tr := range resp.Tracks {
		if tr.TrackID == "" || tr.Reason == "" || tr.Items == nil {
			t.Errorf("track %+v needs id, reason and items", tr)
		}
		for _, it := range tr.Items {
			if it.UnblocksIDs == nil {
				t.Errorf("%s: unblocks_ids must be [] not null", it.ID)
			}
			byID[it.ID] = it
		}
	}
	if got := byID["pl-a"]; got.IssueType != "bug" || got.Assignee != "ada" || len(got.UnblocksIDs) != 1 || got.UnblocksIDs[0] != "pl-b" {
		t.Errorf("pl-a = %+v", got)
	}
	if got := byID["pl-s1"]; got.IssueType != "feature" || got.Assignee != "" || len(got.UnblocksIDs) != 0 {
		t.Errorf("pl-s1 = %+v", got)
	}
	for _, want := range []string{`"unblocks_ids":[]`, `"unblocks_ids":["pl-b"]`, `"issue_type":"bug"`, `"assignee":"ada"`} {
		if !strings.Contains(body, want) {
			t.Errorf("plan body lacks %s", want)
		}
	}
	// The assignee key is left out when empty; the analysis field name is not
	// reused.
	if strings.Contains(body, `"unblocks":`) {
		t.Error("plan must say unblocks_ids, not unblocks")
	}
}

func TestPlanEmptyTracker(t *testing.T) {
	h := planTrackerHandler(t, nil)
	rec := do(h, http.MethodGet, "/api/v1/plan", withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("empty tracker: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"tracks":[]`) {
		t.Errorf("tracks must be []: %s", body)
	}
	resp := getPlan(t, h)
	if len(resp.Tracks) != 0 || resp.TotalActionable != 0 || resp.TotalBlocked != 0 ||
		resp.TotalDeferredOrParked != 0 || resp.TotalInProgress != 0 || resp.Summary != (analysis.PlanSummary{}) {
		t.Errorf("empty plan = %+v", resp)
	}
	for _, field := range []string{`"total_actionable":0`, `"total_blocked":0`, `"total_deferred_or_parked":0`, `"total_in_progress":0`} {
		if !strings.Contains(body, field) {
			t.Errorf("empty plan lacks %s: %s", field, body)
		}
	}
}

func TestPlanConditional(t *testing.T) {
	h := goldenHandler(t)
	rec := do(h, http.MethodGet, "/api/v1/plan", withSession())
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" {
		t.Fatalf("plan: %d etag=%q", rec.Code, etag)
	}
	match := func(r *http.Request) { r.Header.Set("If-None-Match", etag) }
	cond := do(h, http.MethodGet, "/api/v1/plan", withSession(), match)
	if cond.Code != http.StatusNotModified || cond.Body.Len() != 0 {
		t.Errorf("matching ETag: %d body %q", cond.Code, cond.Body.String())
	}
	if other := do(h, http.MethodGet, "/api/v1/plan", withSession(), func(r *http.Request) { r.Header.Set("If-None-Match", `"other"`) }); other.Code != http.StatusOK {
		t.Errorf("other ETag: %d", other.Code)
	}
	head := do(h, http.MethodHead, "/api/v1/plan", withSession())
	if head.Code != http.StatusOK || head.Header().Get("ETag") != etag {
		t.Errorf("HEAD: %d etag %q", head.Code, head.Header().Get("ETag"))
	}
	post := do(h, http.MethodPost, "/api/v1/plan", withSession(), withOrigin(localOrigin))
	if post.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d, want 405", post.Code)
	}
	if do(newTestHandler(t, nil), http.MethodGet, "/api/v1/plan", withSession()).Code != http.StatusServiceUnavailable {
		t.Error("no store must be 503")
	}
}
