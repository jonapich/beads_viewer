package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
	"github.com/Dicklesworthstone/beads_viewer/pkg/tracker"
)

// fakeWriter is the issueWriter the write tests inject. It records every call;
// handle (when set) decides the outcome and may rewrite the tracker file, as a
// real bd write would.
type fakeWriter struct {
	mu     sync.Mutex
	calls  []string
	handle func(call string) (tracker.Result, error)
}

func (f *fakeWriter) do(call string) (tracker.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	h := f.handle
	f.mu.Unlock()
	if h == nil {
		return tracker.Result{}, nil
	}
	return h(call)
}

func (f *fakeWriter) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeWriter) Signoff(_ context.Context, approve bool, note string) (tracker.Result, error) {
	return f.do(fmt.Sprintf("signoff approve=%v note=%q", approve, note))
}

func (f *fakeWriter) AnswerQuestion(_ context.Context, text, removeLabel string) (tracker.Result, error) {
	return f.do(fmt.Sprintf("answer text=%q remove=%s", text, removeLabel))
}

func (f *fakeWriter) SetPriority(_ context.Context, p int) (tracker.Result, error) {
	return f.do(fmt.Sprintf("priority %d", p))
}

func (f *fakeWriter) SetLabel(_ context.Context, label string, on bool) (tracker.Result, error) {
	return f.do(fmt.Sprintf("label %s on=%v", label, on))
}

// writeEnv is a store over a temp tracker, with a handler whose writes go to a
// fakeWriter.
type writeEnv struct {
	t     *testing.T
	store *Store
	path  string
	h     http.Handler
	fw    *fakeWriter

	mu         sync.Mutex
	issues     []model.Issue
	factoryErr error // returned by the writer factory when set
}

// writeBase is the golden tracker plus one issue that already carries the
// close-candidate label.
func writeBase() []model.Issue {
	issues := goldenIssues()
	for i := range issues {
		if issues[i].ID == "fx-later" {
			issues[i].Labels = append(issues[i].Labels, closeCandidateLabel)
		}
	}
	return issues
}

func newWriteEnv(t *testing.T, mutateCfg func(*Config)) *writeEnv {
	t.Helper()
	env := &writeEnv{t: t, fw: &fakeWriter{}, issues: writeBase()}
	env.path = testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), env.issues)

	ctx, cancel := context.WithCancel(context.Background())
	store, err := newStore(ctx, env.path, snapshot.LoadOptions{Now: goldenNow}, snapshot.Load, realComplete)
	if err != nil {
		cancel()
		t.Fatalf("newStore: %v", err)
	}
	t.Cleanup(func() { store.Close(); cancel() })
	env.store = store
	env.h = newTestHandler(t, func(c *Config) {
		c.Store = store
		c.newWriter = func(model.Issue, string) (issueWriter, error) {
			env.mu.Lock()
			defer env.mu.Unlock()
			if env.factoryErr != nil {
				return nil, env.factoryErr
			}
			return env.fw, nil
		}
		if mutateCfg != nil {
			mutateCfg(c)
		}
	})
	return env
}

// edit applies fn to the tracker's issues and rewrites the file, the way a bd
// write plus its export would.
func (e *writeEnv) edit(fn func(issues []model.Issue)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(e.issues)
	testutil.WriteBeadsFile(e.t, trackerDir(e.path), e.issues)
}

func (e *writeEnv) setFactoryErr(err error) {
	e.mu.Lock()
	e.factoryErr = err
	e.mu.Unlock()
}

func issueByID(issues []model.Issue, id string) *model.Issue {
	for i := range issues {
		if issues[i].ID == id {
			return &issues[i]
		}
	}
	return nil
}

func withoutOrigin() reqOpt { return func(r *http.Request) { r.Header.Del("Origin") } }

func withContentType(ct string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Content-Type", ct) }
}

// newJSONRequest builds a write request that passes the guard: session cookie,
// the server's own Origin and a JSON body. Options override any of it.
func newJSONRequest(method, target, body string, opts ...reqOpt) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = localHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", localOrigin)
	withSession()(req)
	for _, o := range opts {
		o(req)
	}
	return req
}

func sendJSON(h http.Handler, method, target, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newJSONRequest(method, target, body, opts...))
	return rec
}

func decodeWrite(t *testing.T, rec *httptest.ResponseRecorder) WriteResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out WriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("write response is not JSON: %v: %q", err, rec.Body.String())
	}
	return out
}

func getMeta(t *testing.T, h http.Handler) MetaResponse {
	t.Helper()
	rec := do(h, http.MethodGet, "/api/v1/meta", withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("meta: %d %s", rec.Code, rec.Body.String())
	}
	var m MetaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) errorDetail {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	e := decodeError(t, rec)
	if e.Code != code {
		t.Fatalf("code = %q, want %q (%s)", e.Code, code, e.Message)
	}
	return e
}

func expectNoCalls(t *testing.T, fw *fakeWriter) {
	t.Helper()
	if calls := fw.callList(); len(calls) != 0 {
		t.Fatalf("the tracker was called: %v", calls)
	}
}

// setPriority makes the fake write a new priority for id into the tracker.
func (e *writeEnv) writesPriority(id string, p int) {
	e.fw.handle = func(string) (tracker.Result, error) {
		e.edit(func(is []model.Issue) { issueByID(is, id).Priority = p })
		return tracker.Result{}, nil
	}
}

func TestWritePriority(t *testing.T) {
	env := newWriteEnv(t, nil)
	waitPhase2(t, env.store)
	before := getMeta(t, env.h)
	signals, unsub := env.store.hub.subscribe()
	defer unsub()
	env.writesPriority("fx-c", 0)

	rec := sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`)
	got := decodeWrite(t, rec)

	if calls := env.fw.callList(); len(calls) != 1 || calls[0] != "priority 0" {
		t.Fatalf("calls = %v", calls)
	}
	if got.ID != "fx-c" || got.Generation != before.Generation+1 || got.DataHash == before.DataHash || got.Warning != "" || got.CreatedID != "" {
		t.Fatalf("response = %+v (before: gen %d, hash %s)", got, before.Generation, before.DataHash)
	}
	after := getMeta(t, env.h)
	if after.DataHash != got.DataHash || after.Generation != got.Generation {
		t.Fatalf("meta = gen %d hash %s, response = gen %d hash %s", after.Generation, after.DataHash, got.Generation, got.DataHash)
	}
	if !expectSignal(signals, time.Second) {
		t.Fatal("a write sent no events signal")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}

	// The list the client refetches already shows the new priority.
	list := do(env.h, http.MethodGet, "/api/v1/issues?view=flat", withSession())
	if !strings.Contains(list.Body.String(), `"id":"fx-c"`) {
		t.Fatal("list lost the issue")
	}
}

func TestWritePriorityNoOpRunsNothing(t *testing.T) {
	env := newWriteEnv(t, nil)
	before := getMeta(t, env.h)
	got := decodeWrite(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":3}`))
	expectNoCalls(t, env.fw)
	if got.Generation != before.Generation || got.DataHash != before.DataHash || got.ID != "fx-c" {
		t.Fatalf("no-op response = %+v, meta gen %d hash %s", got, before.Generation, before.DataHash)
	}
}

func TestWriteCloseCandidate(t *testing.T) {
	env := newWriteEnv(t, nil)
	before := getMeta(t, env.h)

	// PUT adds the label.
	env.fw.handle = func(string) (tracker.Result, error) {
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-c")
			i.Labels = append(i.Labels, closeCandidateLabel)
		})
		return tracker.Result{}, nil
	}
	got := decodeWrite(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/labels/close-candidate", ""))
	if got.Generation != before.Generation+1 || got.DataHash == before.DataHash {
		t.Fatalf("PUT response = %+v", got)
	}
	// Idempotent: the label is there now.
	again := decodeWrite(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/labels/close-candidate", ""))
	if again.Generation != got.Generation || again.DataHash != got.DataHash {
		t.Fatalf("repeated PUT = %+v, want the same view as %+v", again, got)
	}

	// DELETE removes it.
	env.fw.handle = func(string) (tracker.Result, error) {
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-c")
			i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == closeCandidateLabel })
		})
		return tracker.Result{}, nil
	}
	removed := decodeWrite(t, sendJSON(env.h, http.MethodDelete, "/api/v1/issues/fx-c/labels/close-candidate", ""))
	if removed.Generation != got.Generation+1 || removed.DataHash != before.DataHash {
		t.Fatalf("DELETE response = %+v, want gen %d and the original hash %s", removed, got.Generation+1, before.DataHash)
	}
	// Removing a label the issue does not carry is a no-op as well.
	noop := decodeWrite(t, sendJSON(env.h, http.MethodDelete, "/api/v1/issues/fx-c/labels/close-candidate", ""))
	if noop.Generation != removed.Generation {
		t.Fatalf("repeated DELETE = %+v", noop)
	}

	if calls := env.fw.callList(); !slices.Equal(calls, []string{
		"label close-candidate on=true", "label close-candidate on=false",
	}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestWriteSignoffApproveAndDispute(t *testing.T) {
	env := newWriteEnv(t, nil)
	before := getMeta(t, env.h)

	// Dispute: the fallback files a follow-up bead, reported as created_id.
	env.fw.handle = func(string) (tracker.Result, error) {
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-done")
			i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == agentdesk.LabelNeedsSignoff })
		})
		return tracker.Result{CreatedID: "fx-follow"}, nil
	}
	got := decodeWrite(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-done/signoff", `{"approve":false,"note":"  the cart still drops items  "}`))
	if got.CreatedID != "fx-follow" || got.Generation != before.Generation+1 || got.DataHash == before.DataHash {
		t.Fatalf("dispute response = %+v", got)
	}
	if calls := env.fw.callList(); !slices.Equal(calls, []string{`signoff approve=false note="the cart still drops items"`}) {
		t.Fatalf("calls = %v", calls)
	}

	// The issue left the sign-off flow: a second answer is stale.
	expectError(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-done/signoff", `{"approve":true}`), http.StatusConflict, "conflict")
	if len(env.fw.callList()) != 1 {
		t.Fatalf("a stale sign-off reached the tracker: %v", env.fw.callList())
	}
}

func TestWriteSignoffApprove(t *testing.T) {
	env := newWriteEnv(t, nil)
	before := getMeta(t, env.h)
	env.fw.handle = func(string) (tracker.Result, error) {
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-done")
			i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == agentdesk.LabelNeedsSignoff })
		})
		return tracker.Result{}, nil
	}
	got := decodeWrite(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-done/signoff", `{"approve":true}`))
	if got.Generation != before.Generation+1 || got.CreatedID != "" {
		t.Fatalf("approve response = %+v", got)
	}
	if calls := env.fw.callList(); !slices.Equal(calls, []string{`signoff approve=true note=""`}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestWriteAnswer(t *testing.T) {
	env := newWriteEnv(t, nil)
	opts := agentdesk.ParseOptions(issueByID(env.issues, "fx-decide").Description)
	if len(opts) < 2 {
		t.Fatalf("fixture options = %+v", opts)
	}
	env.fw.handle = func(string) (tracker.Result, error) {
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-decide")
			i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == agentdesk.LabelNeedsDecision })
		})
		return tracker.Result{}, nil
	}
	got := decodeWrite(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-decide/answer", `{"option":"b","text":" for EU rates "}`))
	if got.ID != "fx-decide" || got.Generation != 2 {
		t.Fatalf("response = %+v", got)
	}
	want := fmt.Sprintf("answer text=%q remove=needs-decision", agentdesk.DecisionComment(&opts[1], "for EU rates"))
	if calls := env.fw.callList(); !slices.Equal(calls, []string{want}) {
		t.Fatalf("calls = %v, want %q", calls, want)
	}
}

func TestWriteAnswerTextOnlyAndOptionOnly(t *testing.T) {
	env := newWriteEnv(t, nil)
	opts := agentdesk.ParseOptions(issueByID(env.issues, "fx-decide").Description)

	decodeWrite(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-decide/answer", `{"text":"neither, build it ourselves"}`))
	decodeWrite(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-decide/answer", `{"option":"a"}`))
	calls := env.fw.callList()
	want := []string{
		fmt.Sprintf("answer text=%q remove=needs-decision", "Decision: neither, build it ourselves"),
		fmt.Sprintf("answer text=%q remove=needs-decision", agentdesk.DecisionComment(&opts[0], "")),
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
}

func TestWriteBadBodies(t *testing.T) {
	env := newWriteEnv(t, nil)
	long := strings.Repeat("x", maxWriteText+1)
	cases := []struct {
		name, method, target, body string
		opts                       []reqOpt
		status                     int
		code                       string
	}{
		{"signoff without approve", "POST", "/api/v1/issues/fx-done/signoff", `{}`, nil, 400, "bad_request"},
		{"signoff approve with a note", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":true,"note":"fine"}`, nil, 400, "bad_request"},
		{"dispute without a note", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":false}`, nil, 400, "bad_request"},
		{"dispute with a blank note", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":false,"note":"  \n "}`, nil, 400, "bad_request"},
		{"dispute note too long", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":false,"note":"` + long + `"}`, nil, 400, "bad_request"},
		{"dispute note with NUL", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":false,"note":"a\u0000b"}`, nil, 400, "bad_request"},
		{"approve is not a bool", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":"yes"}`, nil, 400, "bad_request"},
		{"unknown field", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":true,"extra":1}`, nil, 400, "bad_request"},
		{"not JSON", "POST", "/api/v1/issues/fx-done/signoff", `approve`, nil, 400, "bad_request"},
		{"empty body", "POST", "/api/v1/issues/fx-done/signoff", ``, nil, 400, "bad_request"},
		{"trailing data", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":true} {"approve":false}`, nil, 400, "bad_request"},
		{"body over the cap", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":false,"note":"` + strings.Repeat("y", maxWriteBody) + `"}`, nil, 400, "bad_request"},
		{"form content type", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":true}`, []reqOpt{withContentType("application/x-www-form-urlencoded")}, 415, "unsupported_media_type"},
		{"no content type", "POST", "/api/v1/issues/fx-done/signoff", `{"approve":true}`, []reqOpt{withContentType("")}, 415, "unsupported_media_type"},
		{"answer without anything", "POST", "/api/v1/issues/fx-decide/answer", `{}`, nil, 400, "bad_request"},
		{"answer with blank text", "POST", "/api/v1/issues/fx-decide/answer", `{"text":"   "}`, nil, 400, "bad_request"},
		{"answer text too long", "POST", "/api/v1/issues/fx-decide/answer", `{"text":"` + long + `"}`, nil, 400, "bad_request"},
		{"answer with an unknown option", "POST", "/api/v1/issues/fx-decide/answer", `{"option":"z"}`, nil, 400, "bad_request"},
		{"priority missing", "PUT", "/api/v1/issues/fx-c/priority", `{}`, nil, 400, "bad_request"},
		{"priority negative", "PUT", "/api/v1/issues/fx-c/priority", `{"priority":-1}`, nil, 400, "bad_request"},
		{"priority above 4", "PUT", "/api/v1/issues/fx-c/priority", `{"priority":5}`, nil, 400, "bad_request"},
		{"priority not a number", "PUT", "/api/v1/issues/fx-c/priority", `{"priority":"P1"}`, nil, 400, "bad_request"},
		{"priority fractional", "PUT", "/api/v1/issues/fx-c/priority", `{"priority":1.5}`, nil, 400, "bad_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectError(t, sendJSON(env.h, tc.method, tc.target, tc.body, tc.opts...), tc.status, tc.code)
		})
	}
	expectNoCalls(t, env.fw)
	if g := env.store.Current().Gen; g != 1 {
		t.Fatalf("a rejected request reloaded the store: gen %d", g)
	}
}

func TestWriteUnknownIssue(t *testing.T) {
	env := newWriteEnv(t, nil)
	for _, tc := range []struct{ method, sub, body string }{
		{"POST", "signoff", `{"approve":true}`},
		{"POST", "answer", `{"text":"x"}`},
		{"PUT", "priority", `{"priority":1}`},
		{"PUT", "labels/close-candidate", ``},
		{"DELETE", "labels/close-candidate", ``},
	} {
		t.Run(tc.method+" "+tc.sub, func(t *testing.T) {
			expectError(t, sendJSON(env.h, tc.method, "/api/v1/issues/no-such-issue/"+tc.sub, tc.body), http.StatusNotFound, "not_found")
		})
	}
	expectNoCalls(t, env.fw)
}

func TestWriteEscapedIssueID(t *testing.T) {
	env := newWriteEnv(t, nil)
	env.edit(func(is []model.Issue) {
		is[0].ID = "fx epic"
		for i := range is {
			for _, d := range is[i].Dependencies {
				if d.DependsOnID == "fx-epic" {
					d.DependsOnID = "fx epic"
				}
				if d.IssueID == "fx-epic" {
					d.IssueID = "fx epic"
				}
			}
		}
	})
	env.store.Reload()
	env.writesPriority("fx epic", 0)
	decodeWrite(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx%20epic/priority", `{"priority":0}`))
	// An id that unescapes to contain "/" is not an issue: the request falls
	// through to the detail endpoint, which refuses a PUT.
	n := len(env.fw.callList())
	expectError(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx%2Fepic/priority", `{"priority":0}`), http.StatusMethodNotAllowed, "method_not_allowed")
	if len(env.fw.callList()) != n {
		t.Fatal("an id containing a slash reached the tracker")
	}
}

func TestWriteNotWritable(t *testing.T) {
	env := newWriteEnv(t, nil)
	env.setFactoryErr(tracker.OriginError(nil))
	for _, tc := range []struct{ method, target, body string }{
		{"POST", "/api/v1/issues/fx-done/signoff", `{"approve":true}`},
		{"POST", "/api/v1/issues/fx-decide/answer", `{"text":"x"}`},
		{"PUT", "/api/v1/issues/fx-c/priority", `{"priority":0}`},
		{"PUT", "/api/v1/issues/fx-c/labels/close-candidate", ``},
	} {
		t.Run(tc.target, func(t *testing.T) {
			e := expectError(t, sendJSON(env.h, tc.method, tc.target, tc.body), http.StatusForbidden, "read_only")
			if !strings.Contains(e.Message, "read-only") {
				t.Fatalf("message = %q", e.Message)
			}
		})
	}
	expectNoCalls(t, env.fw)

	// Read-only wins over a stale precondition (409) and over a no-op (200).
	expectError(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-a/signoff", `{"approve":true}`), http.StatusForbidden, "read_only")
	expectError(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-a/answer", `{"text":"x"}`), http.StatusForbidden, "read_only")
	expectError(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":3}`), http.StatusForbidden, "read_only")
	expectError(t, sendJSON(env.h, http.MethodDelete, "/api/v1/issues/fx-c/labels/close-candidate", ``), http.StatusForbidden, "read_only")
	expectNoCalls(t, env.fw)
}

func TestWriteStalePreconditions(t *testing.T) {
	env := newWriteEnv(t, nil)
	// fx-a is open: it never awaits sign-off. fx-done is closed: it has no
	// question, whatever its labels.
	expectError(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-a/signoff", `{"approve":true}`), http.StatusConflict, "conflict")
	e := expectError(t, sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-a/answer", `{"text":"x"}`), http.StatusConflict, "conflict")
	if !strings.Contains(e.Message, "fx-a") {
		t.Fatalf("message = %q", e.Message)
	}
	expectNoCalls(t, env.fw)
}

func TestWriteTrackerErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		res    tracker.Result
		cfg    func(*Config)
		status int
		code   string
		msg    string
	}{
		{"conflict", tracker.ErrConflict, tracker.Result{}, nil, 409, "conflict", "changed in the tracker"},
		{"command failure", &tracker.CommandError{ExitCode: 1, Stderr: "Error: no such label", Err: errors.New("exit status 1")}, tracker.Result{}, nil,
			502, "tracker_failed", "Error: no such label"},
		{"command failure without stderr", &tracker.CommandError{ExitCode: 1, Err: errors.New("exit status 1")}, tracker.Result{}, nil,
			502, "tracker_failed", "exit status 1"},
		{"follow-up already filed", &tracker.CommandError{Stderr: "comment failed", Err: errors.New("exit status 1")}, tracker.Result{CreatedID: "fx-follow"}, nil,
			502, "tracker_failed", "(follow-up fx-follow was created)"},
		{"other error", errors.New("boom"), tracker.Result{}, nil, 500, "write_failed", "boom"},
		{"timeout", &tracker.CommandError{ExitCode: -1, Err: context.DeadlineExceeded}, tracker.Result{},
			func(c *Config) { c.writeTimeout = time.Minute }, 504, "tracker_timeout", "did not finish"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newWriteEnv(t, tc.cfg)
			env.fw.handle = func(string) (tracker.Result, error) { return tc.res, tc.err }
			e := expectError(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`), tc.status, tc.code)
			if !strings.Contains(e.Message, tc.msg) {
				t.Fatalf("message = %q, want it to contain %q", e.Message, tc.msg)
			}
			// The failed write released the semaphore: the next one runs.
			env.fw.handle = nil
			decodeWrite(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`))
		})
	}
}

func TestWriteTimeoutCancelsTheTrackerContext(t *testing.T) {
	var sawDeadline atomic.Bool
	env := newWriteEnv(t, func(c *Config) { c.writeTimeout = 30 * time.Millisecond })
	env.fw.handle = func(string) (tracker.Result, error) {
		// A real client returns once its context ends; wait for that.
		time.Sleep(200 * time.Millisecond)
		sawDeadline.Store(true)
		return tracker.Result{}, &tracker.CommandError{ExitCode: -1, Err: context.DeadlineExceeded}
	}
	expectError(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`), http.StatusGatewayTimeout, "tracker_timeout")
	if !sawDeadline.Load() {
		t.Fatal("the stub did not run")
	}
}

func TestWriteWarnsInsteadOfFailing(t *testing.T) {
	env := newWriteEnv(t, nil)
	env.fw.handle = func(string) (tracker.Result, error) {
		return tracker.Result{RefreshErr: errors.New("export blew up")}, nil
	}
	got := decodeWrite(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`))
	if !strings.Contains(got.Warning, "export blew up") || !strings.Contains(got.Warning, "Saved") {
		t.Fatalf("warning = %q", got.Warning)
	}
}

func TestWriteWarnsWhenReloadFails(t *testing.T) {
	var failing atomic.Bool
	path := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), writeBase())
	load := func(ctx context.Context, p string, o snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		if failing.Load() {
			return nil, errors.New("tracker file is gone")
		}
		return snapshot.Load(ctx, p, o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store, err := newStore(ctx, path, snapshot.LoadOptions{Now: goldenNow}, load, realComplete)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); cancel() })
	fw := &fakeWriter{handle: func(string) (tracker.Result, error) {
		failing.Store(true)
		return tracker.Result{}, nil
	}}
	h := newTestHandler(t, func(c *Config) {
		c.Store = store
		c.newWriter = func(model.Issue, string) (issueWriter, error) { return fw, nil }
	})
	got := decodeWrite(t, sendJSON(h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`))
	if !strings.Contains(got.Warning, "tracker file is gone") || got.Generation != 1 {
		t.Fatalf("response = %+v", got)
	}
}

func TestWriteGuard(t *testing.T) {
	env := newWriteEnv(t, nil)
	target := "/api/v1/issues/fx-c/priority"
	body := `{"priority":0}`
	for name, opt := range map[string]reqOpt{
		"foreign origin":   withOrigin("http://evil.example"),
		"other port":       withOrigin("http://127.0.0.1:9999"),
		"null origin":      withOrigin("null"),
		"missing origin":   withoutOrigin(),
		"foreign host":     withHost("evil.example"),
		"vite origin, dev": withOrigin("http://localhost:5173"),
	} {
		t.Run(name, func(t *testing.T) {
			expectError(t, sendJSON(env.h, http.MethodPut, target, body, opt), http.StatusForbidden, "forbidden")
		})
	}
	t.Run("no session", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := newJSONRequest(http.MethodPut, target, body)
		req.Header.Del("Cookie")
		env.h.ServeHTTP(rec, req)
		expectError(t, rec, http.StatusUnauthorized, "unauthorized")
	})
	expectNoCalls(t, env.fw)

	// --serve-dev also accepts the Vite origin.
	dev := newWriteEnv(t, func(c *Config) { c.Dev = true; c.Assets = nil })
	decodeWrite(t, sendJSON(dev.h, http.MethodPut, target, body, withOrigin("http://localhost:5173")))
}

func TestWriteMethodsAndPaths(t *testing.T) {
	env := newWriteEnv(t, nil)
	for _, tc := range []struct{ method, sub, allow string }{
		{"GET", "signoff", "POST"},
		{"PUT", "signoff", "POST"},
		{"DELETE", "answer", "POST"},
		{"POST", "priority", "PUT"},
		{"PATCH", "priority", "PUT"},
		{"POST", "labels/close-candidate", "PUT, DELETE"},
		{"GET", "labels/close-candidate", "PUT, DELETE"},
	} {
		t.Run(tc.method+" "+tc.sub, func(t *testing.T) {
			rec := sendJSON(env.h, tc.method, "/api/v1/issues/fx-c/"+tc.sub, `{}`)
			expectError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
			if got := rec.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow = %q, want %q", got, tc.allow)
			}
		})
	}
	// Unknown sub-paths stay a 404, whatever the method.
	for _, target := range []string{"/api/v1/issues/fx-c/other", "/api/v1/issues/fx-c/", "/api/v1/issues/fx-c/labels", "/api/v1/issues/fx-c/labels/other", "/api/v1/issues/fx-c/priority/x"} {
		for _, method := range []string{"GET", "POST"} {
			if rec := sendJSON(env.h, method, target, `{}`); method == "GET" && rec.Code != http.StatusNotFound {
				t.Fatalf("GET %s = %d", target, rec.Code)
			} else if method == "POST" && rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
				t.Fatalf("POST %s = %d", target, rec.Code)
			}
		}
	}
	// The detail endpoint is untouched.
	if rec := do(env.h, http.MethodGet, "/api/v1/issues/fx-c", withSession()); rec.Code != http.StatusOK {
		t.Fatalf("detail = %d", rec.Code)
	}
	expectNoCalls(t, env.fw)
}

func TestWriteWithoutStore(t *testing.T) {
	h := newTestHandler(t, nil)
	expectError(t, sendJSON(h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`), http.StatusServiceUnavailable, "not_ready")
}

// blockingWriter makes every tracker call wait for release, counting how many
// run at once.
type blockingWriter struct {
	fakeWriter
	inflight, maxInflight atomic.Int32
	entered               chan struct{}
	release               chan struct{}
}

func newBlockingEnv(t *testing.T) (*writeEnv, *blockingWriter) {
	env := newWriteEnv(t, nil)
	bw := &blockingWriter{entered: make(chan struct{}, 8), release: make(chan struct{})}
	bw.handle = func(string) (tracker.Result, error) {
		n := bw.inflight.Add(1)
		for {
			m := bw.maxInflight.Load()
			if n <= m || bw.maxInflight.CompareAndSwap(m, n) {
				break
			}
		}
		bw.entered <- struct{}{}
		<-bw.release
		bw.inflight.Add(-1)
		return tracker.Result{}, nil
	}
	env.h = newTestHandler(t, func(c *Config) {
		c.Store = env.store
		c.newWriter = func(model.Issue, string) (issueWriter, error) { return bw, nil }
	})
	return env, bw
}

func waitEntered(t *testing.T, bw *blockingWriter) {
	t.Helper()
	select {
	case <-bw.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the tracker call never started")
	}
}

func TestWritesAreSerialised(t *testing.T) {
	env, bw := newBlockingEnv(t)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	send := func(id string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- sendJSON(env.h, http.MethodPut, "/api/v1/issues/"+id+"/priority", `{"priority":0}`).Code
		}()
	}
	send("fx-a")
	waitEntered(t, bw)
	send("fx-c")
	time.Sleep(150 * time.Millisecond)
	if n := len(bw.callList()); n != 1 {
		t.Fatalf("the second write started while the first runs: %d calls", n)
	}
	close(bw.release)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
	}
	if n := bw.maxInflight.Load(); n != 1 || len(bw.callList()) != 2 {
		t.Fatalf("max in flight = %d, calls = %v", n, bw.callList())
	}
}

func TestQueuedWriteLeavesWhenTheClientDoes(t *testing.T) {
	env, bw := newBlockingEnv(t)
	first := make(chan int, 1)
	go func() {
		first <- sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-a/priority", `{"priority":0}`).Code
	}()
	waitEntered(t, bw)

	ctx, cancel := context.WithCancel(context.Background())
	req := newJSONRequest(http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		env.h.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a queued write kept waiting after its client left")
	}
	expectError(t, rec, http.StatusServiceUnavailable, "cancelled")
	close(bw.release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("first write = %d", code)
	}
	if calls := bw.callList(); len(calls) != 1 {
		t.Fatalf("the abandoned write reached the tracker: %v", calls)
	}
}

// TestQueuedWriteAtShutdownIsRefused: at Ctrl+C the request context and the
// store end together (one parent context, as in bv serve). The queued write
// must be answered 503, never an empty 200, and must not run.
func TestQueuedWriteAtShutdownIsRefused(t *testing.T) {
	for i := 0; i < 20; i++ {
		parent, cancelParent := context.WithCancel(context.Background())
		path := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), writeBase())
		store, err := newStore(parent, path, snapshot.LoadOptions{Now: goldenNow}, snapshot.Load, realComplete)
		if err != nil {
			cancelParent()
			t.Fatal(err)
		}
		bw := &blockingWriter{entered: make(chan struct{}, 8), release: make(chan struct{})}
		bw.handle = func(string) (tracker.Result, error) {
			bw.entered <- struct{}{}
			<-bw.release
			return tracker.Result{}, nil
		}
		h := newTestHandler(t, func(c *Config) {
			c.Store = store
			c.newWriter = func(model.Issue, string) (issueWriter, error) { return bw, nil }
		})
		first := make(chan int, 1)
		go func() {
			first <- sendJSON(h, http.MethodPut, "/api/v1/issues/fx-a/priority", `{"priority":0}`).Code
		}()
		waitEntered(t, bw)

		req := newJSONRequest(http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`).WithContext(parent)
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			h.ServeHTTP(rec, req)
			close(done)
		}()
		time.Sleep(20 * time.Millisecond)
		cancelParent()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a queued write kept waiting at shutdown")
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("round %d: queued write at shutdown = %d %q, want 503", i, rec.Code, rec.Body.String())
		}
		decodeError(t, rec)
		close(bw.release)
		<-first
		store.Close()
		if calls := bw.callList(); len(calls) != 1 {
			t.Fatalf("round %d: the refused write reached the tracker: %v", i, calls)
		}
	}
}

func TestCloseWaitsForAWriteInFlight(t *testing.T) {
	env, bw := newBlockingEnv(t)
	bw.handle = func(string) (tracker.Result, error) {
		bw.entered <- struct{}{}
		<-bw.release
		// The write lands in the file, but the store is closed by the time it is
		// reloaded: nothing may be published.
		env.edit(func(is []model.Issue) { issueByID(is, "fx-c").Priority = 0 })
		return tracker.Result{}, nil
	}
	resp := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		resp <- sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`)
	}()
	waitEntered(t, bw)
	before := env.store.Current()

	closed := make(chan struct{})
	go func() {
		env.store.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while a write was running")
	case <-time.After(150 * time.Millisecond):
	}

	close(bw.release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close never returned")
	}
	got := decodeWrite(t, <-resp)
	if env.store.Current() != before && env.store.Current().Gen != before.Gen {
		t.Fatalf("a reload published after Close: gen %d -> %d", before.Gen, env.store.Current().Gen)
	}
	if got.Generation != before.Gen {
		t.Fatalf("response generation = %d, want the closed store's %d", got.Generation, before.Gen)
	}
	if !strings.Contains(got.Warning, "shutting down") {
		t.Fatalf("a write that landed during Close carries no warning: %+v", got)
	}

	// A write after Close is refused, never run.
	n := len(bw.callList())
	expectError(t, sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-a/priority", `{"priority":0}`), http.StatusServiceUnavailable, "shutting_down")
	if len(bw.callList()) != n {
		t.Fatal("a write ran on a closed store")
	}
	env.store.Close() // idempotent
}

func TestMetaReportsWritability(t *testing.T) {
	dir := t.TempDir()
	origin := func(mod func(*model.IssueOrigin)) *model.IssueOrigin {
		o := &model.IssueOrigin{
			LocalID: "fx", Tracker: "bd", Executable: filepath.Join(dir, "bd.exe"),
			WorkingDirectory: dir, TrackerDirectory: filepath.Join(dir, ".beads"), Database: filepath.Join(dir, ".beads", "db"),
		}
		if mod != nil {
			mod(o)
		}
		return o
	}
	for name, tc := range map[string]struct {
		origin *model.IssueOrigin
		want   bool
	}{
		"complete route":  {origin(nil), true},
		"read-only route": {origin(func(o *model.IssueOrigin) { o.ReadOnlyReason = "locked" }), false},
		"no origin":       {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeGoldenTracker(t)
			load := func(ctx context.Context, p string, o snapshot.LoadOptions) (*snapshot.Snapshot, error) {
				snap, err := snapshot.Load(ctx, p, o)
				if err == nil {
					for i := range snap.Issues {
						snap.Issues[i].Origin = tc.origin
					}
				}
				return snap, err
			}
			ctx, cancel := context.WithCancel(context.Background())
			store, err := newStore(ctx, path, snapshot.LoadOptions{Now: goldenNow}, load, realComplete)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			t.Cleanup(func() { store.Close(); cancel() })
			h := newTestHandler(t, func(c *Config) { c.Store = store })
			if got := getMeta(t, h).Capabilities.Writes; got != tc.want {
				t.Fatalf("capabilities.writes = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestGoldenWriteResults pins the write response and the write errors, which
// the web client checks its types against.
func TestGoldenWriteResults(t *testing.T) {
	env := newWriteEnv(t, nil)
	env.writesPriority("fx-c", 0)
	rec := sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`)
	testutil.NewGoldenFile(t, goldenDir, "write-result.json").AssertJSON(decodeAny(t, rec, http.StatusOK))

	env.fw.handle = func(string) (tracker.Result, error) {
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-done")
			i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == agentdesk.LabelNeedsSignoff })
			is = append(is, model.Issue{
				ID: "fx-follow", Title: "Follow-up: Ship the cart", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask,
				CreatedAt: ts(4, 10), UpdatedAt: ts(4, 10),
			})
			env.issues = is
		})
		return tracker.Result{CreatedID: "fx-follow"}, nil
	}
	rec = sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-done/signoff", `{"approve":false,"note":"the cart still drops items"}`)
	testutil.NewGoldenFile(t, goldenDir, "write-result-dispute.json").AssertJSON(decodeAny(t, rec, http.StatusOK))

	rec = sendJSON(env.h, http.MethodPost, "/api/v1/issues/fx-a/signoff", `{"approve":true}`)
	testutil.NewGoldenFile(t, goldenDir, "error-conflict.json").AssertJSON(decodeAny(t, rec, http.StatusConflict))

	env.setFactoryErr(tracker.OriginError(nil))
	rec = sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/labels/close-candidate", ``)
	testutil.NewGoldenFile(t, goldenDir, "error-read-only.json").AssertJSON(decodeAny(t, rec, http.StatusForbidden))
}

func decodeAny(t *testing.T, rec *httptest.ResponseRecorder, status int) any {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("not JSON: %v: %q", err, rec.Body.String())
	}
	return v
}
