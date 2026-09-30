package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// The split layout draws the Tasks, the Inbox and the selected Task's detail as
// three bordered panels at once, when the terminal has room for them. The mode
// says which panel has the keyboard: the list has the Tasks, the inbox the Inbox,
// the detail the Detail pane. The report, the docs and the prompts stay full
// screen.
const (
	splitMinWidth  = 100
	splitMinHeight = 16
	// splitMinBody is the fewest lines between the header and the footer that
	// still fit the panels: a Tasks panel and a collapsed Inbox.
	splitMinBody = 6
	// leftMin and leftMax bound the Tasks and Inbox column, about 40% of the width.
	leftMin, leftMax = 40, 56
	// stripHeight is the Inbox panel when it has no notes: two borders and a line.
	stripHeight = 3
)

// WithLayout sets the layout to start in: "single" starts without the split
// layout, anything else ("auto", "split") with it when the terminal has room.
// The v key changes it for the rest of the run.
func WithLayout(layout string) Option {
	return func(m *Model) { m.singleLayout = layout == "single" }
}

// roomForSplit is whether the terminal is big enough for the split layout.
func (m Model) roomForSplit() bool {
	return m.width >= splitMinWidth && m.height >= splitMinHeight
}

// splitShowing is whether the screen is the split layout right now.
func (m Model) splitShowing() bool {
	if m.singleLayout || !m.roomForSplit() {
		return false
	}
	switch m.mode {
	case modeList, modeInbox, modeDetail:
		return m.height-len(m.header())-len(m.footer()) >= splitMinBody
	}
	return false
}

// geometry is where the panels go: the column widths and each panel's height,
// borders included.
type geometry struct {
	leftW, rightW  int
	tasksH, inboxH int
	bodyH          int
}

func (m Model) geom() geometry {
	body := m.height - len(m.header()) - len(m.footer())
	left := min(max(m.width*40/100, leftMin), leftMax)
	inboxH := stripHeight
	if m.unfiled > 0 {
		inboxH = min(max(m.unfiled+2, stripHeight+1), max(body/3, stripHeight+1))
	}
	if body-inboxH < 3 {
		inboxH = max(body-3, stripHeight)
	}
	return geometry{leftW: left, rightW: m.width - left, tasksH: body - inboxH, inboxH: inboxH, bodyH: body}
}

// tasksRows, inboxRows and detailRows are how many lines each pane scrolls in.
func (m Model) tasksRows() int {
	if m.splitShowing() {
		return m.geom().tasksH - 2
	}
	return m.listRows()
}

func (m Model) inboxRows() int {
	if m.splitShowing() {
		return m.geom().inboxH - 2
	}
	return m.listRows()
}

func (m Model) detailRows() int {
	if m.splitShowing() {
		return m.geom().bodyH - 2
	}
	return m.listRows()
}

// detailWanted is the Task whose detail is read: the open detail's, or in the
// split layout the one under the Tasks cursor, zero for none.
func (m Model) detailWanted() core.TaskID {
	switch {
	case m.inDetail():
		return m.detailFor
	case m.splitShowing():
		if rows := m.rows(); len(rows) > 0 {
			return rows[min(m.cursor, len(rows)-1)].Task.ID
		}
	}
	return 0
}

// detailReady is whether the detail that was read is the one wanted.
func (m Model) detailReady() bool {
	want := m.detailWanted()
	return m.detailLoaded && want != 0 && m.detail.Task.ID == want
}

// splitBody is the three panels, side by side, filling the space between the
// header and the footer.
func (m Model) splitBody() []string {
	g := m.geom()
	in := func(w int) Model { p := m; p.panelWidth = w - 2; return p }

	tasks := m.panel("Tasks", g.leftW, g.tasksH, m.mode == modeList, in(g.leftW).taskLines(g.tasksH-2))
	inboxTitle := "Inbox"
	if m.unfiled > 0 {
		inboxTitle += fmt.Sprintf(" (%d)", m.unfiled)
	}
	inbox := m.panel(inboxTitle, g.leftW, g.inboxH, m.mode == modeInbox, in(g.leftW).inboxLines(g.inboxH-2, m.mode == modeInbox))

	right := in(g.rightW)
	title, content := "Detail", right.detailLines(g.bodyH-2)
	if m.mode == modeInbox && m.inboxLoaded && len(m.inbox) > 0 {
		title, content = "Note", right.noteLines()
	}
	detail := m.panel(title, g.rightW, g.bodyH, m.mode == modeDetail, content)

	lines := make([]string, 0, g.bodyH)
	for i, l := range append(tasks, inbox...) {
		lines = append(lines, l+detail[i])
	}
	return lines
}

// panel draws content in a rounded border with the title in its top edge. The
// border of the panel with the keyboard is in the accent; the others are faint.
func (m Model) panel(title string, w, h int, focused bool, content []string) []string {
	edge, name := m.th.muted, m.th.muted
	if focused {
		edge, name = m.th.edge, m.th.accent
	}
	inner := w - 2
	label := ansi.Truncate(" "+title+" ", max(inner-1, 0), "")
	top := edge.Render("╭─") + name.Render(label) + edge.Render(strings.Repeat("─", max(inner-1-ansi.StringWidth(label), 0))+"╮")
	lines := []string{top}
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(content) {
			line = ansi.Truncate(content[i], inner, "…")
		}
		pad := strings.Repeat(" ", max(inner-ansi.StringWidth(line), 0))
		lines = append(lines, edge.Render("│")+line+"\x1b[m"+pad+edge.Render("│"))
	}
	return append(lines, edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

// noteLines is the selected inbox note in full, for the pane beside the Inbox.
func (m Model) noteLines() []string {
	n := m.inbox[min(m.inboxCursor, len(m.inbox)-1)]
	lines := []string{"  " + m.th.muted.Render(stampText(n.CreatedAt)+" ("+agoText(m.snap.At.Sub(n.CreatedAt))+")"), ""}
	for _, l := range strings.Split(ansi.Wrap(n.Text, max(m.contentWidth()-4, 1), ""), "\n") {
		lines = append(lines, "  "+l)
	}
	return append(lines, "", "  "+m.th.muted.Render("enter: file · c: new task"))
}

// toggleLayout switches between the split and single layouts, if the terminal has
// room for the split.
func (m Model) toggleLayout() (tea.Model, tea.Cmd) {
	if !m.roomForSplit() {
		m.notice = fmt.Sprintf("The window is too small for the split layout (it needs %d×%d).", splitMinWidth, splitMinHeight)
		return m, nil
	}
	m.singleLayout = !m.singleLayout
	m.scrollToCursor()
	m.scrollInbox()
	m.detailTop = min(m.detailTop, m.detailMaxTop())
	return m, nil
}

// contentWidth is the width the lines of the list, inbox and detail are drawn
// for: a panel's inside in the split layout, otherwise the terminal's.
func (m Model) contentWidth() int {
	if m.panelWidth > 0 {
		return m.panelWidth
	}
	return m.width
}
