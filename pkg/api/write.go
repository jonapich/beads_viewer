package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/tracker"
)

// The write endpoints (beads-c2o.19), one per quick action the TUI has:
//
//	POST   /api/v1/issues/{id}/signoff                {approve, note}
//	POST   /api/v1/issues/{id}/answer                 {option, text}
//	PUT    /api/v1/issues/{id}/priority               {priority}
//	PUT    /api/v1/issues/{id}/labels/close-candidate
//	DELETE /api/v1/issues/{id}/labels/close-candidate
//
// Each runs one pkg/tracker operation, then reloads the store synchronously, so
// the response already carries the new data hash and generation. Security needs
// nothing here: the Host/Origin guard requires an exact Origin on mutating
// methods and the session cookie wraps all of /api.

const (
	// closeCandidateLabel is the triage label the close-candidate toggle sets.
	// pkg/ui has its own constant; pkg/api cannot import it.
	closeCandidateLabel = "close-candidate"

	// defaultWriteTimeout bounds one tracker write: the TUI's brUpdateTimeout.
	defaultWriteTimeout = 30 * time.Second

	// maxWriteBody caps a request body; maxWriteText a note or an answer.
	maxWriteBody = 64 << 10
	maxWriteText = 10000
)

// Sub-paths after /api/v1/issues/{id}/ that are writes.
const (
	subSignoff        = "signoff"
	subAnswer         = "answer"
	subPriority       = "priority"
	subCloseCandidate = "labels/close-candidate"
)

// issueWriter is the part of tracker.Client the write endpoints use; tests
// replace it through Config.newWriter.
type issueWriter interface {
	Signoff(ctx context.Context, approve bool, note string) (tracker.Result, error)
	AnswerQuestion(ctx context.Context, text, removeLabel string) (tracker.Result, error)
	SetPriority(ctx context.Context, p int) (tracker.Result, error)
	SetLabel(ctx context.Context, label string, on bool) (tracker.Result, error)
	// Update is the PATCH field editor (beads-c2o.14): it re-reads the issue
	// through the tracker and returns tracker.ErrConflict when it changed.
	Update(ctx context.Context, diff tracker.FieldDiff) (tracker.Result, error)
	RefreshExport(ctx context.Context) error
}

// writerFor builds the writer for issue. A failure is always the OriginError, so
// it means the issue is read-only. It returns an explicit (nil, err), never a
// typed-nil *tracker.Client inside the interface.
func (s *server) writerFor(issue model.Issue, sourcePath string) (issueWriter, error) {
	if s.cfg.newWriter != nil {
		return s.cfg.newWriter(issue, sourcePath)
	}
	c, err := tracker.New(issue, sourcePath)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func isWriteSub(sub string) bool {
	switch sub {
	case subSignoff, subAnswer, subPriority, subCloseCandidate:
		return true
	}
	return false
}

// splitIssuePath splits /api/v1/issues/{id}[/{sub}] into the issue id and the
// rest. The id is cut from the escaped path at its first "/" and unescaped once,
// as issueIDFromPath does; an empty id, or one that unescapes to contain "/", is
// not an issue. sub is "" when there is no further segment.
func splitIssuePath(r *http.Request) (id, sub string, ok bool) {
	rest, found := strings.CutPrefix(r.URL.EscapedPath(), issuesPathPrefix)
	if !found || rest == "" {
		return "", "", false
	}
	rawID, sub, _ := strings.Cut(rest, "/")
	id, err := url.PathUnescape(rawID)
	if err != nil || id == "" || strings.Contains(id, "/") {
		return "", "", false
	}
	return id, sub, true
}

// allowedMethods lists the methods a write endpoint answers.
func allowedMethods(sub string) []string {
	switch sub {
	case subPriority:
		return []string{http.MethodPut}
	case subCloseCandidate:
		return []string{http.MethodPut, http.MethodDelete}
	}
	return []string{http.MethodPost}
}

// writeRequest is a decoded and validated write body.
type writeRequest struct {
	approve  bool
	note     string
	option   string
	text     string
	priority int
	labelOn  bool // close-candidate: PUT adds, DELETE removes
}

// handleIssueWrite serves the write endpoints; see the comment at the top of
// this file. The steps run in a fixed order: validate the body, take the write
// semaphore, look the issue up (so a queued write sees the previous write's
// result), check the tracker accepts writes (403 before any 409 or no-op),
// check the precondition, run the tracker operation, reload, respond.
func (s *server) handleIssueWrite(w http.ResponseWriter, r *http.Request, id, sub string) {
	if allowed := allowedMethods(sub); !slices.Contains(allowed, r.Method) {
		list := strings.Join(allowed, ", ")
		w.Header().Set("Allow", list)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use "+list)
		return
	}
	store := s.cfg.Store
	if store == nil || store.Current() == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "no tracker data is loaded")
		return
	}

	var req writeRequest
	if sub == subCloseCandidate {
		req.labelOn = r.Method == http.MethodPut
	} else if !s.decodeWriteBody(w, r, sub, &req) {
		return
	}

	v, issue, wr, unlock, ok := s.lockIssue(w, r, store, id)
	if !ok {
		return
	}
	defer unlock()

	var removeLabel string
	switch sub {
	case subSignoff:
		if !closedLike(issue.Status) || agentdesk.SignoffState(*issue) != agentdesk.SignoffPending {
			writeError(w, http.StatusConflict, "conflict", id+" is no longer awaiting sign-off")
			return
		}
	case subAnswer:
		d := decisionOf(issue)
		if d == nil {
			writeError(w, http.StatusConflict, "conflict", id+" is not waiting for an answer")
			return
		}
		removeLabel = d.Label
		if req.option != "" {
			i := slices.IndexFunc(d.Options, func(o agentdesk.Option) bool { return o.Key == req.option })
			if i < 0 {
				writeError(w, http.StatusBadRequest, "bad_request", "option "+req.option+" is not one of the options of "+id)
				return
			}
			req.text = agentdesk.DecisionComment(&d.Options[i], req.text)
		} else {
			req.text = agentdesk.DecisionComment(nil, req.text)
		}
	case subPriority:
		if issue.Priority == req.priority {
			s.respondWrite(w, id, v, tracker.Result{}, nil)
			return
		}
	case subCloseCandidate:
		if slices.Contains(issue.Labels, closeCandidateLabel) == req.labelOn {
			s.respondWrite(w, id, v, tracker.Result{}, nil)
			return
		}
	}

	// bd runs to completion even when the client hangs up or bv is told to stop:
	// killing it halfway through the fallback dispute sequence (create, comment,
	// mark) would leave a half-written dispute. Serve's BaseContext cancels on
	// shutdown, hence WithoutCancel.
	timeout := s.cfg.writeTimeout
	if timeout <= 0 {
		timeout = defaultWriteTimeout
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), timeout)
	defer cancel()

	var res tracker.Result
	var err error
	switch sub {
	case subSignoff:
		res, err = wr.Signoff(wctx, req.approve, req.note)
	case subAnswer:
		res, err = wr.AnswerQuestion(wctx, req.text, removeLabel)
	case subPriority:
		res, err = wr.SetPriority(wctx, req.priority)
	case subCloseCandidate:
		res, err = wr.SetLabel(wctx, closeCandidateLabel, req.labelOn)
	}
	if err != nil {
		writeTrackerError(w, err, res, timeout)
		return
	}
	s.respondWrite(w, id, nil, res, store)
}

// lockIssue is the start every issue write shares: take the write semaphore,
// look the issue up (so a queued write sees the previous write's result), and
// check the tracker accepts writes (403 before any 409 or no-op). On ok the
// caller owns unlock and must call it; otherwise the error response is already
// written.
func (s *server) lockIssue(w http.ResponseWriter, r *http.Request, store *Store, id string) (v *View, issue *model.Issue, wr issueWriter, unlock func(), ok bool) {
	unlock, err := store.lockWrite(r.Context())
	if err != nil {
		// Always answer: when the client really left nobody reads this, but at
		// shutdown the request context and the store end together and the
		// connection is still open, where an empty 200 would report a write that
		// never ran as saved.
		if errors.Is(err, errStoreClosed) {
			writeError(w, http.StatusServiceUnavailable, "shutting_down", "bv serve is shutting down")
		} else {
			writeError(w, http.StatusServiceUnavailable, "cancelled", "the write was cancelled before it ran; nothing was changed")
		}
		return nil, nil, nil, nil, false
	}

	v = store.Current()
	issue = v.Snap.Issue(id)
	if issue == nil {
		unlock()
		writeError(w, http.StatusNotFound, "not_found", "no such issue")
		return nil, nil, nil, nil, false
	}

	// Writability first: a read-only tracker answers 403 whatever the request
	// would otherwise do (a stale 409, or a no-op 200).
	wr, err = s.writerFor(*issue, store.path)
	if err != nil {
		unlock()
		writeError(w, http.StatusForbidden, "read_only", err.Error())
		return nil, nil, nil, nil, false
	}
	return v, issue, wr, unlock, true
}

// respondWrite sends the 200 WriteResponse. cur is the view to report as is (a
// no-op, nothing to reload); with cur nil it reloads store first and reports the
// view that results.
func (s *server) respondWrite(w http.ResponseWriter, id string, cur *View, res tracker.Result, store *Store) {
	var reloadErr error
	if cur == nil {
		cur, reloadErr = store.reload()
	}
	out := WriteResponse{ID: id, Generation: cur.Gen, DataHash: cur.Snap.DataHash, CreatedID: res.CreatedID}
	switch {
	case res.RefreshErr != nil:
		out.Warning = fmt.Sprintf("Saved, but refreshing the export failed: %v. The list may lag until the next change.", res.RefreshErr)
	case reloadErr != nil:
		out.Warning = fmt.Sprintf("Saved, but reloading the tracker failed: %v. The list may lag until the next change.", reloadErr)
	case store != nil && store.ctx.Err() != nil:
		// The write landed while the store was closing: the reload published nothing.
		out.Warning = "Saved, but bv serve is shutting down; the list will not update."
	}
	writeJSON(w, http.StatusOK, out)
}

// writeTrackerError maps a failed tracker operation to an error response. A
// follow-up bead that was already filed is named in the message, since the
// failure came after it.
func writeTrackerError(w http.ResponseWriter, err error, res tracker.Result, timeout time.Duration) {
	status, code, msg := http.StatusInternalServerError, "write_failed", err.Error()
	var cmdErr *tracker.CommandError
	switch {
	case errors.Is(err, tracker.ErrConflict):
		status, code, msg = http.StatusConflict, "conflict", "the issue changed in the tracker; refresh and try again"
	case errors.Is(err, context.DeadlineExceeded):
		status, code, msg = http.StatusGatewayTimeout, "tracker_timeout", fmt.Sprintf("the tracker did not finish within %s", timeout)
	case errors.As(err, &cmdErr):
		status, code = http.StatusBadGateway, "tracker_failed"
		if cmdErr.Stderr != "" {
			msg = cmdErr.Stderr
		}
	}
	if res.CreatedID != "" {
		msg += " (follow-up " + res.CreatedID + " was created)"
	}
	writeError(w, status, code, msg)
}

// decodeWriteBody reads and validates the JSON body of sub into req. It writes
// the error response and returns false when the request is not acceptable.
func (s *server) decodeWriteBody(w http.ResponseWriter, r *http.Request, sub string, req *writeRequest) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send the body as application/json")
		return false
	}
	bad := func(msg string) bool {
		writeError(w, http.StatusBadRequest, "bad_request", msg)
		return false
	}
	var dst any
	var signoff SignoffRequest
	var answer AnswerRequest
	var prio PriorityRequest
	switch sub {
	case subSignoff:
		dst = &signoff
	case subAnswer:
		dst = &answer
	default:
		dst = &prio
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWriteBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return bad("invalid request body: " + err.Error())
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return bad("invalid request body: unexpected data after the JSON value")
	}

	switch sub {
	case subSignoff:
		if signoff.Approve == nil {
			return bad("approve is required")
		}
		if msg := checkText("note", signoff.Note); msg != "" {
			return bad(msg)
		}
		req.approve = *signoff.Approve
		req.note = strings.TrimSpace(signoff.Note)
		if req.approve && req.note != "" {
			return bad("an approval takes no note")
		}
		if !req.approve && req.note == "" {
			return bad("a dispute needs a note")
		}
	case subAnswer:
		if msg := checkText("text", answer.Text); msg != "" {
			return bad(msg)
		}
		if msg := checkText("option", answer.Option); msg != "" {
			return bad(msg)
		}
		req.option = strings.TrimSpace(answer.Option)
		req.text = strings.TrimSpace(answer.Text)
		if req.option == "" && req.text == "" {
			return bad("an answer needs an option or text")
		}
	default:
		if prio.Priority == nil {
			return bad("priority is required")
		}
		if *prio.Priority < 0 || *prio.Priority > 4 {
			return bad("priority must be between 0 and 4")
		}
		req.priority = *prio.Priority
	}
	return true
}

// checkText rejects a value that is too long or that bd cannot receive as an
// argument (NUL ends a C string). It returns the problem, or "".
func checkText(field, s string) string {
	switch {
	case len(s) > maxWriteText:
		return fmt.Sprintf("%s is longer than %d bytes", field, maxWriteText)
	case strings.ContainsRune(s, 0):
		return field + " must not contain NUL bytes"
	}
	return ""
}
