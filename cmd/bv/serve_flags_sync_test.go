package main

import (
	"slices"
	"testing"
)

// serveCompatibleModes are the non-TUI modes that serveExclusiveFlags
// deliberately leaves out: --serve itself, and the flags that print or update
// and exit before any mode runs.
var serveCompatibleModes = []string{
	"serve",
	"version", "help", "check-update", "update", "update-dry-run", "rollback",
}

// TestServeExclusiveFlagsCoverNonTUIModes keeps serveExclusiveFlags (serve.go)
// in step with the non-TUI mode lists in main.go: a headless mode added there
// must either be refused alongside --serve or be named here as compatible.
func TestServeExclusiveFlagsCoverNonTUIModes(t *testing.T) {
	t.Parallel()
	lists := map[string][]string{
		"nonTUIModeFlags":      nonTUIModeFlags,
		"nonRobotPrimaryFlags": nonRobotPrimaryFlags,
	}
	for listName, list := range lists {
		for _, name := range list {
			if slices.Contains(serveExclusiveFlags, name) || slices.Contains(serveCompatibleModes, name) {
				continue
			}
			t.Errorf("--%s is a non-TUI mode in %s but is missing from serveExclusiveFlags: "+
				"add it there, or to serveCompatibleModes in this test if --serve may be combined with it", name, listName)
		}
	}
	// An exemption for a mode that no longer exists would hide a future clash.
	for _, name := range serveCompatibleModes {
		if !slices.Contains(nonTUIModeFlags, name) {
			t.Errorf("serveCompatibleModes lists --%s, which is not in nonTUIModeFlags", name)
		}
		if slices.Contains(serveExclusiveFlags, name) {
			t.Errorf("--%s is both exclusive and compatible with --serve", name)
		}
	}
}
