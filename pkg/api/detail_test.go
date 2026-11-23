package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
)

func getDetail(t *testing.T, h http.Handler, id string) IssueDetail {
	t.Helper()
	rec := do(h, "GET", "/api/v1/issues/"+id, withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", id, rec.Code, rec.Body.String())
	}
	var d IssueDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func refIDs[T any](list []T, id func(T) string) []string {
	out := []string{}
	for _, x := range list {
		out = append(out, id(x))
	}
	return out
}

func TestIssueDetailRelations(t *testing.T) {
	h := goldenHandler(t)

	b := getDetail(t, h, "fx-b")
	if b.Parent == nil || b.Parent.ID != "fx-epic" || b.Parent.Title != "Checkout epic" {
		t.Fatalf("parent = %+v", b.Parent)
	}
	// Active blockers first, then the resolved one; counts agree with the list.
	var got []string
	for _, d := range b.BlockedBy {
		got = append(got, d.ID+"/"+map[bool]string{true: "active", false: "resolved"}[d.Active])
	}
	if want := []string{"fx-a/active", "fx-x/active", "fx-done/resolved"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("blocked_by = %v, want %v", got, want)
	}
	if b.BlockerCounts != (BlockerCountsDTO{Active: 2, Resolved: 1}) || !b.Blocked || b.Ready {
		t.Fatalf("counts/blocked/ready = %+v %v %v", b.BlockerCounts, b.Blocked, b.Ready)
	}
	if b.Triage == nil || b.Triage.Reasons == nil || len(b.Triage.Reasons.All) == 0 || b.Graph == nil || !b.Phase2Ready {
		t.Fatalf("triage/graph = %+v / %+v", b.Triage, b.Graph)
	}

	// The epic lists every child, including the blocked one the list tree moves.
	epic := getDetail(t, h, "fx-epic")
	ids := refIDs(epic.Children, func(r IssueRef) string { return r.ID })
	for _, want := range []string{"fx-a", "fx-b", "fx-c"} {
		if !contains(ids, want) {
			t.Errorf("fx-epic children %v lack %s", ids, want)
		}
	}
	if epic.Parent != nil {
		t.Errorf("fx-epic parent = %+v", epic.Parent)
	}

	// fx-a blocks fx-b (still blocked by it) and has an incoming discovered-from.
	a := getDetail(t, h, "fx-a")
	if len(a.Blocks) != 1 || a.Blocks[0].ID != "fx-b" || !a.Blocks[0].Active {
		t.Fatalf("fx-a blocks = %+v", a.Blocks)
	}
	if a.ActiveDependents != 1 {
		t.Errorf("active_dependents = %d", a.ActiveDependents)
	}
	if len(a.Related) != 1 || a.Related[0].ID != "fx-found" || a.Related[0].Type != "discovered-from" || a.Related[0].Direction != "in" {
		t.Fatalf("fx-a related = %+v", a.Related)
	}
	found := getDetail(t, h, "fx-found")
	if len(found.Related) != 1 || found.Related[0].ID != "fx-a" || found.Related[0].Direction != "out" {
		t.Fatalf("fx-found related = %+v", found.Related)
	}

	// A dangling link is a missing reference, not an error.
	later := getDetail(t, h, "fx-later")
	if len(later.Related) != 1 || later.Related[0].ID != "fx-gone" || !later.Related[0].Missing || later.Related[0].Title != "" {
		t.Fatalf("fx-later related = %+v", later.Related)
	}
	if later.DeferUntil == nil || later.Ready {
		t.Errorf("fx-later = %+v", later)
	}

	// A closed blocker does not block; it lists the open dependent as no longer blocked.
	done := getDetail(t, h, "fx-done")
	if len(done.Blocks) != 1 || done.Blocks[0].ID != "fx-b" || done.Blocks[0].Active {
		t.Fatalf("fx-done blocks = %+v", done.Blocks)
	}
}

func TestIssueDetailCardsAndComments(t *testing.T) {
	h := goldenHandler(t)

	dec := getDetail(t, h, "fx-decide")
	if dec.Decision == nil || dec.Decision.Label != "needs-decision" || len(dec.Decision.Options) != 2 ||
		!dec.Decision.Options[0].Recommended || dec.Decision.WaitingOn != "" {
		t.Fatalf("decision = %+v", dec.Decision)
	}
	if dec.Signoff != nil {
		t.Error("a decision bead has no signoff")
	}
	if len(dec.Comments) != 2 || dec.Comments[0].Kind != "brief" || dec.Comments[0].Text != "GOAL: choose a provider." ||
		!dec.Comments[0].Collapsible || dec.Comments[0].Summary == "" {
		t.Fatalf("comments = %+v", dec.Comments)
	}

	so := getDetail(t, h, "fx-done")
	if so.Signoff == nil || so.Signoff.State != "pending" || !so.Signoff.HasComment || so.Decision != nil {
		t.Fatalf("signoff = %+v decision = %+v", so.Signoff, so.Decision)
	}
	c := so.Comments[0]
	if c.Kind != "signoff" || c.Data == nil || !strings.HasPrefix(c.Summary, "signoff") {
		t.Fatalf("signoff comment = %+v", c)
	}
	calls, _ := c.Data["calls_made"].([]any)
	if len(calls) != 1 {
		t.Fatalf("structured calls_made must survive in comment data: %v", c.Data)
	}

	// Arrays are [] not null on a bare issue.
	rec := do(h, "GET", "/api/v1/issues/fx-c", withSession())
	for _, field := range []string{`"children":[]`, `"blocked_by":[]`, `"blocks":[]`, `"related":[]`, `"links":[]`, `"comments":[]`, `"labels":[]`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("fx-c body lacks %s", field)
		}
	}
}

// customStore loads a tracker of the given issues with the fixed clock.
func customHandler(t *testing.T, issues []model.Issue) http.Handler {
	t.Helper()
	path := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), issues)
	store := goldenStore(t, path)
	waitPhase2(t, store)
	return newTestHandler(t, func(c *Config) { c.Store = store })
}

func TestIssueDetailSpecialCases(t *testing.T) {
	base := func(id string, prio int) model.Issue {
		return model.Issue{ID: id, Title: id, Status: model.StatusOpen, Priority: prio, IssueType: model.TypeTask, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9)}
	}
	p0 := base("p-zero", 0)
	spaced := base("has space", 2)
	spaced.Dependencies = []*model.Dependency{dep("has space", "p-zero", model.DepParentChild)}
	design := base("design-1", 2)
	design.Labels = []string{"needs-design"}
	design.Description = "Next artifact: an HTML slider mock."
	closedDecision := base("closed-decision", 2)
	closedDecision.Status = model.StatusClosed
	closedDecision.Labels = []string{"needs-decision"}
	h := customHandler(t, []model.Issue{p0, spaced, design, closedDecision})

	// P0 must not be dropped from a reference by omitempty.
	d := getDetail(t, h, "has%20space")
	if d.ID != "has space" || d.Parent == nil || d.Parent.Priority == nil || *d.Parent.Priority != 0 {
		t.Fatalf("detail of an encoded id = %+v parent=%+v", d.ID, d.Parent)
	}
	rec := do(h, "GET", "/api/v1/issues/has%20space", withSession())
	if !strings.Contains(rec.Body.String(), `"priority":0`) {
		t.Fatal("a P0 parent reference must serialize its priority")
	}
	if kids := getDetail(t, h, "p-zero").Children; len(kids) != 1 || kids[0].ID != "has space" {
		t.Fatalf("children of p-zero = %+v", kids)
	}

	// needs-design without a mock waits on an artifact.
	dd := getDetail(t, h, "design-1")
	if dd.Decision == nil || dd.Decision.Label != "needs-design" || dd.Decision.ReadyToJudge ||
		dd.Decision.WaitingOn == "" || dd.Decision.Options == nil {
		t.Fatalf("design decision = %+v", dd.Decision)
	}
	// A closed bead is not awaiting a decision.
	if getDetail(t, h, "closed-decision").Decision != nil {
		t.Error("a closed bead has no decision card")
	}

	// No sign-off beads: groups is [] not null.
	rec = do(h, "GET", "/api/v1/signoff", withSession())
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"groups":[]`) || !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Fatalf("empty signoff: %d %s", rec.Code, rec.Body.String())
	}
}

func TestIssueDetailErrors(t *testing.T) {
	h := goldenHandler(t)
	for name, target := range map[string]string{
		"unknown":        "/api/v1/issues/nope",
		"empty id":       "/api/v1/issues/",
		"encoded slash":  "/api/v1/issues/fx-a%2Fb",
		"nested path":    "/api/v1/issues/fx-a/comments",
		"case sensitive": "/api/v1/issues/FX-A",
	} {
		rec := do(h, "GET", target, withSession())
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, rec.Code)
			continue
		}
		if e := decodeError(t, rec); e.Code != "not_found" {
			t.Errorf("%s: code %q", name, e.Code)
		}
		if rec.Header().Get("ETag") != "" {
			t.Errorf("%s: a 404 must carry no ETag", name)
		}
	}
	rec := do(h, "POST", "/api/v1/issues/fx-a", withSession(), withOrigin(localOrigin))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	if rec := do(newTestHandler(t, nil), "GET", "/api/v1/issues/fx-a", withSession()); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no store: %d", rec.Code)
	}
	if rec := do(newTestHandler(t, nil), "GET", "/api/v1/signoff", withSession()); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("signoff without a store: %d", rec.Code)
	}
}

func TestIssueDetailAndSignoffConditionalGet(t *testing.T) {
	h := goldenHandler(t)
	etag := func(target string) string {
		rec := do(h, "GET", target, withSession())
		if rec.Code != http.StatusOK || rec.Header().Get("ETag") == "" {
			t.Fatalf("%s: %d", target, rec.Code)
		}
		return rec.Header().Get("ETag")
	}
	status := func(target, tag string) int {
		return do(h, "GET", target, withSession(), func(r *http.Request) { r.Header.Set("If-None-Match", tag) }).Code
	}
	a, b := etag("/api/v1/issues/fx-a"), etag("/api/v1/issues/fx-b")
	if a == b {
		t.Fatal("different issues need different ETags")
	}
	if status("/api/v1/issues/fx-a", a) != http.StatusNotModified || status("/api/v1/issues/fx-b", a) != http.StatusOK {
		t.Error("detail ETag must be bound to its issue")
	}
	s := etag("/api/v1/signoff")
	if status("/api/v1/signoff", s) != http.StatusNotModified || status("/api/v1/issues/fx-a", s) != http.StatusOK {
		t.Error("signoff ETag must be its own")
	}
}

func TestIssueDetailGraphOnlyAfterPhase2(t *testing.T) {
	gate := make(chan struct{})
	store := newGatedStore(t, writeGoldenTracker(t), gate)
	h := newTestHandler(t, func(c *Config) { c.Store = store })

	p1 := getDetail(t, h, "fx-b")
	if p1.Phase2Ready || p1.Graph != nil {
		t.Fatalf("phase 1 detail must not claim graph metrics: ready=%v graph=%+v", p1.Phase2Ready, p1.Graph)
	}
	if p1.Triage == nil {
		t.Error("small-tier triage is available at phase 1")
	}
	close(gate)
	waitPhase2(t, store)
	if p2 := getDetail(t, h, "fx-b"); !p2.Phase2Ready || p2.Graph == nil {
		t.Fatalf("phase 2 detail = ready=%v graph=%+v", p2.Phase2Ready, p2.Graph)
	}
}

func TestIssueIDFromPath(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/issues/fx-a":         "fx-a",
		"/api/v1/issues/has%20space":  "has space",
		"/api/v1/issues/a%2Bb":        "a+b",
		"/api/v1/issues/%E2%82%AC-1":  "€-1",
		"/api/v1/issues/":             "",
		"/api/v1/issues/a%2Fb":        "",
		"/api/v1/issues/a/b":          "",
		"/api/v1/issues/%2e%2e%2fx":   "",
		"/api/v1/issues/fx-a%252Fbad": "fx-a%2Fbad",
	} {
		r, err := http.NewRequestWithContext(context.Background(), "GET", "http://x"+path, nil)
		if err != nil {
			t.Fatalf("%s: %v", path, err) // a dead table row would claim coverage it lacks
		}
		got, ok := issueIDFromPath(r)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q, %v; want %q", path, got, ok, want)
		}
	}
}

func TestSignoffCallsAreStructuredRows(t *testing.T) {
	h := goldenHandler(t)
	want := []SignoffCall{{Call: "kept the guest checkout", Alternative: "force accounts", Why: "fewer drop-offs"}}

	so := getDetail(t, h, "fx-done")
	if so.Signoff == nil || !reflect.DeepEqual(so.Signoff.Calls, want) {
		t.Fatalf("detail calls = %+v, want %+v", so.Signoff, want)
	}

	rec := do(h, "GET", "/api/v1/signoff", withSession())
	var list SignoffResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Groups) != 1 || len(list.Groups[0].Items) != 1 {
		t.Fatalf("signoff list = %+v", list)
	}
	if got := list.Groups[0].Items[0]; got.ID != "fx-done" || !reflect.DeepEqual(got.Calls, want) {
		t.Fatalf("list item = %+v", got)
	}
}

func TestSignoffCallsFromOtherShapes(t *testing.T) {
	signoffIssue := func(id string, comments ...string) model.Issue {
		issue := model.Issue{ID: id, Title: id, Status: model.StatusClosed, Priority: 2, IssueType: model.TypeTask,
			Labels: []string{"needs-signoff"}, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9), ClosedAt: tsp(2, 9)}
		for i, text := range comments {
			issue.Comments = append(issue.Comments, &model.Comment{ID: id + string(rune('a'+i)), IssueID: id, Author: "claude", Text: text, CreatedAt: ts(2, 9+i)})
		}
		return issue
	}
	epic := model.Issue{ID: "ep", Title: "The epic", Status: model.StatusOpen, Priority: 1, IssueType: model.TypeEpic, CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9)}
	text := signoffIssue("so-text", "bd-kind: signoff\nWhat changed: the beat.\n\n**Calls made on your behalf:**\n- shards thrown goal-side\n- 4 shards, not 6")
	jsonMixed := signoffIssue("so-json", "bd-kind: signoff\n"+`{"calls_made":["plain string call",{"call":"object call","instead_of":"the old way","why":"simpler"},{"why":"no call text"}]}`)
	older := signoffIssue("so-newest", "bd-kind: signoff\nCalls made:\n- old call", "bd-kind: signoff\n"+`{"calls_made":[{"call":"newest call"}]}`)
	none := signoffIssue("so-nocomment")
	child := signoffIssue("so-child", "bd-kind: signoff\n"+`{"calls_made":[{"call":"epic child call"}]}`)
	child.Dependencies = []*model.Dependency{dep("so-child", "ep", model.DepParentChild)}
	h := customHandler(t, []model.Issue{epic, text, jsonMixed, older, none, child})

	calls := func(id string) []SignoffCall {
		d := getDetail(t, h, id)
		if d.Signoff == nil || d.Signoff.Calls == nil {
			t.Fatalf("%s: signoff = %+v (calls must be [] not null)", id, d.Signoff)
		}
		return d.Signoff.Calls
	}
	if got, want := calls("so-text"), []SignoffCall{{Call: "shards thrown goal-side"}, {Call: "4 shards, not 6"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("text body calls = %+v, want %+v", got, want)
	}
	if got, want := calls("so-json"), []SignoffCall{{Call: "plain string call"}, {Call: "object call", Alternative: "the old way", Why: "simpler"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("mixed json calls = %+v, want %+v", got, want)
	}
	if got, want := calls("so-newest"), []SignoffCall{{Call: "newest call"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("newest comment must win: %+v", got)
	}
	if got := calls("so-nocomment"); len(got) != 0 {
		t.Errorf("no signoff comment: calls = %+v", got)
	}

	// The list keeps the epic grouping the epic index provides.
	rec := do(h, "GET", "/api/v1/signoff", withSession())
	var list SignoffResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var epicGroup *SignoffGroupDTO
	for i := range list.Groups {
		if list.Groups[i].Epic == "ep" {
			epicGroup = &list.Groups[i]
		}
	}
	if epicGroup == nil || epicGroup.EpicTitle != "The epic" || len(epicGroup.Items) != 1 ||
		epicGroup.Items[0].Epic != "ep" || !reflect.DeepEqual(epicGroup.Items[0].Calls, []SignoffCall{{Call: "epic child call"}}) {
		t.Fatalf("epic group = %+v (groups %+v)", epicGroup, list.Groups)
	}
}

func TestDecisionQuestion(t *testing.T) {
	h := goldenHandler(t)
	dec := getDetail(t, h, "fx-decide")
	if dec.Decision == nil || dec.Decision.Question != "Which provider should we use?" {
		t.Fatalf("decision = %+v", dec.Decision)
	}
	// No "Question:" line: the field is absent so the client falls back to the title.
	rec := do(customHandler(t, []model.Issue{{
		ID: "q-none", Title: "Pick", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask,
		Labels: []string{"needs-decision"}, Description: "Options: (a) yes (b) no",
		CreatedAt: ts(1, 9), UpdatedAt: ts(1, 9),
	}}), "GET", "/api/v1/issues/q-none", withSession())
	if strings.Contains(rec.Body.String(), `"question"`) {
		t.Fatalf("question must be omitted when the description has none: %s", rec.Body.String())
	}
}
