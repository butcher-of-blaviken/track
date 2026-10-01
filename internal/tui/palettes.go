package tui

import (
	"image/color"
	"sort"

	"charm.land/lipgloss/v2"
)

// The built-in palettes are fixed RGB colours, so the app looks the same whatever
// colours the terminal's own theme has. Each is chosen for the terminal
// background it names: the app does not paint the background, so a dark palette
// in a light terminal is the user's mismatch.
//
// A palette's Background is that terminal background, which the badges and the
// cursor bar use as their text colour.

// scheme is a terminal colour scheme's background, text and six hues.
type scheme struct {
	bg, fg                                  string
	red, green, yellow, blue, magenta, cyan string
	// muted is how much of fg secondary text keeps; zero means mutedOpacity.
	muted float64
}

// palette turns a scheme into the roles of a Palette, as in the mockup: blue is
// the accent and the cursor bar, magenta Focus, green Break, yellow needs-you.
// Secondary text is the text colour partly blended into the background. The
// faint attribute does the same, but would drop below the contrast floor in
// Solarized, whose own text is only about 4:1.
func (s scheme) palette() Palette {
	c := lipgloss.Color
	opacity := s.muted
	if opacity == 0 {
		opacity = mutedOpacity
	}
	return Palette{
		Accent:      c(s.blue),
		Focus:       c(s.magenta),
		Rest:        c(s.green),
		NeedsYou:    c(s.yellow),
		Danger:      c(s.red),
		Tags:        [4]color.Color{c(s.cyan), c(s.yellow), c(s.magenta), c(s.green)},
		Muted:       mix(c(s.fg), c(s.bg), opacity),
		SelectionFG: c(s.bg),
		SelectionBG: c(s.blue),
		Background:  c(s.bg),
	}
}

// mutedOpacity is how much of the text colour secondary text keeps by default.
const mutedOpacity = 0.7

// mix is fg at the given opacity over bg.
func mix(fg, bg color.Color, opacity float64) color.Color {
	fr, fgn, fb, _ := fg.RGBA()
	br, bgn, bb, _ := bg.RGBA()
	blend := func(f, b uint32) uint8 {
		return uint8((float64(f>>8)*opacity + float64(b>>8)*(1-opacity)) + 0.5)
	}
	return color.RGBA{blend(fr, br), blend(fgn, bgn), blend(fb, bb), 0xff}
}

var palettes = map[string]Palette{
	"one-dark": scheme{
		bg: "#282c34", fg: "#abb2bf",
		red: "#e06c75", green: "#98c379", yellow: "#e5c07b", blue: "#61afef", magenta: "#c678dd", cyan: "#56b6c2",
	}.palette(),
	"gruvbox-dark": scheme{
		bg: "#282828", fg: "#ebdbb2",
		red: "#fb4934", green: "#b8bb26", yellow: "#fabd2f", blue: "#83a598", magenta: "#d3869b", cyan: "#8ec07c",
	}.palette(),
	"solarized-dark": scheme{
		bg: "#002b36", fg: "#839496",
		red: "#dc322f", green: "#859900", yellow: "#b58900", blue: "#268bd2", magenta: "#d33682", cyan: "#2aa198",
		muted: 0.85,
	}.palette(),
	"one-light": scheme{
		bg: "#fafafa", fg: "#383a42",
		red: "#e45649", green: "#50a14f", yellow: "#c18401", blue: "#4078f2", magenta: "#a626a4", cyan: "#0184bc",
	}.palette(),
	// Solarized's own green, yellow and cyan are under 3:1 on its light background,
	// so those three are a little darker here.
	"solarized-light": scheme{
		bg: "#fdf6e3", fg: "#657b83",
		red: "#dc322f", green: "#788a00", yellow: "#a37c00", blue: "#268bd2", magenta: "#d33682", cyan: "#269289",
		muted: 0.95,
	}.palette(),
}

// PaletteNames lists the built-in palettes, by name.
func PaletteNames() []string {
	names := make([]string, 0, len(palettes))
	for name := range palettes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuiltinPalette is the built-in palette with the given name.
func BuiltinPalette(name string) (Palette, bool) {
	p, ok := palettes[name]
	return p, ok
}
