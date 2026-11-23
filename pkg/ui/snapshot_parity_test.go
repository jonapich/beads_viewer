package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// The TUI and the HTTP API must see the same issues, counts, triage and tree.
// These tests build the same input through ui.SnapshotBuilder and through
// snapshot.Builder directly and require identical core data.

func parityIssues(now time.Time) []model.Issue {
	mk := func(id string, typ model.IssueType, prio int, status model.Status, deps ...*model.Dependency) model.Issue {
		created := now.Add(-time.Duration(len(id)+prio) * 24 * time.Hour)
		return model.Issue{
			ID: id, Title: "Issue " + id, IssueType: typ, Priority: prio, Status: status,
			CreatedAt: created, UpdatedAt: created, Dependencies: deps,
		}
	}
	child := func(id, parent string) *model.Dependency {
		return &model.Dependency{IssueID: id, DependsOnID: parent, Type: model.DepParentChild}
	}
	blocks := func(id, on string) *model.Dependency {
		return &model.Dependency{IssueID: id, DependsOnID: on, Type: model.DepBlocks}
	}
	return []model.Issue{
		mk("par-e1", model.TypeEpic, 1, model.StatusOpen),
		mk("par-a", model.TypeTask, 2, model.StatusOpen, child("par-a", "par-e1")),
		mk("par-b", model.TypeFeature, 1, model.StatusBlocked, child("par-b", "par-e1"), blocks("par-b", "par-a")),
		mk("par-c", model.TypeBug, 3, model.StatusClosed, child("par-c", "par-e1")),
		mk("par-e2", model.TypeEpic, 0, model.StatusInProgress),
		mk("par-d", model.TypeTask, 2, model.StatusOpen, child("par-d", "par-e2"), blocks("par-d", "par-b")),
		mk("par-solo", model.TypeChore, 4, model.StatusOpen),
	}
}

func treeShape(roots []*snapshot.TreeNode) string {
	var sb strings.Builder
	var walk func(nodes []*snapshot.TreeNode)
	walk = func(nodes []*snapshot.TreeNode) {
		for _, n := range nodes {
			fmt.Fprintf(&sb, "%s@%d e=%v s=%v [", n.Issue.ID, n.Depth, n.Expanded, n.SharedRef)
			walk(n.Children)
			sb.WriteString("] ")
		}
	}
	walk(roots)
	return sb.String()
}

func issueIDs(issues []model.Issue) []string {
	ids := make([]string, len(issues))
	for i := range issues {
		ids[i] = issues[i].ID
	}
	return ids
}

func requireCoreParity(t *testing.T, label string, got *DataSnapshot, want *snapshot.Snapshot) {
	t.Helper()
	if !reflect.DeepEqual(issueIDs(got.Issues), issueIDs(want.Issues)) {
		t.Fatalf("%s: issue order %v != %v", label, issueIDs(got.Issues), issueIDs(want.Issues))
	}
	if !reflect.DeepEqual(issueIDs(got.ViewIssues), issueIDs(want.ViewIssues)) {
		t.Fatalf("%s: view issue order differs", label)
	}
	if got.CountOpen != want.CountOpen || got.CountReady != want.CountReady ||
		got.CountBlocked != want.CountBlocked || got.CountClosed != want.CountClosed {
		t.Fatalf("%s: counts ui=%d/%d/%d/%d core=%d/%d/%d/%d", label,
			got.CountOpen, got.CountReady, got.CountBlocked, got.CountClosed,
			want.CountOpen, want.CountReady, want.CountBlocked, want.CountClosed)
	}
	if !reflect.DeepEqual(got.TriageScores, want.TriageScores) ||
		!reflect.DeepEqual(got.TriageReasons, want.TriageReasons) ||
		!reflect.DeepEqual(got.QuickWinSet, want.QuickWinSet) ||
		!reflect.DeepEqual(got.BlockerSet, want.BlockerSet) ||
		!reflect.DeepEqual(got.UnblocksMap, want.UnblocksMap) {
		t.Fatalf("%s: triage maps differ between ui and core", label)
	}
	if treeShape(got.TreeRoots) != treeShape(want.TreeRoots) {
		t.Fatalf("%s: tree shape\n ui:   %s\n core: %s", label, treeShape(got.TreeRoots), treeShape(want.TreeRoots))
	}
	if len(got.TreeNodeMap) != len(want.TreeNodeMap) {
		t.Fatalf("%s: tree node maps differ in size", label)
	}
	if got.IsPhase2Ready() != want.Phase2Ready {
		t.Fatalf("%s: phase2 readiness ui=%v core=%v", label, got.IsPhase2Ready(), want.Phase2Ready)
	}
}

func TestSnapshotBuilderParityWithCore(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cfg := snapshotBuildConfigDefault()
	cfg.SkipPhase2 = true // deterministic: no background Phase 2 goroutines

	uiBuilder := NewSnapshotBuilder(cloneIssuesForAsync(parityIssues(now))).WithBuildConfig(cfg)
	uiBuilder.analyzer.SetNow(now)
	coreBuilder := snapshot.NewBuilder(snapshot.CloneIssues(parityIssues(now)), nil).WithConfig(cfg.core())
	coreBuilder.Analyzer().SetNow(now)

	uiSnap := uiBuilder.Build()
	coreSnap := coreBuilder.Build()
	requireCoreParity(t, "build", uiSnap, coreSnap)
	if len(uiSnap.TriageScores) == 0 || len(uiSnap.TreeRoots) == 0 {
		t.Fatal("fixture must exercise triage and the tree")
	}
	if len(uiSnap.ListItems) != len(coreSnap.ViewIssues) {
		t.Fatalf("list items=%d, core view issues=%d", len(uiSnap.ListItems), len(coreSnap.ViewIssues))
	}

	phase2 := func(issues []model.Issue) (*analysis.Analyzer, *analysis.GraphStats, analysis.Insights) {
		az := analysis.NewAnalyzer(issues)
		az.SetNow(now)
		stats := az.AnalyzeAsync(nil)
		stats.WaitForPhase2()
		return az, stats, stats.GenerateInsights(stats.NodeCount)
	}
	uiIssues := cloneIssuesForAsync(uiSnap.Issues)
	az, stats, insights := phase2(uiIssues)
	uiP2 := uiSnap.WithPhase2(stats, insights, uiIssues, az)

	coreIssues := snapshot.CloneIssues(coreSnap.Issues)
	az, stats, insights = phase2(coreIssues)
	coreP2 := coreSnap.WithPhase2(stats, insights, coreIssues, az)

	requireCoreParity(t, "phase2", uiP2, coreP2)
	if !uiP2.IsPhase2Ready() || uiP2.phase2Triage == nil {
		t.Fatalf("phase 2 snapshot must be ready with triage: ready=%v", uiP2.IsPhase2Ready())
	}
}

func TestSnapshotBuildConfigForTierMatchesCore(t *testing.T) {
	for _, tier := range []datasetTier{datasetTierUnknown, datasetTierSmall, datasetTierMedium, datasetTierLarge, datasetTierHuge} {
		if got, want := snapshotBuildConfigForTier(tier).core(), snapshot.ConfigForTier(tier); got != want {
			t.Errorf("tier %v: ui core config %+v != snapshot.ConfigForTier %+v", tier, got, want)
		}
		ui := snapshotBuildConfigForTier(tier)
		wantUI := tier != datasetTierHuge
		if ui.PrecomputeBoard != wantUI || ui.PrecomputeGraphLayout != wantUI {
			t.Errorf("tier %v: board/graph layout = %v/%v, want %v", tier, ui.PrecomputeBoard, ui.PrecomputeGraphLayout, wantUI)
		}
	}
}

func TestDataSnapshotCoreAdaptersRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cfg := snapshotBuildConfigDefault()
	cfg.SkipPhase2 = true
	builder := NewSnapshotBuilder(parityIssues(now)).WithBuildConfig(cfg).WithProjectDir("proj")
	builder.analyzer.SetNow(now)
	ds := builder.Build()

	core := ds.coreView()
	if !core.ReferenceTime.Equal(now) || core.ProjectDir != "proj" || core.ExplicitProjectDir != "proj" {
		t.Fatalf("coreView lost metadata: ref=%v dir=%q explicit=%q", core.ReferenceTime, core.ProjectDir, core.ExplicitProjectDir)
	}
	back := dataSnapshotFromCore(core)
	if !reflect.DeepEqual(issueIDs(back.Issues), issueIDs(ds.Issues)) || back.CountReady != ds.CountReady ||
		back.projectDir != ds.projectDir || back.explicitProjectDir != ds.explicitProjectDir ||
		back.phase2Ready != ds.phase2Ready || back.RecipeHash != ds.RecipeHash ||
		!reflect.DeepEqual(back.TriageScores, ds.TriageScores) {
		t.Fatal("core adapters must round-trip the shared fields")
	}
	if back.phase2Triage != nil {
		t.Fatal("dataSnapshotFromCore must leave phase2Triage unset")
	}
}

// fillSnapshotField sets v to a non-zero value derived from name and index.
// Pointers get a fresh zero element; maps and slices get one element.
func fillSnapshotField(v reflect.Value, name string, i int) {
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	timeType := reflect.TypeOf(time.Time{})
	switch {
	case v.Type() == timeType:
		v.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	case v.Type() == errorType:
		v.Set(reflect.ValueOf(errors.New(name)))
	case v.Type() == reflect.TypeOf(snapshot.TierUnknown):
		v.Set(reflect.ValueOf(snapshot.TierLarge))
	}
	if !v.IsZero() {
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(i + 1))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(i) + 1.5)
	case reflect.String:
		v.SetString(name)
	case reflect.Ptr:
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillSnapshotField(s.Index(0), name, i)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		fillSnapshotField(key, name, i)
		elem := reflect.New(v.Type().Elem()).Elem()
		fillSnapshotField(elem, name, i)
		m.SetMapIndex(key, elem)
		v.Set(m)
	case reflect.Struct:
		for f := 0; f < v.NumField(); f++ {
			if v.Field(f).CanSet() {
				fillSnapshotField(v.Field(f), name+"."+v.Type().Field(f).Name, i)
			}
		}
	}
}

// TestDataSnapshotCoreAdaptersCoverAllFields fails when a field is added to
// snapshot.Snapshot but not wired through dataSnapshotFromCore and coreView:
// the TUI's Build and Phase 2 paths would silently drop it.
func TestDataSnapshotCoreAdaptersCoverAllFields(t *testing.T) {
	c := &snapshot.Snapshot{}
	cv := reflect.ValueOf(c).Elem()
	for i := 0; i < cv.NumField(); i++ {
		if !cv.Field(i).CanSet() {
			t.Fatalf("snapshot.Snapshot.%s is unexported; the adapters cannot carry it", cv.Type().Field(i).Name)
		}
		fillSnapshotField(cv.Field(i), cv.Type().Field(i).Name, i)
		if cv.Field(i).IsZero() {
			t.Fatalf("test helper could not fill snapshot.Snapshot.%s (%s); extend fillSnapshotField", cv.Type().Field(i).Name, cv.Field(i).Type())
		}
	}

	// Fields that deliberately do not round-trip.
	skip := map[string]string{
		"ViewRebuilt": "WithPhase2 output only",
		"Triage":      "sourced from phase2Triage, which dataSnapshotFromCore leaves unset",
	}
	got := reflect.ValueOf(dataSnapshotFromCore(c).coreView()).Elem()
	for i := 0; i < cv.NumField(); i++ {
		name := cv.Type().Field(i).Name
		if _, ok := skip[name]; ok {
			continue
		}
		if !reflect.DeepEqual(cv.Field(i).Interface(), got.Field(i).Interface()) {
			t.Errorf("snapshot.Snapshot.%s is not carried through dataSnapshotFromCore/coreView", name)
		}
	}
}
