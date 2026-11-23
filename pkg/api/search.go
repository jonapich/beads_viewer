package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/Dicklesworthstone/beads_viewer/pkg/search"
)

// maxSearchQuery bounds q, in bytes.
const maxSearchQuery = 256

// handleSearch serves GET /api/v1/search?q=. It is the TUI's default list
// filter exactly: sahilm/fuzzy over search.FilterText(issue, repo key), stable
// sorted by score. Semantic and hybrid search are not part of it. Every hit
// comes back in rank order; the client intersects them with its own facets.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "q is required")
		return
	}
	if len(q) > maxSearchQuery {
		writeError(w, http.StatusBadRequest, "bad_request", "q is too long (max 256 bytes)")
		return
	}
	sum := sha256.Sum256([]byte(q))
	writeViewTagged(w, r, variantETag(v.ETag, "q"+hex.EncodeToString(sum[:6])), func() any {
		idx := v.list()
		ranks := search.FilterRanks(q, idx.targets)
		resp := SearchResponse{Query: q, Total: len(ranks), Hits: make([]SearchHit, len(ranks))}
		for i, m := range ranks {
			resp.Hits[i] = SearchHit{ID: idx.items[m.Index].ID, Score: m.Score}
		}
		return resp
	})
}
