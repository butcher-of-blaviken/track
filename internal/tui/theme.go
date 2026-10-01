package tui

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// theme says what things look like. It uses only the 16 ANSI colours, so the
// app takes on the colours of the user's terminal theme, and the faint
// attribute for secondary text: bright black is the background colour in some
// themes, such as Solarized Dark, and would vanish there.
//
// Colour is never the only cue. Done tasks still say "(done)", and the cursor
// row keeps its "> " marker.
type theme struct {
	accent   lipgloss.Style // headings and the key names in the footer
	focus    lipgloss.Style // the Focus phase
	rest     lipgloss.Style // the Break phase and things that went well
	needsYou lipgloss.Style // a hand-off is due, notes are waiting
	danger   lipgloss.Style // errors, and only errors
	muted    lipgloss.Style // timestamps, labels, Done and Archived tasks
	selected lipgloss.Style // the cursor row: reverse video of the terminal's own colours
	match    lipgloss.Style // the characters a search matched
	edge     lipgloss.Style // the border of the panel that has the keyboard
	tags     [4]lipgloss.Style
}

func newTheme() theme {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return theme{
		accent:   fg(lipgloss.Blue).Bold(true),
		focus:    fg(lipgloss.Magenta).Bold(true),
		rest:     fg(lipgloss.Green).Bold(true),
		needsYou: fg(lipgloss.Yellow),
		danger:   fg(lipgloss.Red),
		muted:    lipgloss.NewStyle().Faint(true),
		// Reverse swaps the terminal's own foreground and background, the one pair
		// guaranteed to contrast. Colouring the bar blue did not: ANSI blue is a very
		// dark blue on many black terminals, and the text on it was hard to read.
		selected: lipgloss.NewStyle().Reverse(true),
		match:    lipgloss.NewStyle().Bold(true).Underline(true),
		edge:     fg(lipgloss.Blue),
		tags:     [4]lipgloss.Style{fg(lipgloss.Cyan), fg(lipgloss.Yellow), fg(lipgloss.Magenta), fg(lipgloss.Green)},
	}
}

// tag is the style of a Tag's chip. It depends only on the lower-cased name, so
// a Tag has the same colour on every screen, whatever its casing.
func (th theme) tag(name string) lipgloss.Style {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	// The high bits: FNV's low bits depend only on the low bits of each byte, so
	// they would ignore the difference between upper and lower case anyway.
	return th.tags[(h.Sum32()>>16)%uint32(len(th.tags))]
}

// badge is a pill of the terminal's own background colour on c, for the phase
// and for things that need you.
func (th theme) badge(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Reverse(true).Bold(true)
}

// banner is the bar shown while a bell rings, coloured by what it announces.
func (th theme) banner(kind core.BellKind) lipgloss.Style {
	c := color.Color(lipgloss.Green) // a Focus session completed and a Break began
	if kind == core.BellBreakEnd {
		c = lipgloss.Yellow // the Break is over and it needs you
	}
	return th.badge(c)
}

// taskText is a Task's row text: its title with the characters the search
// matched emphasised, a marker if it is not Active, and its Tags as #tag chips.
// A row that is not Active is muted. The row under the cursor is painted in the
// selected style throughout: a style cannot wrap text that already holds colour
// codes, so every piece is painted on its own.
func (th theme) taskText(tm core.TaskMatch, selected bool) string {
	t := tm.Task
	plain, matched := lipgloss.NewStyle(), th.match
	if t.State != core.StateActive {
		plain = th.muted
	}
	if selected {
		plain, matched = th.selected, th.selected.Bold(true).Underline(true)
	}

	line := th.highlight(t.Title, tm.Runes, plain, matched)
	switch t.State {
	case core.StateDone:
		line += plain.Render(" (done)")
	case core.StateArchived:
		line += plain.Render(" (archived)")
	}
	if len(t.Tags) == 0 {
		return line
	}
	line += plain.Render("  ")
	for i, tag := range t.Tags {
		chip := plain
		if !selected && t.State == core.StateActive {
			chip = th.tag(tag)
		}
		if i > 0 {
			line += plain.Render(" ")
		}
		line += chip.Render("#") + th.highlight(tag, tm.TagRunes[i], chip, matched)
	}
	return line
}

// highlight paints the runes of text at the given rune offsets in matched and
// the rest in plain.
func (th theme) highlight(text string, offsets []int, plain, matched lipgloss.Style) string {
	if len(offsets) == 0 {
		return plain.Render(text)
	}
	hit := make(map[int]bool, len(offsets))
	for _, o := range offsets {
		hit[o] = true
	}
	var out, run strings.Builder
	var runHit bool
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if runHit {
			out.WriteString(matched.Render(run.String()))
		} else {
			out.WriteString(plain.Render(run.String()))
		}
		run.Reset()
	}
	for i, r := range []rune(text) {
		if hit[i] != runHit {
			flush()
			runHit = hit[i]
		}
		run.WriteRune(r)
	}
	flush()
	return out.String()
}
