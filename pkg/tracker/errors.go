package tracker

import (
	"errors"
	"strings"
)

// ErrConflict reports that the issue changed in the tracker after the caller's
// base copy was read. The wording is the TUI's; other callers map it with
// errors.Is and use their own message.
var ErrConflict = errors.New("issue changed in the tracker while editing; reopen it and reconcile the saved draft")

// CommandError is a tracker process that failed, or could not finish.
type CommandError struct {
	// Args is the operation argv after the route flags (no --db path), for
	// example ["signoff", "--json", "--approve", "--", "x-1"].
	Args []string
	// ExitCode is the process exit code; -1 when it did not start, was killed
	// or was cancelled.
	ExitCode int
	// Stderr is the tracker's ANSI-stripped, trimmed diagnostic, possibly
	// ending in " [diagnostics truncated]".
	Stderr string
	// Err is the underlying error. It joins the context error when the context
	// ended, so errors.Is(err, context.DeadlineExceeded) works.
	Err error
}

// Error returns the underlying error text without Stderr, because callers
// compose "<summary> failed: <error> — <stderr>" themselves.
func (e *CommandError) Error() string {
	if e.Err == nil {
		return "tracker command failed"
	}
	return e.Err.Error()
}

func (e *CommandError) Unwrap() error { return e.Err }

// UnknownCommand reports whether the tracker did not know the command, as an
// older build answers a newer subcommand.
func (e *CommandError) UnknownCommand() bool {
	return strings.Contains(e.Stderr, "unknown command")
}
