package api

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	indexName = "index.html"

	// Vite writes content-hashed bundles under assets/, so they can be cached
	// forever; everything else must be revalidated.
	hashedAssetPrefix = "assets/"

	cacheImmutable = "public, max-age=31536000, immutable"
	cacheNoStore   = "no-store"
	cacheRevalid   = "no-cache"
)

// newStaticHandler serves the frontend tree with SPA routing: an existing file
// is served as is, and any other path falls back to index.html so client-side
// routes survive a reload. /api/* never reaches this handler.
func newStaticHandler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
			return
		}
		if assets == nil {
			writeError(w, http.StatusNotFound, "not_found", "this bv build embeds no frontend")
			return
		}

		// path.Clean on a rooted path cannot climb above the root; fs.FS also
		// rejects any name that is not a valid, unrooted, slash path.
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = indexName
		}
		if info, err := fs.Stat(assets, name); err != nil || info.IsDir() {
			name = indexName // SPA fallback
		}

		f, err := assets.Open(name)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "frontend not available")
			return
		}
		defer f.Close()

		cache := cacheRevalid
		switch {
		case name == indexName:
			cache = cacheNoStore
		case strings.HasPrefix(name, hashedAssetPrefix):
			cache = cacheImmutable
		}
		w.Header().Set("Cache-Control", cache)

		rs, ok := f.(io.ReadSeeker)
		if !ok {
			data, err := io.ReadAll(f)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "read frontend asset")
				return
			}
			rs = bytes.NewReader(data)
		}
		http.ServeContent(w, r, name, time.Time{}, rs)
	})
}
