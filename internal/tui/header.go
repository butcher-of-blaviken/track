package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const (
	// barMax and barMin are the widest and narrowest the progress bar gets; below
	// barMin it goes, because a few cells say nothing.
	barMax, barMin = 28, 8
	// barUnknown is the bar's width when the window size is not known.
	barUnknown = 20
	// summaryMinWidth is the narrowest window that shows the day's total.
	summaryMinWidth = 60
)

// titleLine is "Track", and on a wide enough window today's total at the right.
func (m Model) titleLine() string {
	if m.width < summaryMinWidth || !m.loaded || m.today.Sessions == 0 {
		return "Track"
	}
	sessions := "sessions"
	if m.today.Sessions == 1 {
		sessions = "session"
	}
	summary := fmt.Sprintf("today %s · %d %s", minutesText(m.today.Focused), m.today.Sessions, sessions)
	gap := m.width - 2 - len("Track") - ansi.StringWidth(summary)
	if gap < 2 {
		return "Track"
	}
	return "Track" + strings.Repeat(" ", gap) + m.th.muted.Render(summary)
}

// phaseLine is the session at a glance: a badge for the phase, the time left and
// the Task, and a progress bar with its percentage at the right. It gives way as
// the window narrows: the bar shrinks, then goes, then the Task is cut; the time
// left stays.
func (m Model) phaseLine() string {
	switch m.snap.Phase {
	case core.PhaseFocus:
		title := ""
		if m.sessionTask != nil {
			title = m.sessionTask.Title
		}
		return m.progressLine(" Focus ", m.th.p.Focus, title, m.snap.Session.PlannedDuration)
	case core.PhaseBreak:
		return m.progressLine(" Break ", m.th.p.Rest, "", m.snap.Session.BreakDuration)
	}
	line := m.th.muted.Render("Idle")
	if len(m.tasks) > 0 {
		line += "   " + m.th.muted.Render("Pick a task and press enter to start a "+minutesWords(m.focusDuration)+" session")
	}
	return line
}

// progressLine lays out a running phase. total is how long the phase lasts.
func (m Model) progressLine(badge string, colour color.Color, title string, total time.Duration) string {
	left := m.th.badge(colour).Render(badge) + "  " + clockText(m.snap.Remaining)
	leftW := ansi.StringWidth(left)

	frac := 0.0
	if total > 0 {
		frac = float64(total-m.snap.Remaining) / float64(total)
	}
	frac = min(max(frac, 0), 1)
	percent := fmt.Sprintf("%d%%", int(frac*100+0.5))

	if m.width <= 0 { // unknown window: no room to fit to
		line := left
		if title != "" {
			line += "   " + title
		}
		if total > 0 {
			line += "   " + m.bar(colour, barUnknown, frac) + " " + percent
		}
		return line
	}

	avail := m.width - 2 // the line is indented two columns
	withTitle := left
	titleW := ansi.StringWidth(title)
	if title != "" {
		withTitle += "  " + title
	}
	// The bar shrinks to its minimum before anything is cut; if even that does
	// not fit beside the whole Task, the bar and percentage go instead.
	if total > 0 {
		if barW := min(barMax, avail-(leftW+2+titleW)-2-len(percent)-1); barW >= barMin {
			gap := avail - ansi.StringWidth(withTitle) - barW - 1 - len(percent)
			return withTitle + strings.Repeat(" ", gap) + m.bar(colour, barW, frac) + " " + percent
		}
	}
	if title != "" {
		if room := avail - leftW - 2; room >= 2 {
			left += "  " + ansi.Truncate(title, room, "…")
		}
	}
	return left
}

// bar draws frac of width cells filled in the phase colour and the rest faint.
func (m Model) bar(colour color.Color, width int, frac float64) string {
	filled := int(float64(width)*frac + 0.5)
	return lipgloss.NewStyle().Foreground(colour).Render(strings.Repeat("█", filled)) +
		m.th.muted.Render(strings.Repeat("░", width-filled))
}

// minutesWords words a length for a sentence: "30 minute" or "2 hour".
func minutesWords(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%d hour", d/time.Hour)
	}
	return fmt.Sprintf("%d minute", int(d.Round(time.Minute)/time.Minute))
}
