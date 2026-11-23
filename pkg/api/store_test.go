package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func realComplete(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
	return s.CompletePhase2(ctx)
}

// newSeamStore builds a store over the golden tracker with custom seams.
func newSeamStore(t *testing.T, load loadFunc, complete completeFunc) (*Store, error) {
	t.Helper()
	path := writeGoldenTracker(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store, err := newStore(ctx, path, snapshot.LoadOptions{Now: goldenNow}, load, complete)
	if store != nil {
		t.Cleanup(store.Close)
	}
	return store, err
}

func TestStorePhaseLifecycleAndETag(t *testing.T) {
	gate := make(chan struct{})
	var calls atomic.Int32
	store, err := newSeamStore(t, snapshot.Load, func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		calls.Add(1)
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

	v1 := store.Current()
	if v1.Phase2Ready || !strings.HasSuffix(v1.ETag, `-p1"`) || !strings.HasPrefix(v1.ETag, `"`+v1.Snap.DataHash) {
		t.Fatalf("phase 1 view: ready=%v etag=%s", v1.Phase2Ready, v1.ETag)
	}

	close(gate)
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	v2 := store.Current()
	if !v2.Phase2Ready || !strings.HasSuffix(v2.ETag, `-p2"`) || v2.Phase2Err != "" {
		t.Fatalf("phase 2 view: ready=%v etag=%s err=%q", v2.Phase2Ready, v2.ETag, v2.Phase2Err)
	}
	if v2.Snap.DataHash != v1.Snap.DataHash {
		t.Fatal("DataHash must not change at phase 2; the ETag suffix carries the change")
	}
	if v2.Snap.Triage == nil {
		t.Fatal("phase 2 snapshot has no triage")
	}
	// The one-per-snapshot rule: the phase 2 result view never starts another
	// (waitPhase2 above would still be waiting on it).
	if n := calls.Load(); n != 1 {
		t.Fatalf("complete called %d times, want exactly once", n)
	}
}

func TestMetaConditionalGet(t *testing.T) {
	gate := make(chan struct{})
	store, err := newSeamStore(t, snapshot.Load, func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
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
	get := func(method, tag string) (code int, etag string, body string) {
		var opts []reqOpt
		opts = append(opts, withSession())
		if tag != "" {
			opts = append(opts, func(r *http.Request) { r.Header.Set("If-None-Match", tag) })
		}
		rec := do(h, method, "/api/v1/meta", opts...)
		return rec.Code, rec.Header().Get("ETag"), rec.Body.String()
	}

	code, p1, body := get("GET", "")
	if code != http.StatusOK || p1 == "" {
		t.Fatalf("phase 1: %d etag=%q", code, p1)
	}
	var m MetaResponse
	if err := json.Unmarshal([]byte(body), &m); err != nil || m.Phase2Ready {
		t.Fatalf("phase 1 body: %v ready=%v", err, m.Phase2Ready)
	}
	if code, _, body := get("GET", p1); code != http.StatusNotModified || body != "" {
		t.Fatalf("matching tag at phase 1: %d %q", code, body)
	}

	close(gate)
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}

	// The phase 1 tag no longer matches: the client gets the phase 2 body.
	code, p2, body := get("GET", p1)
	if code != http.StatusOK || p2 == p1 {
		t.Fatalf("stale phase 1 tag after the swap: %d etag=%q (was %q)", code, p2, p1)
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil || !m.Phase2Ready {
		t.Fatalf("phase 2 body: %v ready=%v", err, m.Phase2Ready)
	}
	for _, method := range []string{"GET", "HEAD"} {
		if code, tag, body := get(method, p2); code != http.StatusNotModified || tag != p2 || body != "" {
			t.Fatalf("%s with the current tag: %d etag=%q body=%q", method, code, tag, body)
		}
	}
	// List form, weak validators and "*" all count as a match.
	for _, tag := range []string{`"nope", ` + p2, "W/" + p2, "*"} {
		if code, _, _ := get("GET", tag); code != http.StatusNotModified {
			t.Errorf("If-None-Match %q: %d, want 304", tag, code)
		}
	}
	if code, _, _ := get("GET", `"other"`); code != http.StatusOK {
		t.Errorf("non-matching tag: %d, want 200", code)
	}
}

func TestMetaContent(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	rec := do(h, "GET", "/api/v1/meta", withSession())
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("meta: %d cache-control=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	var m MetaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != testVersion || m.Tier != "small" || m.IssueCount != len(goldenIssues()) || m.ViewCount != m.IssueCount {
		t.Fatalf("meta = %+v", m)
	}
	if !m.Phase2Ready || m.Phase2Skipped || m.Phase2Error != "" {
		t.Fatalf("phase flags = %+v", m)
	}
	if m.Source.Kind != "jsonl" || m.Source.Path == "" || m.Capabilities.Writes {
		t.Fatalf("source/capabilities = %+v %+v", m.Source, m.Capabilities)
	}
	if !m.ReferenceTime.Equal(goldenNow) {
		t.Fatalf("reference_time = %v, want %v", m.ReferenceTime, goldenNow)
	}
	if m.Counts.Closed != 1 || m.Counts.Open == 0 {
		t.Fatalf("counts = %+v", m.Counts)
	}

	// Methods other than GET/HEAD.
	rec = do(h, "POST", "/api/v1/meta", withSession(), withOrigin(localOrigin))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST meta: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	decodeError(t, rec)
}

func TestDataEndpointsWithoutStoreAre503(t *testing.T) {
	h := newTestHandler(t, nil)
	rec := do(h, "GET", "/api/v1/meta", withSession())
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != "not_ready" {
		t.Fatalf("code = %q", e.Code)
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatal("errors must not carry an ETag")
	}
}

func TestStoreLoadPanicIsAnError(t *testing.T) {
	_, err := newSeamStore(t, func(context.Context, string, snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		panic("boom")
	}, realComplete)
	if err == nil || !strings.Contains(err.Error(), "snapshot load panicked: boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestStoreLoadErrorHoldsNoStore(t *testing.T) {
	want := errors.New("no such tracker")
	store, err := newSeamStore(t, func(context.Context, string, snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		return nil, want
	}, realComplete)
	if !errors.Is(err, want) || store != nil {
		t.Fatalf("store=%v err=%v", store, err)
	}
	if _, err := NewStore(context.Background(), "does-not-exist.jsonl", snapshot.LoadOptions{}); err == nil {
		t.Fatal("a missing tracker must fail NewStore")
	}
}

func TestStorePhase2PanicKeepsServingPhase1(t *testing.T) {
	store, err := newSeamStore(t, snapshot.Load, func(context.Context, *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		panic("analysis exploded")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	v := store.Current()
	if v.Phase2Ready || !strings.HasSuffix(v.ETag, `-p1e"`) || !strings.Contains(v.Phase2Err, "analysis exploded") {
		t.Fatalf("view after phase 2 panic: ready=%v etag=%s err=%q", v.Phase2Ready, v.ETag, v.Phase2Err)
	}
	if v.Snap == nil || len(v.Snap.Issues) == 0 {
		t.Fatal("phase 1 data must stay available")
	}

	h := newTestHandler(t, func(c *Config) { c.Store = store })
	rec := do(h, "GET", "/api/v1/meta", withSession())
	var m MetaResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &m) != nil || m.Phase2Ready || !strings.Contains(m.Phase2Error, "analysis exploded") {
		t.Fatalf("meta after phase 2 panic: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStorePhase2ErrorKeepsServingPhase1(t *testing.T) {
	store, err := newSeamStore(t, snapshot.Load, func(context.Context, *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		return nil, errors.New("analysis failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if v := store.Current(); v.Phase2Err != "analysis failed" || !strings.HasSuffix(v.ETag, `-p1e"`) {
		t.Fatalf("view = %+v", v)
	}
}

func TestStoreCloseCancelsPhase2(t *testing.T) {
	entered := make(chan struct{})
	observed := make(chan struct{})
	store, err := newSeamStore(t, snapshot.Load, func(ctx context.Context, _ *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		close(entered)
		<-ctx.Done()
		close(observed)
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	v := store.Current()

	done := make(chan struct{})
	go func() { store.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return: phase 2 was not cancelled")
	}
	select {
	case <-observed:
	default:
		t.Fatal("phase 2 never observed ctx.Done")
	}
	// Cancellation is not a failure: the view is left as it was.
	if got := store.Current(); got != v || got.Phase2Err != "" {
		t.Fatalf("view changed on shutdown: %+v", got)
	}
}

func TestStoreBoundToParentContext(t *testing.T) {
	path := writeGoldenTracker(t)
	parent, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	store, err := newStore(parent, path, snapshot.LoadOptions{Now: goldenNow}, snapshot.Load,
		func(ctx context.Context, _ *snapshot.Snapshot) (*snapshot.Snapshot, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	cancel() // the serve context ending (Ctrl+C) cancels phase 2
	if err := store.waitPhase2(testCtx(t)); err != nil {
		t.Fatalf("phase 2 survived the parent context: %v", err)
	}
	store.Close()
}

func TestStaleLoadOptionsAnalysisContextIsReplaced(t *testing.T) {
	var got context.Context
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	path := writeGoldenTracker(t)
	store, err := newStore(context.Background(), path, snapshot.LoadOptions{AnalysisContext: cancelled, Now: goldenNow},
		func(ctx context.Context, p string, o snapshot.LoadOptions) (*snapshot.Snapshot, error) {
			got = o.AnalysisContext
			return snapshot.Load(ctx, p, o)
		}, realComplete)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got == nil || got == cancelled || got.Err() != nil {
		t.Fatal("the store must bind AnalysisContext to its own lifetime")
	}
}
