package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
	"github.com/Dicklesworthstone/beads_viewer/pkg/tracker"
)

// The PATCH /api/v1/issues/{id} tests (beads-c2o.14). The issueWriter methods of
// the field editor live here so write_test.go stays as it was.

func (f *fakeWriter) Update(_ context.Context, d tracker.FieldDiff) (tracker.Result, error) {
	var parts []string
	for _, p := range []struct {
		name string
		v    *string
	}{{"title", d.Title}, {"status", d.Status}, {"priority", d.Priority}, {"assignee", d.Assignee}, {"type", d.Type}, {"description", d.Description}} {
		if p.v != nil {
			parts = append(parts, fmt.Sprintf("%s=%q", p.name, *p.v))
		}
	}
	return f.do("update " + strings.Join(parts, " "))
}

func (f *fakeWriter) RefreshExport(context.Context) error {
	_, err := f.do("refresh")
	return err
}

// base is id's updated_at as a client echoes it.
func (e *writeEnv) base(id string) string {
	return e.store.Current().Snap.Issue(id).UpdatedAt.Format(time.RFC3339Nano)
}

// patch sends PATCH /api/v1/issues/{id} with fields (a JSON object body without
// base_updated_at, e.g. `"title":"x"`) and the current base_updated_at.
func (e *writeEnv) patch(id, fields string, opts ...reqOpt) *httptest.ResponseRecorder {
	return e.patchBase(id, e.base(id), fields, opts...)
}

func (e *writeEnv) patchBase(id, base, fields string, opts ...reqOpt) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"base_updated_at":%q`, base)
	if fields != "" {
		body += "," + fields
	}
	body += "}"
	return sendJSON(e.h, http.MethodPatch, "/api/v1/issues/"+id, body, opts...)
}

func (e *writeEnv) singleCall(t *testing.T, want string) {
	t.Helper()
	if calls := e.fw.callList(); len(calls) != 1 || calls[0] != want {
		t.Fatalf("calls = %q, want [%q]", calls, want)
	}
}

func TestPatchEachField(t *testing.T) {
	for name, tc := range map[string]struct{ id, fields, want string }{
		"title":       {"fx-c", `"title":"Receipt e-mail"`, `update title="Receipt e-mail"`},
		"status":      {"fx-c", `"status":"in_progress"`, `update status="in_progress"`},
		"priority":    {"fx-c", `"priority":0`, `update priority="0"`},
		"assignee":    {"fx-c", `"assignee":"grace"`, `update assignee="grace"`},
		"clear":       {"fx-a", `"assignee":""`, `update assignee=""`},
		"type":        {"fx-c", `"type":"bug"`, `update type="bug"`},
		"description": {"fx-c", `"description":"Line one\n\nLine two"`, `update description="Line one\n\nLine two"`},
		"closed":      {"fx-c", `"status":"closed"`, `update status="closed"`},
		"multi": {
			"fx-c", `"title":"New","status":"blocked","priority":4,"assignee":"grace","type":"feature","description":"d"`,
			`update title="New" status="blocked" priority="4" assignee="grace" type="feature" description="d"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newWriteEnv(t, nil)
			waitPhase2(t, env.store)
			before := getMeta(t, env.h)
			env.fw.handle = func(string) (tracker.Result, error) {
				env.edit(func(is []model.Issue) { issueByID(is, tc.id).Title += "!" })
				return tracker.Result{}, nil
			}
			rec := env.patch(tc.id, tc.fields)
			got := decodeWrite(t, rec)
			env.singleCall(t, tc.want)
			if got.ID != tc.id || got.Generation != before.Generation+1 || got.DataHash == before.DataHash || got.Warning != "" {
				t.Fatalf("response = %+v (before: gen %d)", got, before.Generation)
			}
			if after := getMeta(t, env.h); after.DataHash != got.DataHash || after.Generation != got.Generation {
				t.Fatalf("the response is not the reloaded state: meta = %+v, response = %+v", after, got)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestPatchNormalisesValues(t *testing.T) {
	env := newWriteEnv(t, nil)
	decodeWrite(t, env.patch("fx-c", `"title":"  Padded title \n","status":" open\t","assignee":"  grace ","type":" task ","description":"\n\n  indented\nbody \t\r\n\n"`))
	// status "open" and type "task" equal the snapshot, so only three are sent.
	env.singleCall(t, `update title="Padded title" assignee="grace" description="  indented\nbody"`)
}

func TestPatchSendsOnlyWhatChanged(t *testing.T) {
	env := newWriteEnv(t, nil)
	// fx-a: title "Cart model", in_progress, P1, ada, task, "Cart model description."
	decodeWrite(t, env.patch("fx-a", `"title":"Cart model","status":"in_progress","priority":1,"assignee":"ada","type":"task","description":"\nCart model description.\n\n"`))
	expectNoCalls(t, env.fw)

	decodeWrite(t, env.patch("fx-a", `"title":"Cart model","status":"blocked"`))
	env.singleCall(t, `update status="blocked"`)
}

func TestPatchNoOpsRunNothing(t *testing.T) {
	env := newWriteEnv(t, nil)
	before := getMeta(t, env.h)
	// Clearing an assignee that is empty is a no-op too; null and absent fields stay.
	got := decodeWrite(t, env.patch("fx-c", `"assignee":"","title":null,"status":null,"priority":3`))
	expectNoCalls(t, env.fw)
	if got.ID != "fx-c" || got.Generation != before.Generation || got.DataHash != before.DataHash || got.Warning != "" {
		t.Fatalf("response = %+v, want the current state %+v", got, before)
	}
}

func TestPatchAbsentAndNullFieldsStayUnchanged(t *testing.T) {
	env := newWriteEnv(t, nil)
	decodeWrite(t, env.patch("fx-a", `"title":"Renamed","assignee":null,"description":null`))
	env.singleCall(t, `update title="Renamed"`)
}

func TestPatchValidation(t *testing.T) {
	env := newWriteEnv(t, nil)
	base := env.base("fx-c")
	long := strings.Repeat("a", maxWriteText+1)
	for name, tc := range map[string]struct{ body, msg string }{
		"unknown field":      {`{"base_updated_at":"` + base + `","title":"x","labels":["a"]}`, "labels"},
		"issue_type":         {`{"base_updated_at":"` + base + `","issue_type":"bug"}`, "issue_type"},
		"empty patch":        {`{"base_updated_at":"` + base + `"}`, "no fields to update"},
		"only nulls":         {`{"base_updated_at":"` + base + `","title":null}`, "no fields to update"},
		"missing base":       {`{"title":"x"}`, "base_updated_at is required"},
		"null base":          {`{"base_updated_at":null,"title":"x"}`, "base_updated_at is required"},
		"empty base":         {`{"base_updated_at":"","title":"x"}`, "base_updated_at is required"},
		"bad base":           {`{"base_updated_at":"yesterday","title":"x"}`, "RFC 3339"},
		"base not a string":  {`{"base_updated_at":5,"title":"x"}`, ""},
		"empty title":        {`{"base_updated_at":"` + base + `","title":"  "}`, "title cannot be empty"},
		"multi-line title":   {`{"base_updated_at":"` + base + `","title":"a\nb"}`, "title must be one line"},
		"CR in title":        {`{"base_updated_at":"` + base + `","title":"a\rb"}`, "title must be one line"},
		"blank status":       {`{"base_updated_at":"` + base + `","status":""}`, "status cannot be empty"},
		"multi-line status":  {`{"base_updated_at":"` + base + `","status":"open\nclosed"}`, "status must be one line"},
		"blank type":         {`{"base_updated_at":"` + base + `","type":" "}`, "type cannot be empty"},
		"multi-line type":    {`{"base_updated_at":"` + base + `","type":"a\nb"}`, "type must be one line"},
		"multi-line assign":  {`{"base_updated_at":"` + base + `","assignee":"a\nb"}`, "assignee must be one line"},
		"priority too high":  {`{"base_updated_at":"` + base + `","priority":5}`, "priority must be between 0 and 4"},
		"priority negative":  {`{"base_updated_at":"` + base + `","priority":-1}`, "priority must be between 0 and 4"},
		"priority fraction":  {`{"base_updated_at":"` + base + `","priority":1.5}`, ""},
		"priority string":    {`{"base_updated_at":"` + base + `","priority":"1"}`, ""},
		"long title":         {`{"base_updated_at":"` + base + `","title":"` + long + `"}`, "title is longer than"},
		"long assignee":      {`{"base_updated_at":"` + base + `","assignee":"` + long + `"}`, "assignee is longer than"},
		"NUL in title":       {`{"base_updated_at":"` + base + `","title":"a\u0000b"}`, "NUL"},
		"NUL in description": {`{"base_updated_at":"` + base + `","description":"a\u0000b"}`, "NUL"},
		"long description":   {`{"base_updated_at":"` + base + `","description":"` + strings.Repeat("d", maxDescription+1) + `"}`, "description is longer than 16000 bytes; edit it with bd"},
		"trailing data":      {`{"base_updated_at":"` + base + `","title":"x"} {}`, "unexpected data"},
		"not JSON":           {`title=x`, ""},
		"empty body":         {``, ""},
		"array":              {`[]`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			rec := sendJSON(env.h, http.MethodPatch, "/api/v1/issues/fx-c", tc.body)
			e := expectError(t, rec, http.StatusBadRequest, "bad_request")
			if !strings.Contains(e.Message, tc.msg) {
				t.Fatalf("message = %q, want it to contain %q", e.Message, tc.msg)
			}
		})
	}
	expectNoCalls(t, env.fw)

	t.Run("body over the cap", func(t *testing.T) {
		body := `{"base_updated_at":"` + base + `","description":"` + strings.Repeat("d", maxPatchBody) + `"}`
		expectError(t, sendJSON(env.h, http.MethodPatch, "/api/v1/issues/fx-c", body), http.StatusBadRequest, "bad_request")
	})
	t.Run("wrong content type", func(t *testing.T) {
		expectError(t, env.patch("fx-c", `"title":"x"`, withContentType("text/plain")), http.StatusUnsupportedMediaType, "unsupported_media_type")
	})
	t.Run("no content type", func(t *testing.T) {
		expectError(t, env.patch("fx-c", `"title":"x"`, withContentType("")), http.StatusUnsupportedMediaType, "unsupported_media_type")
	})
	expectNoCalls(t, env.fw)
}

func TestPatchBoundaryValues(t *testing.T) {
	env := newWriteEnv(t, nil)
	// The caps are inclusive: a 16000-byte description, priority 0 and 4, a blank
	// assignee clearing a set one.
	decodeWrite(t, env.patch("fx-c", `"description":"`+strings.Repeat("d", maxDescription)+`"`))
	// Trailing newlines do not count towards the cap: the normalised value is sent.
	decodeWrite(t, env.patch("fx-c", `"description":"`+strings.Repeat("d", maxDescription)+`\n\n\n"`))
	decodeWrite(t, env.patch("fx-c", `"priority":0`))
	decodeWrite(t, env.patch("fx-c", `"priority":4`))
	if n := len(env.fw.callList()); n != 4 {
		t.Fatalf("calls = %v", env.fw.callList())
	}
}

func TestPatchOneLineFieldsCapAt500Bytes(t *testing.T) {
	env := newWriteEnv(t, nil)
	for _, field := range []string{"title", "status", "type", "assignee"} {
		t.Run(field, func(t *testing.T) {
			over := strings.Repeat("a", maxLineField+1)
			e := expectError(t, env.patch("fx-c", fmt.Sprintf(`%q:%q`, field, over)), http.StatusBadRequest, "bad_request")
			if want := field + " is longer than 500 bytes"; e.Message != want {
				t.Fatalf("message = %q, want %q", e.Message, want)
			}
			// Multi-byte text counts in bytes; padding is trimmed before the count.
			expectError(t, env.patch("fx-c", fmt.Sprintf(`%q:%q`, field, strings.Repeat("é", maxLineField/2+1))), http.StatusBadRequest, "bad_request")
			before := len(env.fw.callList())
			decodeWrite(t, env.patch("fx-c", fmt.Sprintf(`%q:%q`, field, "  "+strings.Repeat("a", maxLineField)+"  ")))
			if n := len(env.fw.callList()); n != before+1 {
				t.Fatalf("calls = %v", env.fw.callList())
			}
		})
	}
}

func TestPatchUnknownIssue(t *testing.T) {
	env := newWriteEnv(t, nil)
	rec := env.patchBase("no-such-issue", "2026-03-02T10:00:00Z", `"title":"x"`)
	expectError(t, rec, http.StatusNotFound, "not_found")
	expectNoCalls(t, env.fw)
}

func TestPatchReadOnlyBeatsEverything(t *testing.T) {
	env := newWriteEnv(t, nil)
	env.setFactoryErr(tracker.OriginError(nil))
	stale := "2000-01-01T00:00:00Z"
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"a change":    env.patch("fx-c", `"title":"x"`),
		"a stale":     env.patchBase("fx-c", stale, `"title":"x"`),
		"a no-op":     env.patch("fx-c", `"priority":3`),
		"stale no-op": env.patchBase("fx-c", stale, `"priority":3`),
	} {
		t.Run(name, func(t *testing.T) { expectError(t, rec, http.StatusForbidden, "read_only") })
	}
	// ... but a body that is wrong is still a 400, and an unknown issue a 404.
	expectError(t, env.patch("fx-c", `"title":""`), http.StatusBadRequest, "bad_request")
	expectError(t, env.patchBase("nope", stale, `"title":"x"`), http.StatusNotFound, "not_found")
	expectNoCalls(t, env.fw)
}

func TestPatchStaleBaseIs409(t *testing.T) {
	env := newWriteEnv(t, nil)
	for name, base := range map[string]string{
		"older":      "2026-03-02T09:00:00Z",
		"newer":      "2026-03-02T11:00:00Z",
		"nanosecond": "2026-03-02T10:00:00.000000001Z",
		"other zone": "2026-03-02T10:00:00+01:00",
	} {
		t.Run(name, func(t *testing.T) {
			e := expectError(t, env.patchBase("fx-c", base, `"title":"x"`), http.StatusConflict, "conflict")
			if !strings.Contains(e.Message, "fx-c changed since you opened it") {
				t.Fatalf("message = %q", e.Message)
			}
		})
	}
	// The conflict wins over a no-op, and the tracker is never asked.
	expectError(t, env.patchBase("fx-c", "2026-03-02T09:00:00Z", `"priority":3`), http.StatusConflict, "conflict")
	expectNoCalls(t, env.fw)

	// The same instant in another zone, or with explicit zero fractions, matches.
	decodeWrite(t, env.patchBase("fx-c", "2026-03-02T11:00:00+01:00", `"title":"x"`))
	decodeWrite(t, env.patchBase("fx-c", "2026-03-02T10:00:00.000000000Z", `"title":"y"`))
	if n := len(env.fw.callList()); n != 2 {
		t.Fatalf("calls = %v", env.fw.callList())
	}
}

func TestPatchNanosecondBaseRoundTrips(t *testing.T) {
	// bd times carry nanoseconds; the base must compare exactly.
	nano := time.Date(2026, 3, 2, 10, 0, 0, 123456789, time.UTC)
	env := newWriteEnv(t, nil)
	env.edit(func(is []model.Issue) { issueByID(is, "fx-c").UpdatedAt = nano })
	env.store.Reload()
	if got := env.base("fx-c"); got != "2026-03-02T10:00:00.123456789Z" {
		t.Fatalf("base = %s", got)
	}
	expectError(t, env.patchBase("fx-c", "2026-03-02T10:00:00.123Z", `"title":"x"`), http.StatusConflict, "conflict")
	decodeWrite(t, env.patch("fx-c", `"title":"x"`))
	env.singleCall(t, `update title="x"`)
}

func TestPatchTrackerConflictRefreshesBefore409(t *testing.T) {
	env := newWriteEnv(t, nil)
	waitPhase2(t, env.store)
	before := getMeta(t, env.h)
	env.fw.handle = func(call string) (tracker.Result, error) {
		if call != "refresh" {
			// bd was written to behind bv's back: its export changed.
			env.edit(func(is []model.Issue) {
				i := issueByID(is, "fx-c")
				i.Title = "Changed in bd"
				i.UpdatedAt = ts(5, 10)
			})
			return tracker.Result{}, tracker.ErrConflict
		}
		return tracker.Result{}, nil
	}
	e := expectError(t, env.patch("fx-c", `"title":"Mine"`), http.StatusConflict, "conflict")
	if e.Message == "" {
		t.Fatal("no message")
	}
	if calls := env.fw.callList(); len(calls) != 2 || calls[0] != `update title="Mine"` || calls[1] != "refresh" {
		t.Fatalf("calls = %q, want the update, then a refresh of the export", calls)
	}
	// The store was reloaded before the 409 went out, so the client's refetch sees
	// the new state and can retry with the new base.
	after := getMeta(t, env.h)
	if after.Generation != before.Generation+1 {
		t.Fatalf("generation = %d, want %d: the store was not reloaded", after.Generation, before.Generation+1)
	}
	if got := env.base("fx-c"); got != ts(5, 10).Format(time.RFC3339Nano) {
		t.Fatalf("base after the 409 = %s", got)
	}
	// ... and retrying with it works.
	env.fw.handle = nil
	decodeWrite(t, env.patch("fx-c", `"title":"Mine"`))
}

func TestPatchTrackerErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
		msg    string
	}{
		"bd refuses": {&tracker.CommandError{Stderr: `invalid status "review"`, Err: errors.New("exit 1")}, http.StatusBadGateway, "tracker_failed", `invalid status "review"`},
		"timeout":    {context.DeadlineExceeded, http.StatusGatewayTimeout, "tracker_timeout", ""},
		"other":      {errors.New("boom"), http.StatusInternalServerError, "write_failed", "boom"},
	} {
		t.Run(name, func(t *testing.T) {
			env := newWriteEnv(t, nil)
			env.fw.handle = func(string) (tracker.Result, error) { return tracker.Result{}, tc.err }
			e := expectError(t, env.patch("fx-c", `"status":"review"`), tc.status, tc.code)
			if !strings.Contains(e.Message, tc.msg) {
				t.Fatalf("message = %q, want %q", e.Message, tc.msg)
			}
			env.singleCall(t, `update status="review"`) // no refresh: not a conflict
		})
	}
}

func TestPatchRefreshFailureIsAWarning(t *testing.T) {
	env := newWriteEnv(t, nil)
	env.fw.handle = func(string) (tracker.Result, error) {
		return tracker.Result{RefreshErr: errors.New("bd export failed")}, nil
	}
	got := decodeWrite(t, env.patch("fx-c", `"title":"x"`))
	if !strings.Contains(got.Warning, "bd export failed") {
		t.Fatalf("response = %+v", got)
	}
}

func TestPatchGuard(t *testing.T) {
	env := newWriteEnv(t, nil)
	for name, opt := range map[string]reqOpt{
		"foreign origin":   withOrigin("http://evil.example"),
		"other port":       withOrigin("http://127.0.0.1:9999"),
		"null origin":      withOrigin("null"),
		"missing origin":   withoutOrigin(),
		"foreign host":     withHost("evil.example"),
		"vite origin, dev": withOrigin("http://localhost:5173"),
	} {
		t.Run(name, func(t *testing.T) {
			expectError(t, env.patch("fx-c", `"title":"x"`, opt), http.StatusForbidden, "forbidden")
		})
	}
	t.Run("no session", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := newJSONRequest(http.MethodPatch, "/api/v1/issues/fx-c", `{"base_updated_at":"`+env.base("fx-c")+`","title":"x"}`)
		req.Header.Del("Cookie")
		env.h.ServeHTTP(rec, req)
		expectError(t, rec, http.StatusUnauthorized, "unauthorized")
	})
	expectNoCalls(t, env.fw)

	dev := newWriteEnv(t, func(c *Config) { c.Dev = true; c.Assets = nil })
	decodeWrite(t, dev.patch("fx-c", `"title":"x"`, withOrigin("http://localhost:5173")))
}

func TestPatchWithoutStore(t *testing.T) {
	h := newTestHandler(t, nil)
	body := `{"base_updated_at":"2026-03-02T10:00:00Z","title":"x"}`
	expectError(t, sendJSON(h, http.MethodPatch, "/api/v1/issues/fx-c", body), http.StatusServiceUnavailable, "not_ready")
}

func TestPatchRouting(t *testing.T) {
	env := newWriteEnv(t, nil)
	body := `{"base_updated_at":"` + env.base("fx-c") + `","title":"x"}`
	// Only PATCH /issues/{id} is the editor: other shapes are not routed to it.
	for _, target := range []string{"/api/v1/issues/fx-c/", "/api/v1/issues/fx-c/other", "/api/v1/issues/fx-c/labels", "/api/v1/issues/", "/api/v1/issues"} {
		if rec := sendJSON(env.h, http.MethodPatch, target, body); rec.Code == http.StatusOK {
			t.Fatalf("PATCH %s = 200", target)
		}
	}
	// PATCH on a write sub-path stays the 405 the write endpoints answer.
	rec := sendJSON(env.h, http.MethodPatch, "/api/v1/issues/fx-c/priority", body)
	expectError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if rec.Header().Get("Allow") != "PUT" {
		t.Fatalf("Allow = %q", rec.Header().Get("Allow"))
	}
	// The other methods on the detail are as before.
	if rec := do(env.h, http.MethodGet, "/api/v1/issues/fx-c", withSession()); rec.Code != http.StatusOK {
		t.Fatalf("detail = %d", rec.Code)
	}
	if rec := sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c", body); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT detail = %d", rec.Code)
	}
	// An id with an escaped slash is not an issue: the detail endpoint answers, as for any other method.
	expectError(t, sendJSON(env.h, http.MethodPatch, "/api/v1/issues/fx%2Fc", body), http.StatusMethodNotAllowed, "method_not_allowed")
	expectNoCalls(t, env.fw)

	// An id with characters that need escaping round-trips.
	env.edit(func(is []model.Issue) { is[0].ID, is[0].Title = "fx weird#id", "weird" })
	env.store.Reload()
	b := env.base("fx weird#id")
	decodeWrite(t, sendJSON(env.h, http.MethodPatch, "/api/v1/issues/fx%20weird%23id", fmt.Sprintf(`{"base_updated_at":%q,"title":"x"}`, b)))
}

func TestPatchWritesAreSerialisedWithOtherWrites(t *testing.T) {
	env, bw := newBlockingEnv(t)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		codes <- env.patch("fx-a", `"title":"a"`).Code
	}()
	waitEntered(t, bw)
	wg.Add(1)
	go func() {
		defer wg.Done()
		codes <- sendJSON(env.h, http.MethodPut, "/api/v1/issues/fx-c/priority", `{"priority":0}`).Code
	}()
	time.Sleep(150 * time.Millisecond)
	if n := len(bw.callList()); n != 1 {
		t.Fatalf("the second write started while the PATCH runs: %d calls", n)
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

// A queued PATCH is checked against the state the write before it left behind:
// its base is stale by then and it answers 409 without touching the tracker.
func TestPatchQueuedBehindAnotherWriteSeesItsResult(t *testing.T) {
	env, bw := newBlockingEnv(t)
	base := env.base("fx-c")
	first := make(chan int, 1)
	bw.handle = func(call string) (tracker.Result, error) {
		bw.entered <- struct{}{}
		<-bw.release
		env.edit(func(is []model.Issue) {
			i := issueByID(is, "fx-c")
			i.Title, i.UpdatedAt = "first", ts(6, 10)
		})
		return tracker.Result{}, nil
	}
	go func() { first <- env.patchBase("fx-c", base, `"title":"first"`).Code }()
	waitEntered(t, bw)
	second := make(chan *httptest.ResponseRecorder, 1)
	go func() { second <- env.patchBase("fx-c", base, `"title":"second"`) }()
	time.Sleep(100 * time.Millisecond)
	close(bw.release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("first = %d", code)
	}
	expectError(t, <-second, http.StatusConflict, "conflict")
	if calls := bw.callList(); len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
}

// TestGoldenPatch pins the PATCH request and what a client has to handle that
// the other writes do not produce: the warning of a saved-but-not-reloaded write
// and the stale-base conflict.
func TestGoldenPatch(t *testing.T) {
	full := IssuePatchRequest{
		Title: ptr("Receipt e-mail"), Status: ptr("in_progress"), Priority: ptr(1), Assignee: ptr("ada"),
		Type: ptr("bug"), Description: ptr("Two lines.\n\nSecond paragraph."), BaseUpdatedAt: "2026-03-02T10:00:00Z",
	}
	data, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	var req any
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	testutil.NewGoldenFile(t, goldenDir, "issue-patch-request.json").AssertJSON(req)

	env := newWriteEnv(t, nil)
	env.fw.handle = func(string) (tracker.Result, error) {
		return tracker.Result{RefreshErr: errors.New("bd export timed out")}, nil
	}
	rec := env.patch("fx-c", `"title":"Receipt e-mail"`)
	testutil.NewGoldenFile(t, goldenDir, "write-result-warning.json").AssertJSON(decodeAny(t, rec, http.StatusOK))

	rec = env.patchBase("fx-c", "2026-03-01T10:00:00Z", `"title":"Receipt e-mail"`)
	testutil.NewGoldenFile(t, goldenDir, "error-stale-base.json").AssertJSON(decodeAny(t, rec, http.StatusConflict))
}

func ptr[T any](v T) *T { return &v }
