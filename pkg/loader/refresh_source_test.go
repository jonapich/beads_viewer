package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefreshBDSourceExport(t *testing.T) {
	installTrackerFixture(t)
	t.Setenv(trackerTestModeEnv, "export")
	const stale = "{\"id\":\"bd-1\",\"title\":\"Stale\",\"status\":\"open\",\"issue_type\":\"task\",\"labels\":[\"needs-decision\"]}\n"
	const fresh = "{\"id\":\"bd-1\",\"title\":\"Fresh\",\"status\":\"open\",\"issue_type\":\"task\"}\n"
	payload := filepath.Join(t.TempDir(), "payload.jsonl")
	if err := os.WriteFile(payload, []byte(fresh), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BV_LOADER_TEST_PAYLOAD", payload)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("bd workspace is re-exported", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt"), 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(beadsDir, "issues.jsonl")
		if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
			t.Fatal(err)
		}
		if !IsBDExportPath(path) {
			t.Fatal("bd compatibility export not recognised")
		}
		if err := RefreshBDSourceExport(ctx, path); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != fresh {
			t.Fatalf("export was not refreshed: %q %v", got, err)
		}
	})

	t.Run("other sources are left alone", func(t *testing.T) {
		dir := t.TempDir()
		brPath := filepath.Join(dir, "issues.jsonl") // no bd workspace markers
		bdDir := filepath.Join(t.TempDir(), ".beads")
		if err := os.MkdirAll(filepath.Join(bdDir, "embeddeddolt"), 0o755); err != nil {
			t.Fatal(err)
		}
		sidecar := filepath.Join(bdDir, "beads.jsonl") // bd workspace, but not its export
		for _, path := range []string{brPath, sidecar, "", "git:HEAD~1"} {
			if path != "" && filepath.IsAbs(path) {
				if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if IsBDExportPath(path) {
				t.Fatalf("%q treated as a bd compatibility export", path)
			}
			if err := RefreshBDSourceExport(ctx, path); err != nil {
				t.Fatalf("%q: %v", path, err)
			}
			if filepath.IsAbs(path) {
				if got, err := os.ReadFile(path); err != nil || string(got) != stale {
					t.Fatalf("%q was rewritten: %q %v", path, got, err)
				}
			}
		}
	})
}
