package api

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/Dicklesworthstone/beads_viewer/pkg/watcher"
)

// ETag suffixes. DataHash does not change when Phase 2 completes (the data is
// the same), so the suffix is what makes the ETag change with the view.
const (
	etagPhase1      = "p1"  // phase 1 snapshot, Phase 2 still running
	etagPhase2      = "p2"  // the CompletePhase2 result
	etagPhase2Error = "p1e" // phase 1 data, Phase 2 failed
)

// View is one immutable published state of the store. Handlers read it once per
// request (Store.Current) and use that single View for both the ETag and the
// body, so the two can never disagree.
type View struct {
	Snap *snapshot.Snapshot
	// Gen is the store generation: 1 for the first load, plus one per published
	// reload. A phase 2 swap keeps the Gen of the view it replaces.
	Gen uint64
	// ETag is the quoted entity tag: data hash, generation and a phase suffix.
	// The generation matters because an unchanged data hash does not mean an
	// unchanged body (the reference time moves with every load).
	ETag string
	// Phase2Ready is true only for the CompletePhase2 result. It is not
	// Snapshot.Phase2Ready, which is already true at phase 1 on the huge tier
	// (Phase 2 is skipped there and triage is nil).
	Phase2Ready bool
	// Phase2Err is set when Phase 2 failed; the view then keeps serving the
	// phase 1 data.
	Phase2Err string

	// Lazily built per-view state; see list.go. Handlers reach it through
	// list() and listTree(), never directly.
	listOnce sync.Once
	listIdx  *listIndex
	treeOnce sync.Once
	tree     *ListTree
}

func newView(snap *snapshot.Snapshot, gen uint64, suffix string, phase2Ready bool, phase2Err string) *View {
	return &View{
		Snap:        snap,
		Gen:         gen,
		ETag:        fmt.Sprintf("%q", fmt.Sprintf("%s-g%d-%s", snap.DataHash, gen, suffix)),
		Phase2Ready: phase2Ready,
		Phase2Err:   phase2Err,
	}
}

type loadFunc func(context.Context, string, snapshot.LoadOptions) (*snapshot.Snapshot, error)
type completeFunc func(context.Context, *snapshot.Snapshot) (*snapshot.Snapshot, error)

// watchDebounce is the file-change debounce, the same as the TUI's
// (pkg/ui/model.go).
const watchDebounce = 200 * time.Millisecond

// reloadStatus is the last failed reload. It lives outside View so a failure
// never replaces the served view and never breaks an in-flight phase 2 swap.
type reloadStatus struct {
	Msg string
	At  time.Time
	// Seq goes up on every failure, so the meta ETag changes with each one.
	Seq uint64
}

// Store holds the current View behind an atomic pointer. Phase 1 is loaded
// synchronously by NewStore, so the first request already has data; Phase 2
// runs once per published view and is swapped in when it finishes. Reload (fed
// by StartWatching) publishes a newer view and cancels the older view's Phase 2.
type Store struct {
	ctx    context.Context // bounds loads and phase 2; derived from the serve context
	cancel context.CancelFunc
	cur    atomic.Pointer[View]
	wg     sync.WaitGroup // the watch loop goroutine
	p2wg   sync.WaitGroup // phase 2 goroutines

	path string
	opts snapshot.LoadOptions

	load     loadFunc     // snapshot.Load
	complete completeFunc // (*snapshot.Snapshot).CompletePhase2

	// reloadMu serialises Reload and publish and guards gen, errSeq and
	// stopPhase2.
	reloadMu   sync.Mutex
	gen        uint64
	errSeq     uint64
	stopPhase2 context.CancelFunc // cancels the Phase 2 of the current view
	reloadErr  atomic.Pointer[reloadStatus]

	hub      signalHub
	watching atomic.Bool

	watchMu sync.Mutex
	w       *watcher.Watcher
}

// NewStore loads path as a phase 1 snapshot and starts Phase 2 in the
// background. ctx bounds the store's lifetime: cancelling it (or calling Close)
// cancels Phase 2 and any reload. opts.AnalysisContext is overwritten with a
// per-load context derived from that lifetime.
func NewStore(ctx context.Context, path string, opts snapshot.LoadOptions) (*Store, error) {
	return newStore(ctx, path, opts, snapshot.Load, func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		return s.CompletePhase2(ctx)
	})
}

func newStore(ctx context.Context, path string, opts snapshot.LoadOptions, load loadFunc, complete completeFunc) (*Store, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sctx, cancel := context.WithCancel(ctx)
	s := &Store{ctx: sctx, cancel: cancel, path: path, opts: opts, load: load, complete: complete}
	s.hub.init()

	snap, lctx, lcancel, err := s.loadSnapshot()
	if err != nil {
		cancel()
		return nil, err
	}
	s.gen = 1
	s.publish(newView(snap, s.gen, etagPhase1, false, ""), lctx, lcancel)
	return s, nil
}

// Current returns the current View. It is never nil on a Store from NewStore.
func (s *Store) Current() *View {
	return s.cur.Load()
}

// lastReloadError returns the last failed reload, or nil when the last reload
// succeeded (or none ran).
func (s *Store) lastReloadError() *reloadStatus {
	return s.reloadErr.Load()
}

// Watching reports whether the file watcher is running.
func (s *Store) Watching() bool {
	return s.watching.Load()
}

// Done is closed when the store is closed or its parent context ends.
func (s *Store) Done() <-chan struct{} {
	return s.ctx.Done()
}

// Close stops the watcher, cancels loads and Phase 2, and waits for their
// goroutines to finish. A Reload still running is not interrupted past its load:
// it publishes nothing once the store is closed.
func (s *Store) Close() {
	s.cancel()
	s.watchMu.Lock()
	w := s.w
	s.watchMu.Unlock()
	if w != nil {
		w.Stop()
	}
	s.wg.Wait()
	// Barrier: a Reload that takes reloadMu after this sees the cancelled
	// context and returns; one that holds it now finishes its p2wg.Add first.
	// Either way no p2wg.Add can race the Wait below.
	s.reloadMu.Lock()
	s.reloadMu.Unlock() //nolint:staticcheck // empty critical section is the barrier
	s.p2wg.Wait()
}

// waitPhase2 blocks until every running Phase 2 goroutine has finished or ctx
// ends. Tests only.
func (s *Store) waitPhase2(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.p2wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// publish makes v current, cancels the Phase 2 of the view it replaces and
// starts Phase 2 for v under lctx. It is the only place that starts Phase 2, and
// a phase 2 result view is stored directly rather than published, so
// CompletePhase2 runs once per loaded snapshot. The caller holds reloadMu (or is
// newStore, before the store is shared). publish does not notify.
func (s *Store) publish(v *View, lctx context.Context, lcancel context.CancelFunc) {
	s.cur.Store(v)
	if s.stopPhase2 != nil {
		s.stopPhase2()
	}
	s.stopPhase2 = lcancel
	s.p2wg.Add(1)
	go s.phase2(lctx, v)
}

func (s *Store) phase2(ctx context.Context, v *View) {
	defer s.p2wg.Done()
	next, err := s.safeComplete(ctx, v.Snap)
	if err != nil || next == nil {
		if ctx.Err() != nil {
			return // superseded or shutting down; the failure is the cancellation
		}
		msg := "phase 2 produced no snapshot"
		if err != nil {
			msg = err.Error()
		}
		// CAS, not Store: a newer published view must not be overwritten by a
		// stale phase 2.
		if s.cur.CompareAndSwap(v, newView(v.Snap, v.Gen, etagPhase2Error, false, msg)) {
			s.notify()
		}
		return
	}
	if s.cur.CompareAndSwap(v, newView(next, v.Gen, etagPhase2, true, "")) {
		s.notify()
	}
}

// Reload loads the tracker again and publishes the result as a new view. It is
// safe to call concurrently (calls are serialised). A failed load keeps the
// current view and records the error (see lastReloadError); a load whose data is
// the same as the current view's publishes nothing.
func (s *Store) Reload() {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	if s.ctx.Err() != nil {
		return
	}

	snap, lctx, lcancel, err := s.loadSnapshot()
	if err != nil {
		if s.ctx.Err() != nil {
			return // shutting down; the failure is the cancellation
		}
		s.errSeq++
		s.reloadErr.Store(&reloadStatus{Msg: err.Error(), At: time.Now(), Seq: s.errSeq})
		s.notify()
		return
	}

	if s.ctx.Err() != nil {
		lcancel() // closed while loading: publish nothing
		return
	}

	cur := s.Current()
	if cur != nil && cur.Snap != nil &&
		snap.DataHash == cur.Snap.DataHash && snap.AuthorityHash == cur.Snap.AuthorityHash && cur.Phase2Err == "" {
		// Nothing changed (bd can re-export identical data, and the watcher can
		// fire twice for a write plus rename). Keep the view, and tell clients
		// only if a reload error just cleared.
		lcancel()
		if s.reloadErr.Swap(nil) != nil {
			s.notify()
		}
		return
	}

	s.gen++
	s.publish(newView(snap, s.gen, etagPhase1, false, ""), lctx, lcancel)
	s.reloadErr.Store(nil)
	s.notify()
}

// notify wakes every events subscriber. The payload is read at send time.
func (s *Store) notify() {
	s.hub.notify()
}

// loadSnapshot loads the tracker under a fresh per-load context that also bounds
// the load's background analysis. The caller owns lcancel (publish takes it); on
// error it is already called.
func (s *Store) loadSnapshot() (*snapshot.Snapshot, context.Context, context.CancelFunc, error) {
	lctx, lcancel := context.WithCancel(s.ctx)
	opts := s.opts
	opts.AnalysisContext = lctx
	snap, err := s.safeLoad(s.path, opts)
	if err != nil {
		lcancel()
		return nil, nil, nil, err
	}
	return snap, lctx, lcancel, nil
}

// StartWatching starts a file watcher on the tracker path, with the same
// debounce as the TUI, and reloads whenever it fires. The watcher stops with
// Close. A failure leaves the store serving its loaded data, without updates.
func (s *Store) StartWatching() error {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.w != nil {
		return watcher.ErrAlreadyStarted
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	w, err := watcher.NewWatcher(s.path,
		watcher.WithDebounceDuration(watchDebounce),
		watcher.WithOnError(func(err error) {
			// ErrWatchUnavailable means it fell back to polling; stderr only.
			fmt.Fprintf(os.Stderr, "bv serve: file watcher: %v\n", err)
		}))
	if err != nil {
		return err
	}
	if err := w.Start(); err != nil {
		return err
	}
	s.w = w
	s.watching.Store(true)
	s.wg.Add(1)
	go s.watchLoop(w)
	return nil
}

func (s *Store) watchLoop(w *watcher.Watcher) {
	defer s.wg.Done()
	defer s.watching.Store(false)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-w.Done():
			return
		case <-w.Changed():
			s.Reload()
		}
	}
}

func (s *Store) safeLoad(path string, opts snapshot.LoadOptions) (snap *snapshot.Snapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			snap, err = nil, fmt.Errorf("snapshot load panicked: %v", r)
		}
	}()
	snap, err = s.load(s.ctx, path, opts)
	if err == nil && snap == nil {
		err = fmt.Errorf("snapshot load returned no snapshot")
	}
	return snap, err
}

func (s *Store) safeComplete(ctx context.Context, snap *snapshot.Snapshot) (next *snapshot.Snapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			next, err = nil, fmt.Errorf("phase 2 analysis panicked: %v", r)
		}
	}()
	return s.complete(ctx, snap)
}
