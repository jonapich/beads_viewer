package agentdesk

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// KindHeader starts a kinded comment: "bd-kind: <kind>\n<body>".
const KindHeader = "bd-kind: "

// KindHandoff is the structured handoff comment kind.
const KindHandoff = "handoff"

// KindAnswer is the kind of a decision posted by the user from the desk.
const KindAnswer = "answer"

// ParsedComment is a comment split into its bd kind header and body.
type ParsedComment struct {
	Kind string         // "" for a plain comment
	Body string         // the text after the kind header
	Data map[string]any // the body decoded, when it is a JSON object
}

// Collapsible reports whether the comment should render as a one-line
// summary: a kinded comment, or one whose body is a JSON object.
func (p ParsedComment) Collapsible() bool {
	return p.Kind != "" || p.Data != nil
}

// ParseComment splits a comment's text into kind and body, and decodes a JSON
// object body.
func ParseComment(text string) ParsedComment {
	p := ParsedComment{Body: text}
	if rest, ok := strings.CutPrefix(text, KindHeader); ok {
		kind, body, _ := strings.Cut(rest, "\n")
		p.Kind = strings.TrimSpace(kind)
		p.Body = body
	}
	trimmed := strings.TrimSpace(p.Body)
	if strings.HasPrefix(trimmed, "{") {
		var data map[string]any
		if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
			p.Data = data
		}
	}
	return p
}

// summaryKeys are tried in order for the headline of a structured comment.
var summaryKeys = []string{"summary", "message", "result", "status", "outcome", "note", "text"}

// Summary renders a structured comment as one line, e.g.
// "handoff [code→test] wired the clash beat · next: run the full suite".
func (p ParsedComment) Summary() string {
	kind := p.Kind
	if kind == "" {
		kind = "json"
	}
	if p.Data == nil {
		first, _, _ := strings.Cut(strings.TrimSpace(p.Body), "\n")
		return strings.TrimSpace(kind + " " + first)
	}
	var parts []string
	from, to := scalar(p.Data["from_stage"]), scalar(p.Data["to_stage"])
	switch {
	case from != "" && to != "":
		parts = append(parts, "["+from+"→"+to+"]")
	case scalar(p.Data["stage"]) != "":
		parts = append(parts, "["+scalar(p.Data["stage"])+"]")
	}
	headline := ""
	for _, key := range summaryKeys {
		if v := scalar(p.Data[key]); v != "" {
			headline = v
			break
		}
	}
	if headline != "" {
		parts = append(parts, headline)
	}
	if next := scalar(p.Data["next"]); next != "" {
		parts = append(parts, "· next: "+next)
	}
	if headline == "" {
		keys := sortedKeys(p.Data)
		parts = append(parts, "{"+strings.Join(keys, ", ")+"}")
	}
	return kind + " " + strings.Join(parts, " ")
}

// Expanded renders the body for reading: JSON objects as indented
// "key: value" lines (sorted), anything else unchanged.
func (p ParsedComment) Expanded() string {
	if p.Data == nil {
		return p.Body
	}
	var sb strings.Builder
	keys := make([]string, 0, len(p.Data))
	for k := range p.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if list, ok := p.Data[k].([]any); ok && allScalars(list) {
			fmt.Fprintf(&sb, "%s:\n", k)
			for _, e := range list {
				fmt.Fprintf(&sb, "  - %s\n", scalar(e))
			}
			continue
		}
		v := scalar(p.Data[k])
		if v == "" {
			raw, _ := json.Marshal(p.Data[k])
			v = string(raw)
		}
		fmt.Fprintf(&sb, "%s: %s\n", k, v)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func allScalars(list []any) bool {
	for _, e := range list {
		if scalar(e) == "" {
			return false
		}
	}
	return len(list) > 0
}

// scalar renders strings, numbers and bools; other values give "".
func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(t), " ")
	case float64, bool:
		return fmt.Sprint(t)
	}
	return ""
}
