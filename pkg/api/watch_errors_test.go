package api

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
	"github.com/Dicklesworthstone/beads_viewer/pkg/watcher"
)

// lockedBuffer is a Writer the watcher goroutines and the test can share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestWatchReporterPrintsOnlyWhatLeavesTheViewStale(t *testing.T) {
	var out lockedBuffer
	r := newWatchReporter(`C:\work\.beads\issues.jsonl`, &out)

	// Noise: polling instead of events, and errors that do not stop updates.
	r.handle(watcher.ErrWatchUnavailable)
	r.handle(errors.New("some transient trouble"))
	if out.String() != "" {
		t.Fatalf("noise reached the terminal: %q", out.String())
	}

	r.handle(watcher.ErrFileRemoved)
	want := "bv serve: C:\\work\\.beads\\issues.jsonl is gone; the view stops updating until it comes back\n"
	if out.String() != want {
		t.Fatalf("removal message = %q, want %q", out.String(), want)
	}
	r.handle(watcher.ErrFileRemoved) // the same condition again: said once
	if out.String() != want {
		t.Fatalf("a repeat was printed: %q", out.String())
	}

	// The file came back and was lost again: that is news again.
	r.recovered()
	r.handle(watcher.ErrFileRemoved)
	if out.String() != want+want {
		t.Fatalf("a new loss after recovery was not printed: %q", out.String())
	}
}

func TestWatchReporterPermissionDeniedNamesTheFileOnce(t *testing.T) {
	var out lockedBuffer
	r := newWatchReporter("/data/issues.jsonl", &out)
	for range 3 { // the poller reports it on every pass
		r.handle(watcher.ErrPermission)
	}
	if got, want := out.String(), "bv serve: cannot read /data/issues.jsonl (permission denied); the view stops updating until it can\n"; got != want {
		t.Fatalf("permission message = %q, want %q", got, want)
	}
	// A wrapped error still counts.
	r.recovered()
	r.handle(errors.Join(errors.New("stat"), watcher.ErrPermission))
	if strings.Count(out.String(), "cannot read") != 2 {
		t.Fatalf("a wrapped permission error after recovery was not printed: %q", out.String())
	}
}

// bd replaces the tracker file by writing a temporary one over it on every write: that is silent
// and the store still reloads. A file that is really deleted is reported once, naming the path.
func TestStartWatchingIsSilentForASwapAndReportsARealDelete(t *testing.T) {
	path := writeGoldenTracker(t)
	store := goldenStore(t, path)
	var out lockedBuffer
	store.watchOut = &out
	if err := store.StartWatching(); err != nil {
		t.Fatal(err)
	}

	// A new file written elsewhere, then renamed over the tracker file: the way bd exports.
	swapped := append(goldenIssues(), model.Issue{
		ID: "fx-swap", Title: "Added fx-swap", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask,
		CreatedAt: ts(4, 10), UpdatedAt: ts(4, 10),
	})
	replacement := testutil.WriteBeadsFile(t, testutil.TempBeadsDir(t), swapped)
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the reload after the swap", 10*time.Second, func() bool { return store.Current().Gen >= 2 })
	time.Sleep(2 * watchRemovalGrace)
	if out.String() != "" {
		t.Fatalf("a swap printed %q", out.String())
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the deletion warning", 10*time.Second, func() bool { return out.String() != "" })
	time.Sleep(watchRemovalGrace)
	got := out.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, path) || !strings.Contains(got, "is gone") {
		t.Fatalf("deletion output = %q, want one line naming %s", got, path)
	}
}

func waitUntil(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
