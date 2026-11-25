package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// backend is the I/O boundary of a Client: the process run and the two loader
// calls. Production uses defaultBackend; tests substitute a fake.
type backend struct {
	run        func(ctx context.Context, origin model.IssueOrigin, mutating bool, operation []string) (stdout []byte, diagnostic string, err error)
	revalidate func(sourcePath string, origin model.IssueOrigin) error
	exportBD   func(ctx context.Context, origin model.IssueOrigin) error
	// exportFresh reports whether bd's own auto-export rewrote the JSONL since the
	// given instant (see bdExportFresh). Nil means "unknown", which is false.
	exportFresh func(origin model.IssueOrigin, since time.Time) bool
}

var defaultBackend = backend{run: runTracker, revalidate: loader.RevalidateIssueOrigin, exportBD: loader.RefreshBDExport, exportFresh: bdExportFresh}

// Result is what a write reports besides its error. It is filled in even when
// the write failed.
type Result struct {
	// CreatedID is a follow-up issue captured by an op; kept when a later op fails.
	CreatedID string
	// Output is the tracker's diagnostic (ANSI-stripped stderr).
	Output string
	// RefreshErr is set when the write succeeded but refreshing the export failed.
	RefreshErr error
}

// FieldDiff holds the fields Update changes; nil leaves a field alone. Values
// go to the tracker verbatim as --<field>=<value>, exactly as the $EDITOR flow
// sends them.
type FieldDiff struct {
	Title, Status, Priority, Assignee, Type, Description *string
}

// Len is the number of fields the diff changes.
func (d FieldDiff) Len() int {
	n := 0
	for _, f := range []*string{d.Title, d.Status, d.Priority, d.Assignee, d.Type, d.Description} {
		if f != nil {
			n++
		}
	}
	return n
}

// flags renders the diff in the fixed order title, status, priority, assignee,
// type, description.
func (d FieldDiff) flags() []string {
	var flags []string
	for _, f := range []struct {
		name  string
		value *string
	}{
		{"title", d.Title}, {"status", d.Status}, {"priority", d.Priority},
		{"assignee", d.Assignee}, {"type", d.Type}, {"description", d.Description},
	} {
		if f.value != nil {
			flags = append(flags, "--"+f.name+"="+*f.value)
		}
	}
	return flags
}

// Client writes to the tracker of one issue. Methods are synchronous and apply
// no timeout of their own; the caller's context bounds them.
type Client struct {
	issue      model.Issue
	origin     model.IssueOrigin
	sourcePath string
	b          backend
	// lastOpStart is when the last tracker command of the last write began; see refreshAfterWrite.
	lastOpStart time.Time
}

// New binds a Client to issue and the tracker route in issue.Origin. sourcePath
// is the file the caller's view reads ("" when unknown); it decides whether a
// write is followed by an export refresh. It returns the OriginError when the
// issue cannot be written to.
func New(issue model.Issue, sourcePath string) (*Client, error) {
	if err := OriginError(issue.Origin); err != nil {
		return nil, err
	}
	issue = issue.Clone()
	return &Client{issue: issue, origin: *issue.Origin, sourcePath: sourcePath, b: defaultBackend}, nil
}

// Origin returns the tracker route the client writes to.
func (c *Client) Origin() model.IssueOrigin { return c.origin }

// write runs ops, then refreshes the export when the write succeeded and the
// view needs it.
func (c *Client) write(ctx context.Context, ops []op) (Result, error) {
	result, err := c.runOps(ctx, ops)
	if err == nil {
		c.refreshAfterWrite(ctx, &result)
	}
	return result, err
}

// refreshAfterWrite refreshes the export after a successful write, unless bd
// already did: bd exports after a mutating command when export.auto is on and
// its interval has passed, and a second `bd export` then costs a whole bd
// process (about 1.3 s here) for nothing. The skip needs proof, not a guess at
// bd's config (it layers files and BD_* variables, and has guards that skip an
// export silently): the state file bd writes after an export and the JSONL
// itself must both be newer than the start of the last command the write ran.
// The last command, not the first: with bd's interval throttle an earlier one
// can export and a later one not. Residual risk, accepted: another bd process
// exports during that command while ours is throttled, and the view lags until
// the next change (the same failure as a RefreshErr warning).
func (c *Client) refreshAfterWrite(ctx context.Context, result *Result) {
	if !needsRefresh(c.origin, c.sourcePath) {
		return
	}
	if c.origin.Tracker == "bd" && c.b.exportFresh != nil && !c.lastOpStart.IsZero() && c.b.exportFresh(c.origin, c.lastOpStart) {
		return
	}
	result.RefreshErr = c.RefreshExport(ctx)
}

// SetPriority runs `update --json --priority=<p>`.
func (c *Client) SetPriority(ctx context.Context, p int) (Result, error) {
	return c.write(ctx, []op{{args: []string{"update", "--json", "--priority=" + strconv.Itoa(p)}}})
}

// SetLabel adds or removes one label.
func (c *Client) SetLabel(ctx context.Context, label string, on bool) (Result, error) {
	flag := "--remove-label="
	if on {
		flag = "--add-label="
	}
	return c.write(ctx, []op{{args: []string{"update", "--json", flag + label}}})
}

// AddComment runs `comments add -- <id> <text>`.
func (c *Client) AddComment(ctx context.Context, text string) (Result, error) {
	return c.write(ctx, []op{commentOp(text)})
}

// AnswerQuestion posts text as a comment, then removes removeLabel.
func (c *Client) AnswerQuestion(ctx context.Context, text, removeLabel string) (Result, error) {
	return c.write(ctx, []op{
		commentOp(text),
		{args: []string{"update", "--json", "--remove-label=" + removeLabel}},
	})
}

func commentOp(text string) op {
	return op{args: []string{"comments", "add"}, trailing: []string{text}}
}

// Signoff approves or disputes a closed bead awaiting sign-off.
//
// On bd it runs `bd signoff --json --approve` or `--dispute=<note>`, which does
// the whole job in one command. A tracker without `signoff` (br, or an older
// bd) gets the equivalent sequence: approve removes needs-signoff and sets
// metadata signoff=approved; dispute files a follow-up bead (discovered-from the
// closed one) with the note as its description, then comments on the closed
// bead and marks it disputed. The closed bead is never reopened.
func (c *Client) Signoff(ctx context.Context, approve bool, note string) (Result, error) {
	return c.write(ctx, c.signoffOps(approve, note))
}

func (c *Client) signoffOps(approve bool, note string) []op {
	state := agentdesk.SignoffApproved
	if !approve {
		state = agentdesk.SignoffDisputed
	}
	mark := []string{"update", "--json", "--remove-label=" + agentdesk.LabelNeedsSignoff, "--set-metadata=" + agentdesk.SignoffMetaKey + "=" + state}
	var ops []op
	if !approve {
		ops = append(ops,
			op{noTarget: true, captureID: true, args: []string{"create", "--json",
				"--title=" + agentdesk.DisputeTitle(c.issue.Title), "--description=" + note,
				"--deps=discovered-from:" + c.origin.LocalID, "--priority=" + strconv.Itoa(c.issue.Priority)}},
			op{args: []string{"comments", "add"}, trailing: []string{"Disputed: " + note + " (follow-up {new})"}},
		)
		mark = append(mark, "--set-metadata=signoff_note="+note, "--set-metadata=signoff_followup={new}")
	}
	ops = append(ops, op{args: mark})
	if c.origin.Tracker == "bd" {
		native := op{args: []string{"signoff", "--json", "--approve"}, fallback: ops}
		if !approve {
			native = op{args: []string{"signoff", "--json", "--dispute=" + note}, captureID: true, fallback: ops}
		}
		ops = []op{native}
	}
	return ops
}

// Update saves diff to the client's issue after an optimistic conflict check:
// it re-reads the issue through the tracker and returns ErrConflict when it no
// longer matches the issue the client was built from. This is not an atomic
// compare-and-swap.
func (c *Client) Update(ctx context.Context, diff FieldDiff) (Result, error) {
	var result Result
	if err := OriginError(&c.origin); err != nil {
		return result, err
	}
	if diff.Len() == 0 {
		return result, fmt.Errorf("no fields to update")
	}
	if err := c.b.revalidate(originSource(c.origin, c.sourcePath), c.origin); err != nil {
		return result, err
	}
	data, diagnostic, err := c.b.run(ctx, c.origin, false, []string{"show", "--json", "--", c.origin.LocalID})
	if err != nil {
		result.Output = diagnostic
		return result, fmt.Errorf("verify current tracker issue: %w", err)
	}
	current, err := readLiveIssue(data, c.origin.LocalID)
	if err != nil {
		return result, err
	}
	if !Unchanged(c.issue, current) {
		return result, ErrConflict
	}
	args := append([]string{"update", "--json"}, diff.flags()...)
	args = append(args, "--", c.origin.LocalID)
	c.lastOpStart = time.Now()
	_, result.Output, err = c.b.run(ctx, c.origin, true, args)
	if err != nil {
		return result, err
	}
	c.refreshAfterWrite(ctx, &result)
	return result, nil
}

// EditableFields lists the fields the $EDITOR flow and Update can change, as
// the strings Update sends.
func EditableFields(issue model.Issue) map[string]string {
	return map[string]string{
		"title": issue.Title, "status": string(issue.Status), "priority": strconv.Itoa(issue.Priority),
		"assignee": issue.Assignee, "type": string(issue.IssueType), "description": issue.Description,
	}
}

// Unchanged reports whether current still has the editable fields and the
// update time of original.
func Unchanged(original, current model.Issue) bool {
	return maps.Equal(EditableFields(original), EditableFields(current)) && original.UpdatedAt.Equal(current.UpdatedAt)
}

// readLiveIssue decodes `show --json` output (one issue, or a one-element
// array) and checks it is the issue localID.
func readLiveIssue(data []byte, localID string) (model.Issue, error) {
	data = bytes.TrimSpace(data)
	var issue model.Issue
	if len(data) == 0 {
		return issue, fmt.Errorf("tracker returned no issue")
	}
	if data[0] == '[' {
		var issues []model.Issue
		if err := json.Unmarshal(data, &issues); err != nil {
			return issue, fmt.Errorf("decode tracker issue: %w", err)
		}
		if len(issues) != 1 {
			return issue, fmt.Errorf("tracker returned %d issues, expected one", len(issues))
		}
		issue = issues[0]
	} else if err := json.Unmarshal(data, &issue); err != nil {
		return issue, fmt.Errorf("decode tracker issue: %w", err)
	}
	if issue.ID != localID {
		return issue, fmt.Errorf("tracker returned a different issue")
	}
	return issue, nil
}
