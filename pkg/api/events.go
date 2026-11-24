package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// defaultHeartbeat is how often an idle events stream gets a comment line, so
// proxies and the browser see the connection is alive.
const defaultHeartbeat = 25 * time.Second

// eventsRetryMillis is the reconnect delay the stream asks EventSource for.
const eventsRetryMillis = 2000

// signalHub fans a payload-free "something changed" signal out to subscribers.
// Each subscriber has a 1-buffered channel and notify never blocks, so a slow
// client coalesces a burst of changes into one pending signal (the same shape as
// export.LiveReloadHub). The subscriber reads the current state when it wakes.
type signalHub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func (h *signalHub) init() {
	h.subs = make(map[chan struct{}]struct{})
}

// subscribe returns a signal channel and the function that removes it.
func (h *signalHub) subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *signalHub) notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default: // a signal is already pending
		}
	}
}

// snapshotEvent builds the payload of an events "snapshot" message from the
// store's state right now.
func (s *Store) snapshotEvent() SnapshotEvent {
	v := s.Current()
	ev := SnapshotEvent{Phase: "phase1"}
	if v != nil && v.Snap != nil {
		ev.Generation = v.Gen
		ev.Hash = v.Snap.DataHash
		switch {
		case v.Phase2Ready:
			ev.Phase = "phase2"
		case v.Phase2Err != "":
			ev.Phase = "phase2_error"
		}
	}
	if rs := s.lastReloadError(); rs != nil {
		ev.ReloadError = rs.Msg
	}
	return ev
}

// handleEvents is GET /api/v1/events: a text/event-stream of signal-only
// "snapshot" events. One is sent on connect (so a reconnecting client catches up
// on what it missed), then one per publish, phase 2 swap or reload error change.
// The client refetches what it shows; a redundant signal costs only 304s.
func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}
	st := s.cfg.Store
	if st == nil || st.Current() == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "no tracker data is loaded")
		return
	}

	// Subscribe before the first event so a publish in between is not lost.
	signals, unsubscribe := st.hub.subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	send := func(text string) bool {
		if _, err := io.WriteString(w, text); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	sendSnapshot := func() bool {
		body, err := json.Marshal(st.snapshotEvent())
		if err != nil {
			return false
		}
		return send("event: snapshot\ndata: " + string(body) + "\n\n")
	}

	if !send("retry: "+strconv.Itoa(eventsRetryMillis)+"\n\n") || !sendSnapshot() {
		return
	}

	interval := s.cfg.heartbeat
	if interval <= 0 {
		interval = defaultHeartbeat
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-st.Done():
			return
		case <-signals:
			if !sendSnapshot() {
				return
			}
		case <-ticker.C:
			if !send(": ping\n\n") {
				return
			}
		}
	}
}
