package tracker

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// RefreshExport brings the JSONL export of origin's tracker up to date after a
// write: bd re-exports through the loader, br runs a guarded flush.
func RefreshExport(ctx context.Context, origin model.IssueOrigin, sourcePath string) error {
	return refreshExport(ctx, defaultBackend, origin, sourcePath)
}

// RefreshExport is the package function for this client's origin and source.
func (c *Client) RefreshExport(ctx context.Context) error {
	return refreshExport(ctx, c.b, c.origin, c.sourcePath)
}

func refreshExport(ctx context.Context, b backend, origin model.IssueOrigin, sourcePath string) error {
	if err := b.revalidate(originSource(origin, sourcePath), origin); err != nil {
		return err
	}
	if origin.Tracker == "bd" {
		return b.exportBD(ctx, origin)
	}
	// An installed br may have sync.auto_flush disabled. An explicit guarded
	// flush keeps JSONL viewers current without enabling import or bypassing
	// native stale/empty database protections.
	_, diagnostic, err := b.run(ctx, origin, true, []string{"sync", "--flush-only", "--json"})
	if err != nil && diagnostic != "" {
		return fmt.Errorf("%w: %s", err, diagnostic)
	}
	return err
}

// originSource is the file the view reads: the view's own source when known,
// else the tracker's export (bd) or database (br).
func originSource(origin model.IssueOrigin, sourcePath string) string {
	if sourcePath != "" {
		return sourcePath
	}
	if origin.Tracker == "bd" {
		return filepath.Join(origin.TrackerDirectory, "issues.jsonl")
	}
	return origin.Database
}

// needsRefresh reports whether a write must be followed by an export refresh:
// the view reads a JSONL export (or an unknown source), or the tracker is bd.
func needsRefresh(origin model.IssueOrigin, sourcePath string) bool {
	return origin.Tracker == "bd" || sourcePath == "" || strings.EqualFold(filepath.Ext(sourcePath), ".jsonl")
}
