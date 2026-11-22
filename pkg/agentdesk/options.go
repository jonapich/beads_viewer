package agentdesk

import (
	"regexp"
	"sort"
	"strings"
)

// Option is one answer offered by an awaiting bead.
type Option struct {
	Key         string `json:"key"` // lower-case letter: a, b, c…
	Text        string `json:"text"`
	Recommended bool   `json:"recommended,omitempty"`
}

// maxOptions bounds the letters a..h an answer can be picked with.
const maxOptions = 8

var (
	// "Options:", "Options (pick one):", "Option:" — the header that starts the list.
	optionsHeader = regexp.MustCompile(`(?i)\boptions?\b[^:\n]{0,40}:`)

	// List-line forms, tried in order. Each yields letter, marker extra, text.
	listLineForms = []*regexp.Regexp{
		// "(a) text", "(a, recommended) text"
		regexp.MustCompile(`^\s*(?:[-*+]\s+)?\(([A-Ha-h])(?:\s*,\s*([^)]*))?\)\s*()(.*)$`),
		// "**A** text"
		regexp.MustCompile(`^\s*(?:[-*+]\s+)?\*\*([A-Ha-h])[).:]?\*\*()[\s.:)]*()(.*)$`),
		// "A (recommended, built meanwhile): text"
		regexp.MustCompile(`^\s*(?:[-*+]\s+)?([A-Ha-h])\s*\(([^)]*)\)\s*[:.)-]?()\s*(.*)$`),
		// "A) text", "a. text", "B: text"
		regexp.MustCompile(`^\s*(?:[-*+]\s+)?([A-Ha-h])[).:]()()\s*(.*)$`),
	}
	// A bullet without a letter: "- text".
	bulletLine = regexp.MustCompile(`^\s*[-*+]\s+(.+)$`)

	// Inline marker forms. Paren forms are always recognised; bare capital
	// letters only right after an "Options:" header.
	inlineParen = regexp.MustCompile(`(?:^|[\s;,:])\(([A-Ha-h])(?:\s*,\s*([^)]*))?\)\s*`)
	inlineClose = regexp.MustCompile(`(?:^|[\s;,:])([A-Ha-h])\)\s*`)
	// "A (recommended) keep it" — a capital, then a parenthetical.
	inlineCapParen = regexp.MustCompile(`(?:^|[\s;,.:])([A-H])\s+\(([^)]*)\)\s*:?\s*`)
	// ". B drop it" — a capital after a sentence or list break, then a lower-case word.
	inlineCapWord = regexp.MustCompile(`(?:^|[.;:]\s+|^\s+)([A-H])\s+[a-z]`)

	// The start of the next labelled field after an inline list: ". Files:", ". Built meanwhile:".
	nextField = regexp.MustCompile(`[.;]\s+[A-Z][A-Za-z]*(?:\s+[A-Za-z]+){0,3}:\s`)
	// "(Recommended)" on its own, dropped from the text once flagged.
	bareRecommended = regexp.MustCompile(`(?i)\s*\(\s*recommended\s*\)`)
	recommendedWord = regexp.MustCompile(`(?i)\brecommended\b`)
	// "(a) recommended: seed every trait…" — the option opens with the flag.
	leadingRecommended = regexp.MustCompile(`(?i)^recommended\s*[:,-]\s*`)
	// "RECOMMENDED: A", "Recommended: (b)" naming a letter after the list.
	recommendedLetter = regexp.MustCompile(`(?i)\brecommended\s*[:=-]\s*\(?([a-h])\)?(?:[\s.,;)]|$)`)
	// "Question: keep the wait or skip it?"
	questionLine = regexp.MustCompile(`(?i)\bquestion\s*:\s*([^?\n]+)\?`)
)

// ParseOptions extracts the answers a description offers.
//
// It understands the forms beads use: an inline "Options: (a, recommended)
// …; (b) …" or "Options: A (recommended) …. B …" list, a lettered list on
// its own lines (after an "Options:" header or anywhere), bullets under an
// "Options:" header, a paren-lettered "(a) …, or (b) …" list on any line, and
// a two-way "Options: X (recommended) or Y" or "Question: X or Y?".
// "(Recommended)" or "recommended" in a marker or a parenthetical, or a later
// "Recommended: B", flags the default. It returns nil when the text offers no
// recognisable options.
func ParseOptions(text string) []Option {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	opts := parseOptions(text)
	if opts != nil && Recommended(opts) < 0 {
		if m := recommendedLetter.FindStringSubmatch(text); m != nil {
			idx := int(strings.ToLower(m[1])[0] - 'a')
			if idx < len(opts) {
				opts[idx].Recommended = true
			}
		}
	}
	return opts
}

func parseOptions(text string) []Option {
	if loc := optionsHeader.FindStringIndex(text); loc != nil {
		if opts := parseHeaded(text[loc[1]:]); opts != nil {
			return opts
		}
	}
	if opts := parseLetteredLines(strings.Split(text, "\n"), false); opts != nil {
		return opts
	}
	for _, line := range strings.Split(text, "\n") {
		if opts := parseInline(line, false); opts != nil {
			return opts
		}
	}
	if m := questionLine.FindStringSubmatch(text); m != nil {
		return parseOr(m[1])
	}
	return nil
}

func parseHeaded(rest string) []Option {
	firstLine, after, _ := strings.Cut(rest, "\n")
	if strings.TrimSpace(firstLine) == "" {
		// Block layout: the list follows on the next lines.
		lines := strings.Split(after, "\n")
		if opts := parseLetteredLines(lines, true); opts != nil {
			return opts
		}
		return parseBullets(lines)
	}
	if opts := parseInline(firstLine, true); opts != nil {
		return opts
	}
	// The header line may hold the first lettered item, the rest below it.
	if opts := parseLetteredLines(strings.Split(rest, "\n"), true); opts != nil {
		return opts
	}
	return parseOr(firstLine)
}

func matchListLine(line string) (letter, extra, text string, ok bool) {
	for _, re := range listLineForms {
		if m := re.FindStringSubmatch(line); m != nil {
			return strings.ToLower(m[1]), m[2], m[4], true
		}
	}
	return "", "", "", false
}

// parseLetteredLines reads the first run of consecutive lettered lines that
// starts at A. With headed set, the run must start on the first non-blank line.
func parseLetteredLines(lines []string, headed bool) []Option {
	var opts []Option
	for _, line := range lines {
		letter, extra, text, ok := matchListLine(line)
		if !ok {
			if len(opts) > 0 {
				if strings.TrimSpace(line) == "" || !isContinuation(line) {
					break
				}
				opts[len(opts)-1].Text += " " + strings.TrimSpace(line)
				continue
			}
			if headed && strings.TrimSpace(line) != "" {
				return nil
			}
			continue
		}
		if letter != string(rune('a'+len(opts))) {
			if len(opts) >= 2 {
				break
			}
			opts = nil
			if letter != "a" {
				continue
			}
		}
		opts = append(opts, newOption(letter, text, extra))
		if len(opts) == maxOptions {
			break
		}
	}
	return finish(opts)
}

func isContinuation(line string) bool {
	return strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")
}

func parseBullets(lines []string) []Option {
	var opts []Option
	for _, line := range lines {
		m := bulletLine.FindStringSubmatch(line)
		if m == nil {
			if len(opts) > 0 || strings.TrimSpace(line) != "" {
				break
			}
			continue
		}
		opts = append(opts, newOption(string(rune('a'+len(opts))), m[1], ""))
		if len(opts) == maxOptions {
			break
		}
	}
	return finish(opts)
}

type inlineMark struct {
	letter, extra string
	start, end    int // the marker's span; the option text starts at end
}

// inlineMarks finds candidate letter markers on one line, earliest first;
// overlapping candidates keep the earliest.
func inlineMarks(line string, bare bool) []inlineMark {
	var marks []inlineMark
	add := func(re *regexp.Regexp, extraGroup int, trimEnd func(m []int) int) {
		for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
			mk := inlineMark{letter: strings.ToLower(line[m[2]:m[3]]), start: m[0], end: m[1]}
			if extraGroup > 0 && m[2*extraGroup] >= 0 {
				mk.extra = line[m[2*extraGroup]:m[2*extraGroup+1]]
			}
			if trimEnd != nil {
				mk.end = trimEnd(m)
			}
			marks = append(marks, mk)
		}
	}
	add(inlineParen, 2, nil)
	add(inlineClose, 0, nil)
	if bare {
		add(inlineCapParen, 2, nil)
		// Keep the lower-case word: the text starts right after the letter.
		add(inlineCapWord, 0, func(m []int) int { return m[3] })
	}
	sort.SliceStable(marks, func(i, j int) bool { return marks[i].start < marks[j].start })
	var out []inlineMark
	for _, mk := range marks {
		if len(out) > 0 && mk.start < out[len(out)-1].end {
			continue
		}
		out = append(out, mk)
	}
	return out
}

// parseInline reads "(a, recommended) foo; (b) bar. Files: …" from one line.
func parseInline(line string, bare bool) []Option {
	var run []inlineMark
	for _, mk := range inlineMarks(line, bare) {
		if mk.letter != string(rune('a'+len(run))) {
			if len(run) >= 2 {
				break
			}
			run = nil
			if mk.letter != "a" {
				continue
			}
		}
		run = append(run, mk)
		if len(run) == maxOptions {
			break
		}
	}
	if len(run) < 2 {
		return nil
	}
	var opts []Option
	for i, mk := range run {
		end := len(line)
		if i+1 < len(run) {
			end = run[i+1].start
		}
		body := line[mk.end:end]
		if i == len(run)-1 {
			if loc := nextField.FindStringIndex(body); loc != nil {
				body = body[:loc[0]]
			}
		}
		opts = append(opts, newOption(mk.letter, body, mk.extra))
	}
	return finish(opts)
}

// parseOr splits "X (recommended …) or Y. Files: …" into two options.
func parseOr(line string) []Option {
	if loc := nextField.FindStringIndex(line); loc != nil {
		line = line[:loc[0]]
	}
	depth, split := 0, -1
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ' ':
			if depth == 0 && strings.HasPrefix(line[i:], " or ") {
				if split >= 0 {
					return nil // more than one "or": too ambiguous to split
				}
				split = i
			}
		}
	}
	if split < 0 {
		return nil
	}
	return finish([]Option{newOption("a", line[:split], ""), newOption("b", line[split+len(" or "):], "")})
}

func newOption(key, text, markerExtra string) Option {
	opt := Option{Key: key}
	if recommendedWord.MatchString(markerExtra) || hasRecommendedParen(text) {
		opt.Recommended = true
	}
	text = bareRecommended.ReplaceAllString(text, "")
	text = strings.Join(strings.Fields(text), " ")
	if loc := leadingRecommended.FindStringIndex(text); loc != nil {
		opt.Recommended = true
		text = text[loc[1]:]
	}
	for {
		trimmed := strings.TrimRight(text, " ;,.")
		trimmed = strings.TrimSuffix(trimmed, " or")
		trimmed = strings.TrimSuffix(trimmed, " and")
		if trimmed == text {
			break
		}
		text = trimmed
	}
	opt.Text = strings.TrimSpace(text)
	return opt
}

// hasRecommendedParen finds "recommended" inside a parenthetical.
func hasRecommendedParen(text string) bool {
	depth := 0
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(':
			if depth == 0 {
				start = i
			}
			depth++
		case ')':
			if depth > 0 {
				depth--
				if depth == 0 && recommendedWord.MatchString(text[start:i]) {
					return true
				}
			}
		}
	}
	return false
}

func finish(opts []Option) []Option {
	if len(opts) < 2 {
		return nil
	}
	for _, o := range opts {
		if o.Text == "" {
			return nil
		}
	}
	return opts
}

// Recommended returns the index of the recommended option, or -1.
func Recommended(opts []Option) int {
	for i, o := range opts {
		if o.Recommended {
			return i
		}
	}
	return -1
}

// DecisionComment is the comment text recording an answer: "Decision: (a)
// <option>" for a picked option, "Decision: <text>" for free text, and the
// option followed by " — <text>" when both are given.
func DecisionComment(opt *Option, free string) string {
	free = strings.TrimSpace(free)
	switch {
	case opt != nil && free != "":
		return "Decision: (" + opt.Key + ") " + opt.Text + " — " + free
	case opt != nil:
		return "Decision: (" + opt.Key + ") " + opt.Text
	default:
		return "Decision: " + free
	}
}
