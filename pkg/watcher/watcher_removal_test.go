package watcher

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const testGrace = 250 * time.Millisecond

type removalProbe struct {
	path    string
	w       *Watcher
	removed atomic.Int32
	other   atomic.Int32
	changed atomic.Int32
}

// newRemovalProbe watches a JSONL file with a short grace period, through fsnotify or polling.
func newRemovalProbe(t *testing.T, poll bool) *removalProbe {
	t.Helper()
	p := &removalProbe{path: filepath.Join(t.TempDir(), "issues.jsonl")}
	if err := os.WriteFile(p.path, []byte("{\"id\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(p.path,
		WithDebounceDuration(10*time.Millisecond),
		WithPollInterval(20*time.Millisecond),
		WithForcePoll(poll),
		WithRemovalGrace(testGrace),
		WithOnChange(func() { p.changed.Add(1) }),
		WithOnError(func(err error) {
			if err == ErrFileRemoved {
				p.removed.Add(1)
			} else {
				p.other.Add(1)
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	p.w = w
	return p
}

func waitFor(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func eachMode(t *testing.T, fn func(t *testing.T, poll bool)) {
	t.Helper()
	for _, mode := range []struct {
		name string
		poll bool
	}{{"fsnotify", false}, {"polling", true}} {
		t.Run(mode.name, func(t *testing.T) { fn(t, mode.poll) })
	}
}

// A file replaced by a temporary one renamed over it (how bd exports) is not a removal, and the reload fires.
func TestRemovalGrace_RenameSwapIsSilentAndReloads(t *testing.T) {
	eachMode(t, func(t *testing.T, poll bool) {
		p := newRemovalProbe(t, poll)
		before := p.changed.Load()
		tmp := p.path + ".tmp"
		if err := os.WriteFile(tmp, []byte("{\"id\":1}\n{\"id\":2}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, p.path); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the reload", 2*time.Second, func() bool { return p.changed.Load() > before })
		time.Sleep(2 * testGrace)
		if p.removed.Load() != 0 || p.other.Load() != 0 {
			t.Fatalf("a swap reported errors: removed=%d other=%d", p.removed.Load(), p.other.Load())
		}
	})
}

// A gap that is longer than a rename but shorter than the grace period (delete, then write the new file)
// is also not reported, and the file coming back reloads.
func TestRemovalGrace_ShortGapIsSilentAndReloads(t *testing.T) {
	eachMode(t, func(t *testing.T, poll bool) {
		p := newRemovalProbe(t, poll)
		before := p.changed.Load()
		if err := os.Remove(p.path); err != nil {
			t.Fatal(err)
		}
		time.Sleep(testGrace / 5)
		if err := os.WriteFile(p.path, []byte("{\"id\":1}\n{\"id\":3}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the reload after the file came back", 2*time.Second, func() bool { return p.changed.Load() > before })
		time.Sleep(2 * testGrace)
		if p.removed.Load() != 0 || p.other.Load() != 0 {
			t.Fatalf("a short gap reported errors: removed=%d other=%d", p.removed.Load(), p.other.Load())
		}
	})
}

// A file that is really gone is reported once, after the grace period and not before.
func TestRemovalGrace_RealDeleteIsReportedOnceAfterTheGrace(t *testing.T) {
	eachMode(t, func(t *testing.T, poll bool) {
		p := newRemovalProbe(t, poll)
		start := time.Now()
		if err := os.Remove(p.path); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the removal error", 3*time.Second, func() bool { return p.removed.Load() > 0 })
		if waited := time.Since(start); waited < testGrace {
			t.Fatalf("reported after %v, before the %v grace period", waited, testGrace)
		}
		time.Sleep(3 * testGrace) // further polls and events must not report it again
		if n := p.removed.Load(); n != 1 {
			t.Fatalf("removal reported %d times, want once", n)
		}
	})
}

// A file that comes back and is removed again is a new removal.
func TestRemovalGrace_RemovedAgainAfterReturningIsReportedAgain(t *testing.T) {
	eachMode(t, func(t *testing.T, poll bool) {
		p := newRemovalProbe(t, poll)
		if err := os.Remove(p.path); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the first removal error", 3*time.Second, func() bool { return p.removed.Load() == 1 })
		if err := os.WriteFile(p.path, []byte("{\"id\":9}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := p.changed.Load()
		waitFor(t, "the reload", 2*time.Second, func() bool { return p.changed.Load() > before })
		if err := os.Remove(p.path); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the second removal error", 3*time.Second, func() bool { return p.removed.Load() == 2 })
	})
}

// Stopping during the grace period reports nothing.
func TestRemovalGrace_StopCancelsThePendingReport(t *testing.T) {
	eachMode(t, func(t *testing.T, poll bool) {
		p := newRemovalProbe(t, poll)
		if err := os.Remove(p.path); err != nil {
			t.Fatal(err)
		}
		time.Sleep(testGrace / 5)
		p.w.Stop()
		time.Sleep(2 * testGrace)
		if n := p.removed.Load(); n != 0 {
			t.Fatalf("a stopped watcher reported the removal %d times", n)
		}
	})
}

func TestRemovalGrace_DefaultFollowsTheDebounce(t *testing.T) {
	for _, tc := range []struct {
		debounce, want time.Duration
	}{
		{5 * time.Millisecond, 100 * time.Millisecond},
		{200 * time.Millisecond, 400 * time.Millisecond},
		{5 * time.Second, time.Second},
	} {
		w := &Watcher{debounceDuration: tc.debounce}
		if got := w.effectiveRemovalGraceLocked(); got != tc.want {
			t.Errorf("debounce %v: grace %v, want %v", tc.debounce, got, tc.want)
		}
	}
	w := &Watcher{debounceDuration: time.Second, removalGrace: 7 * time.Millisecond}
	if got := w.effectiveRemovalGraceLocked(); got != 7*time.Millisecond {
		t.Errorf("explicit grace ignored: %v", got)
	}
}
