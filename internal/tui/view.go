package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const promptLabel = "  Add task: "

// View implements tea.Model.
func (m Model) View() tea.View {
	header, footer := m.header(), m.footer()
	rows := m.listRows()
	list := m.listLines(rows)

	lines := append(append(append([]string{}, header...), list...), footer...)
	promptRow := len(header) + len(list) + 1 // the footer's first line is blank

	v := tea.NewView(m.fit(lines))
	v.AltScreen = true
	if m.adding && (m.height <= 0 || promptRow < m.height) {
		c := m.input.Cursor()
		c.X += ansi.StringWidth(promptLabel)
		if m.width > 0 {
			c.X = min(c.X, m.width-1)
		}
		c.Y += promptRow
		v.Cursor = c
	}
	return v
}

func (m Model) header() []string {
	lines := []string{"Track", ""}
	switch {
	case m.err != nil:
		lines = append(lines, "  error: "+m.err.Error())
	case !m.loaded:
		lines = append(lines, "  Loading…")
	default:
		lines = append(lines, "  "+m.phaseLine())
		if m.snap.HandoffPending {
			lines = append(lines, "  Hand-off due")
		}
		if m.unfiled > 0 {
			lines = append(lines, fmt.Sprintf("  Unfiled notes: %d", m.unfiled))
		}
	}
	return append(lines, "")
}

func (m Model) footer() []string {
	if m.adding {
		lines := []string{"", promptLabel + m.input.View()}
		if m.addErr != "" {
			lines = append(lines, "  "+m.addErr)
		}
		return append(lines, "  "+m.helpLine(m.keys.promptHelp()))
	}
	lines := []string{""}
	if m.notice != "" {
		lines = append(lines, "  "+m.notice)
	}
	return append(lines, "  "+m.helpLine(m.keys.listHelp()))
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
		return max(len(m.tasks), 1)
	}
	return max(m.height-len(m.header())-len(m.footer()), 1)
}

func (m Model) listLines(rows int) []string {
	if !m.loaded {
		return nil
	}
	if len(m.tasks) == 0 {
		return []string{"  No tasks yet. Press a to add one."}
	}
	top := clampTop(m.top, m.cursor, rows, len(m.tasks))
	end := min(top+rows, len(m.tasks))
	lines := make([]string, 0, end-top)
	for i := top; i < end; i++ {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		lines = append(lines, "  "+marker+taskText(m.tasks[i]))
	}
	return lines
}

// taskText is a Task's title followed by its Tags as #tag chips.
func taskText(t core.Task) string {
	if len(t.Tags) == 0 {
		return t.Title
	}
	chips := make([]string, len(t.Tags))
	for i, tag := range t.Tags {
		chips[i] = "#" + tag
	}
	return t.Title + "  " + strings.Join(chips, " ")
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
