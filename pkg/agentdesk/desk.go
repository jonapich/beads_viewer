// Package agentdesk derives the views an unattended-agent workflow needs from
// beads data: the inbox of questions awaiting the user, the live pipeline of
// claimed work, per-epic slice metrics, and readable handoff comments.
//
// Everything is read from fields bd already exports (labels, comments,
// assignee, lease times) and from the issue metadata object, where the bd fork
// stores the pipeline stage (`stage`, `stage_changed_at`, `stage_at.<stage>`),
// typed links (`link.<kind>`: [{url,note,at,by}]), claim sessions
// (`claim.session`, `claim.at`) and slice metrics (tools/slice_log.ps1).
package agentdesk

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// Labels that put a bead in the user's inbox.
const (
	LabelNeedsDecision = "needs-decision"
	LabelNeedsDesign   = "needs-design"
)

// AwaitingLabels is the default awaiting_user.labels set, in display order.
var AwaitingLabels = []string{LabelNeedsDecision, LabelNeedsDesign}

// Stages is the PIPPONG stage order (bd config stage.values). Unknown stages
// sort after these.
var Stages = []string{"scout", "plan", "fx", "code", "test", "review", "verify", "done"}

// NoEpic is the group key for beads with no parent chain.
const NoEpic = ""

func stageRank(stage string) int {
	for i, s := range Stages {
		if s == stage {
			return i
		}
	}
	return len(Stages)
}

// isOpen reports whether a bead still counts as live work (not closed or
// tombstoned).
func isOpen(issue *model.Issue) bool {
	return !model.IsClosedLikeStatus(issue.Status)
}

func metaString(meta map[string]any, key string) string {
	switch v := meta[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func metaTime(meta map[string]any, key string) *time.Time {
	s := metaString(meta, key)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// metaNumber returns a numeric metadata value. Strings holding numbers are
// accepted because bd's --set-metadata key=value stores strings.
func metaNumber(meta map[string]any, key string) (float64, bool) {
	switch v := meta[key].(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// EpicIndex resolves the epic a bead belongs to by walking parent-child edges.
type EpicIndex struct {
	byID   map[string]*model.Issue
	parent map[string]string
	cache  map[string]string
}

// NewEpicIndex indexes issues for epic lookup.
func NewEpicIndex(issues []model.Issue) *EpicIndex {
	idx := &EpicIndex{
		byID:   make(map[string]*model.Issue, len(issues)),
		parent: make(map[string]string),
		cache:  make(map[string]string),
	}
	for i := range issues {
		issue := &issues[i]
		idx.byID[issue.ID] = issue
		for _, dep := range issue.Dependencies {
			if dep == nil || dep.Type != model.DepParentChild || dep.DependsOnID == "" {
				continue
			}
			child := dep.IssueID
			if child == "" {
				child = issue.ID
			}
			if _, seen := idx.parent[child]; !seen {
				idx.parent[child] = dep.DependsOnID
			}
		}
	}
	return idx
}

// Issue returns the indexed issue with the given ID, or nil.
func (x *EpicIndex) Issue(id string) *model.Issue { return x.byID[id] }

// EpicOf returns the nearest epic at or above id. Without an epic in the
// chain it returns the topmost ancestor, and NoEpic for a bead with no parent.
func (x *EpicIndex) EpicOf(id string) string {
	if epic, ok := x.cache[id]; ok {
		return epic
	}
	result := NoEpic
	seen := map[string]bool{}
	cur := id
	for cur != "" && !seen[cur] {
		seen[cur] = true
		if issue := x.byID[cur]; issue != nil && issue.IssueType == model.TypeEpic {
			result = cur
			break
		}
		next, ok := x.parent[cur]
		if !ok {
			if cur != id {
				result = cur
			}
			break
		}
		cur = next
	}
	x.cache[id] = result
	return result
}

// Title returns the title of id, or "" when unknown.
func (x *EpicIndex) Title(id string) string {
	if issue := x.byID[id]; issue != nil {
		return issue.Title
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FormatAge renders a duration compactly: 45s, 12m, 3h05m, 2d4h.
func FormatAge(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		if m == 0 {
			return strconv.Itoa(h) + "h"
		}
		return strconv.Itoa(h) + "h" + pad2(m) + "m"
	default:
		days := int(d.Hours()) / 24
		h := int(d.Hours()) - days*24
		if h == 0 {
			return strconv.Itoa(days) + "d"
		}
		return strconv.Itoa(days) + "d" + strconv.Itoa(h) + "h"
	}
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
