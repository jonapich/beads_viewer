package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

func TestRobotNextSchemaRequiredFieldsMatchActualOutcomes(t *testing.T) {
	t.Setenv("BEADS_DIR", t.TempDir())
	t.Setenv("BEADS_DB", "")
	schema := generateRobotSchemas().Commands["robot-next"]
	for _, name := range []string{"empty", "unbound", "partial", "bound"} {
		t.Run(name, func(t *testing.T) {
			issues := []model.Issue{{ID: "test-1", Title: "Inspect ready work", Status: model.StatusOpen, IssueType: model.TypeTask, Priority: 1}}
			if name == "empty" {
				issues = nil
			} else if name == "bound" {
				// Explicit synthetic origin tests handler/schema agreement only;
				// real loader/executable routing belongs to the live E2E suite.
				issues[0].Origin = &model.IssueOrigin{LocalID: "test-1", WorkingDirectory: "/fixture", TrackerDirectory: "/fixture/.beads",
					Database: "/fixture/.beads/beads.db", Tracker: "br", Executable: "/fixture/br", SupportsClaim: true}
			}
			source := RobotSourceReport{Status: "loaded", Valid: len(issues), Visible: len(issues)}
			if name == "partial" {
				source.Errors = 1
			}
			var out bytes.Buffer
			ctx := RobotContext{Issues: issues, SourceAuthority: newRobotSourceAuthority([]RobotSourceReport{source}), Encoder: json.NewEncoder(&out)}
			if err := handleRobotNext(ctx, phaseThreeRobotHandlerConfig{}); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			for _, field := range schema["required"].([]string) {
				if _, ok := fields[field]; !ok {
					t.Errorf("documented required field %q is absent in actual %s response: %s", field, name, out.String())
				}
			}
			var actionable bool
			if err := json.Unmarshal(fields["actionable"], &actionable); err != nil || actionable != (name == "bound") {
				t.Fatalf("wrong handler branch: actionable%v err%v output%s", actionable, err, out.String())
			}
		})
	}
}

// TestRobotPlanAndPriorityIncludeMetadata runs the built binary against a tiny fixture project
// to assert that robot-plan and robot-priority include data_hash, analysis_config, and status.
func TestRobotPlanAndPriorityIncludeMetadata(t *testing.T) {
	dir := t.TempDir()
	// create minimal .beads directory with beads.jsonl
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}
	beads := `{"id":"TEST-1","title":"A","status":"open","priority":1,"issue_type":"task"}
{"id":"TEST-2","title":"B","status":"open","priority":2,"issue_type":"task","dependencies":[{"issue_id":"TEST-2","depends_on_id":"TEST-1","type":"blocks"}]}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(beads), 0o644); err != nil {
		t.Fatalf("write beads: %v", err)
	}

	exe := buildTestBinary(t)

	runAndCheck := func(flag string) {
		cmd := exec.Command(exe, flag)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s failed: %v, out=%s", flag, err, string(out))
		}
		var payload map[string]any
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("%s json: %v", flag, err)
		}
		if _, ok := payload["data_hash"]; !ok {
			t.Fatalf("%s missing data_hash", flag)
		}
		if _, ok := payload["analysis_config"]; !ok {
			t.Fatalf("%s missing analysis_config", flag)
		}
		statusAny, ok := payload["status"]
		if !ok {
			t.Fatalf("%s missing status", flag)
		}

		status, ok := statusAny.(map[string]any)
		if !ok {
			t.Fatalf("%s status not an object", flag)
		}

		// Ensure the status contract is usable at process exit (no pending/empty states).
		expected := []string{"PageRank", "Betweenness", "Eigenvector", "HITS", "Critical", "Cycles", "KCore", "Articulation", "Slack"}
		for _, metric := range expected {
			entryAny, ok := status[metric]
			if !ok {
				t.Fatalf("%s status missing %s", flag, metric)
			}
			entry, ok := entryAny.(map[string]any)
			if !ok {
				t.Fatalf("%s status.%s not an object", flag, metric)
			}
			stateAny, ok := entry["state"]
			if !ok {
				t.Fatalf("%s status.%s missing state", flag, metric)
			}
			state, _ := stateAny.(string)
			if state == "" {
				t.Fatalf("%s status.%s state empty", flag, metric)
			}
			if state == "pending" {
				t.Fatalf("%s status.%s still pending at exit", flag, metric)
			}
		}
	}

	runAndCheck("--robot-plan")
	runAndCheck("--robot-priority")
}

// exeName appends the platform executable suffix (".exe" on Windows).
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// The bv binary is built once per test run: every test that drives it shares
// one executable instead of paying a full `go build` each (about 30 of them).
// Tests only execute the binary; none may modify or delete it.
var (
	testBinaryOnce sync.Once
	testBinaryDir  string
	testBinaryPath string
	testBinaryErr  error
)

// buildTestBinary returns the current module's bv binary for testing, built
// on first use and shared by every test in the package.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	testBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "bv-cmd-test-bin-")
		if err != nil {
			testBinaryErr = fmt.Errorf("create binary dir: %w", err)
			return
		}
		testBinaryDir = dir
		exe := filepath.Join(dir, exeName("bv-testbin"))
		cmd := exec.Command("go", "build", "-o", exe, ".")
		cmd.Dir = "." // build current package
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			testBinaryErr = fmt.Errorf("build bv: %v, out=%s", err, string(out))
			return
		}
		testBinaryPath = exe
	})
	if testBinaryErr != nil {
		t.Fatal(testBinaryErr)
	}
	return testBinaryPath
}

func TestRobotNextFailClosedWhenNoClaimableItem(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}

	beads := `{"id":"BLOCKED-1","title":"Blocked high impact","status":"blocked","priority":0,"issue_type":"bug"}
{"id":"OWNED-1","title":"Already owned","status":"open","assignee":"OtherAgent","priority":1,"issue_type":"task"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(beads), 0o644); err != nil {
		t.Fatalf("write beads: %v", err)
	}

	exe := buildTestBinary(t)
	cmd := exec.Command(exe, "--robot-next")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("robot-next failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("robot-next json: %v\n%s", err, out)
	}
	if got := payload["actionable"]; got != false {
		t.Fatalf("actionable = %v, want false; payload=%v", got, payload)
	}
	if _, ok := payload["claim_command"]; ok {
		t.Fatalf("fail-closed robot-next must not emit claim_command: %s", out)
	}
	if _, ok := payload["status"].(map[string]any); !ok {
		t.Fatalf("robot-next missing metric status: %s", out)
	}
	degraded, ok := payload["degraded"].([]any)
	if !ok || len(degraded) == 0 {
		t.Fatalf("robot-next fail-closed response missing degraded[]: %s", out)
	}
	first, ok := degraded[0].(map[string]any)
	if !ok || first["code"] != "no_actionable_recommendation" {
		t.Fatalf("unexpected degraded payload: %v", degraded)
	}
}

func TestRobotNextPreservesSafeUnboundTopPick(t *testing.T) {
	// This metadata-free input tests diagnostic readiness. Actual command
	// emission belongs to TestRobotActionRoutesLiveTrackers, which initializes
	// and inspects real trackers; ordinary Go tests need no installed br.
	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_DB", "")
	t.Setenv("BD_DB", "")
	t.Setenv("BV_NO_GITIGNORE", "1")
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}

	beads := `{"id":"READY-1","title":"Ready work","status":"open","priority":1,"issue_type":"task"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(beads), 0o644); err != nil {
		t.Fatalf("write beads: %v", err)
	}

	exe := buildTestBinary(t)
	cmd := exec.Command(exe, "--robot-next")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("robot-next failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("robot-next json: %v\n%s", err, out)
	}
	if payload["actionable"] != false || payload["id"] != nil || payload["claim_command"] != nil || payload["show_command"] != nil {
		t.Fatalf("unbound issue emitted a live tracker command: %s", out)
	}
	diagnostic, ok := payload["diagnostic_top_pick"].(map[string]any)
	if !ok || diagnostic["id"] != "READY-1" || diagnostic["title"] != "Ready work" {
		t.Fatalf("ready issue must remain the useful diagnostic candidate: %s", out)
	}
	authority, ok := payload["source_authority"].(map[string]any)
	if !ok || authority["claim_safe"] != true {
		t.Fatalf("unbound route must not erase complete graph authority: %s", out)
	}
	actions, ok := payload["actions"].(map[string]any)
	if !ok || actions["claim"] != nil || actions["show"] != nil {
		t.Fatalf("unbound route must explicitly withhold nested commands: %s", out)
	}
	if reason, ok := actions["unavailable_reason"].(string); !ok || reason == "" {
		t.Fatalf("unbound route must explain why commands are unavailable: %s", out)
	}
	if _, ok := payload["status"].(map[string]any); !ok {
		t.Fatalf("robot-next missing metric status: %s", out)
	}
}

func TestRobotNextClaimablePickRejectsAssignedTopPick(t *testing.T) {
	picks := []analysis.TopPick{{
		ID:    "ASSIGNED-1",
		Title: "Already owned",
		Score: 100,
	}}
	issues := []model.Issue{{
		ID:        "ASSIGNED-1",
		Title:     "Already owned",
		Status:    model.StatusOpen,
		IssueType: model.TypeTask,
		Assignee:  " cc11 ",
	}}

	_, diagnostic, reasons, ok := robotNextClaimablePick(picks, issues, nil, time.Time{})
	if ok {
		t.Fatalf("assigned top pick must not be claimable")
	}
	if diagnostic == nil || diagnostic.ID != "ASSIGNED-1" {
		t.Fatalf("diagnostic = %+v, want ASSIGNED-1", diagnostic)
	}
	if len(reasons) == 0 || !strings.Contains(strings.Join(reasons, "; "), "assigned") {
		t.Fatalf("reasons = %v, want assigned reason", reasons)
	}
}
