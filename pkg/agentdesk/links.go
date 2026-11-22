package agentdesk

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// Link sources.
const (
	SourceTyped   = "typed"   // metadata link.<kind>
	SourceComment = "comment" // a URL found in a comment
)

// Link kinds bv infers for comment URLs, next to bd's typed kinds
// (mock, artifact, capture, commit, doc).
const (
	KindMock     = "mock"
	KindArtifact = "artifact"
	KindCommit   = "commit"
	KindURL      = "url"
)

// Link is one URL attached to a bead.
type Link struct {
	Kind   string     `json:"kind"`
	URL    string     `json:"url"`
	Note   string     `json:"note,omitempty"`
	At     *time.Time `json:"at,omitempty"`
	By     string     `json:"by,omitempty"`
	Source string     `json:"source"`
}

// Judgeable reports whether the link is something the user can judge a
// design question against (a mock or an artifact).
func (l Link) Judgeable() bool {
	return l.Kind == KindMock || l.Kind == KindArtifact
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"'\x60]+`)

// Links returns the bead's typed links (metadata link.<kind>, newest first per
// kind, kinds in name order), then URLs found in its comments that are not
// already typed (newest comment first). Duplicate URLs are dropped.
func Links(issue model.Issue) []Link {
	var out []Link
	seen := map[string]bool{}
	meta := issue.MetadataMap()
	for _, key := range sortedKeys(meta) {
		kind, ok := strings.CutPrefix(key, "link.")
		if !ok || kind == "" {
			continue
		}
		entries, _ := meta[key].([]any)
		var typed []Link
		for _, raw := range entries {
			entry, _ := raw.(map[string]any)
			url := metaString(entry, "url")
			if url == "" || seen[url] {
				continue
			}
			seen[url] = true
			typed = append(typed, Link{
				Kind: kind, URL: url, Note: metaString(entry, "note"),
				At: metaTime(entry, "at"), By: metaString(entry, "by"), Source: SourceTyped,
			})
		}
		sort.SliceStable(typed, func(i, j int) bool {
			if typed[i].At == nil || typed[j].At == nil {
				return typed[i].At != nil
			}
			return typed[i].At.After(*typed[j].At)
		})
		out = append(out, typed...)
	}
	for i := len(issue.Comments) - 1; i >= 0; i-- {
		c := issue.Comments[i]
		if c == nil {
			continue
		}
		for _, url := range FindURLs(c.Text) {
			if seen[url] {
				continue
			}
			seen[url] = true
			at := c.CreatedAt
			out = append(out, Link{Kind: inferKind(url, c.Text), URL: url, At: &at, By: c.Author, Source: SourceComment})
		}
	}
	return out
}

// FindURLs returns the http(s) URLs in text, in order, with trailing
// punctuation and unbalanced closing brackets trimmed.
func FindURLs(text string) []string {
	var out []string
	for _, raw := range urlPattern.FindAllString(text, -1) {
		url := trimURL(raw)
		if len(url) > len("https://") {
			out = append(out, url)
		}
	}
	return out
}

func trimURL(url string) string {
	for url != "" {
		last := url[len(url)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?', '*':
			url = url[:len(url)-1]
			continue
		case ')', ']', '}':
			open := map[byte]byte{')': '(', ']': '[', '}': '{'}[last]
			if strings.Count(url, string(open)) < strings.Count(url, string(last)) {
				url = url[:len(url)-1]
				continue
			}
		}
		break
	}
	return url
}

func inferKind(url, context string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, "claude.ai/") && strings.Contains(lower, "/artifact/"):
		if strings.Contains(strings.ToLower(context), "mock") {
			return KindMock
		}
		return KindArtifact
	case strings.Contains(lower, "/commit/"):
		return KindCommit
	}
	return KindURL
}
