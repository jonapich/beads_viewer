package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// fakeCall is one recorded backend call.
type fakeCall struct {
	kind     string // "run", "revalidate" or "exportBD"
	mutating bool
	args     []string
}

// fakeBackend records every call and answers run from script.
type fakeBackend struct {
	calls         []fakeCall
	script        func(args []string) ([]byte, string, error)
	revalidateErr error
	exportErr     error
}

func (f *fakeBackend) backend() backend {
	return backend{
		run: func(_ context.Context, _ model.IssueOrigin, mutating bool, operation []string) ([]byte, string, error) {
			f.calls = append(f.calls, fakeCall{kind: "run", mutating: mutating, args: slices.Clone(operation)})
			if f.script == nil {
				return nil, "", nil
			}
			return f.script(operation)
		},
		revalidate: func(sourcePath string, _ model.IssueOrigin) error {
			f.calls = append(f.calls, fakeCall{kind: "revalidate", args: []string{sourcePath}})
			return f.revalidateErr
		},
		exportBD: func(context.Context, model.IssueOrigin) error {
			f.calls = append(f.calls, fakeCall{kind: "exportBD"})
			return f.exportErr
		},
	}
}

// runs returns the argv of every run call, in order.
func (f *fakeBackend) runs() [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c.kind == "run" {
			out = append(out, c.args)
		}
	}
	return out
}

// kinds returns the call kinds, in order.
func (f *fakeBackend) kinds() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, c.kind)
	}
	return out
}

func testOrigin(t *testing.T, trackerName string) *model.IssueOrigin {
	t.Helper()
	dir := t.TempDir()
	return &model.IssueOrigin{
		LocalID:          "local-1",
		WorkingDirectory: dir,
		TrackerDirectory: filepath.Join(dir, ".beads"),
		Database:         filepath.Join(dir, ".beads", "beads.db"),
		Tracker:          trackerName,
		Executable:       filepath.Join(dir, trackerName+".exe"),
	}
}

func testIssue(t *testing.T, trackerName string) model.Issue {
	t.Helper()
	return model.Issue{
		ID: "ws-local-1", Title: "Kick drum", Description: "boom", Status: model.StatusClosed,
		Priority: 2, IssueType: model.TypeTask, Assignee: "ann",
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Origin:    testOrigin(t, trackerName),
	}
}

// newTestClient builds a client whose backend is fake. sourcePath "" means the
// export is refreshed after writes.
func newTestClient(t *testing.T, trackerName, sourcePath string, fb *fakeBackend) *Client {
	t.Helper()
	c, err := New(testIssue(t, trackerName), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	c.b = fb.backend()
	return c
}

func unknownCommand(args []string) error {
	return &CommandError{Args: args, ExitCode: 1, Stderr: `Error: unknown command "signoff" for "bd"`, Err: errors.New("exit status 1")}
}

func TestNewRejectsUnwritableOrigin(t *testing.T) {
	issue := testIssue(t, "bd")
	issue.Origin = nil
	if _, err := New(issue, ""); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("nil origin: %v", err)
	}
	issue.Origin = testOrigin(t, "bd")
	issue.Origin.ReadOnlyReason = "frozen"
	if _, err := New(issue, ""); err == nil || err.Error() != "source is read-only: frozen" {
		t.Fatalf("read-only origin: %v", err)
	}
}

func TestNewClonesIssue(t *testing.T) {
	issue := testIssue(t, "bd")
	c, err := New(issue, "")
	if err != nil {
		t.Fatal(err)
	}
	issue.Origin.LocalID = "changed"
	if c.Origin().LocalID != "local-1" || c.issue.Origin.LocalID != "local-1" {
		t.Fatalf("client origin follows the caller's issue: %+v", c.Origin())
	}
}

func TestRunOpsSequence(t *testing.T) {
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		return nil, "diag-" + args[0], nil
	}}
	c := newTestClient(t, "br", "x.db", fb)
	res, err := c.runOps(context.Background(), []op{
		{args: []string{"update", "--json", "--priority=1"}},
		{args: []string{"comments", "add"}, trailing: []string{"hello", "world"}},
		{args: []string{"list", "--json"}, noTarget: true, trailing: []string{"ignored"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"update", "--json", "--priority=1", "--", "local-1"},
		{"comments", "add", "--", "local-1", "hello", "world"},
		{"list", "--json"},
	}
	if got := fb.runs(); len(got) != len(want) {
		t.Fatalf("runs: %q", got)
	} else {
		for i := range want {
			if !slices.Equal(got[i], want[i]) {
				t.Fatalf("run %d: got %q want %q", i, got[i], want[i])
			}
		}
	}
	for _, call := range fb.calls {
		if !call.mutating {
			t.Fatalf("ops must run as mutating calls: %+v", call)
		}
	}
	if res.Output != "diag-list" {
		t.Fatalf("output must be the last op's diagnostic: %q", res.Output)
	}
}

func TestRunOpsStopsAtFirstFailure(t *testing.T) {
	boom := errors.New("boom")
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		if args[0] == "second" {
			return nil, "second failed", boom
		}
		return nil, "ok", nil
	}}
	c := newTestClient(t, "br", "x.db", fb)
	res, err := c.runOps(context.Background(), []op{{args: []string{"first"}}, {args: []string{"second"}}, {args: []string{"third"}}})
	if !errors.Is(err, boom) {
		t.Fatalf("err: %v", err)
	}
	if got := fb.runs(); len(got) != 2 {
		t.Fatalf("a failure must stop the sequence: %q", got)
	}
	if res.Output != "second failed" {
		t.Fatalf("output: %q", res.Output)
	}
}

func TestRunOpsUnknownCommandFallback(t *testing.T) {
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		if args[0] == "signoff" {
			return nil, `Error: unknown command "signoff" for "bd"`, unknownCommand(args)
		}
		return nil, "", nil
	}}
	c := newTestClient(t, "br", "x.db", fb)
	_, err := c.runOps(context.Background(), []op{
		{args: []string{"signoff"}, fallback: []op{{args: []string{"fb1"}}, {args: []string{"fb2"}}}},
		{args: []string{"after"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range fb.runs() {
		names = append(names, r[0])
	}
	if want := []string{"signoff", "fb1", "fb2", "after"}; !slices.Equal(names, want) {
		t.Fatalf("sequence %q, want %q", names, want)
	}
}

func TestRunOpsOtherFailureDoesNotFallBack(t *testing.T) {
	for name, fail := range map[string]func(args []string) error{
		"command error without unknown command": func(args []string) error {
			return &CommandError{Args: args, ExitCode: 1, Stderr: "database locked", Err: errors.New("exit status 1")}
		},
		"plain error": func([]string) error { return errors.New("unknown command but not a CommandError") },
	} {
		t.Run(name, func(t *testing.T) {
			fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
				return nil, "unknown command", fail(args)
			}}
			c := newTestClient(t, "br", "x.db", fb)
			_, err := c.runOps(context.Background(), []op{{args: []string{"signoff"}, fallback: []op{{args: []string{"fb"}}}}})
			if err == nil {
				t.Fatal("failure swallowed")
			}
			if got := fb.runs(); len(got) != 1 {
				t.Fatalf("must not fall back: %q", got)
			}
		})
	}
}

func TestRunOpsFallbackFailureIsReported(t *testing.T) {
	boom := errors.New("fallback broke")
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		switch args[0] {
		case "signoff":
			return nil, "unknown command", unknownCommand(args)
		case "fb":
			return nil, "fb diag", boom
		}
		return nil, "", nil
	}}
	c := newTestClient(t, "br", "x.db", fb)
	res, err := c.runOps(context.Background(), []op{{args: []string{"signoff"}, fallback: []op{{args: []string{"fb"}}}}, {args: []string{"after"}}})
	if !errors.Is(err, boom) || len(fb.runs()) != 2 || res.Output != "fb diag" {
		t.Fatalf("err=%v runs=%q output=%q", err, fb.runs(), res.Output)
	}
}

func TestRunOpsCapturesNewID(t *testing.T) {
	for name, out := range map[string]string{
		"object":   `{"id":"x"}`,
		"array":    `[{"id":"x"}]`,
		"followup": `{"followup":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
				if args[0] == "create" {
					return []byte(out), "", nil
				}
				return nil, "", nil
			}}
			c := newTestClient(t, "br", "x.db", fb)
			res, err := c.runOps(context.Background(), []op{
				{args: []string{"create", "--description={new}"}, noTarget: true, captureID: true},
				{args: []string{"update", "--set-metadata=f={new}"}, trailing: []string{"see {new}"}},
			})
			if err != nil || res.CreatedID != "x" {
				t.Fatalf("err=%v created=%q", err, res.CreatedID)
			}
			got := fb.runs()
			if !slices.Equal(got[0], []string{"create", "--description="}) {
				t.Fatalf("{new} before capture is empty: %q", got[0])
			}
			if want := []string{"update", "--set-metadata=f=x", "--", "local-1", "see x"}; !slices.Equal(got[1], want) {
				t.Fatalf("got %q want %q", got[1], want)
			}
		})
	}
}

func TestRunOpsCaptureErrors(t *testing.T) {
	for name, out := range map[string]string{
		"not json":   "created!",
		"empty list": "[]",
		"no id":      `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			fb := &fakeBackend{script: func([]string) ([]byte, string, error) { return []byte(out), "", nil }}
			c := newTestClient(t, "br", "x.db", fb)
			if _, err := c.runOps(context.Background(), []op{{args: []string{"create"}, noTarget: true, captureID: true}, {args: []string{"next"}}}); err == nil {
				t.Fatal("capture error not reported")
			}
			if len(fb.runs()) != 1 {
				t.Fatalf("must stop after the failed capture: %q", fb.runs())
			}
		})
	}
}

func TestRunOpsCreatedIDSurvivesLaterFailure(t *testing.T) {
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		switch args[0] {
		case "create":
			return []byte(`{"id":"new-9"}`), "", nil
		case "comments":
			return nil, "cannot comment", &CommandError{Args: args, ExitCode: 1, Stderr: "cannot comment", Err: errors.New("exit status 1")}
		}
		return nil, "", nil
	}}
	c := newTestClient(t, "br", "x.db", fb)
	res, err := c.runOps(context.Background(), []op{
		{args: []string{"create"}, noTarget: true, captureID: true},
		{args: []string{"comments", "add"}},
		{args: []string{"update"}},
	})
	if err == nil || res.CreatedID != "new-9" || res.Output != "cannot comment" || len(fb.runs()) != 2 {
		t.Fatalf("err=%v result=%+v runs=%q", err, res, fb.runs())
	}
}

func TestRunOpsNewIDSharedAcrossFallback(t *testing.T) {
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		switch args[0] {
		case "signoff":
			return nil, "unknown command", unknownCommand(args)
		case "create":
			return []byte(`{"id":"fu-1"}`), "", nil
		}
		return nil, "", nil
	}}
	c := newTestClient(t, "br", "x.db", fb)
	res, err := c.runOps(context.Background(), []op{{
		args: []string{"signoff"}, captureID: true,
		fallback: []op{
			{args: []string{"create"}, noTarget: true, captureID: true},
			{args: []string{"update", "--set-metadata=f={new}"}},
		},
	}})
	if err != nil || res.CreatedID != "fu-1" {
		t.Fatalf("err=%v result=%+v", err, res)
	}
	if got := fb.runs(); !slices.Equal(got[2], []string{"update", "--set-metadata=f=fu-1", "--", "local-1"}) {
		t.Fatalf("fallback ops must see the captured id: %q", got)
	}
}

func TestSignoffNativeVsLegacy(t *testing.T) {
	local := "local-1"
	legacyApprove := []string{"update", "--json", "--remove-label=needs-signoff", "--set-metadata=signoff=approved", "--", local}
	legacyDispute := [][]string{
		{"create", "--json", "--title=Dispute: Kick drum", "--description=beat too loud", "--deps=discovered-from:" + local, "--priority=2"},
		{"comments", "add", "--", local, "Disputed: beat too loud (follow-up fu-1)"},
		{"update", "--json", "--remove-label=needs-signoff", "--set-metadata=signoff=disputed",
			"--set-metadata=signoff_note=beat too loud", "--set-metadata=signoff_followup=fu-1", "--", local},
	}
	scriptFor := func(refuseSignoff bool) func([]string) ([]byte, string, error) {
		return func(args []string) ([]byte, string, error) {
			switch args[0] {
			case "signoff":
				if refuseSignoff {
					return nil, `Error: unknown command "signoff" for "bd"`, unknownCommand(args)
				}
				if slices.Contains(args, "--dispute=beat too loud") {
					return []byte(`{"followup":"fu-1"}`), "", nil
				}
			case "create":
				return []byte(`{"id":"fu-1"}`), "", nil
			}
			return nil, "", nil
		}
	}
	assertRuns := func(t *testing.T, fb *fakeBackend, want ...[]string) {
		t.Helper()
		got := fb.runs()
		if len(got) != len(want) {
			t.Fatalf("runs: %q\nwant: %q", got, want)
		}
		for i := range want {
			if !slices.Equal(got[i], want[i]) {
				t.Fatalf("run %d:\n got %q\nwant %q", i, got[i], want[i])
			}
		}
	}

	t.Run("bd approve is one native op", func(t *testing.T) {
		fb := &fakeBackend{script: scriptFor(false)}
		c := newTestClient(t, "bd", "x.db", fb)
		if _, err := c.Signoff(context.Background(), true, ""); err != nil {
			t.Fatal(err)
		}
		assertRuns(t, fb, []string{"signoff", "--json", "--approve", "--", local})
	})
	t.Run("bd dispute is one native op and captures the follow-up", func(t *testing.T) {
		fb := &fakeBackend{script: scriptFor(false)}
		c := newTestClient(t, "bd", "x.db", fb)
		res, err := c.Signoff(context.Background(), false, "beat too loud")
		if err != nil || res.CreatedID != "fu-1" {
			t.Fatalf("err=%v result=%+v", err, res)
		}
		assertRuns(t, fb, []string{"signoff", "--json", "--dispute=beat too loud", "--", local})
	})
	t.Run("br approve uses the legacy list", func(t *testing.T) {
		fb := &fakeBackend{script: scriptFor(false)}
		c := newTestClient(t, "br", "x.db", fb)
		if _, err := c.Signoff(context.Background(), true, ""); err != nil {
			t.Fatal(err)
		}
		assertRuns(t, fb, legacyApprove)
	})
	t.Run("br dispute uses the legacy list", func(t *testing.T) {
		fb := &fakeBackend{script: scriptFor(false)}
		c := newTestClient(t, "br", "x.db", fb)
		res, err := c.Signoff(context.Background(), false, "beat too loud")
		if err != nil || res.CreatedID != "fu-1" {
			t.Fatalf("err=%v result=%+v", err, res)
		}
		assertRuns(t, fb, legacyDispute...)
	})
	t.Run("old bd approve falls back", func(t *testing.T) {
		fb := &fakeBackend{script: scriptFor(true)}
		c := newTestClient(t, "bd", "x.db", fb)
		if _, err := c.Signoff(context.Background(), true, ""); err != nil {
			t.Fatal(err)
		}
		assertRuns(t, fb, []string{"signoff", "--json", "--approve", "--", local}, legacyApprove)
	})
	t.Run("old bd dispute falls back", func(t *testing.T) {
		fb := &fakeBackend{script: scriptFor(true)}
		c := newTestClient(t, "bd", "x.db", fb)
		res, err := c.Signoff(context.Background(), false, "beat too loud")
		if err != nil || res.CreatedID != "fu-1" {
			t.Fatalf("err=%v result=%+v", err, res)
		}
		assertRuns(t, fb, append([][]string{{"signoff", "--json", "--dispute=beat too loud", "--", local}}, legacyDispute...)...)
	})
	t.Run("a failed create stops everything", func(t *testing.T) {
		fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
			switch args[0] {
			case "signoff":
				return nil, "unknown command", unknownCommand(args)
			case "create":
				return nil, "create refused", &CommandError{Args: args, ExitCode: 1, Stderr: "create refused", Err: errors.New("exit status 1")}
			}
			return nil, "", nil
		}}
		c := newTestClient(t, "bd", "x.db", fb)
		res, err := c.Signoff(context.Background(), false, "beat too loud")
		if err == nil || res.CreatedID != "" || res.Output != "create refused" {
			t.Fatalf("err=%v result=%+v", err, res)
		}
		if got := fb.runs(); len(got) != 2 || got[0][0] != "signoff" || got[1][0] != "create" {
			t.Fatalf("nothing but the refused signoff and the failed create may run: %q", got)
		}
	})
}

func TestAnswerAndQuickActions(t *testing.T) {
	cases := []struct {
		name string
		call func(*Client) (Result, error)
		want [][]string
	}{
		{"priority", func(c *Client) (Result, error) { return c.SetPriority(context.Background(), 0) },
			[][]string{{"update", "--json", "--priority=0", "--", "local-1"}}},
		{"label on", func(c *Client) (Result, error) { return c.SetLabel(context.Background(), "close-candidate", true) },
			[][]string{{"update", "--json", "--add-label=close-candidate", "--", "local-1"}}},
		{"label off", func(c *Client) (Result, error) { return c.SetLabel(context.Background(), "close-candidate", false) },
			[][]string{{"update", "--json", "--remove-label=close-candidate", "--", "local-1"}}},
		{"comment", func(c *Client) (Result, error) { return c.AddComment(context.Background(), "hi") },
			[][]string{{"comments", "add", "--", "local-1", "hi"}}},
		{"answer", func(c *Client) (Result, error) {
			return c.AnswerQuestion(context.Background(), "use blue", "awaiting-answer")
		}, [][]string{{"comments", "add", "--", "local-1", "use blue"}, {"update", "--json", "--remove-label=awaiting-answer", "--", "local-1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fb := &fakeBackend{}
			c := newTestClient(t, "br", "x.db", fb) // .db source: no refresh
			if _, err := tc.call(c); err != nil {
				t.Fatal(err)
			}
			got := fb.runs()
			if len(got) != len(tc.want) {
				t.Fatalf("runs %q want %q", got, tc.want)
			}
			for i := range got {
				if !slices.Equal(got[i], tc.want[i]) {
					t.Fatalf("run %d: %q want %q", i, got[i], tc.want[i])
				}
			}
			for _, k := range fb.kinds() {
				if k != "run" {
					t.Fatalf("quick actions must not pre-revalidate (and a .db br source does not refresh): %q", fb.kinds())
				}
			}
		})
	}
}

// showJSON encodes issue the way `show --json` would.
func showJSON(t *testing.T, issue model.Issue, local string) []byte {
	t.Helper()
	issue.ID = local
	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func strp(s string) *string { return &s }

func TestUpdateVerifyThenUpdate(t *testing.T) {
	base := testIssue(t, "br")
	fb := &fakeBackend{}
	fb.script = func(args []string) ([]byte, string, error) {
		if args[0] == "show" {
			return showJSON(t, base, "local-1"), "show warning", nil
		}
		return nil, "update ok", nil
	}
	c := newTestClient(t, "br", "x.db", fb)
	c.issue = base
	res, err := c.Update(context.Background(), FieldDiff{Description: strp("new text"), Title: strp("Snare"), Priority: strp("1")})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"revalidate", "run", "run"}; !slices.Equal(fb.kinds(), want) {
		t.Fatalf("order %q, want %q", fb.kinds(), want)
	}
	if fb.calls[0].args[0] != "x.db" {
		t.Fatalf("revalidate source: %q", fb.calls[0].args)
	}
	if show := fb.calls[1]; show.mutating || !slices.Equal(show.args, []string{"show", "--json", "--", "local-1"}) {
		t.Fatalf("show call: %+v", show)
	}
	update := fb.calls[2]
	if want := []string{"update", "--json", "--title=Snare", "--priority=1", "--description=new text", "--", "local-1"}; !update.mutating || !slices.Equal(update.args, want) {
		t.Fatalf("update call: %+v want %q", update, want)
	}
	if res.Output != "update ok" || res.RefreshErr != nil {
		t.Fatalf("result: %+v", res)
	}
}

func TestUpdateConflict(t *testing.T) {
	base := testIssue(t, "br")
	for name, mutate := range map[string]func(*model.Issue){
		"field":     func(i *model.Issue) { i.Title = "changed elsewhere" },
		"timestamp": func(i *model.Issue) { i.UpdatedAt = i.UpdatedAt.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			live := base
			mutate(&live)
			fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
				return showJSON(t, live, "local-1"), "", nil
			}}
			c := newTestClient(t, "br", "x.db", fb)
			c.issue = base
			res, err := c.Update(context.Background(), FieldDiff{Title: strp("mine")})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("err: %v", err)
			}
			if res.Output != "" || len(fb.runs()) != 1 {
				t.Fatalf("drift must stop before update: result=%+v runs=%q", res, fb.runs())
			}
		})
	}
}

func TestUpdateShowFailure(t *testing.T) {
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		return nil, "db locked", &CommandError{Args: args, ExitCode: 1, Stderr: "db locked", Err: errors.New("exit status 1")}
	}}
	c := newTestClient(t, "br", "x.db", fb)
	res, err := c.Update(context.Background(), FieldDiff{Title: strp("mine")})
	if err == nil || err.Error() != "verify current tracker issue: exit status 1" {
		t.Fatalf("err: %v", err)
	}
	var ce *CommandError
	if !errors.As(err, &ce) || ce.Stderr != "db locked" {
		t.Fatalf("errors.As through the verify wrapper: %v", err)
	}
	if res.Output != "db locked" || len(fb.runs()) != 1 {
		t.Fatalf("result=%+v runs=%q", res, fb.runs())
	}
}

func TestUpdateDecodeFailureLeavesOutputEmpty(t *testing.T) {
	cases := map[string]string{
		"bad json":       "{nope",
		"empty":          "",
		"two issues":     "[{},{}]",
		"different id":   `{"id":"someone-else"}`,
		"bad array json": "[nope",
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			fb := &fakeBackend{script: func([]string) ([]byte, string, error) {
				return []byte(out), "warning: deprecated flag", nil
			}}
			c := newTestClient(t, "br", "x.db", fb)
			res, err := c.Update(context.Background(), FieldDiff{Title: strp("mine")})
			if err == nil {
				t.Fatal("decode failure not reported")
			}
			if res.Output != "" || len(fb.runs()) != 1 {
				t.Fatalf("a successful show's stderr must not leak: result=%+v runs=%q", res, fb.runs())
			}
		})
	}
}

func TestUpdateGuards(t *testing.T) {
	fb := &fakeBackend{}
	c := newTestClient(t, "br", "x.db", fb)
	if _, err := c.Update(context.Background(), FieldDiff{}); err == nil {
		t.Fatal("empty diff accepted")
	}
	if len(fb.calls) != 0 {
		t.Fatalf("empty diff must run nothing: %+v", fb.calls)
	}
	fb.revalidateErr = errors.New("route moved")
	if _, err := c.Update(context.Background(), FieldDiff{Title: strp("x")}); err == nil || err.Error() != "route moved" || len(fb.runs()) != 0 {
		t.Fatalf("revalidate failure: %v runs=%q", err, fb.runs())
	}
	c.origin.ReadOnlyReason = "frozen"
	fb.calls = nil
	if _, err := c.Update(context.Background(), FieldDiff{Title: strp("x")}); err == nil || len(fb.calls) != 0 {
		t.Fatalf("unwritable origin must stop first: %v calls=%+v", err, fb.calls)
	}
}

func TestFieldDiff(t *testing.T) {
	d := FieldDiff{Description: strp("d"), Type: strp("bug"), Assignee: strp("bo"), Priority: strp("p1"), Status: strp("open"), Title: strp("t")}
	if d.Len() != 6 {
		t.Fatalf("len %d", d.Len())
	}
	want := []string{"--title=t", "--status=open", "--priority=p1", "--assignee=bo", "--type=bug", "--description=d"}
	if got := d.flags(); !slices.Equal(got, want) {
		t.Fatalf("flags %q want %q", got, want)
	}
	if (FieldDiff{}).Len() != 0 || (FieldDiff{}).flags() != nil {
		t.Fatal("empty diff")
	}
	// An empty value is still a change; nil is not.
	if got := (FieldDiff{Description: strp("")}).flags(); !slices.Equal(got, []string{"--description="}) {
		t.Fatalf("empty value: %q", got)
	}
}

func TestRefreshConditional(t *testing.T) {
	refreshErr := errors.New("export failed")
	cases := []struct {
		name       string
		tracker    string
		source     string
		wantKinds  []string
		wantFlush  bool
		wantSource string
	}{
		{"bd always exports", "bd", "x.db", []string{"run", "revalidate", "exportBD"}, false, "x.db"},
		{"br jsonl source flushes", "br", "view.JSONL", []string{"run", "revalidate", "run"}, true, "view.JSONL"},
		{"br unknown source flushes", "br", "", []string{"run", "revalidate", "run"}, true, ""},
		{"br db source does not refresh", "br", "view.db", []string{"run"}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fb := &fakeBackend{exportErr: refreshErr}
			fb.script = func(args []string) ([]byte, string, error) {
				if args[0] == "sync" {
					return nil, "flush failed", &CommandError{Args: args, ExitCode: 1, Stderr: "flush failed", Err: refreshErr}
				}
				return nil, "", nil
			}
			c := newTestClient(t, tc.tracker, tc.source, fb)
			res, err := c.SetPriority(context.Background(), 1)
			if err != nil {
				t.Fatalf("a refresh failure must not fail the write: %v", err)
			}
			if !slices.Equal(fb.kinds(), tc.wantKinds) {
				t.Fatalf("calls %q, want %q", fb.kinds(), tc.wantKinds)
			}
			if len(tc.wantKinds) == 1 {
				if res.RefreshErr != nil {
					t.Fatalf("unexpected refresh error: %v", res.RefreshErr)
				}
				return
			}
			if !errors.Is(res.RefreshErr, refreshErr) {
				t.Fatalf("RefreshErr: %v", res.RefreshErr)
			}
			if tc.wantFlush {
				last := fb.calls[len(fb.calls)-1]
				if !last.mutating || !slices.Equal(last.args, []string{"sync", "--flush-only", "--json"}) {
					t.Fatalf("flush call: %+v", last)
				}
				if res.RefreshErr.Error() != "export failed: flush failed" {
					t.Fatalf("br refresh error keeps the diagnostic: %q", res.RefreshErr)
				}
			}
			if rev := fb.calls[1]; rev.kind == "revalidate" && tc.wantSource != "" && rev.args[0] != tc.wantSource {
				t.Fatalf("revalidated source %q, want %q", rev.args[0], tc.wantSource)
			}
		})
	}
}

func TestRefreshNotRunAfterFailedWrite(t *testing.T) {
	fb := &fakeBackend{script: func(args []string) ([]byte, string, error) {
		return nil, "nope", &CommandError{Args: args, Stderr: "nope", Err: errors.New("exit status 1")}
	}}
	c := newTestClient(t, "bd", "x.db", fb)
	if _, err := c.SetPriority(context.Background(), 1); err == nil {
		t.Fatal("write failure swallowed")
	}
	if want := []string{"run"}; !slices.Equal(fb.kinds(), want) {
		t.Fatalf("calls %q", fb.kinds())
	}
}

func TestOriginSource(t *testing.T) {
	bd := model.IssueOrigin{Tracker: "bd", TrackerDirectory: filepath.Join("t", ".beads"), Database: "db"}
	br := model.IssueOrigin{Tracker: "br", TrackerDirectory: filepath.Join("t", ".beads"), Database: "db"}
	if got := originSource(bd, "view"); got != "view" {
		t.Fatal(got)
	}
	if got := originSource(bd, ""); got != filepath.Join("t", ".beads", "issues.jsonl") {
		t.Fatal(got)
	}
	if got := originSource(br, ""); got != "db" {
		t.Fatal(got)
	}
}

func TestCommandError(t *testing.T) {
	inner := errors.New("exit status 3")
	ce := &CommandError{Args: []string{"x"}, ExitCode: 3, Stderr: `Error: unknown command "x" for "bd"`, Err: inner}
	if ce.Error() != "exit status 3" || strings.Contains(ce.Error(), "unknown") {
		t.Fatalf("Error must not include stderr: %q", ce.Error())
	}
	if !ce.UnknownCommand() || (&CommandError{Stderr: "other"}).UnknownCommand() {
		t.Fatal("UnknownCommand")
	}
	wrapped := fmt.Errorf("verify current tracker issue: %w", ce)
	var got *CommandError
	if !errors.As(wrapped, &got) || got != ce || !errors.Is(wrapped, inner) {
		t.Fatal("errors.As/Is through wrapper")
	}
}

// helperOrigin points a br route at executable.
func helperOrigin(t *testing.T, executable string) model.IssueOrigin {
	t.Helper()
	origin := *testOrigin(t, "br")
	origin.Executable = executable
	return origin
}

func TestRunTrackerExitCodeAndStderr(t *testing.T) {
	// The test binary rejects the unknown --db flag with exit status 2 and a
	// diagnostic on stderr, which is a real non-zero exit without a fixture.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostic, err := runTracker(context.Background(), helperOrigin(t, self), true, []string{"update"})
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CommandError, got %T %v", err, err)
	}
	if ce.ExitCode != 2 || ce.Stderr != diagnostic || diagnostic == "" || !slices.Equal(ce.Args, []string{"update"}) {
		t.Fatalf("command error: %+v (diagnostic %q)", ce, diagnostic)
	}
	if strings.Contains(ce.Error(), diagnostic) {
		t.Fatalf("Error must not repeat stderr: %q", ce.Error())
	}
}

func TestRunTrackerNotStarted(t *testing.T) {
	origin := helperOrigin(t, filepath.Join(t.TempDir(), "missing-tracker.exe"))
	_, _, err := runTracker(context.Background(), origin, true, []string{"update"})
	var ce *CommandError
	if !errors.As(err, &ce) || ce.ExitCode != -1 {
		t.Fatalf("err: %#v", err)
	}
}

func TestRunTrackerContextCancelled(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = runTracker(ctx, helperOrigin(t, self), true, []string{"update"})
	var ce *CommandError
	if !errors.As(err, &ce) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err: %v", err)
	}
}

func TestRunTrackerRejectsBadRoute(t *testing.T) {
	origin := helperOrigin(t, "relative.exe")
	_, _, err := runTracker(context.Background(), origin, true, []string{"update"})
	var ce *CommandError
	if err == nil || errors.As(err, &ce) {
		t.Fatalf("origin errors are plain errors: %#v", err)
	}
}

func TestCommandArgsAndEnvironment(t *testing.T) {
	br := model.IssueOrigin{Tracker: "br", Database: "/db"}
	bd := model.IssueOrigin{Tracker: "bd", Database: "/db"}
	op := []string{"show", "--json"}
	cases := []struct {
		name     string
		origin   model.IssueOrigin
		mutating bool
		want     []string
	}{
		{"br read", br, false, []string{"--db", "/db", "--no-auto-import", "--no-auto-flush", "show", "--json"}},
		{"br write", br, true, []string{"--db", "/db", "--no-auto-import", "show", "--json"}},
		{"bd read", bd, false, []string{"--db", "/db", "show", "--json"}},
		{"bd write", bd, true, []string{"--db", "/db", "show", "--json"}},
	}
	for _, tc := range cases {
		if got := commandArgs(tc.origin, tc.mutating, op); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if !slices.Equal(op, []string{"show", "--json"}) {
		t.Fatal("commandArgs modified its input")
	}

	origin := model.IssueOrigin{TrackerDirectory: "/t/.beads", Database: "/t/.beads/beads.db"}
	got := environment([]string{"PATH=/bin", "beads_dir=old", "BEADS_DB=old", "Bd_Db=old", "BEADS_JSONL=old", "KEEP=1", "BEADS_OTHER=2"}, origin)
	want := []string{"PATH=/bin", "KEEP=1", "BEADS_OTHER=2", "BEADS_DIR=/t/.beads", "BEADS_DB=/t/.beads/beads.db", "BD_DB=/t/.beads/beads.db"}
	if !slices.Equal(got, want) {
		t.Fatalf("environment %q want %q", got, want)
	}
}

func TestOutputBufferLimit(t *testing.T) {
	o := &outputBuffer{limit: 4}
	if n, err := o.Write([]byte("abc")); n != 3 || err != nil || o.truncated {
		t.Fatal("first write")
	}
	if n, err := o.Write([]byte("defg")); n != 4 || err != nil || !o.truncated {
		t.Fatal("second write reports full length and truncates")
	}
	if o.buffer.String() != "abcd" {
		t.Fatalf("kept %q", o.buffer.String())
	}
}

func TestOriginError(t *testing.T) {
	good := testOrigin(t, "bd")
	if err := OriginError(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*model.IssueOrigin){
		"read-only":   func(o *model.IssueOrigin) { o.ReadOnlyReason = "why" },
		"tracker":     func(o *model.IssueOrigin) { o.Tracker = "jira" },
		"local id":    func(o *model.IssueOrigin) { o.LocalID = "" },
		"relative":    func(o *model.IssueOrigin) { o.Executable = "bd" },
		"nul":         func(o *model.IssueOrigin) { o.LocalID = "a\x00b" },
		"no database": func(o *model.IssueOrigin) { o.Database = "" },
	}
	for name, mutate := range cases {
		o := *good
		mutate(&o)
		if err := OriginError(&o); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := OriginError(nil); err == nil || err.Error() != "source has no verified live tracker; historical and imported views are read-only" {
		t.Fatalf("nil: %v", err)
	}
}

func TestCapabilitiesOf(t *testing.T) {
	bd := testOrigin(t, "bd")
	bd.SupportsClaim = true
	if got := CapabilitiesOf(bd); got != (Capabilities{Writable: true, Tracker: "bd", Claim: true, Signoff: true}) {
		t.Fatalf("bd: %+v", got)
	}
	ro := testOrigin(t, "br")
	ro.ReadOnlyReason = "stale"
	ro.SupportsClaim = true // the origin says claim works, but it is not writable
	if got := CapabilitiesOf(ro); got.Writable || got.Signoff || got.Claim || got.Tracker != "br" || got.ReadOnlyReason != "source is read-only: stale" {
		t.Fatalf("read-only: %+v", got)
	}
	if got := CapabilitiesOf(nil); got.Writable || got.Tracker != "" || got.ReadOnlyReason == "" {
		t.Fatalf("nil: %+v", got)
	}
}

func TestEditableFieldsAndUnchanged(t *testing.T) {
	a := testIssue(t, "bd")
	f := EditableFields(a)
	if f["priority"] != "2" || f["status"] != "closed" || f["title"] != "Kick drum" || len(f) != 6 {
		t.Fatalf("fields: %v", f)
	}
	b := a
	if !Unchanged(a, b) {
		t.Fatal("identical issues")
	}
	b.Assignee = "bo"
	if Unchanged(a, b) {
		t.Fatal("assignee change missed")
	}
	b = a
	b.Labels = []string{"x"} // not an editable field
	if !Unchanged(a, b) {
		t.Fatal("labels are not compared")
	}
}
