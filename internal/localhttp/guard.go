// Package localhttp holds the request policy shared by bv's local HTTP
// servers (the static-site preview and the `bv serve` GUI API).
//
// Binding a listener to loopback is not enough on its own: a hostile web page
// can point a DNS name it controls at 127.0.0.1 (DNS rebinding) and have the
// victim's browser talk to the server with a foreign Host header. The guard
// here rejects those requests, and rejects cross-origin browser requests
// whose Origin is not the server's own.
package localhttp

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Options tunes Guard. The zero value is the strictest useful policy for a
// read-only server: local Host/Origin only, any method allowed through.
type Options struct {
	// ReadOnly rejects every method other than GET and HEAD with 405 after the
	// Host/Origin check has passed.
	ReadOnly bool

	// RequireOriginOnMutating rejects POST, PUT, PATCH and DELETE requests that
	// carry no Origin header (they still need a matching one when present, as
	// for every other method). Browsers always send Origin on those methods.
	RequireOriginOnMutating bool

	// EmbedderPolicy adds Cross-Origin-Embedder-Policy: require-corp.
	EmbedderPolicy bool

	// ExtraOrigins are additional exact origins (scheme://host:port) accepted
	// as the request Origin and as the request Host, on top of the listener's
	// own loopback origin. Used for the Vite dev server in `bv serve --serve-dev`.
	ExtraOrigins []string

	// ForbiddenMessage is the 403 body text. Defaults to a generic message.
	ForbiddenMessage string

	// Deny writes the 403 response for a request the guard rejects (the
	// no-store header is already set). Nil means http.Error(w, message, status),
	// which is what the static-site preview uses.
	Deny func(w http.ResponseWriter, r *http.Request, status int, message string)
}

// Guard wraps next so that only requests addressed to the listener's local
// origin (port) reach it. Apply it outside any mux so every route shares the
// policy. It also sets the nosniff and same-origin COOP/CORP response headers
// on every response, denied ones included.
func Guard(port int, opts Options, next http.Handler) http.Handler {
	extras := parseExtraOrigins(opts.ExtraOrigins)
	message := opts.ForbiddenMessage
	if message == "" {
		message = "requests must use the local server origin"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if opts.EmbedderPolicy {
			h.Set("Cross-Origin-Embedder-Policy", "require-corp")
		}
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("X-Content-Type-Options", "nosniff")

		origins := r.Header.Values("Origin")
		allowed := hostAllowed(r.Host, port, extras)
		if allowed && len(origins) != 0 {
			allowed = len(origins) == 1 && originAllowed(origins[0], r.Host, port, extras)
		}
		if allowed && len(origins) == 0 && opts.RequireOriginOnMutating && isMutating(r.Method) {
			allowed = false
		}
		if !allowed {
			h.Set("Cache-Control", "no-store")
			if opts.Deny != nil {
				opts.Deny(w, r, http.StatusForbidden, message)
			} else {
				http.Error(w, message, http.StatusForbidden)
			}
			return
		}
		if opts.ReadOnly && r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// extraOrigin is a pre-parsed Options.ExtraOrigins entry.
type extraOrigin struct {
	scheme    string
	port      int
	authority string
}

func parseExtraOrigins(raw []string) []extraOrigin {
	var out []extraOrigin
	for _, s := range raw {
		u, err := url.Parse(s)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			continue
		}
		authority, ok := Authority(u.Host, port)
		if !ok {
			continue
		}
		out = append(out, extraOrigin{scheme: u.Scheme, port: port, authority: authority})
	}
	return out
}

// hostAllowed reports whether a request Host header addresses this server: the
// listener's own loopback authority, or one of the extra origins' authorities.
func hostAllowed(host string, port int, extras []extraOrigin) bool {
	if _, ok := Authority(host, port); ok {
		return true
	}
	for _, e := range extras {
		if a, ok := Authority(host, e.port); ok && a == e.authority {
			return true
		}
	}
	return false
}

// originAllowed reports whether a single Origin header value is acceptable. It
// must be a bare scheme://host[:port] and either the server's own origin (the
// same authority as the Host header) or one of the extra origins.
func originAllowed(raw, host string, port int, extras []extraOrigin) bool {
	origin, err := url.Parse(raw)
	if err != nil {
		return false
	}
	knownScheme := origin.Scheme == "http" || extraSchemeMatch(origin.Scheme, extras)
	if !knownScheme || origin.User != nil || origin.Opaque != "" || origin.Path != "" ||
		origin.RawQuery != "" || origin.ForceQuery || strings.Contains(raw, "#") {
		return false
	}
	if hostAuthority, ok := Authority(host, port); ok {
		if originAuthority, local := Authority(origin.Host, port); local && originAuthority == hostAuthority && origin.Scheme == "http" {
			return true
		}
	}
	for _, e := range extras {
		if origin.Scheme != e.scheme {
			continue
		}
		if a, ok := Authority(origin.Host, e.port); ok && a == e.authority {
			return true
		}
	}
	return false
}

func extraSchemeMatch(scheme string, extras []extraOrigin) bool {
	for _, e := range extras {
		if e.scheme == scheme {
			return true
		}
	}
	return false
}

// Authority normalises a Host header or Origin host against the expected port.
// It returns the canonical "host:port" and true only for a loopback host
// ("localhost" or a loopback IP) whose explicit or default (80) port equals
// port. Everything else, including a missing port that does not default to
// port, userinfo, signed or non-numeric ports, is rejected.
func Authority(authority string, port int) (string, bool) {
	host, rawPort, err := net.SplitHostPort(authority)
	if err != nil {
		// An omitted port means HTTP's default, not the server's chosen port.
		if strings.ContainsAny(authority, ":[]") {
			return "", false
		}
		host, rawPort = authority, "80"
	}
	for _, digit := range rawPort {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	parsedPort, err := strconv.Atoi(rawPort)
	if err != nil || parsedPort != port || port <= 0 || port > 65535 {
		return "", false
	}
	host = strings.ToLower(host)
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", false
		}
		host = ip.String()
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), true
}
