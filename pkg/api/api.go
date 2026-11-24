// Package api is the HTTP side of `bv serve`: a localhost-only server for the
// web GUI. It serves the embedded frontend and a token-protected JSON API
// under /api/v1.
//
// Design (docs/adr/0001-gui-architecture.md):
//   - The handler is built by NewHandler from an explicit Config; there are no
//     package globals, so tests drive it with httptest.
//   - Every request passes the shared Host/Origin guard (internal/localhttp).
//   - /auth?token=... trades the per-run bootstrap token for an HttpOnly cookie
//     and redirects to /. Every /api/* request needs that cookie.
//   - pkg/api never imports pkg/ui or bubbletea (enforced by a test).
package api

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/internal/localhttp"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// Vite's dev server origins, allowed alongside the server's own origin when
// Config.Dev is set (`bv serve --serve-dev`).
var devOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}

// Config is everything NewHandler needs. Port and Token are required.
type Config struct {
	// Port is the TCP port the server listens on. It anchors the Host/Origin
	// guard, so it must be the port actually bound (not 0).
	Port int

	// Token is the per-run bootstrap secret accepted by /auth. See NewToken.
	Token string

	// Version is reported by /api/v1/health.
	Version string

	// Assets is the frontend tree (index.html at its root). Nil serves no
	// frontend. Ignored when Dev is set.
	Assets fs.FS

	// Dev serves the API only, with no embedded frontend, and also accepts the
	// Vite dev server origins. The frontend then runs from `npm run dev`,
	// proxying /api and /auth here.
	Dev bool

	// Store backs the data endpoints. Nil answers them with 503 not_ready.
	// The caller owns the store and closes it after the server stops.
	Store *Store

	// heartbeat is the events stream's keep-alive interval; 0 means 25 s. Tests
	// shorten it.
	heartbeat time.Duration

	// newWriter builds the tracker client one write goes through; nil means
	// tracker.New. Tests swap in a stub. An error means the issue cannot be
	// written to (403 read_only).
	newWriter func(issue model.Issue, sourcePath string) (issueWriter, error)

	// writeTimeout bounds one tracker write; 0 means defaultWriteTimeout.
	writeTimeout time.Duration
}

// NewHandler returns the complete `bv serve` handler: guard, headers, /auth,
// /api/* and the static frontend.
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, errors.New("api: Config.Port must be the bound listener port")
	}
	if cfg.Token == "" {
		return nil, errors.New("api: Config.Token is required")
	}
	s := &server{
		cfg:        cfg,
		cookieName: cookieNamePrefix + strconv.Itoa(cfg.Port),
	}
	if !cfg.Dev {
		s.static = newStaticHandler(cfg.Assets)
	}

	opts := localhttp.Options{
		RequireOriginOnMutating: true,
		ForbiddenMessage:        "requests must use the local bv serve origin",
		// API errors are always JSON, guard denials included.
		Deny: func(w http.ResponseWriter, _ *http.Request, status int, message string) {
			writeError(w, status, "forbidden", message)
		},
	}
	if cfg.Dev {
		opts.ExtraOrigins = devOrigins
	}
	return localhttp.Guard(cfg.Port, opts, http.HandlerFunc(s.route)), nil
}

type server struct {
	cfg        Config
	cookieName string
	static     http.Handler // nil in dev mode
}

// route is the top-level router. It is hand-rolled instead of an http.ServeMux
// so that URL paths are never rewritten or redirected before the checks below
// run, and so /api/* never falls through to the SPA handler.
func (s *server) route(w http.ResponseWriter, r *http.Request) {
	setCommonHeaders(w.Header())
	path := r.URL.Path
	switch {
	case path == "/auth":
		s.handleAuth(w, r)
	case path == "/api" || strings.HasPrefix(path, "/api/"):
		s.requireSession(http.HandlerFunc(s.serveAPI)).ServeHTTP(w, r)
	case s.static != nil:
		s.static.ServeHTTP(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found",
			"this server runs with --serve-dev and serves no frontend; use the Vite dev server")
	}
}

// serveAPI dispatches an authenticated /api/* request. Later slices add their
// endpoints here; anything unknown is a JSON 404, never the SPA fallback.
// Snapshot-backed GETs answer If-None-Match with 304 (see conditional.go).
func (s *server) serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/api/v1/health":
		s.handleHealth(w, r)
	case "/api/v1/meta":
		s.handleMeta(w, r)
	case "/api/v1/events":
		s.handleEvents(w, r)
	case "/api/v1/issues":
		s.handleIssues(w, r)
	case "/api/v1/search":
		s.handleSearch(w, r)
	case "/api/v1/signoff":
		s.handleSignoff(w, r)
	case "/api/v1/triage":
		s.handleTriage(w, r)
	case "/api/v1/insights":
		s.handleInsights(w, r)
	case "/api/v1/plan":
		s.handlePlan(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, issuesPathPrefix) {
			// /api/v1/issues/{id}/{action} is a write; everything else under
			// issues/ (the detail, and unknown shapes, which 404 there) is the detail.
			if id, sub, ok := splitIssuePath(r); ok && isWriteSub(sub) {
				s.handleIssueWrite(w, r, id, sub)
				return
			}
			// PATCH /api/v1/issues/{id} is the field editor (beads-c2o.14).
			if r.Method == http.MethodPatch {
				if id, ok := issueIDFromPath(r); ok {
					s.handleIssuePatch(w, r, id)
					return
				}
			}
			s.handleIssueDetail(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
	}
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.cfg.Version,
	})
}

// contentSecurityPolicy is default-src 'self' plus the directives that do not
// fall back to it: no framing, no <base> rewriting, same-origin form posts.
const contentSecurityPolicy = "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// setCommonHeaders sets the response headers every route shares. The guard has
// already added nosniff and COOP/CORP.
func setCommonHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	// The bootstrap token travels in a URL; never leak it through Referer.
	h.Set("Referrer-Policy", "no-referrer")
}

// Server timeouts. There is deliberately no ReadTimeout or WriteTimeout: the
// SSE stream (beads-c2o.6) is a long-lived response, and a global write
// deadline would cut it off.
const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 2 * time.Second
)

// Serve serves handler on ln until ctx is cancelled, then shuts down
// gracefully (force-closing after a short grace period). A clean shutdown
// returns nil. The caller owns ln only until Serve returns; Serve closes it.
func Serve(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		// Every request context ends when ctx does, so long-lived handlers (the
		// events stream) return at once instead of holding Shutdown for the
		// grace period. Handlers that must finish work during shutdown need
		// context.WithoutCancel.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	var shutdownErr error
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Shutdown leaves connections open when its context expires. That is
		// routine, not a failure: browsers keep speculative preconnect sockets
		// that never send a request, and net/http will not close those for 5 s.
		// The user asked to stop, so Close is the bounded fallback and Serve
		// still reports a clean stop.
		if !errors.Is(err, context.DeadlineExceeded) {
			shutdownErr = err
		}
		_ = srv.Close()
	}
	serveErr := <-done
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}
