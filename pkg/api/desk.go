package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
)

// The agent desk endpoints (beads-c2o.11) serve pkg/agentdesk's own types as
// they are: the GUI shows the same inbox, pipeline and slice report the TUI
// and the robot flags do. The handlers only make the empty collections [].

// handleInbox serves GET /api/v1/inbox: the open beads labelled needs-decision
// or needs-design, grouped by epic, in agentdesk.BuildInbox order. Its total
// equals meta.counts.awaiting.
func (s *server) handleInbox(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	writeView(w, r, v, func() any {
		inbox := agentdesk.BuildInbox(v.Snap.Issues, agentdesk.AwaitingLabels)
		if inbox.Groups == nil {
			inbox.Groups = []agentdesk.InboxGroup{}
		}
		return inbox
	})
}

// handlePipeline serves GET /api/v1/pipeline: the in-progress beads with their
// stage and staleness. Staleness depends on the clock, not only on the data, so
// the response is measured at the start of the current minute and its ETag is
// the snapshot's plus that minute (<snapshot tag>~m<unix minutes>): the same
// body for the same ETag, and a client that polls sees stale flags appear.
func (s *server) handlePipeline(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	now := s.now().Truncate(time.Minute)
	etag := variantETag(v.ETag, "m"+strconv.FormatInt(now.Unix()/60, 10))
	writeViewTagged(w, r, etag, func() any {
		p := agentdesk.BuildPipeline(v.Snap.Issues, now, agentdesk.DefaultStaleAfter)
		if p.Rows == nil {
			p.Rows = []agentdesk.PipelineRow{}
		}
		return p
	})
}

// handleSliceMetrics serves GET /api/v1/slice-metrics: the per-epic summary of
// beads that carry slice_outcome metadata (agentdesk.BuildSliceReport).
func (s *server) handleSliceMetrics(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	writeView(w, r, v, func() any {
		report := agentdesk.BuildSliceReport(v.Snap.Issues)
		if report.Epics == nil {
			report.Epics = []agentdesk.SliceMetrics{}
		}
		return report
	})
}

// now is the clock of the time-dependent endpoints; tests replace it.
func (s *server) now() time.Time {
	if s.cfg.now != nil {
		return s.cfg.now().UTC()
	}
	return time.Now().UTC()
}
