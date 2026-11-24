package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/tracker"
)

// PATCH /api/v1/issues/{id} (beads-c2o.14) edits the fields the TUI's O editor
// edits: title, status, priority, assignee, type and description. It copies the
// editor's semantics: an absent field stays as it is, only fields that differ
// from the snapshot are sent (verbatim, through tracker.Update), assignee ""
// clears the assignee, and the tracker owns the status/type vocabulary (bv only
// refuses blank or multi-line values; bd's own refusal comes back as a 502
// tracker_failed). No close reason and no --force are ever sent, as in the TUI.
//
// Conflicts are checked twice, both under the write semaphore: base_updated_at
// against the snapshot (a cheap 409 before any bd call), then tracker.Update's
// own re-read of the issue (ErrConflict, also 409).

const (
	// maxDescription caps a description: it travels to bd as one argument, a
	// Windows command line holds at most 32767 UTF-16 units and Go's quoting can
	// double a value, and UTF-8 bytes are never fewer than UTF-16 units.
	maxDescription = 16000

	// maxLineField caps title, status, type and assignee: bd's own title limit,
	// and it keeps the whole command line far below the Windows limit.
	maxLineField = 500

	// maxPatchBody bounds a PATCH body: a maximal description whose every byte
	// JSON-escapes to a six-byte \u00XX still fits.
	maxPatchBody = 128 << 10
)

// patchRequest is a decoded and validated PATCH body: the values to set (already
// normalised), nil where the request leaves a field alone.
type patchRequest struct {
	title, status, assignee, issueType, description *string
	priority                                        *int
	base                                            time.Time
}

// normalizeDescription is how the O editor compares and sends a description: the
// body after the frontmatter loses leading newlines and trailing whitespace.
func normalizeDescription(s string) string {
	return strings.TrimRight(strings.TrimLeft(s, "\n"), " \t\n\r")
}

// handleIssuePatch serves PATCH /api/v1/issues/{id}; the router only sends PATCH
// here, so there is no 405. The steps run in the order of handleIssueWrite (503
// first, then the body, the write semaphore, the issue lookup, 403 read_only)
// plus the base_updated_at check, then the no-op answer, the tracker update, the
// reload and the response.
func (s *server) handleIssuePatch(w http.ResponseWriter, r *http.Request, id string) {
	store := s.cfg.Store
	if store == nil || store.Current() == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "no tracker data is loaded")
		return
	}
	req, ok := decodePatchBody(w, r)
	if !ok {
		return
	}

	v, issue, wr, unlock, ok := s.lockIssue(w, r, store, id)
	if !ok {
		return
	}
	defer unlock()

	if !req.base.Equal(issue.UpdatedAt) {
		writeError(w, http.StatusConflict, "conflict", id+" changed since you opened it; reload and try again")
		return
	}

	// Send only what differs from the snapshot, as the editor does.
	current := tracker.EditableFields(*issue)
	current["description"] = normalizeDescription(current["description"])
	var diff tracker.FieldDiff
	set := func(dst **string, key string, val *string) {
		if val != nil && *val != current[key] {
			*dst = val
		}
	}
	var prio *string
	if req.priority != nil {
		p := strconv.Itoa(*req.priority)
		prio = &p
	}
	set(&diff.Title, "title", req.title)
	set(&diff.Status, "status", req.status)
	set(&diff.Priority, "priority", prio)
	set(&diff.Assignee, "assignee", req.assignee)
	set(&diff.Type, "type", req.issueType)
	set(&diff.Description, "description", req.description)
	if diff.Len() == 0 {
		s.respondWrite(w, id, v, tracker.Result{}, nil)
		return
	}

	// As in handleIssueWrite: bd runs to completion whatever the client does.
	timeout := s.cfg.writeTimeout
	if timeout <= 0 {
		timeout = defaultWriteTimeout
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), timeout)
	defer cancel()

	res, err := wr.Update(wctx, diff)
	if err != nil {
		if errors.Is(err, tracker.ErrConflict) {
			// The tracker moved on without us: bring the export and the store up to
			// date before answering, or the client's reload reads the same stale
			// snapshot and its next attempt conflicts forever. Best effort. Running
			// the refresh unconditionally is deliberate: where tracker.Update would
			// skip it (no export to refresh) it is a harmless flush, and its
			// errors are ignored.
			_ = wr.RefreshExport(wctx)
			_, _ = store.reload()
		}
		writeTrackerError(w, err, res, timeout)
		return
	}
	s.respondWrite(w, id, nil, res, store)
}

// decodePatchBody reads and validates the JSON body. It writes the error
// response and returns false when the request is not acceptable.
func decodePatchBody(w http.ResponseWriter, r *http.Request) (patchRequest, bool) {
	var out patchRequest
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send the body as application/json")
		return out, false
	}
	bad := func(msg string) (patchRequest, bool) {
		writeError(w, http.StatusBadRequest, "bad_request", msg)
		return out, false
	}
	var req IssuePatchRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPatchBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return bad("invalid request body: " + err.Error())
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return bad("invalid request body: unexpected data after the JSON value")
	}

	if req.BaseUpdatedAt == "" {
		return bad("base_updated_at is required")
	}
	base, err := time.Parse(time.RFC3339Nano, req.BaseUpdatedAt)
	if err != nil {
		return bad("base_updated_at must be an RFC 3339 time")
	}
	out.base = base
	if req.Title == nil && req.Status == nil && req.Priority == nil && req.Assignee == nil && req.Type == nil && req.Description == nil {
		return bad("no fields to update")
	}

	// line trims a one-line field and checks it; blankOK allows "" (assignee).
	line := func(field string, in *string, blankOK bool) (*string, string) {
		if in == nil {
			return nil, ""
		}
		v := strings.TrimSpace(*in)
		switch {
		case v == "" && !blankOK:
			return nil, field + " cannot be empty"
		case strings.ContainsAny(v, "\r\n"):
			return nil, field + " must be one line"
		}
		if len(v) > maxLineField {
			return nil, fmt.Sprintf("%s is longer than %d bytes", field, maxLineField)
		}
		if msg := checkText(field, v); msg != "" {
			return nil, msg
		}
		return &v, ""
	}
	for _, f := range []struct {
		name    string
		in      *string
		dst     **string
		blankOK bool
	}{
		{"title", req.Title, &out.title, false},
		{"status", req.Status, &out.status, false},
		{"assignee", req.Assignee, &out.assignee, true},
		{"type", req.Type, &out.issueType, false},
	} {
		v, msg := line(f.name, f.in, f.blankOK)
		if msg != "" {
			return bad(msg)
		}
		*f.dst = v
	}
	if req.Priority != nil {
		if *req.Priority < 0 || *req.Priority > 4 {
			return bad("priority must be between 0 and 4")
		}
		out.priority = req.Priority
	}
	if req.Description != nil {
		d := normalizeDescription(*req.Description)
		switch {
		case len(d) > maxDescription:
			return bad(fmt.Sprintf("description is longer than %d bytes; edit it with bd", maxDescription))
		case strings.ContainsRune(d, 0):
			return bad("description must not contain NUL bytes")
		}
		out.description = &d
	}
	return out, true
}
