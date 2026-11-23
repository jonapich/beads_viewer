package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
	"github.com/Dicklesworthstone/beads_viewer/pkg/tracker"
)

const issuesPathPrefix = "/api/v1/issues/"

// issueIDFromPath extracts the issue id from /api/v1/issues/{id}. The id is
// taken from the escaped path and unescaped once, so a client that percent-
// encodes it (encodeURIComponent) round-trips. An empty id, or one containing
// "/", is not an issue.
func issueIDFromPath(r *http.Request) (string, bool) {
	rest, ok := strings.CutPrefix(r.URL.EscapedPath(), issuesPathPrefix)
	if !ok || rest == "" {
		return "", false
	}
	id, err := url.PathUnescape(rest)
	if err != nil || id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// handleIssueDetail serves GET /api/v1/issues/{id}.
func (s *server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	id, ok := issueIDFromPath(r)
	var issue *model.Issue
	if ok {
		issue = v.Snap.Issue(id)
	}
	if issue == nil {
		// Errors carry no ETag, so this is decided before the conditional check.
		writeError(w, http.StatusNotFound, "not_found", "no such issue")
		return
	}
	sum := sha256.Sum256([]byte(id))
	writeViewTagged(w, r, variantETag(v.ETag, "i"+hex.EncodeToString(sum[:6])), func() any {
		return buildIssueDetail(v, issue)
	})
}

func refOf(snap *snapshot.Snapshot, id string) IssueRef {
	other := snap.Issue(id)
	if other == nil {
		return IssueRef{ID: id, Missing: true}
	}
	prio := other.Priority
	return IssueRef{
		ID: other.ID, Title: other.Title, Status: string(other.Status),
		Priority: &prio, IssueType: string(other.IssueType),
	}
}

func isParentChild(dep *model.Dependency) bool {
	return dep != nil && dep.Type == model.DepParentChild
}

func buildIssueDetail(v *View, issue *model.Issue) IssueDetail {
	snap := v.Snap
	var readiness *model.ReadinessIndex
	if snap.Analyzer != nil {
		readiness = snap.Analyzer.Readiness()
	}
	d := IssueDetail{
		ID: issue.ID, Title: issue.Title, Description: issue.Description,
		Design: issue.Design, AcceptanceCriteria: issue.AcceptanceCriteria, Notes: issue.Notes,
		Status: string(issue.Status), Priority: issue.Priority, IssueType: string(issue.IssueType),
		Assignee: issue.Assignee, EstimatedMinutes: issue.EstimatedMinutes,
		CreatedAt: issue.CreatedAt, CreatedBy: issue.CreatedBy, UpdatedAt: issue.UpdatedAt,
		DueDate: issue.DueDate, DeferUntil: issue.DeferUntil, ClosedAt: issue.ClosedAt, CloseReason: issue.CloseReason,
		StartedAt: issue.StartedAt, LeaseExpiresAt: issue.LeaseExpiresAt, HeartbeatAt: issue.HeartbeatAt,
		Labels:     append([]string{}, issue.Labels...),
		SourceRepo: issue.SourceRepo,
		Metadata:   issue.MetadataMap(),
		Children:   []IssueRef{}, BlockedBy: []DepRef{}, Blocks: []DepRef{}, Related: []RelatedRef{},
		Phase2Ready:    v.Phase2Ready,
		Ready:          snap.IsReady(issue.ID),
		Links:          agentdesk.Links(*issue),
		Comments:       []CommentDTO{},
		ReadOnlyReason: tracker.CapabilitiesOf(issue.Origin).ReadOnlyReason,
	}
	if d.Links == nil {
		d.Links = []agentdesk.Link{}
	}
	if issue.ExternalRef != nil {
		d.ExternalRef = *issue.ExternalRef
	}

	// Parent: the first parent-child dependency whose target is loaded; failing
	// that, the first one at all, shown as a missing reference.
	var firstParent string
	for _, dep := range issue.Dependencies {
		if !isParentChild(dep) || dep.DependsOnID == "" || dep.DependsOnID == issue.ID {
			continue
		}
		if firstParent == "" {
			firstParent = dep.DependsOnID
		}
		if snap.Issue(dep.DependsOnID) != nil {
			firstParent = dep.DependsOnID
			break
		}
	}
	if firstParent != "" {
		ref := refOf(snap, firstParent)
		d.Parent = &ref
	}

	// Blocking dependencies of this issue, deduplicated; open blockers first.
	for _, b := range snapshot.Blockers(snap.IssueMap, readiness, *issue) {
		d.BlockedBy = append(d.BlockedBy, DepRef{IssueRef: refOf(snap, b.ID), Active: b.Active})
	}
	slices.SortStableFunc(d.BlockedBy, func(a, b DepRef) int {
		switch {
		case a.Active == b.Active:
			return 0
		case a.Active:
			return -1
		default:
			return 1
		}
	})
	active, resolved := snapshot.BlockerCounts(snap.IssueMap, readiness, *issue)
	d.BlockerCounts = BlockerCountsDTO{Active: active, Resolved: resolved}
	d.ActiveDependents = snapshot.ActiveDependentsCount(snap.Issues, issue.ID)
	d.Blocked = !closedLike(issue.Status) && (issue.Status == model.StatusBlocked || active > 0)

	// One pass over the other issues for the reverse relations, in snapshot order.
	for i := range snap.Issues {
		other := &snap.Issues[i]
		if other.ID == issue.ID {
			continue
		}
		var blocks, child bool
		var relatedTypes []string
		for _, dep := range other.Dependencies {
			if dep == nil || dep.DependsOnID != issue.ID {
				continue
			}
			switch {
			case isParentChild(dep):
				child = true
			case dep.Type.IsBlocking():
				blocks = true
			default:
				relatedTypes = append(relatedTypes, string(dep.Type))
			}
		}
		if child {
			d.Children = append(d.Children, refOf(snap, other.ID))
		}
		if blocks && !closedLike(other.Status) {
			d.Blocks = append(d.Blocks, DepRef{
				IssueRef: refOf(snap, other.ID),
				Active:   snapshot.IsBlockerUnsatisfied(snap.IssueMap, readiness, other.ID, issue.ID),
			})
		}
		for _, typ := range uniqueStrings(relatedTypes) {
			d.Related = append(d.Related, RelatedRef{IssueRef: refOf(snap, other.ID), Type: typ, Direction: "in"})
		}
	}
	// Outgoing related links come first.
	var out []RelatedRef
	seenRelated := map[string]bool{}
	for _, dep := range issue.Dependencies {
		if dep == nil || dep.DependsOnID == "" || isParentChild(dep) || dep.Type.IsBlocking() {
			continue
		}
		key := string(dep.Type) + "\x00" + dep.DependsOnID
		if seenRelated[key] {
			continue
		}
		seenRelated[key] = true
		out = append(out, RelatedRef{IssueRef: refOf(snap, dep.DependsOnID), Type: string(dep.Type), Direction: "out"})
	}
	d.Related = append(out, d.Related...)
	if d.Related == nil {
		d.Related = []RelatedRef{}
	}

	if score, ok := snap.TriageScores[issue.ID]; ok {
		t := &DetailTriage{
			Score: score, QuickWin: snap.QuickWinSet[issue.ID], IsBlocker: snap.BlockerSet[issue.ID],
			UnblocksIDs: append([]string{}, snap.UnblocksMap[issue.ID]...),
		}
		if reasons, ok := snap.TriageReasons[issue.ID]; ok {
			reasons.All = append([]string{}, reasons.All...)
			t.Reasons = &reasons
		}
		d.Triage = t
	}
	if v.Phase2Ready && snap.Tier != snapshot.TierHuge && snap.Analysis != nil {
		d.Graph = graphOf(snap.Analysis, issue.ID)
	}

	if agentdesk.SignoffState(*issue) != "" {
		item := signoffOf(*issue)
		d.Signoff = &item
	}
	d.Decision = decisionOf(issue)

	for _, c := range issue.Comments {
		if c == nil {
			continue
		}
		p := agentdesk.ParseComment(c.Text)
		dto := CommentDTO{
			ID: c.ID, Author: c.Author, CreatedAt: c.CreatedAt, Kind: p.Kind,
			Text: p.Body, Data: p.Data, Collapsible: p.Collapsible(),
		}
		if dto.Collapsible {
			dto.Summary = p.Summary()
		}
		d.Comments = append(d.Comments, dto)
	}
	return d
}

func uniqueStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func graphOf(stats *analysis.GraphStats, id string) *DetailGraph {
	return &DetailGraph{
		PageRank:          stats.GetPageRankScore(id),
		Betweenness:       stats.GetBetweennessScore(id),
		Eigenvector:       stats.GetEigenvectorScore(id),
		Hub:               stats.GetHubScore(id),
		Authority:         stats.GetAuthorityScore(id),
		CriticalPathScore: stats.GetCriticalPathScore(id),
	}
}

// decisionOf is the "Needs your decision" data, from the same rules as the
// agent-desk inbox (agentdesk.BuildInbox): an open bead labelled needs-decision
// or needs-design.
func decisionOf(issue *model.Issue) *DecisionInfo {
	if closedLike(issue.Status) {
		return nil
	}
	label := ""
	for _, l := range agentdesk.AwaitingLabels {
		if slices.Contains(issue.Labels, l) {
			label = l
			break
		}
	}
	if label == "" {
		return nil
	}
	info := &DecisionInfo{
		Label:    label,
		Question: agentdesk.Question(issue.Description),
		Options:  agentdesk.ParseOptions(issue.Description),
	}
	if info.Options == nil {
		info.Options = []agentdesk.Option{}
	}
	for _, link := range agentdesk.Links(*issue) {
		if link.Judgeable() {
			info.ReadyToJudge = true
			break
		}
	}
	if slices.Contains(issue.Labels, agentdesk.LabelNeedsDesign) && !info.ReadyToJudge {
		info.WaitingOn = agentdesk.WaitingOn(issue.Description)
	}
	return info
}

// signoffOf is the sign-off card data: agentdesk.ParseSignoff plus the calls
// as structured rows. The rows come from the newest "bd-kind: signoff" comment
// (the one ParseSignoff reads): objects in calls_made become call / alternative
// (or instead_of) / why, strings become a bare call, and a text body gives one
// bare call per parsed line.
func signoffOf(issue model.Issue) SignoffDTO {
	item := agentdesk.ParseSignoff(issue)
	dto := SignoffDTO{SignoffItem: item, Calls: []SignoffCall{}}
	if !item.HasComment {
		return dto
	}
	for i := len(issue.Comments) - 1; i >= 0; i-- {
		c := issue.Comments[i]
		if c == nil {
			continue
		}
		p := agentdesk.ParseComment(c.Text)
		if p.Kind != agentdesk.KindSignoff {
			continue
		}
		if raw, ok := p.Data["calls_made"].([]any); ok {
			for _, e := range raw {
				switch t := e.(type) {
				case map[string]any:
					call := SignoffCall{
						Call:        callField(t, "call"),
						Alternative: firstNonEmpty(callField(t, "alternative"), callField(t, "instead_of")),
						Why:         callField(t, "why"),
					}
					if call.Call != "" {
						dto.Calls = append(dto.Calls, call)
					}
				case string:
					if s := strings.TrimSpace(t); s != "" {
						dto.Calls = append(dto.Calls, SignoffCall{Call: s})
					}
				}
			}
			return dto
		}
		break
	}
	for _, line := range item.CallsMade {
		dto.Calls = append(dto.Calls, SignoffCall{Call: line})
	}
	return dto
}

func callField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// handleSignoff serves GET /api/v1/signoff: the closed beads awaiting sign-off,
// grouped by epic, as agentdesk.BuildSignoffs returns them, each with its calls
// as structured rows. On the huge tier closed issues are not loaded, so this is
// empty (meta.loaded_open_only says why).
func (s *server) handleSignoff(w http.ResponseWriter, r *http.Request) {
	v := s.snapshotView(w, r)
	if v == nil {
		return
	}
	writeView(w, r, v, func() any {
		built := agentdesk.BuildSignoffs(v.Snap.Issues)
		out := SignoffResponse{Total: built.Total, Groups: []SignoffGroupDTO{}}
		for _, g := range built.Groups {
			group := SignoffGroupDTO{Epic: g.Epic, EpicTitle: g.EpicTitle, Items: []SignoffDTO{}}
			for _, item := range g.Items {
				dto := SignoffDTO{SignoffItem: item, Calls: []SignoffCall{}}
				if issue := v.Snap.Issue(item.ID); issue != nil {
					full := signoffOf(*issue)
					full.Epic = item.Epic // the epic index fills it; ParseSignoff alone does not
					dto = full
				}
				group.Items = append(group.Items, dto)
			}
			out.Groups = append(out.Groups, group)
		}
		return out
	})
}
