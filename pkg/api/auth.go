package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
)

// cookieNamePrefix is followed by the listening port. Cookies are scoped to a
// host, not a host:port, so two `bv serve` instances on 127.0.0.1 would
// otherwise overwrite each other's session cookie.
const cookieNamePrefix = "bv_session_"

// tokenBytes is the entropy of the per-run bootstrap token.
const tokenBytes = 32

// NewToken returns a fresh per-run bootstrap token: 32 random bytes, hex
// encoded.
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate serve token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// tokenEqual compares two tokens in constant time.
func tokenEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// handleAuth trades the bootstrap token for a session cookie and redirects to
// the app root, so the token leaves the address bar and browser history.
func (s *server) handleAuth(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return
	}
	if !tokenEqual(r.URL.Query().Get("token"), s.cfg.Token) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    s.cfg.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// requireSession rejects requests that lack the session cookie set by /auth.
// SSE clients cannot set headers, which is why this is a cookie and not a
// bearer token.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cookieName)
		if err != nil || !tokenEqual(c.Value, s.cfg.Token) {
			w.Header().Set("Cache-Control", "no-store")
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid session; open the URL printed by bv serve")
			return
		}
		next.ServeHTTP(w, r)
	})
}
