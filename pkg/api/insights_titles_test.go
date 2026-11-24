package api

import (
	"testing"
)

// TestInsightsTitles checks the titles map: it covers every id in articulation,
// orphans and cycles after the cap, leaves out ids that are not in the snapshot,
// and is an object (never null) even when no list has an id.
func TestInsightsTitles(t *testing.T) {
	h := goldenHandler(t)
	titleOf := map[string]string{}
	for _, is := range goldenIssues() {
		titleOf[is.ID] = is.Title
	}

	resp, res := getInsights(t, h, "?limit=1")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if len(resp.Orphans) != 1 || len(resp.Cycles) != 1 {
		t.Fatalf("limit=1 lists: orphans %v cycles %v", resp.Orphans, resp.Cycles)
	}
	ids := append(append([]string{}, resp.Articulation...), resp.Orphans...)
	for _, c := range resp.Cycles {
		ids = append(ids, c...)
	}
	for _, id := range ids {
		if got, want := resp.Titles[id], titleOf[id]; want == "" || got != want {
			t.Errorf("titles[%q] = %q, want %q", id, got, want)
		}
	}
	// Only ids that appear in a capped list are present.
	listed := map[string]bool{}
	for _, id := range ids {
		listed[id] = true
	}
	for id := range resp.Titles {
		if !listed[id] {
			t.Errorf("titles carries %q, which is in no capped list", id)
		}
	}
	// The cycle is two issues: both are titled.
	if len(resp.Cycles[0]) < 2 {
		t.Errorf("cycle = %v", resp.Cycles[0])
	}

	// A wider limit adds the extra ids.
	full, _ := getInsights(t, h, "?limit=25")
	if len(full.Titles) < len(resp.Titles) {
		t.Errorf("limit=25 titles %d < limit=1 titles %d", len(full.Titles), len(resp.Titles))
	}
	for _, id := range full.Orphans {
		if full.Titles[id] != titleOf[id] {
			t.Errorf("limit=25 titles[%q] = %q", id, full.Titles[id])
		}
	}
}

// TestInsightsTitlesLeaveOutUnknownIDs drives buildInsights directly with a
// snapshot whose lists name an id the snapshot does not hold.
func TestInsightsTitlesLeaveOutUnknownIDs(t *testing.T) {
	store := goldenStore(t, writeGoldenTracker(t))
	waitPhase2(t, store)
	v := store.Current()

	// Work on a copy of the insights so the shared snapshot stays untouched.
	snap := *v.Snap
	in := snap.Insights
	in.Orphans = append([]string{"fx-ghost"}, in.Orphans...)
	in.Articulation = []string{"fx-ghost", "fx-a"}
	in.Cycles = [][]string{{"fx-ghost", "fx-cy1"}}
	snap.Insights = in
	copyView := &View{Snap: &snap, Phase2Ready: true, ETag: v.ETag}

	resp := buildInsights(copyView, 25)
	if _, ok := resp.Titles["fx-ghost"]; ok {
		t.Error("an id outside the snapshot must be left out of titles")
	}
	if resp.Titles["fx-a"] != "Cart model" || resp.Titles["fx-cy1"] != "Cycle one" {
		t.Errorf("titles = %v", resp.Titles)
	}

	// With no ids anywhere, titles is an empty object, not nil.
	in.Orphans, in.Articulation, in.Cycles = nil, nil, nil
	snap.Insights = in
	if got := buildInsights(copyView, 25).Titles; got == nil || len(got) != 0 {
		t.Errorf("empty titles = %#v, want an empty non-nil map", got)
	}
}
