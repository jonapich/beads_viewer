package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// eventStream is an open GET /api/v1/events. Blocks (an event or a comment, as
// the blank-line separated text) arrive on blocks; it closes when the stream
// ends.
type eventStream struct {
	blocks chan string
	cancel context.CancelFunc
}

func (e *eventStream) next(t *testing.T, d time.Duration) string {
	t.Helper()
	select {
	case b, ok := <-e.blocks:
		if !ok {
			t.Fatal("the stream ended")
		}
		return b
	case <-time.After(d):
		t.Fatal("no event within " + d.String())
	}
	return ""
}

func (e *eventStream) quiet(d time.Duration) bool {
	select {
	case <-e.blocks:
		return false
	case <-time.After(d):
		return true
	}
}

func (e *eventStream) snapshot(t *testing.T) SnapshotEvent {
	t.Helper()
	block := e.next(t, 5*time.Second)
	data, ok := strings.CutPrefix(block, "event: snapshot\ndata: ")
	if !ok {
		t.Fatalf("not a snapshot event: %q", block)
	}
	var ev SnapshotEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		t.Fatalf("bad snapshot data %q: %v", data, err)
	}
	return ev
}

// openStream connects to base (an httptest server, or a bound listener) with a
// session cookie and the Host the guard expects.
func openStream(t *testing.T, base, host string, port int) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	req.AddCookie(&http.Cookie{Name: cookieNamePrefix + strconv.Itoa(port), Value: testToken})
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		resp.Body.Close()
		t.Fatalf("events: %d content-type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("events Cache-Control = %q, want no-store", cc)
	}
	es := &eventStream{blocks: make(chan string, 64), cancel: cancel}
	go func() {
		defer close(es.blocks)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var cur []string
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				if len(cur) > 0 {
					es.blocks <- strings.Join(cur, "\n")
					cur = nil
				}
				continue
			}
			cur = append(cur, line)
		}
	}()
	return es
}

// eventsServer serves the full handler for store on an httptest server and
// counts the requests being handled.
func eventsServer(t *testing.T, store *Store, mutate func(*Config)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	h := newTestHandler(t, func(c *Config) {
		c.Store = store
		if mutate != nil {
			mutate(c)
		}
	})
	var active atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active.Add(1)
		defer active.Add(-1)
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &active
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEventsSnapshotOnConnectThenPerPublish(t *testing.T) {
	f := newReloadFixture(t, func(f *reloadFixture) completeFunc {
		return func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
			select {
			case <-f.gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return realComplete(ctx, s)
		}
	})
	srv, _ := eventsServer(t, f.store, nil)
	es := openStream(t, srv.URL, localHost, testPort)

	if got := es.next(t, 5*time.Second); got != "retry: 2000" {
		t.Fatalf("first block = %q, want the retry hint", got)
	}
	ev := es.snapshot(t)
	if ev.Generation != 1 || ev.Phase != "phase1" || ev.Hash != f.store.Current().Snap.DataHash || ev.ReloadError != "" {
		t.Fatalf("connect event = %+v", ev)
	}
	if !es.quiet(100 * time.Millisecond) {
		t.Fatal("unexpected event with nothing published")
	}

	rewriteTracker(t, f.path, "fx-new")
	f.store.Reload()
	ev = es.snapshot(t)
	if ev.Generation != 2 || ev.Phase != "phase1" {
		t.Fatalf("reload event = %+v", ev)
	}
	if !es.quiet(100 * time.Millisecond) {
		t.Fatal("a reload must send exactly one event")
	}

	close(f.gate) // the phase 2 swap is an event too
	ev = es.snapshot(t)
	if ev.Generation != 2 || ev.Phase != "phase2" {
		t.Fatalf("phase 2 event = %+v", ev)
	}
	if !es.quiet(100 * time.Millisecond) {
		t.Fatal("unexpected event after the phase 2 swap")
	}

	// A reload error is a snapshot event too.
	boom := errors.New("tracker is unreadable")
	f.fail.Store(&boom)
	f.store.Reload()
	ev = es.snapshot(t)
	if ev.ReloadError != boom.Error() || ev.Generation != 2 || ev.Phase != "phase2" {
		t.Fatalf("reload error event = %+v", ev)
	}
}

func TestEventsHeartbeat(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store) // no phase 2 swap event in the way
	srv, _ := eventsServer(t, store, func(c *Config) { c.heartbeat = 20 * time.Millisecond })
	es := openStream(t, srv.URL, localHost, testPort)
	es.next(t, 5*time.Second) // retry
	es.snapshot(t)
	for range 2 {
		if got := es.next(t, 5*time.Second); got != ": ping" {
			t.Fatalf("block = %q, want a heartbeat comment", got)
		}
	}
}

func TestEventsHandlerEndsWhenClientLeavesOrStoreCloses(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	srv, active := eventsServer(t, store, nil)

	es := openStream(t, srv.URL, localHost, testPort)
	es.snapshot2(t)
	waitFor(t, "the handler to start", func() bool { return active.Load() == 1 })
	es.cancel()
	waitFor(t, "the handler to end after the client left", func() bool { return active.Load() == 0 })

	es = openStream(t, srv.URL, localHost, testPort)
	es.snapshot2(t)
	waitFor(t, "the handler to start", func() bool { return active.Load() == 1 })
	store.Close()
	waitFor(t, "the handler to end after Store.Close", func() bool { return active.Load() == 0 })
}

// snapshot2 consumes the retry hint and the connect snapshot.
func (e *eventStream) snapshot2(t *testing.T) SnapshotEvent {
	t.Helper()
	e.next(t, 5*time.Second)
	return e.snapshot(t)
}

func TestEventsEndpointGuards(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	h := newTestHandler(t, func(c *Config) { c.Store = store })

	if rec := do(h, "GET", "/api/v1/events"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", rec.Code)
	}
	rec := do(h, "POST", "/api/v1/events", withSession(), withOrigin(localOrigin))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET" {
		t.Fatalf("POST: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	decodeError(t, rec)

	noStore := newTestHandler(t, nil)
	rec = do(noStore, "GET", "/api/v1/events", withSession())
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no store: %d", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != "not_ready" {
		t.Fatalf("code = %q", e.Code)
	}
}

func TestServeEndsEventStreamsPromptly(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	h, err := NewHandler(Config{Port: port, Token: testToken, Version: testVersion, Assets: testAssets(), Store: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, ln, h) }()

	es := openStream(t, "http://"+ln.Addr().String(), ln.Addr().String(), port)
	es.snapshot2(t)

	start := time.Now()
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
		if took := time.Since(start); took >= time.Second {
			t.Fatalf("Serve took %v to stop with a stream connected (grace is %v)", took, shutdownTimeout)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return")
	}
}
