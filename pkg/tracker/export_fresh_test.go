package tracker

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func TestBDExportFresh(t *testing.T) {
	since := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	setup := func(t *testing.T, state string, stateOK bool, jsonlMod time.Time) model.IssueOrigin {
		t.Helper()
		dir := t.TempDir()
		if stateOK {
			if err := os.WriteFile(filepath.Join(dir, "export-state.json"), []byte(state), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if !jsonlMod.IsZero() {
			p := filepath.Join(dir, "issues.jsonl")
			if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(p, jsonlMod, jsonlMod); err != nil {
				t.Fatal(err)
			}
		}
		return model.IssueOrigin{TrackerDirectory: dir}
	}
	newer := since.Add(time.Second)
	older := since.Add(-time.Second)
	stateAt := func(ts time.Time) string {
		return `{"v":1,"last_dolt_commit":"x","timestamp":"` + ts.Format(time.RFC3339Nano) + `","issues":3}`
	}

	for _, tc := range []struct {
		name    string
		state   string
		stateOK bool
		jsonl   time.Time
		want    bool
	}{
		{"both fresh", stateAt(newer), true, newer, true},
		{"exactly at the start", stateAt(since), true, since, true},
		{"state file missing", "", false, newer, false},
		{"state file malformed", "{not json", true, newer, false},
		{"state without a timestamp", `{"v":1}`, true, newer, false},
		{"state older than the op", stateAt(older), true, newer, false},
		{"jsonl missing", stateAt(newer), true, time.Time{}, false},
		{"jsonl older than the op", stateAt(newer), true, older, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bdExportFresh(setup(t, tc.state, tc.stateOK, tc.jsonl), since); got != tc.want {
				t.Fatalf("bdExportFresh = %v, want %v", got, tc.want)
			}
		})
	}
	if bdExportFresh(model.IssueOrigin{}, since) {
		t.Fatal("an origin with no tracker directory is never fresh")
	}
}

// freshClient is a client whose exportFresh answers fresh and records the instant it was asked about.
func freshClient(t *testing.T, tracker string, fb *fakeBackend, fresh bool, since *[]time.Time) *Client {
	t.Helper()
	c := newTestClient(t, tracker, "", fb)
	c.b.exportFresh = func(_ model.IssueOrigin, s time.Time) bool {
		*since = append(*since, s)
		return fresh
	}
	return c
}

func TestWriteSkipsTheExportBDAlreadyDid(t *testing.T) {
	var since []time.Time
	fb := &fakeBackend{}
	res, err := freshClient(t, "bd", fb, true, &since).SetPriority(context.Background(), 1)
	if err != nil || res.RefreshErr != nil {
		t.Fatalf("result: %+v, %v", res, err)
	}
	if want := []string{"run"}; !slices.Equal(fb.kinds(), want) {
		t.Fatalf("calls %q, want %q: a fresh export must not be redone", fb.kinds(), want)
	}
	if len(since) != 1 {
		t.Fatalf("exportFresh asked %d times", len(since))
	}
}

func TestWriteExportsWhenBDDidNot(t *testing.T) {
	var since []time.Time
	fb := &fakeBackend{}
	if _, err := freshClient(t, "bd", fb, false, &since).SetPriority(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if want := []string{"run", "revalidate", "exportBD"}; !slices.Equal(fb.kinds(), want) {
		t.Fatalf("calls %q, want %q", fb.kinds(), want)
	}
}

// With no exportFresh at all (a backend that cannot tell) the export always runs.
func TestWriteExportsWithoutAFreshnessCheck(t *testing.T) {
	fb := &fakeBackend{}
	c := newTestClient(t, "bd", "", fb)
	if c.b.exportFresh != nil {
		t.Fatal("the fake backend must not answer freshness")
	}
	if _, err := c.SetPriority(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(fb.kinds(), "exportBD") {
		t.Fatalf("calls %q: no export", fb.kinds())
	}
}

// The freshness window starts with the LAST command of a multi-command write: an earlier one can
// export and a later one be throttled.
func TestFreshnessIsAskedAboutTheLastCommand(t *testing.T) {
	var calls []time.Time
	script := func([]string) ([]byte, string, error) {
		calls = append(calls, time.Now())
		time.Sleep(5 * time.Millisecond)
		return nil, "", nil
	}
	var since []time.Time
	fb := &fakeBackend{script: script}
	if _, err := freshClient(t, "bd", fb, true, &since).AnswerQuestion(context.Background(), "Decision: (a) x", "needs-decision"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || len(since) != 1 {
		t.Fatalf("%d commands, %d freshness checks", len(calls), len(since))
	}
	if !since[0].After(calls[0]) || since[0].After(calls[1]) {
		t.Fatalf("since %v must fall after the first command (%v) and not after the last (%v)", since[0], calls[0], calls[1])
	}
}

// A native command that falls back to several: the window starts with the last fallback command.
func TestFreshnessAfterAFallback(t *testing.T) {
	var calls []time.Time
	script := func(args []string) ([]byte, string, error) {
		calls = append(calls, time.Now())
		time.Sleep(5 * time.Millisecond)
		if args[0] == "signoff" {
			return nil, "", unknownCommand(args)
		}
		return nil, "", nil
	}
	var since []time.Time
	fb := &fakeBackend{script: script}
	if _, err := freshClient(t, "bd", fb, true, &since).Signoff(context.Background(), true, ""); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 { // the unknown native command, then the one-op approve fallback
		t.Fatalf("%d commands", len(calls))
	}
	if !since[0].After(calls[0]) || since[0].After(calls[1]) {
		t.Fatalf("since %v must follow the failed native command (%v) and not the fallback (%v)", since[0], calls[0], calls[1])
	}
}

func TestFailedWriteChecksNothing(t *testing.T) {
	var since []time.Time
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) { return nil, "no", unknownCommand(args) }}
	if _, err := freshClient(t, "bd", fb, true, &since).SetPriority(context.Background(), 1); err == nil {
		t.Fatal("failure swallowed")
	}
	if len(since) != 0 || slices.Contains(fb.kinds(), "exportBD") {
		t.Fatalf("a failed write refreshed or checked: %d checks, calls %q", len(since), fb.kinds())
	}
}

func TestBRIsNeverSkipped(t *testing.T) {
	var since []time.Time
	fb := &fakeBackend{}
	if _, err := freshClient(t, "br", fb, true, &since).SetPriority(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(since) != 0 {
		t.Fatal("br must not be asked about bd's export")
	}
	runs := fb.runs()
	if len(runs) != 2 || runs[1][0] != "sync" {
		t.Fatalf("br runs %q, want the write then a guarded flush", runs)
	}
}

func TestUpdateSkipsTheExportBDAlreadyDid(t *testing.T) {
	var since []time.Time
	fb := &fakeBackend{}
	base := testIssue(t, "bd")
	fb.script = func(args []string) ([]byte, string, error) {
		if args[0] == "show" {
			return showJSON(t, base, "local-1"), "", nil
		}
		return nil, "update ok", nil
	}
	c := freshClient(t, "bd", fb, true, &since)
	c.issue = base
	if _, err := c.Update(context.Background(), FieldDiff{Title: strp("Snare")}); err != nil {
		t.Fatal(err)
	}
	// The conflict check's revalidate and show still run; only the second bd process is gone.
	if want := []string{"revalidate", "run", "run"}; !slices.Equal(fb.kinds(), want) {
		t.Fatalf("calls %q, want %q", fb.kinds(), want)
	}
	if len(since) != 1 {
		t.Fatalf("exportFresh asked %d times", len(since))
	}
}
