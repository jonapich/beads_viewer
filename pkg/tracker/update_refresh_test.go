package tracker

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// TestUpdateRefreshes covers Update followed by the export refresh (the editor
// tests in pkg/ui cover it end to end): a bd tracker always re-exports after a
// successful update, a refresh failure is reported without failing the update,
// and a failed update refreshes nothing.
func TestUpdateRefreshes(t *testing.T) {
	refreshErr := errors.New("export failed")
	newClient := func(fb *fakeBackend) *Client {
		base := testIssue(t, "bd")
		fb.script = func(args []string) ([]byte, string, error) {
			if args[0] == "show" {
				return showJSON(t, base, "local-1"), "", nil
			}
			return nil, "update ok", nil
		}
		c := newTestClient(t, "bd", "x.db", fb)
		c.issue = base
		return c
	}
	diff := FieldDiff{Title: strp("Snare")}

	t.Run("export refreshed after the update", func(t *testing.T) {
		fb := &fakeBackend{}
		res, err := newClient(fb).Update(context.Background(), diff)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"revalidate", "run", "run", "revalidate", "exportBD"}; !slices.Equal(fb.kinds(), want) {
			t.Fatalf("calls %q, want %q", fb.kinds(), want)
		}
		if src := fb.calls[3].args[0]; src != "x.db" {
			t.Fatalf("refresh revalidated source %q, want x.db", src)
		}
		if res.Output != "update ok" || res.RefreshErr != nil {
			t.Fatalf("result: %+v", res)
		}
	})

	t.Run("refresh failure does not fail the update", func(t *testing.T) {
		fb := &fakeBackend{exportErr: refreshErr}
		res, err := newClient(fb).Update(context.Background(), diff)
		if err != nil {
			t.Fatalf("a refresh failure must not fail the update: %v", err)
		}
		if !errors.Is(res.RefreshErr, refreshErr) || res.Output != "update ok" {
			t.Fatalf("result: %+v", res)
		}
	})

	t.Run("no refresh after a failed update", func(t *testing.T) {
		fb := &fakeBackend{}
		c := newClient(fb)
		inner := fb.script
		fb.script = func(args []string) ([]byte, string, error) {
			if args[0] == "update" {
				return nil, "nope", &CommandError{Args: args, Stderr: "nope", Err: errors.New("exit status 1")}
			}
			return inner(args)
		}
		if _, err := c.Update(context.Background(), diff); err == nil {
			t.Fatal("update failure swallowed")
		}
		if want := []string{"revalidate", "run", "run"}; !slices.Equal(fb.kinds(), want) {
			t.Fatalf("calls %q, want %q", fb.kinds(), want)
		}
	})
}
