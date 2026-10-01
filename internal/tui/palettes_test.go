package tui_test

import (
	"image/color"
	"math"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

// luminance is the WCAG relative luminance of c.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v>>8) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the WCAG contrast ratio of two colours, from 1 to 21.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// minContrast is the least contrast any text may have on its background: 3:1,
// the WCAG minimum for bold and large text and for interface components.
const minContrast = 3.0

func TestPalettes_EveryColourReadsOnItsBackground(t *testing.T) {
	for _, name := range tui.PaletteNames() {
		p, _ := tui.BuiltinPalette(name)
		// Badges and the cursor bar put the Background on a colour, which is the
		// same ratio as that colour on the Background.
		roles := map[string]color.Color{
			"accent": p.Accent, "focus": p.Focus, "rest": p.Rest, "needs you": p.NeedsYou, "danger": p.Danger, "muted": p.Muted,
			"tag 0": p.Tags[0], "tag 1": p.Tags[1], "tag 2": p.Tags[2], "tag 3": p.Tags[3],
		}
		for role, c := range roles {
			if got := contrast(c, p.Background); got < minContrast {
				t.Errorf("%s: %s is %.2f:1 on the background, want at least %.1f:1", name, role, got, minContrast)
			}
		}
		if got := contrast(p.SelectionFG, p.SelectionBG); got < minContrast {
			t.Errorf("%s: the cursor bar is %.2f:1, want at least %.1f:1", name, got, minContrast)
		}
	}
}

func TestPalettes_AreCompleteAndNamed(t *testing.T) {
	want := []string{"gruvbox-dark", "one-dark", "one-light", "solarized-dark", "solarized-light"}
	got := tui.PaletteNames()
	if len(got) != len(want) {
		t.Fatalf("palettes = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("palettes = %v, want %v", got, want)
		}
		p, ok := tui.BuiltinPalette(name)
		if !ok {
			t.Fatalf("%s has no palette", name)
		}
		for role, c := range map[string]color.Color{
			"accent": p.Accent, "focus": p.Focus, "rest": p.Rest, "needs you": p.NeedsYou, "danger": p.Danger, "muted": p.Muted,
			"selection fg": p.SelectionFG, "selection bg": p.SelectionBG, "background": p.Background,
			"tag 0": p.Tags[0], "tag 1": p.Tags[1], "tag 2": p.Tags[2], "tag 3": p.Tags[3],
		} {
			if c == nil {
				t.Errorf("%s: %s is not set, so it would fall back to the terminal's own", name, role)
			}
		}
	}
	if _, ok := tui.BuiltinPalette("nope"); ok {
		t.Error("an unknown name has a palette")
	}
}
