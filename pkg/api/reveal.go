package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Show in Explorer (beads-c2o.64 S4): POST /api/v1/file/reveal {path} selects one file in the
// operating system's file manager. It only reveals, never opens: the file is not run, and its
// default application is never started. The path goes through the same resolution and allowed
// roots as GET /api/v1/file, so only a file under a served directory can be named; the argument
// handed to the program is the resolved absolute path, not the text the request carried.
// Windows only (explorer.exe /select,<path>); elsewhere there is no revealer and the route
// answers 501. Like the other actions it needs a writable tracker (the same gate as the issue
// writes), and the guard already demands the app's Origin on every POST.

// revealer selects abs in the file manager. Nil: revealing is not supported here.
type revealer func(abs string) error

// maxRevealBody bounds the JSON body of a reveal request.
const maxRevealBody = 4 << 10

type revealRequest struct {
	Path string `json:"path"`
}

// revealFn is the configured revealer (tests inject one), else the platform's.
func (s *server) revealFn() revealer {
	if s.cfg.reveal != nil {
		return s.cfg.reveal
	}
	return defaultReveal
}

// canReveal is what meta capabilities.reveal reports: files are served, the platform can reveal,
// and the tracker takes writes.
func (s *server) canReveal(v *View) bool {
	return s.fileSet != nil && s.revealFn() != nil && v != nil && v.Snap != nil && writable(v.Snap)
}

// handleFileReveal serves POST /api/v1/file/reveal.
func (s *server) handleFileReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	fsys := s.files()
	if fsys == nil {
		writeError(w, http.StatusNotFound, "files_disabled", "this server serves no local files")
		return
	}
	reveal := s.revealFn()
	if reveal == nil {
		writeError(w, http.StatusNotImplemented, "reveal_unsupported", "showing a file in the file manager is only supported on Windows")
		return
	}
	var v *View
	if s.cfg.Store != nil {
		v = s.cfg.Store.Current()
	}
	if v == nil || v.Snap == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "no tracker data is loaded")
		return
	}
	if !writable(v.Snap) {
		writeError(w, http.StatusForbidden, "read_only", "this tracker is read-only, so bv takes no actions on it")
		return
	}

	var req revealRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRevealBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body: "+err.Error())
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body: unexpected data after the JSON value")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "invalid_path", "path is required")
		return
	}

	res := fsys.resolve(req.Path, false)
	// A file that exists under a root can be shown even when bv does not preview its type or it is
	// over the preview size cap; anything else is the same error GET /file gives.
	switch res.status {
	case fileOK, fileUnsupported, fileTooLarge:
	default:
		writeFileStatusError(w, res.status, fsys.maxBytes)
		return
	}
	if err := reveal(res.abs); err != nil {
		writeError(w, http.StatusInternalServerError, "reveal_failed", "could not start the file manager")
		return
	}
	writeJSON(w, http.StatusOK, RevealResponse{Path: res.abs})
}
