package agentdesk

import (
	"sort"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// DefaultStaleAfter is how long a claim may go without activity (heartbeat,
// stage change or any update) before the pipeline flags it.
const DefaultStaleAfter = 2 * time.Hour

// PipelineRow is one in-progress bead.
type PipelineRow struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Epic           string     `json:"epic,omitempty"`
	Stage          string     `json:"stage,omitempty"`
	StageSince     *time.Time `json:"stage_since,omitempty"`
	StageAge       string     `json:"stage_age,omitempty"`
	StageAgeSecs   int64      `json:"stage_age_seconds"`
	Holder         string     `json:"holder,omitempty"`  // assignee
	Session        string     `json:"session,omitempty"` // metadata claim.session
	ClaimedAt      *time.Time `json:"claimed_at,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt    *time.Time `json:"heartbeat_at,omitempty"`
	LastActivity   time.Time  `json:"last_activity"`
	Stale          bool       `json:"stale"`
	StaleReason    string     `json:"stale_reason,omitempty"`
}

// Pipeline is the live view of claimed work.
type Pipeline struct {
	GeneratedAt time.Time      `json:"generated_at"`
	StaleAfter  string         `json:"stale_after"`
	Total       int            `json:"total"`
	Stale       int            `json:"stale"`
	ByStage     map[string]int `json:"by_stage"`
	Rows        []PipelineRow  `json:"rows"`
}

// BuildPipeline lists the in-progress beads with their stage, claim and
// staleness at now. A claim is stale when its lease has expired, or when it
// has no live lease and nothing happened for staleAfter (<= 0 means
// DefaultStaleAfter). Stale rows come first, then by stage order, then the
// longest in stage first.
func BuildPipeline(issues []model.Issue, now time.Time, staleAfter time.Duration) Pipeline {
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	epics := NewEpicIndex(issues)
	p := Pipeline{GeneratedAt: now, StaleAfter: FormatAge(staleAfter), ByStage: map[string]int{}}
	for i := range issues {
		issue := &issues[i]
		if issue.Status.Normalize() != model.StatusInProgress {
			continue
		}
		meta := issue.MetadataMap()
		row := PipelineRow{
			ID: issue.ID, Title: issue.Title, Epic: epics.EpicOf(issue.ID),
			Stage: metaString(meta, "stage"), Holder: issue.Assignee,
			Session: metaString(meta, "claim.session"), ClaimedAt: metaTime(meta, "claim.at"),
			LeaseExpiresAt: issue.LeaseExpiresAt, HeartbeatAt: issue.HeartbeatAt,
		}
		if row.ClaimedAt == nil {
			row.ClaimedAt = issue.StartedAt
		}
		row.StageSince = metaTime(meta, "stage_changed_at")
		if row.StageSince == nil {
			row.StageSince = row.ClaimedAt
		}
		if row.StageSince != nil {
			age := now.Sub(*row.StageSince)
			row.StageAge = FormatAge(age)
			row.StageAgeSecs = int64(age.Seconds())
		}
		row.LastActivity = latest(issue.UpdatedAt, row.StageSince, row.HeartbeatAt, row.ClaimedAt)
		switch {
		case row.LeaseExpiresAt != nil && !row.LeaseExpiresAt.After(now):
			row.Stale = true
			row.StaleReason = "lease expired " + FormatAge(now.Sub(*row.LeaseExpiresAt)) + " ago"
		case row.LeaseExpiresAt != nil:
			// A live lease vouches for the claim.
		case now.Sub(row.LastActivity) > staleAfter:
			row.Stale = true
			row.StaleReason = "no activity for " + FormatAge(now.Sub(row.LastActivity))
		}
		key := row.Stage
		if key == "" {
			key = "unstaged"
		}
		p.ByStage[key]++
		if row.Stale {
			p.Stale++
		}
		p.Rows = append(p.Rows, row)
	}
	p.Total = len(p.Rows)
	sort.SliceStable(p.Rows, func(i, j int) bool {
		a, b := p.Rows[i], p.Rows[j]
		if a.Stale != b.Stale {
			return a.Stale
		}
		if ra, rb := stageRank(a.Stage), stageRank(b.Stage); ra != rb {
			return ra < rb
		}
		if a.StageAgeSecs != b.StageAgeSecs {
			return a.StageAgeSecs > b.StageAgeSecs
		}
		return a.ID < b.ID
	})
	return p
}

func latest(base time.Time, others ...*time.Time) time.Time {
	out := base
	for _, t := range others {
		if t != nil && t.After(out) {
			out = *t
		}
	}
	return out
}
