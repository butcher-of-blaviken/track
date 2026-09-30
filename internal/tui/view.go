package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const (
	addLabel     = "  Add task: "
	handoffLabel = "  Note: "
	findLabel    = "  Find: "
	startLabel   = "  Start: "
	fileLabel    = "  File: "
)

// promptLabel is the text before the input on the open prompt's line.
func (m Model) promptLabel() string {
	switch {
	case m.mode == modeHandoff, m.mode == modeNote:
		return handoffLabel
	case m.mode == modePicker && m.pick == pickFile:
		return fileLabel
	case m.mode == modePicker && m.pick == pickFilter:
		return findLabel
	case m.mode == modePicker:
		return startLabel
	}
	return addLabel
}

// View implements tea.Model.
func (m Model) View() tea.View {
	header, footer := m.header(), m.footer()
	rows := m.listRows()
	list := m.listLines(rows)

	lines := append(append(append([]string{}, header...), list...), footer...)
	// The footer's first line is blank; the hand-off prompt has a context line
	// above the input.
	promptRow := len(header) + len(list) + 1
	if m.mode == modeHandoff || (m.mode == modePicker && m.pick == pickFile) {
		promptRow++
	}

	v := tea.NewView(m.fit(lines))
	v.AltScreen = true
	if m.mode != modeList && m.mode != modeConfirmBreak && m.mode != modeResume && m.mode != modeInbox && (m.height <= 0 || promptRow < m.height) {
		c := m.input.Cursor()
		c.X += ansi.StringWidth(m.promptLabel())
		if m.width > 0 {
			c.X = min(c.X, m.width-1)
		}
		c.Y += promptRow
		v.Cursor = c
	}
	return v
}

func (m Model) header() []string {
	lines := []string{"Track"}
	if banner := m.banner(); banner != "" {
		lines = append(lines, banner)
	}
	lines = append(lines, "")
	switch {
	case m.err != nil:
		lines = append(lines, "  error: "+m.err.Error())
	case !m.loaded:
		lines = append(lines, "  Loading…")
	default:
		lines = append(lines, "  "+m.phaseLine())
		if m.snap.Phase == core.PhaseFocus && m.snap.Session.PlannedDuration != m.focusDuration {
			lines = append(lines, "  This session is "+durationText(m.snap.Session.PlannedDuration)+"; the current setting is "+durationText(m.focusDuration)+".")
		}
		if m.snap.HandoffPending {
			lines = append(lines, "  Hand-off due")
		}
		if m.unfiled > 0 {
			lines = append(lines, fmt.Sprintf("  Unfiled notes: %d (i to file)", m.unfiled))
		}
		if m.showAll {
			lines = append(lines, "  Showing: all tasks")
		}
		if m.filter != "" && m.mode != modePicker {
			lines = append(lines, "  Filter: "+m.filter+"  (esc clears)")
		}
	}
	return append(lines, "")
}

// banner is the reverse-video bar shown while a bell is ringing and unsilenced.
func (m Model) banner() string {
	b := m.snap.Bell
	if b == nil || m.bell.acked {
		return ""
	}
	text := "Focus complete — Break started (press any key)"
	if b.Kind == core.BellBreakEnd {
		text = "Break over — ready for the next session (press any key)"
	}
	line := "  " + text
	if pad := m.width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return lipgloss.NewStyle().Reverse(true).Bold(true).Render(line)
}

func (m Model) footer() []string {
	if m.mode == modeInbox {
		lines := []string{""}
		if m.notice != "" {
			lines = append(lines, "  "+m.notice)
		}
		return append(lines, "  "+m.helpLine(m.keys.inboxHelp()))
	}
	if m.mode == modeResume {
		lines := []string{"", "  Resume " + strconv.Quote(m.sessionTask.Title) + "?"}
		if n := m.resumeNote; n != nil {
			lines = append(lines, "  Last note ("+agoText(m.snap.At.Sub(n.CreatedAt))+"): "+n.Text)
		}
		return append(lines, "  "+m.helpLine(m.keys.resumeHelp()))
	}
	if m.mode == modeConfirmBreak {
		return []string{
			"",
			"  Break in progress (" + clockText(m.snap.Remaining) + " left). Start anyway?",
			"  " + m.helpLine(m.keys.confirmBreakHelp()),
		}
	}
	if m.mode != modeList {
		lines := []string{""}
		help := m.keys.promptHelp()
		if m.mode == modeHandoff {
			lines = append(lines, m.handoffContext())
			help = m.keys.handoffHelp()
		}
		if m.mode == modeNote {
			help = m.keys.noteHelp()
		}
		if m.mode == modePicker && m.pick == pickFile {
			lines = append(lines, "  Filing: "+strconv.Quote(m.fileNote.Text))
		}
		if m.mode == modePicker {
			help = m.keys.pickerHelp(m.pickAccept())
		}
		lines = append(lines, m.promptLabel()+m.input.View())
		if m.promptErr != "" {
			lines = append(lines, "  "+m.promptErr)
		}
		return append(lines, "  "+m.helpLine(help))
	}
	lines := []string{""}
	if m.notice != "" {
		lines = append(lines, "  "+m.notice)
	}
	return append(lines, "  "+m.helpLine(m.keys.listHelp()))
}

// handoffContext names what the hand-off note is for: the Task and how long
// ago its session ended.
func (m Model) handoffContext() string {
	line := "  Hand-off for: "
	if m.sessionTask != nil {
		line += m.sessionTask.Title
	}
	if m.snap.Session != nil {
		if ended, ok := m.snap.Session.EndedAt(m.snap.At); ok {
			line += " (ended " + agoText(m.snap.At.Sub(ended)) + ")"
		}
	}
	return line
}

// agoText words a positive duration coarsely, e.g. "just now", "3m ago", "2h 5m ago".
func agoText(d time.Duration) string {
	mins := int(d / time.Minute)
	switch {
	case mins < 1:
		return "just now"
	case mins < 60:
		return fmt.Sprintf("%dm ago", mins)
	}
	return fmt.Sprintf("%dh %dm ago", mins/60, mins%60)
}

// helpLine renders key hints with the Bubbles help styling, dropping whole
// hints from the end, and marking that with an ellipsis, when they do not fit
// the terminal. help's own width handling is not used: when the ellipsis has no
// room it adds the overflowing hint anyway, which the line cut would then split.
func (m Model) helpLine(bindings []key.Binding) string {
	avail := m.width - 2 // the footer is indented two columns
	for n := len(bindings); n > 0; n-- {
		line := m.help.ShortHelpView(bindings[:n])
		if m.width <= 0 || ansi.StringWidth(line) <= avail {
			if n < len(bindings) {
				if tail := " " + m.help.Styles.Ellipsis.Inline(true).Render(m.help.Ellipsis); m.width <= 0 || ansi.StringWidth(line+tail) <= avail {
					line += tail
				}
			}
			return line
		}
	}
	return ""
}

// listRows is how many Task rows fit between the header and footer.
func (m Model) listRows() int {
	if m.height <= 0 {
		if m.mode == modeInbox {
			return max(len(m.inbox), 1)
		}
		lines := 0
		for _, h := range rowHeights(m.rows()) {
			lines += h
		}
		return max(lines, 1)
	}
	return max(m.height-len(m.header())-len(m.footer()), 1)
}

func (m Model) listLines(rows int) []string {
	if !m.loaded {
		return nil
	}
	if m.mode == modeInbox {
		return m.inboxLines(rows)
	}
	matches := m.rows()
	if len(matches) == 0 {
		query := strings.TrimSpace(m.input.Value())
		switch {
		case m.mode == modePicker && m.pick == pickStart && query != "":
			return []string{"  + Create " + strconv.Quote(query) + " and start"}
		case m.mode == modePicker && query != "", m.mode != modePicker && m.filter != "":
			return []string{"  No matches"}
		}
		return []string{"  No tasks yet. Press a to add one."}
	}
	top := windowTop(rowHeights(matches), min(m.top, len(matches)-1), min(m.cursor, len(matches)-1), rows)
	lines := make([]string, 0, rows)
	for i := top; i < len(matches) && len(lines) < rows; i++ {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		lines = append(lines, "  "+marker+matchText(matches[i]))
		if matches[i].Note != nil && len(lines) < rows {
			lines = append(lines, noteLine(matches[i], m.snap.At))
		}
	}
	return lines
}

// inboxLines is the Unfiled notes, oldest first, one per line.
func (m Model) inboxLines(rows int) []string {
	switch {
	case !m.inboxLoaded:
		return []string{"  Loading…"}
	case len(m.inbox) == 0:
		return []string{"  Inbox is empty."}
	}
	heights := make([]int, len(m.inbox))
	for i := range heights {
		heights[i] = 1
	}
	top := windowTop(heights, min(m.inboxTop, len(m.inbox)-1), min(m.inboxCursor, len(m.inbox)-1), rows)
	lines := make([]string, 0, rows)
	for i := top; i < len(m.inbox) && len(lines) < rows; i++ {
		marker := "  "
		if i == m.inboxCursor {
			marker = "> "
		}
		n := m.inbox[i]
		lines = append(lines, "  "+marker+n.Text+" ("+agoText(m.snap.At.Sub(n.CreatedAt))+")")
	}
	return lines
}

// rowHeights is how many lines each match takes: one, plus one for its note.
func rowHeights(matches []core.TaskMatch) []int {
	heights := make([]int, len(matches))
	for i, tm := range matches {
		heights[i] = 1
		if tm.Note != nil {
			heights[i] = 2
		}
	}
	return heights
}

// noteLine is the line under a row whose match came from a note.
func noteLine(tm core.TaskMatch, now time.Time) string {
	n := tm.Note
	return "      ↳ " + highlight(n.Note.Text, n.Runes) + " (" + agoText(now.Sub(n.Note.CreatedAt)) + ")"
}

// pickAccept is what Enter does in the open picker, for the footer.
func (m Model) pickAccept() string {
	switch {
	case m.pick == pickFilter:
		return "filter"
	case m.pick == pickFile:
		return "file"
	case len(m.rows()) == 0 && strings.TrimSpace(m.input.Value()) != "":
		return "create"
	}
	return "start"
}

var matchStyle = lipgloss.NewStyle().Bold(true).Underline(true)

// matchText is a Task's row text: its title with the characters the search
// matched highlighted, a marker if it is not Active, and its Tags as #tag chips.
func matchText(tm core.TaskMatch) string {
	t := tm.Task
	line := t.Title
	if len(tm.Runes) > 0 {
		line = highlight(t.Title, tm.Runes)
	}
	switch t.State {
	case core.StateDone:
		line += " (done)"
	case core.StateArchived:
		line += " (archived)"
	}
	if len(t.Tags) == 0 {
		return line
	}
	chips := make([]string, len(t.Tags))
	for i, tag := range t.Tags {
		if runes := tm.TagRunes[i]; len(runes) > 0 {
			tag = highlight(tag, runes)
		}
		chips[i] = "#" + tag
	}
	return line + "  " + strings.Join(chips, " ")
}

// highlight styles the runes of title at the given rune offsets.
func highlight(title string, offsets []int) string {
	hit := make(map[int]bool, len(offsets))
	for _, o := range offsets {
		hit[o] = true
	}
	var out, run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			out.WriteString(matchStyle.Render(run.String()))
			run.Reset()
		}
	}
	for i, r := range []rune(title) {
		if hit[i] {
			run.WriteRune(r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
}

// phaseLine is the panel's state line: the phase, its countdown, and for Focus
// the Task being worked on.
func (m Model) phaseLine() string {
	switch m.snap.Phase {
	case core.PhaseFocus:
		line := "Focus   " + clockText(m.snap.Remaining)
		if m.sessionTask != nil {
			line += "   " + m.sessionTask.Title
		}
		return line
	case core.PhaseBreak:
		return "Break   " + clockText(m.snap.Remaining)
	}
	return "Idle"
}

// durationText words a duration compactly, e.g. "45m", "1h" or "1h30m".
func durationText(d time.Duration) string {
	text := d.Round(time.Second).String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}

// clockText formats d as mm:ss, rounding up so the display reaches 00:00 only
// when the time is really up.
func clockText(d time.Duration) string {
	secs := int((max(d, 0) + time.Second - 1) / time.Second)
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

// fit cuts the lines to the terminal size, if it is known.
func (m Model) fit(lines []string) string {
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	if m.width > 0 {
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.width, "…")
		}
	}
	return strings.Join(lines, "\n")
}
