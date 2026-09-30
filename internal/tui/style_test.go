package tui_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
)

// attrs are the text attributes and colours in force on one character.
type attrs struct {
	bold, faint, underline, reverse bool
	fg, bg                          int // ANSI 0..15, or -1 for the terminal's own
}

// styledRune is one character of the screen with how it is styled.
type styledRune struct {
	r rune
	attrs
}

// styledLines reads the raw view the way a terminal would, following every
// reset, and returns each line as styled characters.
func styledLines(raw string) [][]styledRune {
	var lines [][]styledRune
	var line []styledRune
	cur := attrs{fg: -1, bg: -1}
	for i := 0; i < len(raw); {
		if raw[i] == 0x1b && i+1 < len(raw) && raw[i+1] == '[' {
			j := i + 2
			for j < len(raw) && (raw[j] < 0x40 || raw[j] > 0x7e) {
				j++
			}
			if j < len(raw) && raw[j] == 'm' {
				cur = applySGR(cur, raw[i+2:j])
			}
			i = j + 1
			continue
		}
		r, size := rune(raw[i]), 1
		if r >= 0x80 {
			r = []rune(raw[i:min(i+4, len(raw))])[0]
			size = len(string(r))
		}
		i += size
		if r == '\n' {
			lines = append(lines, line)
			line = nil
			continue
		}
		line = append(line, styledRune{r, cur})
	}
	return append(lines, line)
}

func applySGR(cur attrs, params string) attrs {
	if params == "" {
		return attrs{fg: -1, bg: -1}
	}
	for _, p := range strings.Split(params, ";") {
		n, _ := strconv.Atoi(p)
		switch {
		case n == 0:
			cur = attrs{fg: -1, bg: -1}
		case n == 1:
			cur.bold = true
		case n == 2:
			cur.faint = true
		case n == 4:
			cur.underline = true
		case n == 7:
			cur.reverse = true
		case n >= 30 && n <= 37:
			cur.fg = n - 30
		case n >= 90 && n <= 97:
			cur.fg = n - 90 + 8
		case n >= 40 && n <= 47:
			cur.bg = n - 40
		case n >= 100 && n <= 107:
			cur.bg = n - 100 + 8
		case n == 39:
			cur.fg = -1
		case n == 49:
			cur.bg = -1
		}
	}
	return cur
}

// raw is the view with its styling left in.
func raw(m tea.Model) string { return m.View().Content }

// styleOf returns how text is styled on screen. Every character of text must
// be styled the same way; otherwise the test stops.
func styleOf(t *testing.T, m tea.Model, text string) attrs {
	t.Helper()
	want := []rune(text)
	for _, line := range styledLines(raw(m)) {
		for i := 0; i+len(want) <= len(line); i++ {
			ok := true
			for j, r := range want {
				if line[i+j].r != r {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			first := line[i].attrs
			for j := range want {
				if want[j] != ' ' && line[i+j].attrs != first {
					t.Fatalf("%q is not styled uniformly: %q is %+v but %q is %+v\n%s", text, string(want[0]), first, string(want[j]), line[i+j].attrs, screen(m))
				}
			}
			return first
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, screen(m))
	return attrs{}
}

const (
	ansiBlack = iota
	ansiRed
	ansiGreen
	ansiYellow
	ansiBlue
	ansiMagenta
	ansiCyan
	ansiWhite
)

// styledLineWith returns the styled characters of the first line containing text.
func styledLineWith(t *testing.T, m tea.Model, text string) []styledRune {
	t.Helper()
	for _, line := range styledLines(raw(m)) {
		var b strings.Builder
		for _, c := range line {
			b.WriteRune(c.r)
		}
		if strings.Contains(b.String(), text) {
			return line
		}
	}
	t.Fatalf("no line contains %q:\n%s", text, screen(m))
	return nil
}

func wantNoWiderThan(t *testing.T, what string, m tea.Model, width int) {
	t.Helper()
	for _, line := range strings.Split(raw(m), "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("%s: a line is %d columns wide in a %d column window: %q", what, w, width, ansi.Strip(line))
		}
	}
}

// styleOnLine is styleOf for text on the first line that contains lineText, for
// words such as "Break" that appear more than once.
func styleOnLine(t *testing.T, m tea.Model, lineText, text string) attrs {
	t.Helper()
	line := styledLineWith(t, m, lineText)
	var b strings.Builder
	for _, c := range line {
		b.WriteRune(c.r)
	}
	at := strings.Index(b.String(), text)
	if at < 0 {
		t.Fatalf("%q is not on the line containing %q", text, lineText)
	}
	i := len([]rune(b.String()[:at]))
	first := line[i].attrs
	for j, r := range []rune(text) {
		if r != ' ' && line[i+j].attrs != first {
			t.Fatalf("%q is not styled uniformly on the line containing %q", text, lineText)
		}
	}
	return first
}
