package tui_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

// A Palette is what a theme is built from. The default palette is today's ANSI
// look, pinned by the theme tests; these pin that a palette's own colours reach
// the screen, in every place the default uses a colour.

// testPalette uses a different RGB colour for every role, so a role painted in
// the wrong colour shows up. The numbers avoid 1, 2, 4 and 7, the attributes the
// styled-screen helpers read.
func testPalette() tui.Palette {
	return tui.Palette{
		Accent:      lipgloss.Color("#0a0b0c"),
		Focus:       lipgloss.Color("#0d0e0f"),
		Rest:        lipgloss.Color("#101112"),
		NeedsYou:    lipgloss.Color("#131415"),
		Danger:      lipgloss.Color("#161718"),
		Tags:        [4]color.Color{lipgloss.Color("#191a1b"), lipgloss.Color("#1c1d1e"), lipgloss.Color("#1f2021"), lipgloss.Color("#222324")},
		Muted:       lipgloss.Color("#252627"),
		SelectionFG: lipgloss.Color("#28292a"),
		SelectionBG: lipgloss.Color("#2b2c2d"),
		Background:  lipgloss.Color("#2e2f30"),
	}
}

// fg and bg are the escape parameters that set an RGB foreground or background.
func fg(r, g, b int) string { return fmt.Sprintf("38;2;%d;%d;%d", r, g, b) }
func bg(r, g, b int) string { return fmt.Sprintf("48;2;%d;%d;%d", r, g, b) }

func themed(r *rig) tea.Model {
	return tui.New(r.tracker, tui.WithTick(noTick), tui.WithLayout("single"), tui.WithTheme(testPalette()))
}

func TestPalette_TheCursorRowUsesTheSelectionColoursNotReverseVideo(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD")
	m := send(booted(themed(r)), tea.WindowSizeMsg{Width: 60, Height: 20})
	line := styledLineWith(t, m, "> write the PRD")
	if line[0].reverse {
		t.Errorf("the cursor row is reverse video: %+v", line[0].attrs)
	}
	got := raw(m)
	for _, want := range []string{fg(0x28, 0x29, 0x2a), bg(0x2b, 0x2c, 0x2d)} {
		if !strings.Contains(got, want) {
			t.Errorf("the screen lacks %q for the cursor row:\n%q", want, got)
		}
	}
}

func TestPalette_PhaseBadgesAreTheBackgroundColourOnThePhaseColour(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	got := raw(booted(themed(r)))
	for _, want := range []string{bg(0x0d, 0x0e, 0x0f), fg(0x2e, 0x2f, 0x30), fg(0x0d, 0x0e, 0x0f)} {
		if !strings.Contains(got, want) {
			t.Errorf("the Focus line lacks %q (badge background, badge text, bar):\n%q", want, got)
		}
	}
	if strings.Contains(got, "\x1b[7m") || strings.Contains(got, ";7m") {
		t.Errorf("a badge is reverse video although the palette sets the Background:\n%q", got)
	}
}

func TestPalette_FooterKeysAreTheAccentAndMutedTextIsAColourNotFaint(t *testing.T) {
	got := raw(booted(themed(newRig(t))))
	if !strings.Contains(got, fg(0x0a, 0x0b, 0x0c)) {
		t.Errorf("no footer key in the accent:\n%q", got)
	}
	if !strings.Contains(got, fg(0x25, 0x26, 0x27)) {
		t.Errorf("no muted text in the muted colour:\n%q", got)
	}
}
