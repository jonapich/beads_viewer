package agentdesk

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// InboxItem is one bead awaiting the user.
type InboxItem struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Label        string    `json:"label"` // needs-decision or needs-design (the first that applies)
	Labels       []string  `json:"awaiting_labels"`
	Priority     int       `json:"priority"`
	Status       string    `json:"status"`
	Epic         string    `json:"epic,omitempty"`
	ReadyToJudge bool      `json:"ready_to_judge"`
	WaitingOn    string    `json:"waiting_on,omitempty"`
	Links        []Link    `json:"links,omitempty"`
	Options      []Option  `json:"options,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LinkCount is the number of links on the bead.
func (it InboxItem) LinkCount() int { return len(it.Links) }

// IsDecision reports whether the item carries needs-decision.
func (it InboxItem) IsDecision() bool { return slices.Contains(it.Labels, LabelNeedsDecision) }

// AnswerLabel is the label an answer removes: needs-decision when present,
// else the item's awaiting label.
func (it InboxItem) AnswerLabel() string {
	if it.IsDecision() {
		return LabelNeedsDecision
	}
	return it.Label
}

// InboxGroup is the awaiting beads of one epic.
type InboxGroup struct {
	Epic      string      `json:"epic"`
	EpicTitle string      `json:"epic_title,omitempty"`
	Ready     int         `json:"ready_to_judge"`
	Items     []InboxItem `json:"items"`
}

// Inbox is every bead awaiting the user, grouped by epic.
type Inbox struct {
	Total  int          `json:"total"`
	Ready  int          `json:"ready_to_judge"`
	Groups []InboxGroup `json:"groups"`
}

// Items returns the inbox items in display order.
func (in Inbox) Items() []InboxItem {
	out := make([]InboxItem, 0, in.Total)
	for _, g := range in.Groups {
		out = append(out, g.Items...)
	}
	return out
}

// BuildInbox collects the open beads labelled with any of labels (nil means
// AwaitingLabels), grouped by epic. Within a group, beads ready to judge come
// first, then by priority, then by ID. Groups with ready beads come first,
// then larger groups; the no-epic group comes last.
func BuildInbox(issues []model.Issue, labels []string) Inbox {
	if len(labels) == 0 {
		labels = AwaitingLabels
	}
	epics := NewEpicIndex(issues)
	groups := map[string]*InboxGroup{}
	var inbox Inbox
	for i := range issues {
		issue := &issues[i]
		if !isOpen(issue) {
			continue
		}
		var awaiting []string
		for _, l := range labels {
			if slices.Contains(issue.Labels, l) {
				awaiting = append(awaiting, l)
			}
		}
		if len(awaiting) == 0 {
			continue
		}
		item := InboxItem{
			ID: issue.ID, Title: issue.Title, Label: awaiting[0], Labels: awaiting,
			Priority: issue.Priority, Status: string(issue.Status),
			Epic: epics.EpicOf(issue.ID), Links: Links(*issue), UpdatedAt: issue.UpdatedAt,
		}
		for _, l := range item.Links {
			if l.Judgeable() {
				item.ReadyToJudge = true
				break
			}
		}
		if slices.Contains(awaiting, LabelNeedsDesign) && !item.ReadyToJudge {
			item.WaitingOn = WaitingOn(issue.Description)
		}
		// Design beads offer options too once there is something to judge.
		item.Options = ParseOptions(issue.Description)
		g := groups[item.Epic]
		if g == nil {
			g = &InboxGroup{Epic: item.Epic, EpicTitle: epics.Title(item.Epic)}
			groups[item.Epic] = g
		}
		g.Items = append(g.Items, item)
		if item.ReadyToJudge {
			g.Ready++
			inbox.Ready++
		}
		inbox.Total++
	}
	for _, g := range groups {
		sort.SliceStable(g.Items, func(i, j int) bool {
			a, b := g.Items[i], g.Items[j]
			if a.ReadyToJudge != b.ReadyToJudge {
				return a.ReadyToJudge
			}
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return a.ID < b.ID
		})
		inbox.Groups = append(inbox.Groups, *g)
	}
	sort.SliceStable(inbox.Groups, func(i, j int) bool {
		a, b := inbox.Groups[i], inbox.Groups[j]
		if (a.Epic == NoEpic) != (b.Epic == NoEpic) {
			return b.Epic == NoEpic
		}
		if (a.Ready > 0) != (b.Ready > 0) {
			return a.Ready > 0
		}
		if len(a.Items) != len(b.Items) {
			return len(a.Items) > len(b.Items)
		}
		return a.Epic < b.Epic
	})
	return inbox
}

var (
	// "Next artifact: poet direction, then an fx-coder HTML slider mock …"
	waitingLine = regexp.MustCompile(`(?im)^\s*(?:next artifact|waiting on|needs|artifact)\s*:\s*(.+)$`)
	// An artifact named in prose: "an HTML slider mock", "a capture".
	artifactWord = regexp.MustCompile(`(?i)\b(mock(?:-?up)?|capture|artifact|prototype|sketch|preview)s?\b`)
)

// WaitingOn names the artifact a needs-design bead is waiting for: its
// "Next artifact:" (or "Waiting on:") line, else the first sentence naming a
// mock, capture or artifact, else "an artifact".
func WaitingOn(description string) string {
	return capWords(waitingOn(description), maxWaitingOn)
}

// maxWaitingOn caps the waiting-on text so it fits a row.
const maxWaitingOn = 120

func waitingOn(description string) string {
	if m := waitingLine.FindStringSubmatch(description); m != nil {
		return firstSentence(m[1])
	}
	for _, line := range strings.Split(description, "\n") {
		if artifactWord.MatchString(line) {
			for _, s := range splitSentences(line) {
				if artifactWord.MatchString(s) {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return "an artifact"
}

// capWords shortens s to at most n bytes at a word boundary, adding "…".
func capWords(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndexByte(s[:n], ' ')
	if cut <= 0 {
		cut = n
	}
	return strings.TrimRight(s[:cut], " ,;:") + "…"
}

func firstSentence(s string) string {
	parts := splitSentences(s)
	if len(parts) == 0 {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(parts[0])
}

// splitSentences splits on ". " and ";" keeping file names like "a.gd" whole.
func splitSentences(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if (s[i] == '.' && (i+1 == len(s) || s[i+1] == ' ')) || s[i] == ';' {
			if part := strings.TrimSpace(s[start:i]); part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	if part := strings.TrimSpace(s[start:]); part != "" {
		out = append(out, part)
	}
	return out
}
