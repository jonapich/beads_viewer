package tracker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

const (
	// DiagnosticLimit caps the stderr (and the stdout of mutating calls) kept
	// from a tracker process.
	DiagnosticLimit = 16 << 10
	// ShowOutputLimit caps the stdout kept from a read such as `show --json`.
	ShowOutputLimit = 8 << 20
)

// outputBuffer keeps at most limit bytes and notes that it dropped the rest.
type outputBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (o *outputBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := o.limit - o.buffer.Len(); len(p) > remaining {
		p = p[:remaining]
		o.truncated = true
	}
	_, _ = o.buffer.Write(p)
	return n, nil
}

// OriginError reports why origin cannot be written to, or nil when it carries a
// complete, absolute tracker route.
func OriginError(origin *model.IssueOrigin) error {
	if origin == nil {
		return fmt.Errorf("source has no verified live tracker; historical and imported views are read-only")
	}
	if origin.ReadOnlyReason != "" {
		return fmt.Errorf("source is read-only: %s", origin.ReadOnlyReason)
	}
	if (origin.Tracker != "br" && origin.Tracker != "bd") || origin.LocalID == "" ||
		!filepath.IsAbs(origin.Executable) || !filepath.IsAbs(origin.WorkingDirectory) ||
		!filepath.IsAbs(origin.TrackerDirectory) || !filepath.IsAbs(origin.Database) {
		return fmt.Errorf("source has no complete, absolute tracker route")
	}
	for _, value := range []string{origin.LocalID, origin.Executable, origin.WorkingDirectory, origin.TrackerDirectory, origin.Database} {
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("tracker route contains an invalid NUL byte")
		}
	}
	return nil
}

// environment returns parent without the tracker routing variables, plus the
// route of origin.
func environment(parent []string, origin model.IssueOrigin) []string {
	result := make([]string, 0, len(parent)+3)
	for _, entry := range parent {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "BEADS_DIR") || strings.EqualFold(key, "BEADS_DB") || strings.EqualFold(key, "BD_DB") || strings.EqualFold(key, "BEADS_JSONL") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "BEADS_DIR="+origin.TrackerDirectory, "BEADS_DB="+origin.Database, "BD_DB="+origin.Database)
}

// commandArgs builds the full argv (after the executable): the route flags,
// then operation. br additionally never auto-imports, and never auto-flushes on
// a read.
func commandArgs(origin model.IssueOrigin, mutating bool, operation []string) []string {
	args := []string{"--db", origin.Database}
	if origin.Tracker == "br" {
		args = append(args, "--no-auto-import")
		if !mutating {
			args = append(args, "--no-auto-flush")
		}
	}
	return append(args, operation...)
}

// runTracker runs the origin's tracker executable with operation, in the
// origin's working directory and routing environment. It returns stdout, the
// ANSI-stripped stderr, and a *CommandError when the process failed. A route
// that fails OriginError is a plain error and nothing runs.
func runTracker(ctx context.Context, origin model.IssueOrigin, mutating bool, operation []string) ([]byte, string, error) {
	if err := OriginError(&origin); err != nil {
		return nil, "", err
	}
	cmd := exec.CommandContext(ctx, origin.Executable, commandArgs(origin, mutating, operation)...)
	cmd.Dir = origin.WorkingDirectory
	cmd.Env = environment(os.Environ(), origin)
	cmd.WaitDelay = time.Second
	limit := ShowOutputLimit
	if mutating {
		limit = DiagnosticLimit
	}
	stdout := &outputBuffer{limit: limit}
	stderr := &outputBuffer{limit: DiagnosticLimit}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = errors.Join(ctx.Err(), err)
	}
	diagnostic := strings.TrimSpace(ansi.Strip(stderr.buffer.String()))
	if stderr.truncated {
		diagnostic += " [diagnostics truncated]"
	}
	if !mutating && stdout.truncated {
		err = errors.Join(err, fmt.Errorf("tracker issue response exceeds %d bytes", ShowOutputLimit))
	}
	if err != nil {
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		err = &CommandError{Args: slices.Clone(operation), ExitCode: exitCode, Stderr: diagnostic, Err: err}
	}
	return stdout.buffer.Bytes(), diagnostic, err
}
