package api

import (
	"net/http"
	"strings"
)

// notModified implements If-None-Match for a GET or HEAD. It always sets the
// ETag header; on a match it also writes a bodyless 304 and returns true. The
// header is a comma list, "*" matches anything, and a W/ prefix is ignored
// (weak comparison, RFC 9110 section 13.1.2).
func notModified(w http.ResponseWriter, r *http.Request, etag string) bool {
	w.Header().Set("ETag", etag)
	values := r.Header.Values("If-None-Match")
	if len(values) == 0 {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, line := range values {
		for _, tag := range strings.Split(line, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "*" || strings.TrimPrefix(tag, "W/") == want {
				w.WriteHeader(http.StatusNotModified)
				return true
			}
		}
	}
	return false
}

// writeView writes build() as the JSON response for v, or a 304 when the
// client's If-None-Match already names v's ETag (build is then not called).
// Errors never carry an ETag, so they do not go through here.
func writeView(w http.ResponseWriter, r *http.Request, v *View, build func() any) {
	if notModified(w, r, v.ETag) {
		return
	}
	writeJSON(w, http.StatusOK, build())
}
