package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
)

// trackerDir is the temp dir that holds path's .beads directory.
func trackerDir(path string) string { return filepath.Dir(filepath.Dir(path)) }

// rewriteTracker replaces the fixture tracker's issues with the golden issues
// plus extra, so the next load has a different data hash.
func rewriteTracker(t *testing.T, path string, extra ...string) {
	t.Helper()
	issues := goldenIssues()
	for _, id := range extra {
		issues = append(issues, model.Issue{
			ID: id, Title: "Added " + id, Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask,
			CreatedAt: ts(4, 10), UpdatedAt: ts(4, 10),
		})
	}
	testutil.WriteBeadsFile(t, trackerDir(path), issues)
}

// expectSignal reports whether the hub woke ch within d.
func expectSignal(ch <-chan struct{}, d time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}

// reloadFixture is a store over the fixture tracker whose load can be made to
// fail and whose Phase 2 can be gated.
type reloadFixture struct {
	store *Store
	path  string
	gate  chan struct{}
	fail  atomic.Pointer[error]
}

func newReloadFixture(t *testing.T, complete func(f *reloadFixture) completeFunc) *reloadFixture {
	t.Helper()
	f := &reloadFixture{path: writeGoldenTracker(t), gate: make(chan struct{})}
	load := func(ctx context.Context, p string, o snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		if err := f.fail.Load(); err != nil {
			return nil, *err
		}
		return snapshot.Load(ctx, p, o)
	}
	if complete == nil {
		complete = func(*reloadFixture) completeFunc { return realComplete }
	}
	ctx, cancel := context.WithCancel(context.Background())
	store, err := newStore(ctx, f.path, snapshot.LoadOptions{Now: goldenNow}, load, complete(f))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); cancel() })
	f.store = store
	return f
}

func TestReloadPublishesNewGeneration(t *testing.T) {
	f := newReloadFixture(t, nil)
	waitPhase2(t, f.store)
	v1 := f.store.Current()
	if v1.Gen != 1 || !strings.Contains(v1.ETag, "-g1-") {
		t.Fatalf("first view: gen=%d etag=%s", v1.Gen, v1.ETag)
	}
	signals, unsub := f.store.hub.subscribe()
	defer unsub()

	rewriteTracker(t, f.path, "fx-new")
	f.store.Reload()
	if !expectSignal(signals, time.Second) {
		t.Fatal("a published reload sent no signal")
	}
	v2 := f.store.Current()
	if v2 == v1 || v2.Gen != 2 || v2.ETag == v1.ETag || !strings.Contains(v2.ETag, "-g2-") {
		t.Fatalf("second view: gen=%d etag=%s (was %s)", v2.Gen, v2.ETag, v1.ETag)
	}
	if len(v2.Snap.Issues) != len(v1.Snap.Issues)+1 {
		t.Fatalf("reload did not pick up the new issue: %d -> %d", len(v1.Snap.Issues), len(v2.Snap.Issues))
	}

	// Phase 2 of the new view finishes, keeps the generation, and signals.
	waitPhase2(t, f.store)
	v3 := f.store.Current()
	if v3.Gen != 2 || !v3.Phase2Ready || !strings.HasSuffix(v3.ETag, `-g2-p2"`) {
		t.Fatalf("after phase 2: gen=%d ready=%v etag=%s", v3.Gen, v3.Phase2Ready, v3.ETag)
	}
}

func TestReloadWithSameDataPublishesNothing(t *testing.T) {
	f := newReloadFixture(t, nil)
	waitPhase2(t, f.store)
	v1 := f.store.Current()
	signals, unsub := f.store.hub.subscribe()
	defer unsub()

	f.store.Reload() // notifies synchronously, so the check is instant
	select {
	case <-signals:
		t.Fatal("a no-op reload sent a signal")
	default:
	}
	if got := f.store.Current(); got != v1 || got.Gen != 1 {
		t.Fatalf("a no-op reload replaced the view: gen=%d", got.Gen)
	}
}

func TestReloadErrorKeepsViewAndShowsInMeta(t *testing.T) {
	f := newReloadFixture(t, nil)
	waitPhase2(t, f.store)
	v1 := f.store.Current()
	h := newTestHandler(t, func(c *Config) { c.Store = f.store })
	meta := func() (MetaResponse, string, string) {
		t.Helper()
		rec := do(h, "GET", "/api/v1/meta", withSession())
		var m MetaResponse
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &m) != nil {
			t.Fatalf("meta: %d %s", rec.Code, rec.Body.String())
		}
		return m, rec.Header().Get("ETag"), rec.Body.String()
	}
	issuesTag := func() string {
		return do(h, "GET", "/api/v1/issues", withSession()).Header().Get("ETag")
	}
	m0, metaTag0, _ := meta()
	issuesTag0 := issuesTag()
	if m0.ReloadError != "" || m0.ReloadErrorAt != nil {
		t.Fatalf("fresh meta carries a reload error: %+v", m0)
	}
	signals, unsub := f.store.hub.subscribe()
	defer unsub()

	boom := errors.New("tracker is unreadable")
	f.fail.Store(&boom)
	f.store.Reload()
	if !expectSignal(signals, time.Second) {
		t.Fatal("a reload error sent no signal")
	}
	if f.store.Current() != v1 {
		t.Fatal("a failed reload replaced the view")
	}
	m1, metaTag1, _ := meta()
	if m1.ReloadError != boom.Error() || m1.ReloadErrorAt == nil || m1.Generation != 1 || metaTag1 == metaTag0 {
		t.Fatalf("meta after a failed reload: %+v etag %s -> %s", m1, metaTag0, metaTag1)
	}
	if issuesTag() != issuesTag0 {
		t.Fatal("a reload error must change only the meta ETag")
	}
	// A conditional GET with the old tag gets the new body, one with the new tag a 304.
	rec := do(h, "GET", "/api/v1/meta", withSession(), func(r *http.Request) { r.Header.Set("If-None-Match", metaTag1) })
	if rec.Code != http.StatusNotModified {
		t.Fatalf("meta with the current tag: %d", rec.Code)
	}

	// A second failure changes the meta ETag again.
	f.store.Reload()
	if _, tag, _ := meta(); tag == metaTag1 {
		t.Fatal("a second failure kept the meta ETag")
	}

	// The next good reload (same data) clears the error and tells clients.
	f.fail.Store(nil)
	expectSignal(signals, 10*time.Millisecond) // drain
	f.store.Reload()
	if !expectSignal(signals, time.Second) {
		t.Fatal("clearing a reload error sent no signal")
	}
	m2, metaTag2, _ := meta()
	if m2.ReloadError != "" || m2.ReloadErrorAt != nil || metaTag2 != metaTag0 {
		t.Fatalf("meta after recovery: %+v etag %s (want %s)", m2, metaTag2, metaTag0)
	}
}

func TestReloadSupersedesPhase2(t *testing.T) {
	first := make(chan error, 1)
	complete := func(f *reloadFixture) completeFunc {
		return func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
			if len(s.Issues) == len(goldenIssues()) {
				<-ctx.Done() // the first view's phase 2 never finishes on its own
				first <- ctx.Err()
				return nil, ctx.Err()
			}
			select {
			case <-f.gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return realComplete(ctx, s)
		}
	}
	f := newReloadFixture(t, complete)
	rewriteTracker(t, f.path, "fx-new")
	f.store.Reload()

	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first phase 2 ended with %v, want cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("publishing a newer view did not cancel the old phase 2")
	}
	close(f.gate)
	waitPhase2(t, f.store)
	v := f.store.Current()
	if v.Gen != 2 || !v.Phase2Ready || v.Phase2Err != "" || len(v.Snap.Issues) != len(goldenIssues())+1 {
		t.Fatalf("view after the superseded phase 2: gen=%d ready=%v err=%q issues=%d", v.Gen, v.Phase2Ready, v.Phase2Err, len(v.Snap.Issues))
	}
}

func TestReloadAfterCloseIsANoOp(t *testing.T) {
	f := newReloadFixture(t, nil)
	f.store.Close()
	v := f.store.Current()
	rewriteTracker(t, f.path, "fx-new")
	f.store.Reload()
	if f.store.Current() != v {
		t.Fatal("Reload after Close published a view")
	}
}

func TestStartWatchingReloadsOnFileChange(t *testing.T) {
	path := writeGoldenTracker(t)
	store := goldenStore(t, path)
	if store.Watching() {
		t.Fatal("a new store is not watching")
	}
	if err := store.StartWatching(); err != nil {
		t.Fatal(err)
	}
	if !store.Watching() {
		t.Fatal("StartWatching did not report watching")
	}
	if err := store.StartWatching(); err == nil {
		t.Fatal("a second StartWatching must fail")
	}
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	var m MetaResponse
	if rec := do(h, "GET", "/api/v1/meta", withSession()); json.Unmarshal(rec.Body.Bytes(), &m) != nil || !m.Capabilities.Events {
		t.Fatalf("meta capabilities.events = %v, want true while watching", m.Capabilities.Events)
	}

	rewriteTracker(t, path, "fx-live")
	deadline := time.Now().Add(10 * time.Second)
	for store.Current().Gen < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the store did not reload after the tracker file changed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(store.Current().Snap.Issues); n != len(goldenIssues())+1 {
		t.Fatalf("reloaded view has %d issues", n)
	}

	store.Close()
	if store.Watching() {
		t.Fatal("Close left the watcher running")
	}
}

func TestStartWatchingWithoutEventsCapabilityByDefault(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	var m MetaResponse
	rec := do(h, "GET", "/api/v1/meta", withSession())
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || m.Capabilities.Events {
		t.Fatalf("capabilities.events = %v (err %v), want false without a watcher", m.Capabilities.Events, err)
	}
}

// TestCloseDuringReloadPublishesNothing: a Reload that is mid-load when Close
// runs must not publish, start phase 2 or notify once the store is closed, and
// Close must not hang or race its WaitGroup.
func TestCloseDuringReloadPublishesNothing(t *testing.T) {
	path := writeGoldenTracker(t)
	entered := make(chan struct{})
	gate := make(chan struct{})
	var loads atomic.Int32
	load := func(ctx context.Context, p string, o snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		if loads.Add(1) == 2 {
			close(entered)
			<-gate // ignores ctx on purpose: a load that finishes after Close
		}
		return snapshot.Load(ctx, p, o)
	}
	store, err := newStore(context.Background(), path, snapshot.LoadOptions{Now: goldenNow}, load, realComplete)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	waitPhase2(t, store)
	v1 := store.Current()
	signals, unsub := store.hub.subscribe()
	defer unsub()

	rewriteTracker(t, path, "fx-new")
	reloaded := make(chan struct{})
	go func() { store.Reload(); close(reloaded) }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the reload never started loading")
	}
	closed := make(chan struct{})
	go func() { store.Close(); close(closed) }()
	waitFor(t, "Close to cancel the store", func() bool { return store.ctx.Err() != nil })
	close(gate)

	for name, ch := range map[string]chan struct{}{"Reload": reloaded, "Close": closed} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s did not return", name)
		}
	}
	if got := store.Current(); got != v1 || got.Gen != 1 {
		t.Fatalf("a reload that finished after Close published: gen=%d", got.Gen)
	}
	select {
	case <-signals:
		t.Fatal("a reload that finished after Close sent a signal")
	default:
	}
}
