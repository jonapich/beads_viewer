package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
)

// goldenDir holds the fixtures shared with the web client: Go writes them
// (GENERATE_GOLDEN=1) and asserts them, fixtures.typecheck.ts checks the
// TypeScript types against them, so drift fails a test on either side.
const goldenDir = "../../web/src/lib/api/__fixtures__"

// goldenNow is the fixed reference instant of every golden fixture.
var goldenNow = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func ts(day, hour int) time.Time { return time.Date(2026, 3, day, hour, 0, 0, 0, time.UTC) }

func tsp(day, hour int) *time.Time { t := ts(day, hour); return &t }

func dep(from, to string, typ model.DependencyType) *model.Dependency {
	return &model.Dependency{IssueID: from, DependsOnID: to, Type: typ, CreatedAt: ts(1, 9)}
}

// goldenIssues is the fixture tracker (about a dozen issues) the endpoint
// fixtures are generated from. Later slices of beads-c2o.5 reuse it.
func goldenIssues() []model.Issue {
	issue := func(id, title string, status model.Status, prio int, typ model.IssueType) model.Issue {
		return model.Issue{
			ID: id, Title: title, Description: title + " description.", Status: status, Priority: prio, IssueType: typ,
			CreatedAt: ts(1, 10), UpdatedAt: ts(2, 10),
		}
	}
	epic := issue("fx-epic", "Checkout epic", model.StatusOpen, 1, model.TypeEpic)
	epic.Labels = []string{"checkout"}

	child := func(id, title string, status model.Status, prio int) model.Issue {
		i := issue(id, title, status, prio, model.TypeTask)
		i.Dependencies = []*model.Dependency{dep(id, "fx-epic", model.DepParentChild)}
		return i
	}
	a := child("fx-a", "Cart model", model.StatusInProgress, 1)
	a.Assignee = "ada"
	// fx-b is blocked by two open blockers, one of them in another epic: the
	// list tree shows it under both.
	b := child("fx-b", "Payment form", model.StatusOpen, 2)
	b.Dependencies = append(b.Dependencies, dep("fx-b", "fx-a", model.DepBlocks), dep("fx-b", "fx-x", model.DepBlocks),
		dep("fx-b", "fx-done", model.DepBlocks)) // fx-done is closed: a resolved blocker
	c := child("fx-c", "Receipt email", model.StatusOpen, 3)

	otherEpic := issue("fx-epic2", "Platform epic", model.StatusOpen, 2, model.TypeEpic)
	x := issue("fx-x", "Upgrade gateway SDK", model.StatusOpen, 1, model.TypeTask)
	x.Dependencies = []*model.Dependency{dep("fx-x", "fx-epic2", model.DepParentChild)}

	// A blocking 2-cycle: neither can be placed under the other, so tree
	// building has to promote a root.
	cy1 := issue("fx-cy1", "Cycle one", model.StatusOpen, 3, model.TypeTask)
	cy1.Dependencies = []*model.Dependency{dep("fx-cy1", "fx-cy2", model.DepBlocks)}
	cy2 := issue("fx-cy2", "Cycle two", model.StatusOpen, 3, model.TypeTask)
	cy2.Dependencies = []*model.Dependency{dep("fx-cy2", "fx-cy1", model.DepBlocks)}

	// Closed and awaiting sign-off.
	signoff := issue("fx-done", "Ship the cart", model.StatusClosed, 2, model.TypeFeature)
	signoff.ClosedAt = tsp(3, 9)
	signoff.CloseReason = "done"
	signoff.Labels = []string{"needs-signoff"}
	signoff.Comments = []*model.Comment{{
		ID: "c1", IssueID: "fx-done", Author: "claude", CreatedAt: ts(3, 9),
		Text: "bd-kind: signoff\n" + `{"what_changed":"the cart ships","calls_made":[{"call":"kept the guest checkout","alternative":"force accounts","why":"fewer drop-offs"}],"commits":["abc1234"]}`,
	}}

	decision := issue("fx-decide", "Pick a payment provider", model.StatusOpen, 2, model.TypeTask)
	decision.Labels = []string{"needs-decision"}
	decision.Description = "Question: Which provider should we use?\n\nOptions:\n(a, recommended) Stripe: lowest effort\n(b) Adyen: better EU rates"
	decision.Comments = []*model.Comment{
		{ID: "c2", IssueID: "fx-decide", Author: "jon", Text: "bd-kind: brief\nGOAL: choose a provider.", CreatedAt: ts(2, 11)},
		{ID: "c3", IssueID: "fx-decide", Author: "claude", Text: "bd-kind: advice\nGo with Stripe.", CreatedAt: ts(2, 12)},
	}

	deferred := issue("fx-later", "Gift cards", model.StatusOpen, 4, model.TypeFeature)
	deferred.DeferUntil = tsp(30, 9)
	// A dangling related link: the target is not in the tracker.
	deferred.Dependencies = []*model.Dependency{dep("fx-later", "fx-gone", model.DepRelated)}

	related := issue("fx-found", "Rounding bug found in cart", model.StatusOpen, 2, model.TypeBug)
	related.Dependencies = []*model.Dependency{dep("fx-found", "fx-a", model.DepDiscoveredFrom)}

	return []model.Issue{epic, a, b, c, otherEpic, x, cy1, cy2, signoff, decision, deferred, related}
}

// writeGoldenTracker writes goldenIssues to a temp .beads/beads.jsonl.
func writeGoldenTracker(t *testing.T) string {
	t.Helper()
	dir := testutil.TempBeadsDir(t)
	return testutil.WriteBeadsFile(t, dir, goldenIssues())
}

// goldenStore loads the fixture tracker with the fixed clock.
func goldenStore(t *testing.T, path string) *Store {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	store, err := NewStore(ctx, path, snapshot.LoadOptions{Now: goldenNow})
	if err != nil {
		cancel()
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close(); cancel() })
	return store
}

func waitPhase2(t *testing.T, s *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.waitPhase2(ctx); err != nil {
		t.Fatalf("phase 2 did not finish: %v", err)
	}
}

// getGolden requests path through the full handler and returns the decoded
// JSON (to compare against a golden file) plus the recorded response.
func getGolden(t *testing.T, store *Store, target string) (any, int) {
	t.Helper()
	h := newTestHandler(t, func(c *Config) { c.Store = store })
	rec := do(h, http.MethodGet, target, withSession())
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("GET %s: not JSON: %v: %q", target, err, rec.Body.String())
	}
	return v, rec.Code
}

// scrubSource replaces the machine-specific paths and the load time (the wall
// clock of the load) so fixtures are portable and stable.
func scrubSource(t *testing.T, v any) {
	t.Helper()
	if m, ok := v.(map[string]any); ok {
		m["loaded_at"] = goldenNow.Format(time.RFC3339)
	}
	src, ok := v.(map[string]any)["source"].(map[string]any)
	if !ok {
		t.Fatalf("meta has no source object: %v", v)
	}
	src["path"] = "SOURCE"
	src["project_dir"] = "PROJECT"
}

func TestGoldenMeta(t *testing.T) {
	path := writeGoldenTracker(t)

	// Phase 1 is captured while Phase 2 is held back, so the fixture does not
	// depend on timing.
	gate := make(chan struct{})
	gated := newGatedStore(t, path, gate)
	phase1, code := getGolden(t, gated, "/api/v1/meta")
	if code != http.StatusOK {
		t.Fatalf("meta phase 1: %d", code)
	}
	scrubSource(t, phase1)
	testutil.NewGoldenFile(t, goldenDir, "meta-phase1.json").AssertJSON(phase1)
	close(gate)

	store := goldenStore(t, path)
	waitPhase2(t, store)
	phase2, code := getGolden(t, store, "/api/v1/meta")
	if code != http.StatusOK {
		t.Fatalf("meta phase 2: %d", code)
	}
	scrubSource(t, phase2)
	testutil.NewGoldenFile(t, goldenDir, "meta.json").AssertJSON(phase2)
}

// TestGoldenSnapshotEvent pins the data of the events "snapshot" message.
func TestGoldenSnapshotEvent(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)
	data, err := json.Marshal(store.snapshotEvent())
	if err != nil {
		t.Fatal(err)
	}
	var ev any
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatal(err)
	}
	testutil.NewGoldenFile(t, goldenDir, "event-snapshot.json").AssertJSON(ev)
}

// newGatedStore is a store whose Phase 2 blocks until gate closes.
func newGatedStore(t *testing.T, path string, gate <-chan struct{}) *Store {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	complete := func(ctx context.Context, s *snapshot.Snapshot) (*snapshot.Snapshot, error) {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return s.CompletePhase2(ctx)
	}
	store, err := newStore(ctx, path, snapshot.LoadOptions{Now: goldenNow}, snapshot.Load, complete)
	if err != nil {
		cancel()
		t.Fatalf("newStore: %v", err)
	}
	t.Cleanup(func() { store.Close(); cancel() })
	return store
}

func TestGoldenIssuesTreeAndSearch(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)

	issues, code := getGolden(t, store, "/api/v1/issues?view=tree")
	if code != http.StatusOK {
		t.Fatalf("issues: %d", code)
	}
	testutil.NewGoldenFile(t, goldenDir, "issues-tree.json").AssertJSON(issues)

	// The flat view has no "tree": the TS type keeps it optional, and this
	// fixture is what proves that.
	flat, code := getGolden(t, store, "/api/v1/issues?view=flat")
	if code != http.StatusOK {
		t.Fatalf("issues flat: %d", code)
	}
	testutil.NewGoldenFile(t, goldenDir, "issues-flat.json").AssertJSON(flat)

	hits, code := getGolden(t, store, "/api/v1/search?q=cart")
	if code != http.StatusOK {
		t.Fatalf("search: %d", code)
	}
	testutil.NewGoldenFile(t, goldenDir, "search.json").AssertJSON(hits)
}

func TestGoldenIssueDetailAndSignoff(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)

	// The blocked child (parent, active and resolved blockers, triage, graph),
	// the decision bead (kinded comments, options), and the signoff bead.
	for target, name := range map[string]string{
		"/api/v1/issues/fx-b":      "issue-detail.json",
		"/api/v1/issues/fx-decide": "issue-detail-decision.json",
		"/api/v1/issues/fx-done":   "issue-detail-signoff.json",
		"/api/v1/signoff":          "signoff.json",
	} {
		got, code := getGolden(t, store, target)
		if code != http.StatusOK {
			t.Fatalf("%s: %d", target, code)
		}
		testutil.NewGoldenFile(t, goldenDir, name).AssertJSON(got)
	}

	missing, code := getGolden(t, store, "/api/v1/issues/no-such-issue")
	if code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", code)
	}
	testutil.NewGoldenFile(t, goldenDir, "error-not-found.json").AssertJSON(missing)
}

func TestGoldenTriageAndInsights(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)
	for target, name := range map[string]string{
		"/api/v1/triage":           "triage.json",
		"/api/v1/insights?limit=3": "insights.json",
	} {
		got, code := getGolden(t, store, target)
		if code != http.StatusOK {
			t.Fatalf("%s: %d", target, code)
		}
		testutil.NewGoldenFile(t, goldenDir, name).AssertJSON(got)
	}
}
