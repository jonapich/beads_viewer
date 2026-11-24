package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
)

func withIfNoneMatch(etag string) reqOpt {
	return func(r *http.Request) { r.Header.Set("If-None-Match", etag) }
}

// deskIssues is goldenIssues plus the pipeline and slice metadata the desk
// fixtures need: three claimed beads (one stale, one with a live lease, one
// staged without a lease) and three logged slices in two epics, one with an
// escaped bug. The other golden fixtures do not use it.
func deskIssues() []model.Issue {
	issues := goldenIssues()
	meta := func(m map[string]any) json.RawMessage {
		raw, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		return raw
	}
	for i := range issues {
		is := &issues[i]
		switch is.ID {
		case "fx-a": // claimed, staged, recent: not stale; also a logged slice
			is.Metadata = meta(map[string]any{
				"stage": "code", "stage_changed_at": "2026-03-10T10:30:00Z", "claim.session": "s-fx-a",
				"slice_outcome": "committed", "review_rounds": 1, "test_runs": 4, "test_red_runs": 1,
				"slice_wall_minutes": 30, "coder_tokens": 12000, "reviewer_tokens": 4000, "other_agent_tokens": 500,
			})
		case "fx-b": // a stopped slice with unknown tokens
			is.Metadata = meta(map[string]any{"slice_outcome": "stopped", "review_rounds": 3, "test_runs": 2, "slice_wall_minutes": 90, "coder_tokens": 20000})
		case "fx-x": // a slice of the other epic
			is.Metadata = meta(map[string]any{"slice_outcome": "committed", "review_rounds": 2, "slice_wall_minutes": 45})
		case "fx-found": // an escaped bug caused by fx-a, and one naming no slice
			is.Metadata = meta(map[string]any{"caused_by": "fx-a"})
		case "fx-c": // claimed and forgotten: stale, unstaged
			is.Status = model.StatusInProgress
			is.Assignee = "grace"
			is.StartedAt = tsp(2, 10)
		case "fx-cy1": // a live lease vouches for it
			is.Status = model.StatusInProgress
			is.Assignee = "lead"
			lease := goldenNow.Add(12 * time.Minute)
			is.LeaseExpiresAt = &lease
			is.Metadata = meta(map[string]any{"stage": "review", "stage_changed_at": "2026-03-10T11:20:00Z"})
		}
	}
	// Two design beads: one with a mock to judge (in the checkout epic, so its
	// group is ready to judge), one still waiting for its artifact (no epic).
	ready := model.Issue{
		ID: "fx-style", Title: "Pick the receipt style", Description: "Question: Which receipt style?\n\nOptions:\n(a) compact\n(b, recommended) detailed",
		Status: model.StatusOpen, Priority: 1, IssueType: model.TypeTask, Labels: []string{"needs-design"},
		CreatedAt: ts(1, 10), UpdatedAt: ts(8, 10),
		Dependencies: []*model.Dependency{dep("fx-style", "fx-epic", model.DepParentChild)},
		Metadata:     meta(map[string]any{"link.mock": []any{map[string]any{"url": "https://example.com/mock-receipt", "note": "receipt mock", "at": "2026-03-08T09:00:00Z", "by": "poet"}}}),
	}
	waiting := model.Issue{
		ID: "fx-logo", Title: "Choose the logo direction", Description: "Next artifact: a poet direction, then an HTML mock.",
		Status: model.StatusOpen, Priority: 3, IssueType: model.TypeTask, Labels: []string{"needs-design"},
		CreatedAt: ts(1, 10), UpdatedAt: ts(5, 10),
	}
	return append(issues, ready, waiting)
}

func deskHandler(t *testing.T, issues []model.Issue, now time.Time) http.Handler {
	t.Helper()
	path := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), issues)
	store := goldenStore(t, path)
	waitPhase2(t, store)
	return newTestHandler(t, func(c *Config) {
		c.Store = store
		c.now = func() time.Time { return now }
	})
}

func getJSON[T any](t *testing.T, h http.Handler, target string) T {
	t.Helper()
	rec := do(h, http.MethodGet, target, withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", target, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: %v: %s", target, err, rec.Body.String())
	}
	return out
}

func TestGoldenDesk(t *testing.T) {
	path := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), deskIssues())
	store := goldenStore(t, path)
	waitPhase2(t, store)
	h := newTestHandler(t, func(c *Config) {
		c.Store = store
		c.now = func() time.Time { return goldenNow }
	})
	for target, name := range map[string]string{
		"/api/v1/inbox":         "inbox.json",
		"/api/v1/pipeline":      "pipeline.json",
		"/api/v1/slice-metrics": "slice-metrics.json",
	} {
		rec := do(h, http.MethodGet, target, withSession())
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, rec.Code, rec.Body.String())
		}
		var got any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		testutil.NewGoldenFile(t, goldenDir, name).AssertJSON(got)
	}

	// The empty collections are [], never null: the fixture the TS types check.
	empty := deskHandler(t, []model.Issue{{ID: "only", Title: "Only", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9)}}, goldenNow)
	for target, name := range map[string]string{
		"/api/v1/inbox":         "inbox-empty.json",
		"/api/v1/pipeline":      "pipeline-empty.json",
		"/api/v1/slice-metrics": "slice-metrics-empty.json",
	} {
		rec := do(empty, http.MethodGet, target, withSession())
		var got any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		testutil.NewGoldenFile(t, goldenDir, name).AssertJSON(got)
	}
}

func TestInboxEndpoint(t *testing.T) {
	h := goldenHandler(t)
	inbox := getJSON[agentdesk.Inbox](t, h, "/api/v1/inbox")
	if inbox.Total != 1 || len(inbox.Groups) != 1 || len(inbox.Groups[0].Items) != 1 {
		t.Fatalf("inbox = %+v", inbox)
	}
	it := inbox.Groups[0].Items[0]
	if it.ID != "fx-decide" || it.Label != "needs-decision" || len(it.Options) != 2 || !it.Options[0].Recommended {
		t.Fatalf("item = %+v", it)
	}

	// Same order and content as agentdesk.BuildInbox on the same issues.
	want := agentdesk.BuildInbox(goldenIssues(), agentdesk.AwaitingLabels)
	if want.Total != inbox.Total || want.Groups[0].Items[0].ID != it.ID {
		t.Fatalf("inbox differs from BuildInbox: %+v vs %+v", inbox, want)
	}

	// An unlabelled tracker has an empty list, not null.
	rec := do(deskHandler(t, []model.Issue{{ID: "x", Title: "x", Status: model.StatusOpen, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9)}}, goldenNow),
		http.MethodGet, "/api/v1/inbox", withSession())
	if !strings.Contains(rec.Body.String(), `"groups":[]`) {
		t.Fatalf("empty inbox must carry groups: []: %s", rec.Body.String())
	}
}

// Epic grouping, ready-to-judge and waiting-on reach the wire as BuildInbox
// computes them, and the server's order is kept.
func TestInboxGroupsAndDesignItems(t *testing.T) {
	h := deskHandler(t, deskIssues(), goldenNow)
	inbox := getJSON[agentdesk.Inbox](t, h, "/api/v1/inbox")
	if inbox.Total != 3 || inbox.Ready != 1 || len(inbox.Groups) != 2 {
		t.Fatalf("inbox = %+v", inbox)
	}
	epic, none := inbox.Groups[0], inbox.Groups[1]
	if epic.Epic != "fx-epic" || epic.EpicTitle != "Checkout epic" || epic.Ready != 1 || len(epic.Items) != 1 {
		t.Fatalf("epic group = %+v", epic)
	}
	style := epic.Items[0]
	if style.ID != "fx-style" || style.Label != "needs-design" || !style.ReadyToJudge || len(style.Links) != 1 ||
		style.Links[0].Kind != "mock" || style.Links[0].URL != "https://example.com/mock-receipt" || len(style.Options) != 2 {
		t.Fatalf("ready design item = %+v", style)
	}
	if none.Epic != "" || len(none.Items) != 2 || none.Items[0].ID != "fx-decide" || none.Items[1].ID != "fx-logo" {
		t.Fatalf("no-epic group = %+v", none)
	}
	if logo := none.Items[1]; logo.ReadyToJudge || logo.WaitingOn != "a poet direction, then an HTML mock" {
		t.Fatalf("waiting design item = %+v", logo)
	}
}

func TestInboxConditionalAndMethods(t *testing.T) {
	h := goldenHandler(t)
	for _, target := range []string{"/api/v1/inbox", "/api/v1/slice-metrics"} {
		first := do(h, http.MethodGet, target, withSession())
		etag := first.Header().Get("ETag")
		if first.Code != http.StatusOK || etag == "" {
			t.Fatalf("%s: %d etag %q", target, first.Code, etag)
		}
		if again := do(h, http.MethodGet, target, withSession(), withIfNoneMatch(etag)); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
			t.Errorf("%s: matching ETag = %d, want a bodyless 304", target, again.Code)
		}
	}
	for _, target := range []string{"/api/v1/inbox", "/api/v1/pipeline", "/api/v1/slice-metrics"} {
		if rec := do(h, http.MethodPost, target, withSession(), withOrigin(localOrigin)); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", target, rec.Code)
		}
		if rec := do(h, http.MethodGet, target); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a session = %d, want 401", target, rec.Code)
		}
	}
	// No data loaded: 503 like every snapshot endpoint.
	bare := newTestHandler(t, nil)
	for _, target := range []string{"/api/v1/inbox", "/api/v1/pipeline", "/api/v1/slice-metrics"} {
		if rec := do(bare, http.MethodGet, target, withSession()); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s without a store = %d, want 503", target, rec.Code)
		}
	}
}

func TestPipelineEndpoint(t *testing.T) {
	h := deskHandler(t, deskIssues(), goldenNow)
	p := getJSON[agentdesk.Pipeline](t, h, "/api/v1/pipeline")
	var ids []string
	for _, r := range p.Rows {
		ids = append(ids, r.ID)
	}
	// Stale first, then by stage order: code before review.
	if !reflect.DeepEqual(ids, []string{"fx-c", "fx-a", "fx-cy1"}) {
		t.Fatalf("rows = %v", ids)
	}
	if p.Total != 3 || p.Stale != 1 || p.ByStage["code"] != 1 || p.ByStage["review"] != 1 || p.ByStage["unstaged"] != 1 {
		t.Fatalf("summary = %+v", p)
	}
	if !p.GeneratedAt.Equal(goldenNow) {
		t.Fatalf("generated_at = %v, want the pinned clock", p.GeneratedAt)
	}
	if r := p.Rows[0]; !r.Stale || !strings.HasPrefix(r.StaleReason, "no activity for ") {
		t.Fatalf("stale row = %+v", r)
	}
	if r := p.Rows[2]; r.Stale || r.LeaseExpiresAt == nil {
		t.Fatalf("leased row = %+v", r)
	}
}

// Staleness depends on the clock: the ETag moves with the minute, so a client
// polling a quiet tracker still sees a claim go stale, and the body is a
// function of the ETag (the clock is cut to the minute).
func TestPipelineETagFollowsTheMinute(t *testing.T) {
	// fx-cy1's lease runs out 12 minutes after goldenNow.
	clock := goldenNow.Add(30 * time.Second)
	path := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), deskIssues())
	store := goldenStore(t, path)
	waitPhase2(t, store)
	h := newTestHandler(t, func(c *Config) {
		c.Store = store
		c.now = func() time.Time { return clock }
	})

	first := do(h, http.MethodGet, "/api/v1/pipeline", withSession())
	etag := first.Header().Get("ETag")
	snapTag := do(h, http.MethodGet, "/api/v1/inbox", withSession()).Header().Get("ETag")
	if !strings.HasPrefix(etag, strings.TrimSuffix(snapTag, `"`)+"~m") {
		t.Fatalf("pipeline ETag %q must extend the snapshot ETag %q with ~m<minute>", etag, snapTag)
	}
	if !strings.Contains(etag, "~m"+strconv.FormatInt(goldenNow.Unix()/60, 10)) {
		t.Fatalf("ETag %q does not carry the unix minute %d", etag, goldenNow.Unix()/60)
	}

	clock = goldenNow.Add(50 * time.Second) // same minute: 304, same body
	if rec := do(h, http.MethodGet, "/api/v1/pipeline", withSession(), withIfNoneMatch(etag)); rec.Code != http.StatusNotModified {
		t.Fatalf("same minute = %d, want 304", rec.Code)
	}

	clock = goldenNow.Add(13 * time.Minute) // lease expired: new minute, new ETag, new flag
	rec := do(h, http.MethodGet, "/api/v1/pipeline", withSession(), withIfNoneMatch(etag))
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag {
		t.Fatalf("next minute = %d etag %q (old %q), want 200 with a new ETag", rec.Code, rec.Header().Get("ETag"), etag)
	}
	var p agentdesk.Pipeline
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Stale != 2 {
		t.Fatalf("after the lease expired, stale = %d, want 2: %+v", p.Stale, p.Rows)
	}
}

func TestSliceMetricsEndpoint(t *testing.T) {
	h := deskHandler(t, deskIssues(), goldenNow)
	r := getJSON[agentdesk.SliceReport](t, h, "/api/v1/slice-metrics")
	if r.Total.Slices != 3 || r.Total.Committed != 2 || r.Total.Stopped != 1 || r.Total.EscapedBugs != 1 {
		t.Fatalf("total = %+v", r.Total)
	}
	if len(r.Epics) != 2 || r.Epics[0].Epic != "fx-epic" || r.Epics[0].Slices != 2 || r.Epics[0].EpicTitle != "Checkout epic" || r.Epics[0].EscapedBugs != 1 {
		t.Fatalf("epics = %+v", r.Epics)
	}
	if want := agentdesk.BuildSliceReport(deskIssues()); !reflect.DeepEqual(r.Total, want.Total) {
		t.Fatalf("total differs from BuildSliceReport: %+v vs %+v", r.Total, want.Total)
	}
	// No slice metadata: a report with epics [] and zero totals.
	empty := getJSON[agentdesk.SliceReport](t, goldenHandler(t), "/api/v1/slice-metrics")
	if empty.Total.Slices != 0 || empty.Epics == nil || len(empty.Epics) != 0 {
		t.Fatalf("empty report = %+v", empty)
	}
}

// meta.counts.awaiting and signoff_pending are what the inbox and sign-off
// lists total, on a tracker that tempts a wrong count: a closed decision, an
// approved and a disputed sign-off, a needs-signoff label on an open bead.
func TestMetaDeskCounts(t *testing.T) {
	mk := func(id string, status model.Status, labels ...string) model.Issue {
		return model.Issue{ID: id, Title: id, Status: status, Priority: 2, IssueType: model.TypeTask, Labels: labels, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9), ClosedAt: tsp(2, 9)}
	}
	approved := mk("approved", model.StatusClosed, "needs-signoff")
	approved.Metadata = json.RawMessage(`{"signoff":"approved"}`)
	disputed := mk("disputed", model.StatusClosed, "needs-signoff")
	disputed.Metadata = json.RawMessage(`{"signoff":"disputed"}`)
	issues := []model.Issue{
		mk("dec", model.StatusOpen, "needs-decision"),
		mk("both", model.StatusInProgress, "needs-decision", "needs-design"), // counted once
		mk("design", model.StatusOpen, "needs-design"),
		mk("closed-dec", model.StatusClosed, "needs-decision"),
		mk("open-so", model.StatusOpen, "needs-signoff"),
		mk("so1", model.StatusClosed, "needs-signoff"),
		mk("so2", model.StatusClosed, "needs-signoff"),
		approved, disputed,
	}
	h := deskHandler(t, issues, goldenNow)

	meta := getJSON[MetaResponse](t, h, "/api/v1/meta")
	if meta.Counts.Awaiting != 3 || meta.Counts.SignoffPending != 2 {
		t.Fatalf("counts = %+v, want awaiting 3 and signoff_pending 2", meta.Counts)
	}
	if inbox := getJSON[agentdesk.Inbox](t, h, "/api/v1/inbox"); inbox.Total != meta.Counts.Awaiting {
		t.Errorf("inbox total %d != meta awaiting %d", inbox.Total, meta.Counts.Awaiting)
	}
	if so := getJSON[SignoffResponse](t, h, "/api/v1/signoff"); so.Total != meta.Counts.SignoffPending {
		t.Errorf("signoff total %d != meta signoff_pending %d", so.Total, meta.Counts.SignoffPending)
	}

	// The fixture tracker: one decision, one sign-off.
	g := getJSON[MetaResponse](t, goldenHandler(t), "/api/v1/meta")
	if g.Counts.Awaiting != 1 || g.Counts.SignoffPending != 1 {
		t.Fatalf("golden counts = %+v", g.Counts)
	}

	// The wire names the nav badge reads.
	rec := do(h, http.MethodGet, "/api/v1/meta", withSession())
	var raw struct {
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Counts["awaiting"] != 3 || raw.Counts["signoff_pending"] != 2 {
		t.Fatalf("raw counts = %v", raw.Counts)
	}
}
