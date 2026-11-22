package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// deskMode is the tab shown by the agent desk.
type deskMode int

const (
	deskInbox    deskMode = iota // I: questions awaiting the user
	deskPipeline                 // W: live in-progress work
	deskMetrics                  // M: slice metrics per epic
	deskSignoff                  // D: closed beads awaiting sign-off
	deskModeCount
)

func (d deskMode) title() string {
	switch d {
	case deskPipeline:
		return "Pipeline"
	case deskMetrics:
		return "Slice metrics"
	case deskSignoff:
		return "Sign-off"
	default:
		return "Inbox"
	}
}

// deskActionKind is what the parent must do after a desk key.
type deskActionKind int

const (
	deskActionNone      deskActionKind = iota
	deskActionClose                    // leave the desk
	deskActionAnswer                   // post Comment on IssueID and remove RemoveLabel
	deskActionOpenURL                  // open URL in the browser
	deskActionShowIssue                // select IssueID in the main list
	deskActionStatus                   // show Status in the status bar
	deskActionApprove                  // sign off IssueID
	deskActionDispute                  // dispute IssueID with the note in Comment
)

// deskAction is the result of a key the desk handled.
type deskAction struct {
	Kind        deskActionKind
	IssueID     string
	Comment     string
	RemoveLabel string
	URL         string
	Status      string
}

// deskRow is one inbox display line: a group header or an item.
type deskRow struct {
	group *agentdesk.InboxGroup
	item  int // index into items; -1 for a header
}

// AgentDeskModel is the agent desk: the inbox of beads awaiting the user
// (answer in place, open links), the live pipeline of claimed work, and the
// per-epic slice metrics. It owns no tracker access; answers and link opens
// come back to the parent as deskActions.
type AgentDeskModel struct {
	theme         Theme
	mode          deskMode
	width, height int

	inbox    agentdesk.Inbox
	items    []agentdesk.InboxItem
	rows     []deskRow
	issues   map[string]*model.Issue
	cursor   int // index into items
	scroll   int // first visible row
	answered map[string]string

	answering    bool
	answerCursor int // index into the selected item's options; len(options) is "type an answer"
	typing       bool
	noteFor      *agentdesk.Option // option the typed text annotates, nil for a free answer
	input        textinput.Model

	pipeline   agentdesk.Pipeline
	pipeCursor int

	metrics     agentdesk.SliceReport
	metCursor   int
	generatedAt time.Time

	signoffs   agentdesk.Signoffs
	signItems  []agentdesk.SignoffItem
	signRows   []signRow
	signCursor int
	signScroll int
	signDone   map[string]string // id → "approved"/"disputed" while the write is in flight
	disputing  bool
}

// NewAgentDeskModel creates an empty desk.
func NewAgentDeskModel(theme Theme) AgentDeskModel {
	ti := textinput.New()
	ti.Prompt = "Decision: "
	ti.Placeholder = "type the answer, enter posts, esc goes back"
	ti.CharLimit = 2000
	return AgentDeskModel{theme: theme, input: ti, answered: map[string]string{}, signDone: map[string]string{}}
}

// SetData rebuilds every tab from issues at now, keeping the inbox cursor on
// the same bead when it is still listed.
func (d *AgentDeskModel) SetData(issues []model.Issue, now time.Time) {
	selected := d.SelectedIssueID()
	d.generatedAt = now
	d.issues = make(map[string]*model.Issue, len(issues))
	for i := range issues {
		d.issues[issues[i].ID] = &issues[i]
	}
	d.inbox = agentdesk.BuildInbox(issues, nil)
	d.items = d.inbox.Items()
	d.rows = d.rows[:0]
	next := 0
	for gi := range d.inbox.Groups {
		g := &d.inbox.Groups[gi]
		d.rows = append(d.rows, deskRow{group: g, item: -1})
		for range g.Items {
			d.rows = append(d.rows, deskRow{group: g, item: next})
			next++
		}
	}
	// Answers already reflected in the data no longer need their marker.
	listed := map[string]bool{}
	for _, it := range d.items {
		listed[it.ID] = true
	}
	for id := range d.answered {
		if !listed[id] {
			delete(d.answered, id)
		}
	}
	d.cursor = 0
	for i, it := range d.items {
		if it.ID == selected {
			d.cursor = i
			break
		}
	}
	d.clampInbox()
	if d.answering && d.SelectedIssueID() != selected {
		d.stopAnswering()
	}

	d.pipeline = agentdesk.BuildPipeline(issues, now, 0)
	d.pipeCursor = min(d.pipeCursor, max(0, len(d.pipeline.Rows)-1))
	d.metrics = agentdesk.BuildSliceReport(issues)
	d.metCursor = min(d.metCursor, max(0, len(d.metrics.Epics)-1))
	d.setSignoffs(agentdesk.BuildSignoffs(issues), selected)
}

// SetSize records the body area.
func (d *AgentDeskModel) SetSize(width, height int) {
	d.width, d.height = width, height
	d.input.Width = max(10, width-len(d.input.Prompt)-4)
	d.clampInbox()
	d.clampSignoff()
}

// SetMode switches tabs.
func (d *AgentDeskModel) SetMode(mode deskMode) {
	d.mode = mode
	d.stopAnswering()
	d.stopDisputing()
}

// Mode returns the current tab.
func (d AgentDeskModel) Mode() deskMode { return d.mode }

// Typing reports whether the answer text input is capturing keys.
func (d AgentDeskModel) Typing() bool { return (d.answering && d.typing) || d.disputing }

// Answering reports whether the answer panel is open.
func (d AgentDeskModel) Answering() bool { return d.answering }

// SelectedIssueID returns the bead under the cursor of the current tab.
func (d AgentDeskModel) SelectedIssueID() string {
	switch d.mode {
	case deskPipeline:
		if d.pipeCursor >= 0 && d.pipeCursor < len(d.pipeline.Rows) {
			return d.pipeline.Rows[d.pipeCursor].ID
		}
	case deskMetrics:
		if d.metCursor >= 0 && d.metCursor < len(d.metrics.Epics) {
			return d.metrics.Epics[d.metCursor].Epic
		}
	case deskSignoff:
		if it := d.selectedSignoff(); it != nil {
			return it.ID
		}
	default:
		if it := d.selectedItem(); it != nil {
			return it.ID
		}
	}
	return ""
}

func (d *AgentDeskModel) selectedItem() *agentdesk.InboxItem {
	if d.cursor >= 0 && d.cursor < len(d.items) {
		return &d.items[d.cursor]
	}
	return nil
}

// MarkAnswered records that an answer was posted for id, so the row shows it
// until the reload drops the bead from the inbox.
func (d *AgentDeskModel) MarkAnswered(id, summary string) {
	d.answered[id] = summary
}

// ClearAnswered drops the answered marker, e.g. when the write failed.
func (d *AgentDeskModel) ClearAnswered(id string) {
	delete(d.answered, id)
}

// StatusLine summarises the current tab for the status bar.
func (d AgentDeskModel) StatusLine() string {
	switch d.mode {
	case deskPipeline:
		return fmt.Sprintf("Pipeline: %d in progress • %d stale • enter shows bead • tab/I/M switch • q closes", d.pipeline.Total, d.pipeline.Stale)
	case deskMetrics:
		return fmt.Sprintf("Slice metrics: %d slices in %d epics • %d escaped bugs • tab/I/W/D switch • q closes", d.metrics.Total.Slices, len(d.metrics.Epics), d.metrics.Total.EscapedBugs)
	case deskSignoff:
		return fmt.Sprintf("Sign-off: %d closed beads await you • a approve • d dispute • s skip • 1-9 open link • q closes", d.signoffs.Total)
	default:
		return fmt.Sprintf("Inbox: %d awaiting you • %d ready to judge • enter answers • 1-9 open link • tab/W/M switch • q closes", d.inbox.Total, d.inbox.Ready)
	}
}

// Update handles a key. It reports whether the key was consumed and what the
// parent must do; unconsumed keys fall through to the global handlers.
func (d *AgentDeskModel) Update(msg tea.KeyMsg) (deskAction, tea.Cmd, bool) {
	if d.answering {
		action, cmd := d.updateAnswer(msg)
		return action, cmd, true
	}
	if d.disputing {
		action, cmd := d.updateDispute(msg)
		return action, cmd, true
	}
	key := msg.String()
	if d.mode == deskSignoff {
		if action, handled := d.updateSignoff(key); handled {
			return action, nil, true
		}
	}
	switch key {
	case "q", "esc":
		return deskAction{Kind: deskActionClose}, nil, true
	case "tab":
		d.SetMode((d.mode + 1) % deskModeCount)
		return deskAction{}, nil, true
	case "shift+tab":
		d.SetMode((d.mode + deskModeCount - 1) % deskModeCount)
		return deskAction{}, nil, true
	case "D":
		d.SetMode(deskSignoff)
		return deskAction{}, nil, true
	case "I":
		d.SetMode(deskInbox)
		return deskAction{}, nil, true
	case "W":
		d.SetMode(deskPipeline)
		return deskAction{}, nil, true
	case "M":
		d.SetMode(deskMetrics)
		return deskAction{}, nil, true
	case "j", "down":
		d.move(1)
	case "k", "up":
		d.move(-1)
	case "ctrl+d", "pgdown":
		d.move(max(1, d.listHeight()/2))
	case "ctrl+u", "pgup":
		d.move(-max(1, d.listHeight()/2))
	case "g", "home":
		d.move(-1 << 30)
	case "G", "end":
		d.move(1 << 30)
	case "enter":
		if d.mode == deskInbox {
			return d.startAnswering(), nil, true
		}
		if id := d.SelectedIssueID(); id != "" {
			return deskAction{Kind: deskActionShowIssue, IssueID: id}, nil, true
		}
	case "v":
		if id := d.SelectedIssueID(); id != "" {
			return deskAction{Kind: deskActionShowIssue, IssueID: id}, nil, true
		}
	case "o":
		if d.mode == deskInbox {
			return d.openLink(-1), nil, true
		}
		return deskAction{}, nil, false
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if d.mode == deskInbox {
			n, _ := strconv.Atoi(key)
			return d.openLink(n - 1), nil, true
		}
		return deskAction{}, nil, false
	default:
		return deskAction{}, nil, false
	}
	return deskAction{}, nil, true
}

func (d *AgentDeskModel) move(delta int) {
	switch d.mode {
	case deskPipeline:
		d.pipeCursor = clampInt(d.pipeCursor+delta, 0, len(d.pipeline.Rows)-1)
	case deskMetrics:
		d.metCursor = clampInt(d.metCursor+delta, 0, len(d.metrics.Epics)-1)
	case deskSignoff:
		d.signCursor = clampInt(d.signCursor+delta, 0, len(d.signItems)-1)
		d.clampSignoff()
	default:
		d.cursor = clampInt(d.cursor+delta, 0, len(d.items)-1)
		d.clampInbox()
	}
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(hi, v))
}

// openLink opens link n of the selected item; n < 0 opens the first
// judgeable link, else the first link.
func (d *AgentDeskModel) openLink(n int) deskAction {
	it := d.selectedItem()
	if it == nil {
		return deskAction{}
	}
	if len(it.Links) == 0 {
		return deskAction{Kind: deskActionStatus, Status: it.ID + " has no links"}
	}
	if n < 0 {
		n = 0
		for i, l := range it.Links {
			if l.Judgeable() {
				n = i
				break
			}
		}
	}
	if n >= len(it.Links) {
		return deskAction{Kind: deskActionStatus, Status: fmt.Sprintf("%s has %d links", it.ID, len(it.Links))}
	}
	return deskAction{Kind: deskActionOpenURL, URL: it.Links[n].URL, IssueID: it.ID}
}

func (d *AgentDeskModel) startAnswering() deskAction {
	it := d.selectedItem()
	if it == nil {
		return deskAction{}
	}
	d.answering = true
	d.typing = false
	d.noteFor = nil
	d.answerCursor = 0
	if r := agentdesk.Recommended(it.Options); r >= 0 {
		d.answerCursor = r
	}
	if len(it.Options) == 0 {
		d.beginTyping(nil)
	}
	return deskAction{}
}

func (d *AgentDeskModel) beginTyping(note *agentdesk.Option) {
	d.typing = true
	d.noteFor = note
	d.input.Reset()
	if note != nil {
		d.input.Prompt = "Decision: (" + note.Key + ") … — "
	} else {
		d.input.Prompt = "Decision: "
	}
	d.input.Focus()
}

func (d *AgentDeskModel) stopAnswering() {
	d.answering = false
	d.typing = false
	d.noteFor = nil
	d.input.Blur()
	d.input.Reset()
}

func (d *AgentDeskModel) updateAnswer(msg tea.KeyMsg) (deskAction, tea.Cmd) {
	it := d.selectedItem()
	if it == nil {
		d.stopAnswering()
		return deskAction{}, nil
	}
	key := msg.String()
	if d.typing {
		switch key {
		case "esc":
			if len(it.Options) == 0 {
				d.stopAnswering()
			} else {
				d.typing = false
				d.input.Blur()
			}
			return deskAction{}, nil
		case "enter":
			text := strings.TrimSpace(d.input.Value())
			if text == "" {
				return deskAction{Kind: deskActionStatus, Status: "Type an answer first (esc goes back)"}, nil
			}
			return d.post(agentdesk.DecisionComment(d.noteFor, text)), nil
		}
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		return deskAction{}, cmd
	}
	n := len(it.Options)
	switch {
	case key == "esc" || key == "q":
		d.stopAnswering()
	case key == "j" || key == "down":
		d.answerCursor = min(n, d.answerCursor+1)
	case key == "k" || key == "up":
		d.answerCursor = max(0, d.answerCursor-1)
	case key == "t" || key == "/":
		d.beginTyping(nil)
	case key == "n":
		if d.answerCursor < n {
			opt := it.Options[d.answerCursor]
			d.beginTyping(&opt)
		}
	case key == "enter":
		if d.answerCursor >= n {
			d.beginTyping(nil)
			return deskAction{}, nil
		}
		opt := it.Options[d.answerCursor]
		return d.post(agentdesk.DecisionComment(&opt, "")), nil
	case len(key) == 1 && key[0] >= 'a' && key[0] < 'a'+byte(n):
		opt := it.Options[key[0]-'a']
		return d.post(agentdesk.DecisionComment(&opt, "")), nil
	}
	return deskAction{}, nil
}

// post closes the answer panel and asks the parent to write the answer. The
// cursor moves to the next bead so a batch can be cleared in one sitting.
func (d *AgentDeskModel) post(comment string) deskAction {
	it := d.selectedItem()
	action := deskAction{Kind: deskActionAnswer, IssueID: it.ID, Comment: comment, RemoveLabel: it.AnswerLabel()}
	d.stopAnswering()
	if d.cursor < len(d.items)-1 {
		d.cursor++
		d.clampInbox()
	}
	return action
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// previewHeight is the inbox's lower pane, sized to the body.
func (d AgentDeskModel) previewHeight() int {
	h := d.height * 2 / 5
	return clampInt(h, 6, 18)
}

// listHeight is the number of rows the upper table shows.
func (d AgentDeskModel) listHeight() int {
	h := d.height - 2 // tab bar + header
	switch d.mode {
	case deskInbox:
		h -= d.previewHeight() + 1
	case deskSignoff:
		h -= d.signoffPreviewHeight() + 1
	}
	return max(1, h)
}

// cursorRow maps the item cursor to its display row.
func (d AgentDeskModel) cursorRow() int {
	for i, r := range d.rows {
		if r.item == d.cursor {
			return i
		}
	}
	return 0
}

func (d *AgentDeskModel) clampInbox() {
	d.cursor = clampInt(d.cursor, 0, len(d.items)-1)
	row := d.cursorRow()
	rows := d.listHeight()
	if row < d.scroll {
		d.scroll = row
	}
	if row >= d.scroll+rows {
		d.scroll = row - rows + 1
	}
	// Keep the group header visible above its first item when there is room.
	if d.scroll == row && row > 0 && d.rows[row-1].item < 0 && rows >= 2 {
		d.scroll = row - 1
	}
	d.scroll = max(0, d.scroll)
}

// View renders the current tab.
func (d AgentDeskModel) View() string {
	width := max(40, d.width)
	var b strings.Builder
	b.WriteString(d.renderTabs(width))
	b.WriteString("\n")
	switch d.mode {
	case deskPipeline:
		b.WriteString(d.renderPipeline(width))
	case deskMetrics:
		b.WriteString(d.renderMetrics(width))
	case deskSignoff:
		b.WriteString(d.renderSignoff(width))
	default:
		b.WriteString(d.renderInbox(width))
	}
	return b.String()
}

func (d AgentDeskModel) renderTabs(width int) string {
	var parts []string
	for _, m := range []struct {
		mode deskMode
		key  string
	}{{deskInbox, "I"}, {deskPipeline, "W"}, {deskMetrics, "M"}, {deskSignoff, "D"}} {
		label := fmt.Sprintf(" %s %s ", m.key, m.mode.title())
		switch m.mode {
		case deskInbox:
			label = fmt.Sprintf(" %s Inbox (%d) ", m.key, d.inbox.Total)
		case deskPipeline:
			label = fmt.Sprintf(" %s Pipeline (%d", m.key, d.pipeline.Total)
			if d.pipeline.Stale > 0 {
				label += fmt.Sprintf(", %d stale", d.pipeline.Stale)
			}
			label += ") "
		case deskSignoff:
			label = fmt.Sprintf(" %s Sign-off (%d) ", m.key, d.signoffs.Total)
		}
		if m.mode == d.mode {
			label = d.theme.Selected.Render(label)
		} else {
			label = d.theme.MutedText.Render(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func (d AgentDeskModel) renderInbox(width int) string {
	var b strings.Builder
	if len(d.items) == 0 {
		b.WriteString(d.theme.MutedText.Render("Nothing awaits you: no open bead carries needs-decision or needs-design."))
		b.WriteString("\n")
		return b.String()
	}
	idW, labelW, linksW := 14, 9, 7
	for _, it := range d.items {
		idW = max(idW, len(it.ID)+2)
	}
	idW = min(idW, 22)
	titleW := max(10, width-idW-labelW-linksW-6)
	header := padRight("", 2) + padRight("ID", idW) + padRight("Title", titleW) + " " + padRight("Label", labelW) + padRight("Links", linksW)
	b.WriteString(d.theme.Header.Render(truncate(header, width)))
	b.WriteString("\n")

	rows := d.listHeight()
	end := min(len(d.rows), d.scroll+rows)
	for i := d.scroll; i < end; i++ {
		r := d.rows[i]
		if r.item < 0 {
			title := r.group.EpicTitle
			name := r.group.Epic
			if name == agentdesk.NoEpic {
				name, title = "(no epic)", ""
			}
			line := fmt.Sprintf("▾ %s %s  · %d", name, title, len(r.group.Items))
			if r.group.Ready > 0 {
				line += fmt.Sprintf(" · %d ready to judge", r.group.Ready)
			}
			b.WriteString(d.theme.SecondaryText.Render(truncate(line, width)))
			b.WriteString("\n")
			continue
		}
		it := d.items[r.item]
		mark := "  "
		switch {
		case d.answered[it.ID] != "":
			mark = "✓ "
		case it.ReadyToJudge:
			mark = "★ "
		}
		label := "decision"
		if it.Label == agentdesk.LabelNeedsDesign {
			label = "design"
		}
		links := ""
		if n := it.LinkCount(); n > 0 {
			links = fmt.Sprintf("🔗%d", n)
		}
		title := it.Title
		switch {
		case d.answered[it.ID] != "":
			title = "answered: " + d.answered[it.ID]
		case it.ReadyToJudge:
			title += "  [ready to judge]"
		case it.WaitingOn != "":
			title += "  (waiting on: " + it.WaitingOn + ")"
		}
		line := mark + padRight(truncate(it.ID, idW-1), idW) + padRight(truncate(title, titleW), titleW) + " " + padRight(label, labelW) + padRight(links, linksW)
		if r.item == d.cursor {
			line = d.theme.Selected.Render(truncate(line, width-d.theme.Selected.GetHorizontalFrameSize()))
		} else if d.answered[it.ID] != "" {
			line = d.theme.MutedText.Render(truncate(line, width))
		} else {
			line = truncate(line, width)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	for i := end - d.scroll; i < rows; i++ {
		b.WriteString("\n")
	}
	b.WriteString(d.theme.MutedText.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(d.renderPreview(width))
	return b.String()
}

// renderPreview shows the selected bead: options (interactive while
// answering), what it waits on, numbered links and the description head.
func (d AgentDeskModel) renderPreview(width int) string {
	it := d.selectedItem()
	if it == nil {
		return ""
	}
	budget := d.previewHeight()
	var lines []string
	add := func(s string) {
		for _, l := range strings.Split(s, "\n") {
			lines = append(lines, l)
		}
	}
	// wrapped splits s to width, indenting continuation lines by indent.
	// (wrapText collapses whitespace, so the first-line prefix is added after.)
	wrapped := func(first, s string, w, indent int) []string {
		parts := strings.Split(wrapText(s, max(10, w-indent)), "\n")
		parts[0] = first + parts[0]
		for i := 1; i < len(parts); i++ {
			parts[i] = strings.Repeat(" ", indent) + parts[i]
		}
		return parts
	}
	for _, l := range wrapped(it.ID+"  ", it.Title, width, len(it.ID)+2) {
		add(d.theme.PrimaryBold.Render(l))
	}
	if it.WaitingOn != "" {
		for _, l := range wrapped("Waiting on: ", it.WaitingOn, width, 12) {
			add(l)
		}
	}
	if len(it.Options) > 0 {
		for i, o := range it.Options {
			text := o.Text
			prefix := "  "
			selected := d.answering && !d.typing && d.answerCursor == i
			w := width
			if selected {
				prefix = "▸ "
				w -= d.theme.Selected.GetHorizontalFrameSize()
			}
			indent := 2 + len(o.Key) + 3 // under the text, past "(k) "
			optLines := wrapped(prefix+"("+o.Key+") ", text, w, indent)
			if o.Recommended {
				const mark = "  ★ recommended"
				last := len(optLines) - 1
				if len([]rune(optLines[last]+mark)) <= w {
					optLines[last] += mark
				} else {
					optLines = append(optLines, strings.Repeat(" ", indent)+strings.TrimSpace(mark))
				}
			}
			for _, l := range optLines {
				if selected {
					l = d.theme.Selected.Render(padRight(l, w))
				}
				add(l)
			}
		}
		if d.answering && !d.typing {
			other := "  ✎ type an answer"
			if d.answerCursor == len(it.Options) {
				add(d.theme.Selected.Render(truncate("▸ ✎ type an answer", width-d.theme.Selected.GetHorizontalFrameSize())))
			} else {
				add(other)
			}
		}
	} else if it.IsDecision() {
		add(d.theme.MutedText.Render("No options found in the description; enter types a free answer."))
	}
	if d.answering && d.typing {
		add(d.input.View())
	}
	if d.answering {
		hint := "a-" + string(rune('a'+max(0, len(it.Options)-1))) + " or enter posts • n adds a note to the option • t types • esc cancels"
		if len(it.Options) == 0 || d.typing {
			hint = "enter posts • esc cancels"
		}
		add(d.theme.MutedText.Render(truncate(hint+" • removes "+it.AnswerLabel(), width)))
	}
	for i, l := range it.Links {
		if i >= 9 {
			add(d.theme.MutedText.Render(fmt.Sprintf("   … %d more", len(it.Links)-9)))
			break
		}
		tag := l.Kind
		if l.Source == agentdesk.SourceComment {
			tag += " (comment)"
		}
		add(truncate(fmt.Sprintf("[%d] %-18s %s", i+1, tag, l.URL), width))
	}
	if issue := d.issues[it.ID]; issue != nil && len(lines) < budget {
		desc := wrapText(strings.Join(strings.Fields(issue.Description), " "), width)
		for _, l := range strings.Split(desc, "\n") {
			if len(lines) >= budget {
				break
			}
			add(d.theme.MutedText.Render(l))
		}
	}
	if len(lines) > budget {
		lines = lines[:budget]
	}
	return strings.Join(lines, "\n") + "\n"
}

func (d AgentDeskModel) renderPipeline(width int) string {
	var b strings.Builder
	p := d.pipeline
	summary := fmt.Sprintf("%d in progress · %d stale (no activity for %s, or lease expired)", p.Total, p.Stale, p.StaleAfter)
	var stages []string
	for _, s := range append(append([]string{}, agentdesk.Stages...), "unstaged") {
		if n := p.ByStage[s]; n > 0 {
			stages = append(stages, fmt.Sprintf("%s %d", s, n))
		}
	}
	if len(stages) > 0 {
		summary += " · " + strings.Join(stages, " · ")
	}
	if len(p.Rows) == 0 {
		b.WriteString(d.theme.MutedText.Render("No bead is in progress."))
		b.WriteString("\n")
		return b.String()
	}
	stageW, idW, sessW, ageW := 8, 14, 14, 9
	for _, r := range p.Rows {
		idW = max(idW, len(r.ID)+2)
	}
	idW = min(idW, 22)
	flagW := 30
	titleW := max(10, width-stageW-idW-sessW-ageW-flagW-d.theme.Selected.GetHorizontalFrameSize())
	header := padRight("Stage", stageW) + padRight("ID", idW) + padRight("Title", titleW) + " " + padRight("Session", sessW) + padRight("In stage", ageW) + "Flag"
	b.WriteString(d.theme.Header.Render(truncate(header, width)))
	b.WriteString("\n")
	rows := max(1, d.listHeight()-1)
	start := max(0, d.pipeCursor-rows+1)
	end := min(len(p.Rows), start+rows)
	for i := start; i < end; i++ {
		r := p.Rows[i]
		stage := r.Stage
		if stage == "" {
			stage = "—"
		}
		who := r.Session
		if who == "" {
			who = r.Holder
		}
		age := r.StageAge
		if age == "" {
			age = "?"
		}
		flag := ""
		if r.Stale {
			flag = "⚠ " + r.StaleReason
		} else if r.LeaseExpiresAt != nil {
			flag = "lease " + agentdesk.FormatAge(r.LeaseExpiresAt.Sub(d.generatedAt)) + " left"
		}
		line := padRight(stage, stageW) + padRight(truncate(r.ID, idW-1), idW) + padRight(truncate(r.Title, titleW), titleW) + " " +
			padRight(truncate(who, sessW-1), sessW) + padRight(age, ageW) + truncate(flag, flagW)
		switch {
		case i == d.pipeCursor:
			line = d.theme.Selected.Render(truncate(line, width-d.theme.Selected.GetHorizontalFrameSize()))
		case r.Stale:
			line = d.theme.PriorityUpArrow.Render(truncate(line, width))
		default:
			line = truncate(line, width)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(d.theme.MutedText.Render(truncate(summary, width)))
	b.WriteString("\n")
	return b.String()
}

func (d AgentDeskModel) renderMetrics(width int) string {
	var b strings.Builder
	r := d.metrics
	if r.Total.Slices == 0 {
		b.WriteString(d.theme.MutedText.Render("No slice metrics: no bead carries slice_outcome metadata (tools/slice_log.ps1)."))
		b.WriteString("\n")
		return b.String()
	}
	cols := []struct {
		name string
		w    int
	}{{"Epic", 14}, {"Title", 0}, {"Slices", 7}, {"✓/✗", 7}, {"Rev/sl", 7}, {"1st ok", 7}, {"Tests", 7}, {"Red", 5}, {"Min/sl", 7}, {"Tokens k (cod/rev/oth)", 23}, {"Esc", 4}}
	fixed := 0
	for _, c := range cols {
		fixed += c.w
	}
	// Leave room for the selected row's frame so its last column survives.
	cols[1].w = max(8, width-fixed-d.theme.Selected.GetHorizontalFrameSize())
	row := func(cells []string) string {
		var sb strings.Builder
		for i, c := range cells {
			sb.WriteString(padRight(truncate(c, cols[i].w-1), cols[i].w))
		}
		return sb.String()
	}
	var head []string
	for _, c := range cols {
		head = append(head, c.name)
	}
	b.WriteString(d.theme.Header.Render(truncate(row(head), width)))
	b.WriteString("\n")
	cells := func(m agentdesk.SliceMetrics) []string {
		epic, title := m.Epic, m.EpicTitle
		if epic == agentdesk.NoEpic {
			epic = "(no epic)"
		}
		k := func(v int64) string { return strconv.FormatInt((v+500)/1000, 10) }
		tokens := k(m.CoderTokens) + "/" + k(m.ReviewerTokens) + "/" + k(m.OtherTokens)
		if m.TokensUnknown > 0 {
			tokens += fmt.Sprintf(" (%d?)", m.TokensUnknown)
		}
		return []string{
			epic, title, strconv.Itoa(m.Slices), fmt.Sprintf("%d/%d", m.Committed, m.Stopped),
			fmt.Sprintf("%.1f", m.AvgReviewRounds()), fmt.Sprintf("%d%%", int(100*float64(m.FirstPass)/float64(max(1, m.Slices))+0.5)),
			strconv.Itoa(m.TestRuns), strconv.Itoa(m.TestRedRuns), fmt.Sprintf("%.0f", m.AvgWallMinutes()), tokens, strconv.Itoa(m.EscapedBugs),
		}
	}
	rows := max(1, d.listHeight()-2)
	start := max(0, d.metCursor-rows+1)
	end := min(len(r.Epics), start+rows)
	for i := start; i < end; i++ {
		line := row(cells(r.Epics[i]))
		if i == d.metCursor {
			line = d.theme.Selected.Render(truncate(line, width-d.theme.Selected.GetHorizontalFrameSize()))
		} else {
			line = truncate(line, width)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	total := cells(r.Total)
	total[0], total[1] = "ALL", fmt.Sprintf("%d epics", len(r.Epics))
	b.WriteString(d.theme.PrimaryBold.Render(truncate(row(total), width)))
	b.WriteString("\n")
	if len(r.UnmatchedEscapes) > 0 {
		var names []string
		for _, e := range r.UnmatchedEscapes {
			names = append(names, e.Bug+"→"+e.Cause)
		}
		b.WriteString(d.theme.MutedText.Render(truncate("caused_by names no logged slice: "+strings.Join(names, ", "), width)))
		b.WriteString("\n")
	}
	return b.String()
}
