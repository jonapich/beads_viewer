package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeShutdownStore stands in for *api.Store: Close blocks until gate is closed,
// like a Close waiting for a bd write.
type fakeShutdownStore struct {
	inFlight bool
	gate     chan struct{}
	events   *eventLog
}

func (f *fakeShutdownStore) WriteInFlight() bool { return f.inFlight }

func (f *fakeShutdownStore) Close() {
	f.events.add("close-start")
	<-f.gate
	f.events.add("close-end")
}

type eventLog struct {
	mu sync.Mutex
	ev []string
}

func (l *eventLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ev = append(l.ev, e)
}

func (l *eventLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.ev...)
}

func TestCloseStoreAnnouncesAndReleasesBeforeWaiting(t *testing.T) {
	events := &eventLog{}
	store := &fakeShutdownStore{inFlight: true, gate: make(chan struct{}), events: events}
	out := &lockedBuffer{}

	done := make(chan struct{})
	go func() {
		closeStore(store, out, func() { events.add("release") })
		close(done)
	}()

	// While the write is "running": the message is out, the signal handling is
	// already given back (a second Ctrl+C can quit), and Close is still waiting.
	waitForURL(t, out, "Waiting for a tracker write to finish")
	deadline := time.Now().Add(5 * time.Second)
	for len(events.get()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := strings.Join(events.get(), ","); got != "release,close-start" {
		t.Fatalf("events while waiting = %q, want release before close", got)
	}
	select {
	case <-done:
		t.Fatal("closeStore returned before the store closed")
	default:
	}

	close(store.gate)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("closeStore did not return once the store closed")
	}
	if got := strings.Join(events.get(), ","); got != "release,close-start,close-end" {
		t.Fatalf("events = %q", got)
	}
}

func TestCloseStoreIsQuietWithoutAWriteInFlight(t *testing.T) {
	events := &eventLog{}
	gate := make(chan struct{})
	close(gate)
	out := &lockedBuffer{}
	closeStore(&fakeShutdownStore{gate: gate, events: events}, out, nil) // nil release is fine
	if out.String() != "" {
		t.Fatalf("unexpected output: %q", out.String())
	}
	if got := strings.Join(events.get(), ","); got != "close-start,close-end" {
		t.Fatalf("events = %q", got)
	}
}

// TestServeGUIReleasesSignalsAtShutdown runs serveGUI over a real tracker and
// cancels the context (what the first Ctrl+C does): the signal handling is
// released exactly once, before "stopped", and with no write running nothing is
// announced. The second-Ctrl+C-quits half needs a real console, see the bead.
func TestServeGUIReleasesSignalsAtShutdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	issue := `{"id":"t-1","title":"Smoke","status":"open","priority":2,"issue_type":"task"}` + "\n"
	if err := os.WriteFile(path, []byte(issue), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	released := 0
	out := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- serveGUI(ctx, serveOptions{
			NoOpen:     true,
			Stdout:     out,
			BeadsPath:  path,
			ProjectDir: dir,
			ReleaseSignals: func() {
				mu.Lock()
				released++
				mu.Unlock()
			},
		})
	}()
	waitForURL(t, out, "http://127.0.0.1:")
	mu.Lock()
	if released != 0 {
		t.Fatalf("signals released before shutdown (%d)", released)
	}
	mu.Unlock()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveGUI: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveGUI did not stop after cancel")
	}
	mu.Lock()
	defer mu.Unlock()
	if released != 1 {
		t.Fatalf("ReleaseSignals called %d times, want 1", released)
	}
	if !strings.Contains(out.String(), "stopped") || strings.Contains(out.String(), "Waiting for a tracker write") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
