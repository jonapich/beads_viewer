package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// bdExportFresh reports whether bd's own auto-export rewrote origin's JSONL at
// or after since: <TrackerDirectory>/export-state.json parses and its timestamp
// (bd writes it after a successful export) is not before since, and
// <TrackerDirectory>/issues.jsonl (the file RefreshBDExport publishes, and the
// one bd writes because bv runs it with BEADS_DIR set) exists with a
// modification time not before since. Any error is false, so a custom
// export.path, a missing state file or a bd that skipped the export all fall
// back to an explicit export.
func bdExportFresh(origin model.IssueOrigin, since time.Time) bool {
	if origin.TrackerDirectory == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(origin.TrackerDirectory, "export-state.json"))
	if err != nil {
		return false
	}
	var state struct {
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Timestamp.IsZero() || state.Timestamp.Before(since) {
		return false
	}
	info, err := os.Stat(filepath.Join(origin.TrackerDirectory, "issues.jsonl"))
	return err == nil && !info.ModTime().Before(since)
}
