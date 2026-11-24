package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
)

// bd keeps .beads/issues.jsonl current only when bv exports it (bd's own
// auto-export is off by default and throttled). A reload that just re-reads the
// JSONL therefore misses changes made through bd, such as a cleared
// needs-decision label, until the next relaunch runs `bd export`.

const (
	exportInboxTwoLabelled = "{\"id\":\"t-1\",\"title\":\"One\",\"status\":\"open\",\"priority\":2,\"issue_type\":\"task\",\"labels\":[\"needs-decision\"]}\n" +
		"{\"id\":\"t-2\",\"title\":\"Two\",\"status\":\"open\",\"priority\":2,\"issue_type\":\"task\",\"labels\":[\"needs-decision\"]}\n"
	exportInboxOneLabelled = "{\"id\":\"t-1\",\"title\":\"One\",\"status\":\"open\",\"priority\":2,\"issue_type\":\"task\"}\n" +
		"{\"id\":\"t-2\",\"title\":\"Two\",\"status\":\"open\",\"priority\":2,\"issue_type\":\"task\",\"labels\":[\"needs-decision\"]}\n"
)

// bdWorkspaceFixture writes a bd workspace whose issues.jsonl is the stale
// export and returns the path of that file.
func bdWorkspaceFixture(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(path, []byte(exportInboxTwoLabelled), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// stubSourceExport replaces the bd export with one that records its calls. On
// success it publishes the label-cleared JSONL, as `bd export` would after the
// label was removed in Dolt; with failure set it fails and leaves the file.
func stubSourceExport(t *testing.T, failure error) *[]string {
	t.Helper()
	var calls []string
	previous := refreshSourceExport
	refreshSourceExport = func(_ context.Context, sourcePath string) error {
		calls = append(calls, sourcePath)
		if failure != nil {
			return failure
		}
		return os.WriteFile(sourcePath, []byte(exportInboxOneLabelled), 0o600)
	}
	t.Cleanup(func() { refreshSourceExport = previous })
	return &calls
}

func newInboxModel(t *testing.T, path string) *Model {
	t.Helper()
	issues := []model.Issue{
		{ID: "t-1", Title: "One", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, Labels: []string{"needs-decision"}},
		{ID: "t-2", Title: "Two", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, Labels: []string{"needs-decision"}},
	}
	m := NewModel(issues, nil, path)
	t.Cleanup(m.Stop)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.openAgentDesk(deskInbox)
	if m.agentDesk.inbox.Total != 2 {
		t.Fatalf("setup: inbox total=%d, want 2", m.agentDesk.inbox.Total)
	}
	return m
}

// pressRefresh presses Ctrl+R and plays the resulting commands the way the
// Bubble Tea runtime would: the command runs off the update goroutine and its
// message is fed back to Update; in background mode the reload result arrives
// through the worker.
func pressRefresh(t *testing.T, m *Model, background bool) {
	t.Helper()
	var received chan tea.Msg
	if background {
		worker, err := NewBackgroundWorker(WorkerConfig{BeadsPath: m.beadsPath})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(worker.Stop)
		m.backgroundWorker = worker
		received = make(chan tea.Msg, 1)
		go func() { received <- WaitForBackgroundWorkerMsgCmd(worker)() }()
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("Ctrl+R returned no reload command")
	}
	_, cmd = m.Update(cmd())
	if background {
		if cmd != nil {
			t.Fatal("export completion started an extra command in background mode")
		}
		select {
		case msg := <-received:
			m.Update(msg)
		case <-time.After(5 * time.Second):
			t.Fatal("Ctrl+R did not deliver a snapshot")
		}
	}
}

func TestForceRefreshExportsBDBeforeReload(t *testing.T) {
	for _, background := range []bool{true, false} {
		name := "file mode"
		if background {
			name = "background mode"
		}
		t.Run(name, func(t *testing.T) {
			path := bdWorkspaceFixture(t)
			calls := stubSourceExport(t, nil)
			m := newInboxModel(t, path)

			pressRefresh(t, m, background)

			if len(*calls) != 1 || (*calls)[0] != path {
				t.Fatalf("bd export calls = %v, want one for %s", *calls, path)
			}
			if got := m.agentDesk.inbox.Total; got != 1 {
				t.Fatalf("inbox total after the label was cleared = %d, want 1", got)
			}
			if m.statusIsError || strings.Contains(m.statusMsg, "export failed") {
				t.Fatalf("successful refresh status = %q (error=%v)", m.statusMsg, m.statusIsError)
			}
		})
	}
}

func TestForceRefreshReportsFailedBDExport(t *testing.T) {
	for _, background := range []bool{true, false} {
		name := "file mode"
		if background {
			name = "background mode"
		}
		t.Run(name, func(t *testing.T) {
			path := bdWorkspaceFixture(t)
			stubSourceExport(t, errors.New("bd export: exit status 1\nsecond line of noise"))
			m := newInboxModel(t, path)

			pressRefresh(t, m, background)

			// The reload goes ahead with the previous export...
			if got := m.agentDesk.inbox.Total; got != 2 {
				t.Fatalf("inbox total = %d, want 2 from the last export", got)
			}
			// ...and the status bar says why the data may be stale.
			want := "Refreshed from the last export; bd export failed: bd export: exit status 1"
			if m.statusMsg != want || !m.statusIsError {
				t.Fatalf("status = %q (error=%v), want %q as an error", m.statusMsg, m.statusIsError, want)
			}
			if m.exportWarning != "" {
				t.Fatalf("export warning was not consumed: %q", m.exportWarning)
			}
		})
	}
}

func TestForceRefreshSkipsExportOutsideBDWorkspace(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	if err := os.WriteFile(path, []byte(exportInboxTwoLabelled), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := stubSourceExport(t, nil)
	m := newInboxModel(t, path)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("Ctrl+R returned no reload command")
	}
	if _, ok := cmd().(FileChangedMsg); !ok {
		t.Fatal("non-bd source did not reload directly")
	}
	if len(*calls) != 0 {
		t.Fatalf("export ran for a source that is not a bd compatibility export: %v", *calls)
	}
}
