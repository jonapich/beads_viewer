package agentdesk

import (
	"sort"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// A slice bead is one carrying tools/slice_log.ps1 metadata; slice_outcome is
// always written.
const sliceOutcomeKey = "slice_outcome"

// SliceMetrics sums the logged slices of one epic (or all of them).
type SliceMetrics struct {
	Epic            string `json:"epic"`
	EpicTitle       string `json:"epic_title,omitempty"`
	Slices          int    `json:"slices"`
	Committed       int    `json:"committed"`
	Stopped         int    `json:"stopped"`
	ReviewRounds    int    `json:"review_rounds"`
	FirstPass       int    `json:"first_pass_approvals"` // review_rounds == 1
	ReviewBlocking  int    `json:"review_blocking"`
	TestRuns        int    `json:"test_runs"`
	TestRedRuns     int    `json:"test_red_runs"`
	TestTimeouts    int    `json:"test_timeouts"`
	TestWallMinutes int    `json:"test_wall_minutes"`
	WallMinutes     int    `json:"slice_wall_minutes"`
	FollowupBeads   int    `json:"followup_beads"`
	CoderTokens     int64  `json:"coder_tokens"`
	ReviewerTokens  int64  `json:"reviewer_tokens"`
	OtherTokens     int64  `json:"other_agent_tokens"`
	TokensUnknown   int    `json:"slices_with_unknown_tokens"`
	EscapedBugs     int    `json:"escaped_bugs"`
}

// AvgReviewRounds is review rounds per slice.
func (s SliceMetrics) AvgReviewRounds() float64 { return ratio(s.ReviewRounds, s.Slices) }

// AvgWallMinutes is wall minutes per slice.
func (s SliceMetrics) AvgWallMinutes() float64 { return ratio(s.WallMinutes, s.Slices) }

// TotalTokens sums the three agent roles.
func (s SliceMetrics) TotalTokens() int64 { return s.CoderTokens + s.ReviewerTokens + s.OtherTokens }

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// EscapedBug is a bead whose metadata caused_by names a slice.
type EscapedBug struct {
	Bug   string `json:"bug"`
	Cause string `json:"cause"`
}

// SliceReport is the per-epic slice summary.
type SliceReport struct {
	Total SliceMetrics   `json:"total"`
	Epics []SliceMetrics `json:"epics"`
	// UnmatchedEscapes name a cause that is not a logged slice.
	UnmatchedEscapes []EscapedBug `json:"unmatched_escapes,omitempty"`
}

// BuildSliceReport sums slice metadata per epic, the way tools/slice_stats.ps1
// counts it: a token figure absent from a slice (logged as unknown) adds
// nothing and is counted in TokensUnknown; an escaped bug is any bead of any
// status whose caused_by names a slice, attributed to that slice's epic.
// Epics are ordered by slice count, then ID.
func BuildSliceReport(issues []model.Issue) SliceReport {
	epics := NewEpicIndex(issues)
	byEpic := map[string]*SliceMetrics{}
	sliceEpic := map[string]string{}
	report := SliceReport{Total: SliceMetrics{Epic: "all"}}
	for i := range issues {
		issue := &issues[i]
		meta := issue.MetadataMap()
		outcome := metaString(meta, sliceOutcomeKey)
		if outcome == "" {
			continue
		}
		epic := epics.EpicOf(issue.ID)
		sliceEpic[issue.ID] = epic
		m := byEpic[epic]
		if m == nil {
			m = &SliceMetrics{Epic: epic, EpicTitle: epics.Title(epic)}
			byEpic[epic] = m
		}
		for _, target := range []*SliceMetrics{m, &report.Total} {
			addSlice(target, meta, outcome)
		}
	}
	for i := range issues {
		cause := metaString(issues[i].MetadataMap(), "caused_by")
		if cause == "" {
			continue
		}
		epic, ok := sliceEpic[cause]
		if !ok {
			report.UnmatchedEscapes = append(report.UnmatchedEscapes, EscapedBug{Bug: issues[i].ID, Cause: cause})
			continue
		}
		byEpic[epic].EscapedBugs++
		report.Total.EscapedBugs++
	}
	for _, m := range byEpic {
		report.Epics = append(report.Epics, *m)
	}
	sort.SliceStable(report.Epics, func(i, j int) bool {
		a, b := report.Epics[i], report.Epics[j]
		if a.Slices != b.Slices {
			return a.Slices > b.Slices
		}
		return a.Epic < b.Epic
	})
	return report
}

func addSlice(m *SliceMetrics, meta map[string]any, outcome string) {
	m.Slices++
	switch outcome {
	case "committed":
		m.Committed++
	case "stopped":
		m.Stopped++
	}
	num := func(key string) int {
		v, _ := metaNumber(meta, key)
		return int(v)
	}
	rounds := num("review_rounds")
	m.ReviewRounds += rounds
	if rounds == 1 {
		m.FirstPass++
	}
	m.ReviewBlocking += num("review_blocking")
	m.TestRuns += num("test_runs")
	m.TestRedRuns += num("test_red_runs")
	m.TestTimeouts += num("test_timeouts")
	m.TestWallMinutes += num("test_wall_minutes")
	m.WallMinutes += num("slice_wall_minutes")
	m.FollowupBeads += num("followup_beads")
	unknown := false
	for _, tk := range []struct {
		key string
		dst *int64
	}{{"coder_tokens", &m.CoderTokens}, {"reviewer_tokens", &m.ReviewerTokens}, {"other_agent_tokens", &m.OtherTokens}} {
		if v, ok := metaNumber(meta, tk.key); ok {
			*tk.dst += int64(v)
		} else {
			unknown = true
		}
	}
	if unknown {
		m.TokensUnknown++
	}
}
