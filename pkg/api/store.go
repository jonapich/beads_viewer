package api

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
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
	// ETag is the quoted entity tag: data hash plus a phase suffix.
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

func newView(snap *snapshot.Snapshot, suffix string, phase2Ready bool, phase2Err string) *View {
	return &View{
		Snap:        snap,
		ETag:        fmt.Sprintf("%q", snap.DataHash+"-"+suffix),
		Phase2Ready: phase2Ready,
		Phase2Err:   phase2Err,
	}
}

type loadFunc func(context.Context, string, snapshot.LoadOptions) (*snapshot.Snapshot, error)
type completeFunc func(context.Context, *snapshot.Snapshot) (*snapshot.Snapshot, error)

// Store holds the current View behind an atomic pointer. Phase 1 is loaded
// synchronously by NewStore, so the first request already has data; Phase 2
// runs once per published view and is swapped in when it finishes.
type Store struct {
	ctx    context.Context // bounds phase 2; derived from the serve context
	cancel context.CancelFunc
	cur    atomic.Pointer[View]
	wg     sync.WaitGroup // phase 2 goroutines

	load     loadFunc     // snapshot.Load
	complete completeFunc // (*snapshot.Snapshot).CompletePhase2
}

// NewStore loads path as a phase 1 snapshot and starts Phase 2 in the
// background. ctx bounds the store's lifetime: cancelling it (or calling Close)
// cancels Phase 2. opts.AnalysisContext is overwritten with that lifetime.
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
	s := &Store{ctx: sctx, cancel: cancel, load: load, complete: complete}
	opts.AnalysisContext = sctx

	snap, err := s.safeLoad(path, opts)
	if err != nil {
		cancel()
		return nil, err
	}
	s.publish(newView(snap, etagPhase1, false, ""))
	return s, nil
}

// Current returns the current View. It is never nil on a Store from NewStore.
func (s *Store) Current() *View {
	return s.cur.Load()
}

// Close cancels Phase 2 and waits for its goroutines to finish.
func (s *Store) Close() {
	s.cancel()
	s.wg.Wait()
}

// waitPhase2 blocks until every running Phase 2 goroutine has finished or ctx
// ends. Tests only.
func (s *Store) waitPhase2(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// publish makes v current and starts Phase 2 for it. It is the only place that
// starts Phase 2, and a phase 2 result view is stored directly rather than
// published, so CompletePhase2 runs once per loaded snapshot.
func (s *Store) publish(v *View) {
	s.cur.Store(v)
	s.wg.Add(1)
	go s.phase2(v)
}

func (s *Store) phase2(v *View) {
	defer s.wg.Done()
	next, err := s.safeComplete(v.Snap)
	if err != nil || next == nil {
		if s.ctx.Err() != nil {
			return // shutting down; the failure is the cancellation
		}
		msg := "phase 2 produced no snapshot"
		if err != nil {
			msg = err.Error()
		}
		// CAS, not Store: a newer published view must not be overwritten by a
		// stale phase 2 (reloads arrive with beads-c2o.6).
		s.cur.CompareAndSwap(v, newView(v.Snap, etagPhase2Error, false, msg))
		return
	}
	s.cur.CompareAndSwap(v, newView(next, etagPhase2, true, ""))
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

func (s *Store) safeComplete(snap *snapshot.Snapshot) (next *snapshot.Snapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			next, err = nil, fmt.Errorf("phase 2 analysis panicked: %v", r)
		}
	}()
	return s.complete(s.ctx, snap)
}
