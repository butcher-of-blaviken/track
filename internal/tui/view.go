package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const (
	addLabel     = "  Add task: "
	handoffLabel = "  Note: "
	findLabel    = "  Find: "
	startLabel   = "  Start: "
	fileLabel    = "  File: "
	searchLabel  = "  Search: "
)

// promptLabel is the text before the input on the open prompt's line.
func (m Model) promptLabel() string {
	switch {
	case m.mode == modeHandoff, m.mode == modeNote:
		return handoffLabel
	case m.mode == modeDocsFind:
		return searchLabel
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
	var list []string
	if m.splitShowing() {
		list = m.splitBody()
	} else {
		list = m.listLines(m.listRows())
	}

	lines := append(append(append([]string{}, header...), list...), footer...)
	// The footer's first line is blank; the hand-off prompt has a context line
	// above the input.
	promptRow := len(header) + len(list) + 1
	if m.mode == modeHandoff || (m.mode == modePicker && m.pick == pickFile) {
		promptRow++
	}

	v := tea.NewView(m.fit(lines))
	v.AltScreen = true
	if m.mode != modeList && m.mode != modeConfirmBreak && m.mode != modeResume && m.mode != modeInbox && m.mode != modeDetail && m.mode != modeReport && m.mode != modeDocs && (m.height <= 0 || promptRow < m.height) {
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
	lines := []string{m.titleLine()}
	if banner := m.banner(); banner != "" {
		lines = append(lines, banner)
	}
	lines = append(lines, "")
	switch {
	case m.err != nil:
		lines = append(lines, "  "+m.th.danger.Render("error: "+m.err.Error()))
	case !m.loaded:
		lines = append(lines, "  "+m.th.muted.Render("Loading…"))
	default:
		lines = append(lines, "  "+m.phaseLine())
		if m.snap.Phase == core.PhaseFocus && m.snap.Session.PlannedDuration != m.focusDuration {
			lines = append(lines, "  "+m.th.needsYou.Render("This session is "+durationText(m.snap.Session.PlannedDuration)+"; the current setting is "+durationText(m.focusDuration)+"."))
		}
		if m.snap.HandoffPending {
			lines = append(lines, "  "+m.th.badge(m.th.p.NeedsYou).Render(" Hand-off due "))
		}
		if m.unfiled > 0 {
			lines = append(lines, "  "+m.th.needsYou.Render(fmt.Sprintf("Unfiled notes: %d (i to file)", m.unfiled)))
		}
		if m.showAll {
			lines = append(lines, "  "+m.th.muted.Render("Showing: all tasks"))
		}
		if m.filter != "" && m.mode != modePicker {
			lines = append(lines, "  "+m.th.muted.Render("Filter: "+m.filter+"  (esc clears)"))
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
	return m.th.banner(b.Kind).Render(line)
}

func (m Model) footer() []string {
	switch m.mode {
	case modeDocs:
		return m.viewFooter(m.docsStatus(), m.keys.docsHelp(), m.keys.docsFull())
	case modeReport:
		return m.viewFooter(m.notice, m.keys.reportHelp(), m.keys.reportFull())
	case modeDetail:
		return m.viewFooter(m.notice, m.keys.detailHelp(), m.keys.detailFull())
	case modeInbox:
		return m.viewFooter(m.notice, m.keys.inboxHelp(), m.keys.inboxFull())
	case modeList:
		return m.viewFooter(m.notice, m.keys.listHelp(), m.keys.listFull())
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
	lines := []string{""}
	help := m.keys.promptHelp()
	if m.mode == modeHandoff {
		lines = append(lines, m.handoffContext())
		help = m.keys.handoffHelp()
	}
	if m.mode == modeNote {
		help = m.keys.noteHelp()
	}
	if m.mode == modeDocsFind {
		help = m.keys.docsFindHelp()
	}
	if m.mode == modePicker && m.pick == pickFile {
		lines = append(lines, "  Filing: "+strconv.Quote(m.fileNote.Text))
	}
	if m.mode == modePicker {
		help = m.keys.pickerHelp(m.pickAccept())
	}
	lines = append(lines, m.promptLabel()+m.input.View())
	if m.promptErr != "" {
		lines = append(lines, "  "+m.th.danger.Render(m.promptErr))
	}
	return append(lines, "  "+m.helpLine(help))
}

// viewFooter is the footer of a view with a full help: a blank line, the notice
// if any, and either the short help line or, while the full help is open, its
// columns.
func (m Model) viewFooter(notice string, short []key.Binding, full [][]key.Binding) []string {
	lines := []string{""}
	if notice != "" {
		lines = append(lines, "  "+notice)
	}
	if m.showHelp {
		return append(lines, m.fullHelpLines(full, len(lines))...)
	}
	return append(lines, "  "+m.helpLine(short))
}

// fullHelpLines renders the help columns, dropping whole columns from the right
// until they fit the width, and cutting lines so that the header, the lines
// already in the footer and at least one list row still fit the height.
func (m Model) fullHelpLines(groups [][]key.Binding, used int) []string {
	render := func(n int) []string {
		lines := strings.Split(m.help.FullHelpView(groups[:n]), "\n")
		for i := range lines {
			lines[i] = "  " + lines[i]
		}
		return lines
	}
	widest := func(lines []string) int {
		w := 0
		for _, line := range lines {
			w = max(w, ansi.StringWidth(line))
		}
		return w
	}
	n := len(groups)
	lines := render(n)
	for n > 1 && m.width > 0 && widest(lines) > m.width {
		n--
		lines = render(n)
	}
	if m.height > 0 {
		budget := max(m.height-len(m.header())-1-used, 1)
		lines = lines[:min(len(lines), budget)]
	}
	return lines
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

// cursorRow is a list row with its marker. The row under the cursor keeps its
// "> " and is painted as a bar that runs to the edge of the window.
func (m Model) cursorRow(cursor bool, body string) string {
	if !cursor {
		return "    " + body
	}
	line := "  " + m.th.selected.Render("> ") + body
	if pad := m.contentWidth() - ansi.StringWidth(line); pad > 0 {
		line += m.th.selected.Render(strings.Repeat(" ", pad))
	}
	return line
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
		if m.inDocs() {
			return max(m.docs.TotalLineCount(), 1)
		}
		if m.inDetail() {
			return detailHeadLines + len(m.detailBody())
		}
		if m.mode == modeReport {
			return reportHeadLines + max(len(m.reportBody()), 1)
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
		return m.inboxLines(rows, true)
	}
	if m.inDocs() {
		return strings.Split(m.docs.View(), "\n")
	}
	if m.inDetail() {
		return m.detailLines(rows)
	}
	if m.mode == modeReport {
		return m.reportLines(rows)
	}
	return m.taskLines(rows)
}

// taskLines is the Task rows, the ones a filter or the open picker leaves.
func (m Model) taskLines(rows int) []string {
	if !m.loaded {
		return nil
	}
	matches := m.rows()
	if len(matches) == 0 {
		query := strings.TrimSpace(m.input.Value())
		switch {
		case m.mode == modePicker && m.pick == pickStart && query != "":
			return []string{"  " + m.th.accent.Render("+ Create "+strconv.Quote(query)+" and start")}
		case m.mode == modePicker && query != "", m.mode != modePicker && m.filter != "":
			return []string{"  " + m.th.muted.Render("No matches")}
		}
		return []string{"  " + m.th.muted.Render("No tasks yet. Press a to add one.")}
	}
	top := windowTop(rowHeights(matches), min(m.top, len(matches)-1), min(m.cursor, len(matches)-1), rows)
	lines := make([]string, 0, rows)
	for i := top; i < len(matches) && len(lines) < rows; i++ {
		lines = append(lines, m.cursorRow(i == m.cursor, m.th.taskText(matches[i], i == m.cursor)))
		if matches[i].Note != nil && len(lines) < rows {
			lines = append(lines, m.th.noteLine(matches[i], m.snap.At))
		}
	}
	return lines
}

// detailHistoryShown is how many of a Task's latest sessions the detail lists.
const detailHistoryShown = 5

// detailLines is the Task detail: the Task, its focused time, its state and when
// it was created, then a scrolling body of its latest sessions and its note log.
func (m Model) detailLines(rows int) []string {
	switch {
	case m.detailWanted() == 0:
		return []string{"  " + m.th.muted.Render("No task selected.")}
	case !m.detailReady():
		return []string{"  " + m.th.muted.Render("Loading…")}
	}
	d := m.detail
	lines := []string{
		"  " + m.th.taskText(core.TaskMatch{Task: d.Task}, false),
		"  " + m.th.muted.Render("Focused: ") + focusText(d.Focused, d.Sessions),
		"  " + m.th.muted.Render("State: ") + d.Task.State.String() + "   " + m.th.muted.Render("Created: ") + stampText(d.Task.CreatedAt),
		"",
	}
	body := m.detailBody()
	avail := max(rows-detailHeadLines, 1)
	top := min(m.detailTop, max(len(body)-avail, 0))
	return append(lines, body[top:min(top+avail, len(body))]...)
}

// detailBody is what scrolls under the detail's head: the latest sessions, newest
// first, and the note log, newest first.
func (m Model) detailBody() []string {
	d := m.detail
	var body []string
	if len(d.History) > 0 {
		body = append(body, "  "+m.th.accent.Render("Sessions"))
		for _, s := range d.History[:min(len(d.History), detailHistoryShown)] {
			body = append(body, "  "+m.th.muted.Render(stampText(s.StartedAt))+"  "+m.sessionText(s, d.At))
		}
		if more := len(d.History) - detailHistoryShown; more > 0 {
			body = append(body, "  "+m.th.muted.Render(fmt.Sprintf("+%d earlier", more)))
		}
		body = append(body, "")
	}
	body = append(body, "  "+m.th.accent.Render("Notes"))
	if len(d.Notes) == 0 {
		return append(body, "  "+m.th.muted.Render("No notes yet. Press n to add one."))
	}
	for i := len(d.Notes) - 1; i >= 0; i-- {
		n := d.Notes[i]
		line := "  " + m.th.muted.Render(stampText(n.CreatedAt)) + "  "
		if n.SessionID != 0 {
			line += m.th.needsYou.Render("hand-off") + "  "
		}
		body = append(body, line+n.Text)
	}
	return body
}

// sessionText is a session's length and how it ended: "30m  completed", or how
// much of the planned time it had when it was stopped or still has run.
func (m Model) sessionText(s core.FocusSession, at time.Time) string {
	planned, elapsed := minutesText(s.PlannedDuration), minutesText(s.Elapsed(at))
	switch s.Outcome(at) {
	case core.OutcomeCompleted:
		return planned + "  " + m.th.rest.UnsetBold().Render("completed")
	case core.OutcomeStoppedEarly:
		return elapsed + " of " + planned + "  " + m.th.needsYou.Render("stopped early")
	}
	return elapsed + " of " + planned + "  " + m.th.focus.UnsetBold().Render("running")
}

// stampText is a time as the detail shows it, in the local zone.
func stampText(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

// reportHeadLines is the lines above the scrolling part of the report: the
// title, the totals and a blank.
const reportHeadLines = 3

// reportLines is the report: a title, the totals, and the tables, scrolled by
// reportTop. A day's report, unlike the week's, also lists what was finished
// and the notes written that day.
func (m Model) reportLines(rows int) []string {
	if !m.reportLoaded {
		return []string{"  " + m.th.muted.Render("Loading…")}
	}
	r := m.report
	name, other, when := "Today", "This week", "today"
	switch {
	case r.Period == core.PeriodWeek:
		name, other, when = other, name, "this week"
	case !m.reportDay.IsZero():
		name = r.From.Format("Mon 2 Jan")
		when = "on " + name
	}
	lines := []string{"  " + m.th.accent.Render("Report: "+name) + m.th.muted.Render("   (tab: "+other+")")}
	if r.Sessions == 0 && !m.reportHasExtras() {
		lines = append(lines, "  "+m.th.muted.Render(fmt.Sprintf("Break overrides: %d", r.Overrides)), "", "  "+m.th.muted.Render("No focused time "+when+"."))
		return lines
	}
	lines = append(lines, "  "+m.th.muted.Render("Focused: ")+focusText(r.Focused, r.Sessions)+m.th.muted.Render("   Break overrides: "+fmt.Sprint(r.Overrides)), "")
	body := m.reportBody()
	avail := max(rows-reportHeadLines, 1)
	top := min(m.reportTop, max(len(body)-avail, 0))
	return append(lines, body[top:min(top+avail, len(body))]...)
}

// reportHasExtras is whether a day's report has notes or finished tasks to show
// beyond its focused time. The week's report does not list them.
func (m Model) reportHasExtras() bool {
	r := m.report
	return r.Period == core.PeriodToday && len(r.Finished)+len(r.Notes)+len(r.Unfiled) > 0
}

// reportBody is the report's tables, line by line.
func (m Model) reportBody() []string {
	r := m.report
	if !m.reportLoaded || (r.Sessions == 0 && !m.reportHasExtras()) {
		return nil
	}
	extras := m.reportHasExtras()
	var body []string
	section := func(title string) {
		if len(body) > 0 {
			body = append(body, "")
		}
		body = append(body, "  "+m.th.accent.Render(title))
	}
	if extras && len(r.Finished) > 0 {
		section("Finished")
		for _, t := range r.Finished {
			body = append(body, "    "+m.th.taskText(core.TaskMatch{Task: t}, false))
		}
	}
	notes := map[core.TaskID][]core.Note{}
	if extras {
		for _, tn := range r.Notes {
			notes[tn.Task.ID] = tn.Notes
		}
	}
	noteRows := func(id core.TaskID) {
		for _, n := range notes[id] {
			body = append(body, "              "+n.Text)
		}
		delete(notes, id)
	}
	if len(r.Tasks) > 0 || len(notes) > 0 {
		section("By task")
		for _, tt := range r.Tasks {
			body = append(body, "    "+fmt.Sprintf("%-8s", minutesText(tt.Focused))+"  "+m.th.taskText(core.TaskMatch{Task: tt.Task}, false))
			noteRows(tt.Task.ID)
		}
		for _, tn := range r.Notes {
			if _, left := notes[tn.Task.ID]; left && extras {
				body = append(body, "    "+strings.Repeat(" ", 8)+"  "+m.th.taskText(core.TaskMatch{Task: tn.Task}, false))
				noteRows(tn.Task.ID)
			}
		}
	}
	if extras && len(r.Unfiled) > 0 {
		section("Notes")
		for _, n := range r.Unfiled {
			body = append(body, "    "+n.Text)
		}
	}
	if len(r.Tags) > 0 {
		section("By tag")
		for _, tag := range r.Tags {
			label := m.th.tag(tag.Tag).Render("#" + tag.Tag)
			if tag.Untagged {
				label = m.th.muted.Render("(untagged)")
			}
			body = append(body, "    "+fmt.Sprintf("%-8s", minutesText(tag.Focused))+"  "+label)
		}
	}
	return body
}

// minutesText is a duration as "1h 25m", "25m" or "40s" (see focusText).
func minutesText(d time.Duration) string {
	return strings.TrimSuffix(focusText(d, 1), " over 1 session")
}

// focusText words a Task's total focused time: whole minutes from one minute up,
// seconds below that.
func focusText(d time.Duration, sessions int) string {
	if sessions == 0 {
		return "none yet"
	}
	var amount string
	switch mins := int(d.Round(time.Minute) / time.Minute); {
	case d < time.Minute:
		amount = fmt.Sprintf("%ds", int(d/time.Second))
	case mins < 60:
		amount = fmt.Sprintf("%dm", mins)
	case mins%60 == 0:
		amount = fmt.Sprintf("%dh", mins/60)
	default:
		amount = fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	if sessions == 1 {
		return amount + " over 1 session"
	}
	return fmt.Sprintf("%s over %d sessions", amount, sessions)
}

// inboxLines is the Unfiled notes, oldest first, one per line. The cursor shows
// on the note it is on if marked; in the split layout the Inbox panel leaves it
// out while another panel has the keyboard.
func (m Model) inboxLines(rows int, marked bool) []string {
	switch {
	case !m.inboxLoaded:
		return []string{"  " + m.th.muted.Render("Loading…")}
	case len(m.inbox) == 0:
		return []string{"  " + m.th.muted.Render("Inbox is empty.")}
	}
	heights := make([]int, len(m.inbox))
	for i := range heights {
		heights[i] = 1
	}
	top := windowTop(heights, min(m.inboxTop, len(m.inbox)-1), min(m.inboxCursor, len(m.inbox)-1), rows)
	lines := make([]string, 0, rows)
	for i := top; i < len(m.inbox) && len(lines) < rows; i++ {
		n := m.inbox[i]
		age := " (" + agoText(m.snap.At.Sub(n.CreatedAt)) + ")"
		if marked && i == m.inboxCursor {
			lines = append(lines, m.cursorRow(true, m.th.selected.Render(n.Text+age)))
			continue
		}
		lines = append(lines, m.cursorRow(false, n.Text+m.th.muted.Render(age)))
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
func (th theme) noteLine(tm core.TaskMatch, now time.Time) string {
	n := tm.Note
	return th.muted.Render("      ↳ ") + th.highlight(n.Note.Text, n.Runes, th.muted, th.match) + th.muted.Render(" ("+agoText(now.Sub(n.Note.CreatedAt))+")")
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
