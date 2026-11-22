package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// op is one tracker invocation: `<tracker> <args...> -- <id> <trailing...>`.
// With noTarget the `-- <id>` part is left out. With captureID the issue ID in
// the op's JSON output replaces "{new}" in the args of later ops.
type op struct {
	args      []string
	trailing  []string
	noTarget  bool
	captureID bool
	// fallback runs instead when the tracker does not know the op's command
	// (an older build answering "unknown command").
	fallback []op
}

// createdIssueID reads the issue ID from a tracker's `create --json` output,
// which is an object or a one-element array, or the follow-up ID from
// `signoff --json --dispute` output.
func createdIssueID(out []byte) (string, error) {
	out = bytes.TrimSpace(out)
	var followup struct {
		Followup string `json:"followup"`
	}
	if json.Unmarshal(out, &followup) == nil && followup.Followup != "" {
		return followup.Followup, nil
	}
	var one struct {
		ID string `json:"id"`
	}
	if len(out) > 0 && out[0] == '[' {
		var many []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out, &many); err != nil || len(many) == 0 {
			return "", fmt.Errorf("tracker create returned no issue")
		}
		one.ID = many[0].ID
	} else if err := json.Unmarshal(out, &one); err != nil {
		return "", fmt.Errorf("tracker create output is not JSON: %w", err)
	}
	if one.ID == "" {
		return "", fmt.Errorf("tracker create returned no issue id")
	}
	return one.ID, nil
}

func substituteNewID(values []string, id string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ReplaceAll(v, "{new}", id)
	}
	return out
}

// runOps runs each op against the client's tracker in order, stopping at the
// first failure. Result.Output is the diagnostic of the last op that ran and
// Result.CreatedID is set as soon as an ID is captured, so both survive a later
// failure.
func (c *Client) runOps(ctx context.Context, ops []op) (Result, error) {
	var result Result
	newID := ""
	var run func([]op) error
	run = func(ops []op) error {
		for _, o := range ops {
			args := substituteNewID(o.args, newID)
			if !o.noTarget {
				args = append(append(args, "--", c.origin.LocalID), substituteNewID(o.trailing, newID)...)
			}
			out, diagnostic, err := c.b.run(ctx, c.origin, true, args)
			result.Output = diagnostic
			var commandErr *CommandError
			if err != nil && o.fallback != nil && errors.As(err, &commandErr) && commandErr.UnknownCommand() {
				if err := run(o.fallback); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if o.captureID {
				if newID, err = createdIssueID(out); err != nil {
					return err
				}
				result.CreatedID = newID
			}
		}
		return nil
	}
	err := run(ops)
	return result, err
}
