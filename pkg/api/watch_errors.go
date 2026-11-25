package api

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Dicklesworthstone/beads_viewer/pkg/debug"
	"github.com/Dicklesworthstone/beads_viewer/pkg/watcher"
)

// watchReporter decides what `bv serve` says about file watcher errors. Most
// are noise to a person at the terminal: the watcher falls back to polling, or a
// file is replaced and comes back within the grace period. Those go to the debug
// log (BV_DEBUG) with the path. Only an error that leaves the view stale for
// good is printed, once until the file is readable again, and it names the file
// and what is happening.
type watchReporter struct {
	path string
	out  io.Writer

	mu     sync.Mutex
	warned string // the stale-view warning already printed, until the file recovers
}

func newWatchReporter(path string, out io.Writer) *watchReporter {
	return &watchReporter{path: path, out: out}
}

// handle is the watcher's OnError callback.
func (r *watchReporter) handle(err error) {
	debug.Log("bv serve: file watcher on %s: %v", r.path, err)
	var warning string
	switch {
	case errors.Is(err, watcher.ErrFileRemoved):
		warning = fmt.Sprintf("bv serve: %s is gone; the view stops updating until it comes back", r.path)
	case errors.Is(err, watcher.ErrPermission):
		warning = fmt.Sprintf("bv serve: cannot read %s (permission denied); the view stops updating until it can", r.path)
	default:
		return // ErrWatchUnavailable (polling instead) and the like: updates continue
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.warned == warning {
		return
	}
	r.warned = warning
	fmt.Fprintln(r.out, warning)
}

// recovered is called when the watcher sees the file change again: a later loss is news again.
func (r *watchReporter) recovered() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.warned != "" {
		debug.Log("bv serve: file watcher on %s: updating again", r.path)
		r.warned = ""
	}
}
