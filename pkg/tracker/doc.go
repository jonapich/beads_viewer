// Package tracker is bv's write path to a live bd/br tracker.
//
// A Client is bound to one issue (and so to that issue's tracker route, the
// model.IssueOrigin the loader attached to it). Its methods mirror the actions
// the TUI offers: priority and label edits, sign-off with the br/old-bd
// fallback, answering a question, and the verify-then-update save of the $EDITOR
// flow. They run the tracker executable synchronously under the caller's
// context and return a Result plus an error.
//
// The package is UI-free: it must not import bubbletea, bubbles, lipgloss or
// pkg/ui, directly or through another in-module package (enforced by
// imports_test.go). pkg/ui wraps these calls in tea.Cmd, and pkg/api calls them
// directly.
//
// Failures of the tracker process are *CommandError, which carry the tracker's
// stderr; Error() deliberately leaves the stderr out so callers can print it
// next to the error without repeating it.
package tracker
