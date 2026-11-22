package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/agentdesk"
	tea "github.com/charmbracelet/bubbletea"
)

// signRow is one sign-off display line: a group header or an item.
type signRow struct {
	group *agentdesk.SignoffGroup
	item  int // index into signItems; -1 for a header
}

// setSignoffs installs a new sign-off list, keeping the cursor on selected
// when it is still pending.
func (d *AgentDeskModel) setSignoffs(s agentdesk.Signoffs, selected string) {
	d.signoffs = s
	d.signItems = s.Items()
	d.signRows = d.signRows[:0]
	next := 0
	for gi := range d.signoffs.Groups {
		g := &d.signoffs.Groups[gi]
		d.signRows = append(d.signRows, signRow{group: g, item: -1})
		for range g.Items {
			d.signRows = append(d.signRows, signRow{group: g, item: next})
			next++
		}
	}
	listed := map[string]bool{}
	for i, it := range d.signItems {
		listed[it.ID] = true
		if it.ID == selected && d.mode == deskSignoff {
			d.signCursor = i
		}
	}
	for id := range d.signDone {
		if !listed[id] {
			delete(d.signDone, id)
		}
	}
	if d.disputing && (d.selectedSignoff() == nil || d.selectedSignoff().ID != selected) {
		d.stopDisputing()
	}
	d.clampSignoff()
}

func (d *AgentDeskModel) selectedSignoff() *agentdesk.SignoffItem {
	if d.signCursor >= 0 && d.signCursor < len(d.signItems) {
		return &d.signItems[d.signCursor]
	}
	return nil
}

// PendingSignoffs is the number of closed beads awaiting sign-off.
func (d AgentDeskModel) PendingSignoffs() int { return d.signoffs.Total }

// MarkSignedOff records an approve/dispute in flight so the row shows it
// until the reload drops the bead.
func (d *AgentDeskModel) MarkSignedOff(id, state string) { d.signDone[id] = state }

// ClearSignedOff drops the in-flight marker, e.g. when the write failed.
func (d *AgentDeskModel) ClearSignedOff(id string) { delete(d.signDone, id) }

func (d AgentDeskModel) signoffPreviewHeight() int {
	return clampInt(d.height/2, 8, 24)
}

func (d *AgentDeskModel) clampSignoff() {
	d.signCursor = clampInt(d.signCursor, 0, len(d.signItems)-1)
	row := 0
	for i, r := range d.signRows {
		if r.item == d.signCursor {
			row = i
			break
		}
	}
	rows := d.listHeight()
	if d.mode != deskSignoff {
		rows = max(1, d.height-3-d.signoffPreviewHeight())
	}
	if row < d.signScroll {
		d.signScroll = row
	}
	if row >= d.signScroll+rows {
		d.signScroll = row - rows + 1
	}
	if d.signScroll == row && row > 0 && d.signRows[row-1].item < 0 && rows >= 2 {
		d.signScroll = row - 1
	}
	d.signScroll = max(0, d.signScroll)
}

// updateSignoff handles the sign-off tab's own keys.
func (d *AgentDeskModel) updateSignoff(key string) (deskAction, bool) {
	it := d.selectedSignoff()
	switch key {
	case "a":
		if it == nil {
			return deskAction{}, true
		}
		action := deskAction{Kind: deskActionApprove, IssueID: it.ID}
		d.advanceSignoff()
		return action, true
	case "d":
		if it == nil {
			return deskAction{}, true
		}
		d.disputing = true
		d.input.Reset()
		d.input.Prompt = "Dispute note: "
		d.input.Placeholder = "what is wrong; becomes the follow-up bead's description"
		d.input.Focus()
		return deskAction{}, true
	case "s":
		d.advanceSignoff()
		return deskAction{}, true
	case "o", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if it == nil {
			return deskAction{}, true
		}
		n := 0
		if key != "o" {
			n, _ = strconv.Atoi(key)
			n--
		}
		if n >= len(it.Links) {
			return deskAction{Kind: deskActionStatus, Status: fmt.Sprintf("%s has %d links", it.ID, len(it.Links))}, true
		}
		return deskAction{Kind: deskActionOpenURL, URL: it.Links[n].URL, IssueID: it.ID}, true
	}
	return deskAction{}, false
}

func (d *AgentDeskModel) advanceSignoff() {
	if d.signCursor < len(d.signItems)-1 {
		d.signCursor++
		d.clampSignoff()
	}
}

func (d *AgentDeskModel) stopDisputing() {
	if !d.disputing {
		return
	}
	d.disputing = false
	d.input.Blur()
	d.input.Reset()
	d.input.Prompt = "Decision: "
	d.input.Placeholder = "type the answer, enter posts, esc goes back"
}

func (d *AgentDeskModel) updateDispute(msg tea.KeyMsg) (deskAction, tea.Cmd) {
	it := d.selectedSignoff()
	if it == nil {
		d.stopDisputing()
		return deskAction{}, nil
	}
	switch msg.String() {
	case "esc":
		d.stopDisputing()
		return deskAction{}, nil
	case "enter":
		note := strings.TrimSpace(d.input.Value())
		if note == "" {
			return deskAction{Kind: deskActionStatus, Status: "Type what is wrong first (esc cancels)"}, nil
		}
		action := deskAction{Kind: deskActionDispute, IssueID: it.ID, Comment: note}
		d.stopDisputing()
		d.advanceSignoff()
		return action, nil
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return deskAction{}, cmd
}

func (d AgentDeskModel) renderSignoff(width int) string {
	var b strings.Builder
	if len(d.signItems) == 0 {
		b.WriteString(d.theme.MutedText.Render("Nothing to sign off: no closed bead carries needs-signoff."))
		b.WriteString("\n")
		return b.String()
	}
	idW := 14
	for _, it := range d.signItems {
		idW = max(idW, len(it.ID)+2)
	}
	idW = min(idW, 22)
	closedW, callsW := 12, 8
	titleW := max(10, width-idW-closedW-callsW-2-d.theme.Selected.GetHorizontalFrameSize())
	header := "  " + padRight("ID", idW) + padRight("Title", titleW) + padRight("Closed", closedW) + "Calls"
	b.WriteString(d.theme.Header.Render(truncate(header, width)))
	b.WriteString("\n")
	rows := d.listHeight()
	end := min(len(d.signRows), d.signScroll+rows)
	for i := d.signScroll; i < end; i++ {
		r := d.signRows[i]
		if r.item < 0 {
			name, title := r.group.Epic, r.group.EpicTitle
			if name == agentdesk.NoEpic {
				name, title = "(no epic)", ""
			}
			b.WriteString(d.theme.SecondaryText.Render(truncate(fmt.Sprintf("▾ %s %s  · %d", name, title, len(r.group.Items)), width)))
			b.WriteString("\n")
			continue
		}
		it := d.signItems[r.item]
		mark, title := "  ", it.Title
		if done := d.signDone[it.ID]; done != "" {
			mark, title = "✓ ", done+": "+it.Title
		} else if !it.HasComment {
			title += "  (no signoff comment)"
		}
		closed := ""
		if it.ClosedAt != nil {
			closed = agentdesk.FormatAge(d.generatedAt.Sub(*it.ClosedAt)) + " ago"
		}
		calls := ""
		if n := len(it.CallsMade); n > 0 {
			calls = strconv.Itoa(n)
		}
		line := mark + padRight(truncate(it.ID, idW-1), idW) + padRight(truncate(title, titleW-1), titleW) + padRight(closed, closedW) + calls
		switch {
		case r.item == d.signCursor:
			line = d.theme.Selected.Render(truncate(line, width-d.theme.Selected.GetHorizontalFrameSize()))
		case d.signDone[it.ID] != "":
			line = d.theme.MutedText.Render(truncate(line, width))
		default:
			line = truncate(line, width)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	for i := end - d.signScroll; i < rows; i++ {
		b.WriteString("\n")
	}
	b.WriteString(d.theme.MutedText.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(d.renderSignoffPreview(width))
	return b.String()
}

// renderSignoffPreview leads with the signoff comment in full: the calls made
// on the user's behalf first, then what changed, what was removed or
// renamed, where to see it, links, captures and commits.
func (d AgentDeskModel) renderSignoffPreview(width int) string {
	it := d.selectedSignoff()
	if it == nil {
		return ""
	}
	var lines []string
	add := func(s string) { lines = append(lines, truncate(s, width)) }
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		add(d.theme.SecondaryText.Render(title))
		for _, l := range items {
			for _, w := range strings.Split(wrapText(l, max(10, width-4)), "\n") {
				add("  " + w)
			}
		}
	}
	for _, w := range strings.Split(wrapText(it.ID+"  "+it.Title, max(10, width)), "\n") {
		add(d.theme.PrimaryBold.Render(w))
	}
	if d.disputing {
		add(d.input.View())
		add(d.theme.MutedText.Render("enter files a follow-up bead linked to " + it.ID + " and marks it disputed (it stays closed) • esc cancels"))
	} else {
		add(d.theme.MutedText.Render("a approve (removes needs-signoff) • d dispute with a note • s skip"))
	}
	if !it.HasComment {
		add(d.theme.MutedText.Render("No `bd-kind: signoff` comment on this bead."))
	}
	section("Calls made on your behalf", it.CallsMade)
	section("What changed", it.WhatChanged)
	section("Removed or renamed", it.RemovedOrRenamed)
	section("Where to see it", it.WhereToSee)
	if len(it.Links) > 0 {
		var links []string
		for i, l := range it.Links {
			if i >= 9 {
				break
			}
			links = append(links, fmt.Sprintf("[%d] %s %s", i+1, l.Kind, l.URL))
		}
		section("Links", links)
	}
	section("Captures", it.Captures)
	section("Commits", it.Commits)
	if it.Raw != "" {
		section("Other", strings.Split(it.Raw, "\n"))
	}
	if budget := d.signoffPreviewHeight(); len(lines) > budget {
		lines = append(lines[:budget-1], d.theme.MutedText.Render("… (v shows the bead in full)"))
	}
	return strings.Join(lines, "\n") + "\n"
}
