package api

import (
	"net/http"
	"strconv"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/Dicklesworthstone/beads_viewer/pkg/tracker"
)

// snapshotView is the common start of every snapshot-backed GET: it enforces
// GET/HEAD and returns the current View, or writes the error and returns nil.
// The caller must use the returned View for both the ETag and the body.
func (s *server) snapshotView(w http.ResponseWriter, r *http.Request) *View {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return nil
	}
	if s.cfg.Store != nil {
		if v := s.cfg.Store.Current(); v != nil && v.Snap != nil {
			return v
		}
	}
	writeError(w, http.StatusServiceUnavailable, "not_ready", "no tracker data is loaded")
	return nil
}

func (s *server) handleMeta(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	// The reload error is read once, so the ETag and the body agree. It is not
	// part of the View, so it gets its own ETag variant (Seq changes per failure).
	rs := s.cfg.Store.lastReloadError()
	etag := v.ETag
	if rs != nil {
		etag = variantETag(etag, "e"+strconv.FormatUint(rs.Seq, 10))
	}
	writeViewTagged(w, r, etag, func() any { return s.buildMeta(v, rs) })
}

func (s *server) buildMeta(v *View, rs *reloadStatus) MetaResponse {
	snap := v.Snap
	awaiting, signoffs := agentdesk.BadgeCounts(snap.Issues)
	m := MetaResponse{
		Version:             s.cfg.Version,
		DataHash:            snap.DataHash,
		Phase2Ready:         v.Phase2Ready,
		Phase2Skipped:       snap.Tier == snapshot.TierHuge,
		Phase2Error:         v.Phase2Err,
		Tier:                snap.Tier.String(),
		IssueCount:          len(snap.Issues),
		ViewCount:           len(snap.ViewIssues),
		SourceIssueCount:    snap.SourceIssueCountHint,
		LoadedOpenOnly:      snap.LoadedOpenOnly,
		TruncatedCount:      snap.TruncatedCount,
		LargeDatasetWarning: snap.LargeDatasetWarning,
		LoadWarningCount:    snap.LoadWarningCount,
		AuthorityComplete:   snap.AuthorityComplete,
		ReferenceTime:       snap.ReferenceTime,
		Source: MetaSource{
			Path:       snap.BeadsPath,
			Kind:       sourceKind(snap.BeadsPath),
			ProjectDir: snap.ProjectDir,
		},
		Recipe: snap.RecipeName,
		Counts: MetaCounts{
			Open:    snap.CountOpen,
			Ready:   snap.CountReady,
			Blocked: snap.CountBlocked,
			Closed:  snap.CountClosed,

			Awaiting:       awaiting,
			SignoffPending: signoffs,
		},
		Capabilities: MetaCaps{Writes: writable(snap), Events: s.cfg.Store.Watching(), Files: s.fileSet != nil, Reveal: s.canReveal(v)},
		Generation:   v.Gen,
		LoadedAt:     snap.CreatedAt,
	}
	if rs != nil {
		at := rs.At
		m.ReloadError = rs.Msg
		m.ReloadErrorAt = &at
	}
	if len(snap.Issues) > 0 {
		m.Source.Tracker = tracker.CapabilitiesOf(snap.Issues[0].Origin).Tracker
	}
	return m
}

// writable reports whether the served tracker accepts writes: the first issue's
// origin carries a complete tracker route. bv serve rejects --workspace and
// --repo, so there is one tracker and the first issue stands for it. An empty
// tracker has nothing to write to.
func writable(snap *snapshot.Snapshot) bool {
	return len(snap.Issues) > 0 && tracker.CapabilitiesOf(snap.Issues[0].Origin).Writable
}

// sourceKind classifies a tracker path the way snapshot.Load does.
func sourceKind(path string) string {
	if snapshot.IsSQLitePath(path) {
		return "sqlite"
	}
	return "jsonl"
}
