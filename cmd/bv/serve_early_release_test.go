package main

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestServeGUIReleasesSignalsOnCancel holds the HTTP server's drain open with a
// connection that never sends a request (like a browser's speculative
// preconnect; api.Serve waits for it up to its shutdown timeout). The signal
// handling must be given back as soon as the context is cancelled, not after
// that drain, or a second Ctrl+C in the window is swallowed.
func TestServeGUIReleasesSignalsOnCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	issue := `{"id":"t-1","title":"Smoke","status":"open","priority":2,"issue_type":"task"}` + "\n"
	if err := os.WriteFile(path, []byte(issue), 0o644); err != nil {
		t.Fatal(err)
	}

	var released atomic.Int32
	out := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- serveGUI(ctx, serveOptions{
			NoOpen:         true,
			Stdout:         out,
			BeadsPath:      path,
			ProjectDir:     dir,
			ReleaseSignals: func() { released.Add(1) },
		})
	}()
	signIn, err := url.Parse(waitForURL(t, out, "http://127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", signIn.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for released.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if released.Load() != 1 {
		t.Fatalf("ReleaseSignals called %d times after cancel, want 1", released.Load())
	}
	select {
	case err := <-done:
		t.Fatalf("serveGUI returned (%v) before the idle connection held the drain open: the release proved nothing", err)
	default:
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveGUI: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveGUI did not stop after cancel")
	}
	if got := released.Load(); got != 1 {
		t.Fatalf("ReleaseSignals called %d times in total, want 1", got)
	}
}
