package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.fit(m.lines()))
	v.AltScreen = true
	return v
}

func (m Model) lines() []string {
	lines := []string{"Track", ""}
	switch {
	case m.err != nil:
		lines = append(lines, "  error: "+m.err.Error())
	case !m.loaded:
		lines = append(lines, "  Loading…")
	default:
		lines = append(lines, "  "+phaseLine(m.snap))
		if m.snap.HandoffPending {
			lines = append(lines, "  Hand-off due")
		}
	}
	return append(lines, "", "  q quit")
}

func phaseLine(s core.Snapshot) string {
	switch s.Phase {
	case core.PhaseFocus:
		return "Focus   " + clockText(s.Remaining)
	case core.PhaseBreak:
		return "Break   " + clockText(s.Remaining)
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
			if r := []rune(line); len(r) > m.width {
				lines[i] = string(r[:m.width])
			}
		}
	}
	return strings.Join(lines, "\n")
}
