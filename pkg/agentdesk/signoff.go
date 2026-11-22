package agentdesk

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// Sign-off conventions: a closed bead awaiting the user's post-review carries
// LabelNeedsSignoff and a "bd-kind: signoff" comment; metadata signoff holds
// the state once decided.
const (
	LabelNeedsSignoff = "needs-signoff"
	KindSignoff       = "signoff"
	SignoffMetaKey    = "signoff"

	SignoffPending  = "pending"
	SignoffApproved = "approved"
	SignoffDisputed = "disputed"
)

// SignoffItem is one closed bead awaiting sign-off.
type SignoffItem struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Epic       string     `json:"epic,omitempty"`
	Priority   int        `json:"priority"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	State      string     `json:"state"`
	HasComment bool       `json:"has_signoff_comment"`
	// The signoff comment's four parts, as lists of lines.
	CallsMade        []string `json:"calls_made,omitempty"`
	WhatChanged      []string `json:"what_changed,omitempty"`
	RemovedOrRenamed []string `json:"removed_or_renamed,omitempty"`
	WhereToSee       []string `json:"where_to_see,omitempty"`
	Commits          []string `json:"commits,omitempty"`
	Captures         []string `json:"captures,omitempty"`
	Links            []Link   `json:"links,omitempty"`
	// Raw is the comment body when it fits none of the parts.
	Raw string `json:"raw,omitempty"`
}

// SignoffGroup is the pending sign-offs of one epic.
type SignoffGroup struct {
	Epic      string        `json:"epic"`
	EpicTitle string        `json:"epic_title,omitempty"`
	Items     []SignoffItem `json:"items"`
}

// Signoffs is every closed bead pending sign-off, grouped by epic.
type Signoffs struct {
	Total  int            `json:"total"`
	Groups []SignoffGroup `json:"groups"`
}

// Items returns the sign-off items in display order.
func (s Signoffs) Items() []SignoffItem {
	out := make([]SignoffItem, 0, s.Total)
	for _, g := range s.Groups {
		out = append(out, g.Items...)
	}
	return out
}

// SignoffState returns the bead's sign-off state: metadata signoff when set,
// else pending for a bead labelled needs-signoff, else "".
func SignoffState(issue model.Issue) string {
	if s := metaString(issue.MetadataMap(), SignoffMetaKey); s != "" {
		return s
	}
	if slices.Contains(issue.Labels, LabelNeedsSignoff) {
		return SignoffPending
	}
	return ""
}

// BuildSignoffs lists the closed beads pending sign-off, newest closed first
// within each epic; the group holding the newest sign-off comes first and the
// no-epic group last.
func BuildSignoffs(issues []model.Issue) Signoffs {
	epics := NewEpicIndex(issues)
	groups := map[string]*SignoffGroup{}
	var out Signoffs
	for i := range issues {
		issue := &issues[i]
		if isOpen(issue) || SignoffState(*issue) != SignoffPending {
			continue
		}
		item := ParseSignoff(*issue)
		item.Epic = epics.EpicOf(issue.ID)
		g := groups[item.Epic]
		if g == nil {
			g = &SignoffGroup{Epic: item.Epic, EpicTitle: epics.Title(item.Epic)}
			groups[item.Epic] = g
		}
		g.Items = append(g.Items, item)
		out.Total++
	}
	newest := func(a, b *time.Time) bool {
		if a == nil || b == nil {
			return a != nil
		}
		return a.After(*b)
	}
	for _, g := range groups {
		sort.SliceStable(g.Items, func(i, j int) bool {
			a, b := g.Items[i], g.Items[j]
			if !timeEqual(a.ClosedAt, b.ClosedAt) {
				return newest(a.ClosedAt, b.ClosedAt)
			}
			return a.ID < b.ID
		})
		out.Groups = append(out.Groups, *g)
	}
	sort.SliceStable(out.Groups, func(i, j int) bool {
		a, b := out.Groups[i], out.Groups[j]
		if (a.Epic == NoEpic) != (b.Epic == NoEpic) {
			return b.Epic == NoEpic
		}
		if !timeEqual(a.Items[0].ClosedAt, b.Items[0].ClosedAt) {
			return newest(a.Items[0].ClosedAt, b.Items[0].ClosedAt)
		}
		return a.Epic < b.Epic
	})
	return out
}

func timeEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// signoffParts maps the comment's part names (JSON keys or text headings,
// normalised to lower-case words) to the item field they fill.
var signoffParts = []struct {
	names []string
	field func(*SignoffItem) *[]string
}{
	{[]string{"calls made on your behalf", "calls made", "calls", "decisions", "calls made for you"}, func(s *SignoffItem) *[]string { return &s.CallsMade }},
	{[]string{"what changed", "changed", "changes", "summary"}, func(s *SignoffItem) *[]string { return &s.WhatChanged }},
	{[]string{"removed or renamed", "removed", "renamed", "removed renamed"}, func(s *SignoffItem) *[]string { return &s.RemovedOrRenamed }},
	{[]string{"where to see it", "where to see", "where", "see it", "how to see"}, func(s *SignoffItem) *[]string { return &s.WhereToSee }},
	{[]string{"commits", "commit", "sha", "shas"}, func(s *SignoffItem) *[]string { return &s.Commits }},
	{[]string{"captures", "capture", "screenshots"}, func(s *SignoffItem) *[]string { return &s.Captures }},
}

func signoffField(item *SignoffItem, name string) *[]string {
	norm := strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(strings.ToLower(name))), " ")
	for _, p := range signoffParts {
		if slices.Contains(p.names, norm) {
			return p.field(item)
		}
	}
	return nil
}

var (
	// "What changed:", "## Calls made on your behalf", "**Where to see it**:".
	signoffHeading  = regexp.MustCompile(`^\s*(?:#{1,6}\s*)?(?:\*\*)?([A-Za-z][A-Za-z _/-]{1,40}?)(?:\*\*)?\s*:\s*(?:\*\*)?\s*(.*)$`)
	markdownHeading = regexp.MustCompile(`^\s*#{1,6}\s+(.+?)\s*$`)
	listMarker      = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	commitSHA       = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	capturePath     = regexp.MustCompile(`(?i)[^\s'"()]+\.(?:png|jpe?g|gif|webp|webm|mp4)\b`)
)

// ParseSignoff reads a bead's latest signoff comment into its parts. Commit
// SHAs and capture paths found anywhere in the comment are collected too.
func ParseSignoff(issue model.Issue) SignoffItem {
	item := SignoffItem{
		ID: issue.ID, Title: issue.Title, Priority: issue.Priority, ClosedAt: issue.ClosedAt,
		State: SignoffState(issue), Links: Links(issue),
	}
	var body string
	for i := len(issue.Comments) - 1; i >= 0; i-- {
		c := issue.Comments[i]
		if c == nil {
			continue
		}
		if p := ParseComment(c.Text); p.Kind == KindSignoff {
			item.HasComment = true
			body = p.Body
			if p.Data != nil {
				fillFromJSON(&item, p.Data)
			} else {
				fillFromText(&item, p.Body)
			}
			break
		}
	}
	if body != "" {
		for _, sha := range commitSHA.FindAllString(body, -1) {
			if hasHexLetterAndDigit(sha) && !slices.Contains(item.Commits, sha) {
				item.Commits = append(item.Commits, sha)
			}
		}
		for _, path := range capturePath.FindAllString(body, -1) {
			if !slices.Contains(item.Captures, path) && !strings.HasPrefix(path, "http") {
				item.Captures = append(item.Captures, path)
			}
		}
	}
	return item
}

// hasHexLetterAndDigit filters words like "deadbeef" or "1234567" that are
// unlikely to be SHAs only loosely: a SHA needs both a digit and a letter.
func hasHexLetterAndDigit(s string) bool {
	return strings.ContainsAny(s, "0123456789") && strings.ContainsAny(s, "abcdef")
}

func fillFromJSON(item *SignoffItem, data map[string]any) {
	var rest []string
	for _, key := range sortedKeys(data) {
		lines := jsonLines(data[key])
		if dst := signoffField(item, key); dst != nil {
			*dst = append(*dst, lines...)
			continue
		}
		for _, l := range lines {
			rest = append(rest, key+": "+l)
		}
	}
	item.Raw = strings.Join(rest, "\n")
}

func jsonLines(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, jsonLines(e)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, k := range sortedKeys(t) {
			for _, l := range jsonLines(t[k]) {
				out = append(out, k+": "+l)
			}
		}
		return out
	default:
		if s := scalar(v); s != "" {
			return []string{s}
		}
	}
	return nil
}

func fillFromText(item *SignoffItem, body string) {
	var current *[]string
	var rest []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := markdownHeading.FindStringSubmatch(line); m != nil {
			if dst := signoffField(item, strings.Trim(m[1], "*: ")); dst != nil {
				current = dst
				continue
			}
		}
		if m := signoffHeading.FindStringSubmatch(line); m != nil {
			if dst := signoffField(item, m[1]); dst != nil {
				current = dst
				if tail := strings.TrimSpace(m[2]); tail != "" {
					*current = append(*current, tail)
				}
				continue
			}
		}
		text := strings.TrimSpace(listMarker.ReplaceAllString(line, ""))
		if current != nil {
			*current = append(*current, text)
		} else {
			rest = append(rest, text)
		}
	}
	item.Raw = strings.Join(rest, "\n")
}

// DisputeTitle is the follow-up bead's title for a dispute of a closed bead.
func DisputeTitle(closedTitle string) string {
	return capWords("Dispute: "+closedTitle, 200)
}
