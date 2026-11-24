package api

import (
	"context"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

func TestStoreWriteInFlight(t *testing.T) {
	store, err := newSeamStore(t, snapshot.Load, realComplete)
	if err != nil {
		t.Fatal(err)
	}
	if store.WriteInFlight() {
		t.Fatal("a fresh store has no write in flight")
	}

	unlock, err := store.lockWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !store.WriteInFlight() {
		t.Fatal("a held write semaphore must report a write in flight")
	}

	// Close waits for the write; it is only a barrier, so it must not cancel it.
	closed := make(chan struct{})
	go func() { store.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a write was in flight")
	case <-time.After(100 * time.Millisecond):
	}
	if !store.WriteInFlight() {
		t.Fatal("Close must not release a write in flight")
	}

	unlock()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return after the write finished")
	}
	if store.WriteInFlight() {
		t.Fatal("no write in flight after Close")
	}
}
