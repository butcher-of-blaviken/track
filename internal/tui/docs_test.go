package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keyShftH = tea.KeyPressMsg{Code: 'H', Text: "H", Mod: tea.ModShift}
	keyShftN = tea.KeyPressMsg{Code: 'N', Text: "N", Mod: tea.ModShift}
	keyShftG = tea.KeyPressMsg{Code: 'G', Text: "G", Mod: tea.ModShift}
	keyG     = tea.KeyPressMsg{Code: 'g', Text: "g"}
	keyQ     = tea.KeyPressMsg{Code: 'q', Text: "q"}
)

// docsText is long enough to scroll in a small window, has a line longer than
// it to wrap, and mentions "fish" in three places, one in capitals.
func docsText() string {
	var b strings.Builder
	b.WriteString("# Track docs\n")
	for i := 1; i <= 40; i++ {
		switch i {
		case 5:
			b.WriteString("In fish an unquoted ##tag starts a comment.\n")
		case 20:
			b.WriteString("Shells such as FISH need quotes.\n")
		case 38:
			b.WriteString("A fish and a long tail: " + strings.Repeat("and so on ", 12) + "THE END OF THE LONG LINE\n")
		default:
			fmt.Fprintf(&b, "line %02d of the docs\n", i)
		}
	}
	b.WriteString("the last line\n")
	return b.String()
}

func (r *rig) docsModel(text string) tea.Model {
	m := tui.New(r.tracker, tui.WithTick(noTick), tui.WithDocs(text))
	m = booted(m).(tui.Model)
	return send(m, tea.WindowSizeMsg{Width: 50, Height: 16})
}

func TestDocs_HOpensThemFromEveryViewAndEscReturnsToIt(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD")
	r.unfile(t, "a loose note")
	views := map[string]struct {
		open []tea.KeyPressMsg
		want string
	}{
		"list":   {nil, "write the PRD"},
		"inbox":  {[]tea.KeyPressMsg{keyI}, "a loose note"},
		"detail": {[]tea.KeyPressMsg{keyL}, "Focused:"},
		"report": {[]tea.KeyPressMsg{keyR}, "Report:"},
	}
	for name, v := range views {
		m := r.docsModel(docsText())
		m = press(m, v.open...)
		wantScreen(t, name, m, v.want)
		m = press(m, keyShftH)
		wantScreen(t, name+" docs", m, "Track docs", "line 01")
		wantNoText(t, name+" docs hide the view", m, v.want)
		m = press(m, keyEsc)
		wantScreen(t, name+" back", m, v.want)
		wantNoText(t, name+" back", m, "Track docs")
	}
}

func TestDocs_ALowercaseHStillGoesBackInTheDetail(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD")
	m := press(r.docsModel(docsText()), keyL)
	m = press(m, keyH)
	wantScreen(t, "h leaves the detail, not the docs", m, "write the PRD")
	wantNoText(t, "no docs", m, "Track docs")
}

func TestDocs_WithoutDocsHSaysSo(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = press(m, keyShftH)
	wantScreen(t, "no docs", m, "No documentation in this build")
}

func TestDocs_ScrollingPagingAndTheEnds(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH)
	wantScreen(t, "top", m, "Track docs", "line 01")
	wantNoText(t, "top", m, "the last line")

	m = press(m, keyJ, keyJ, keyJ)
	wantNoText(t, "scrolled three lines", m, "Track docs")
	m = press(m, keyShftG)
	wantScreen(t, "bottom", m, "the last line")
	wantNoText(t, "bottom", m, "Track docs")
	m = press(m, keyG)
	wantScreen(t, "top again", m, "Track docs")

	m = press(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	wantNoText(t, "a page down", m, "Track docs")
	m = press(m, tea.KeyPressMsg{Code: 'b', Text: "b"})
	wantScreen(t, "a page up", m, "Track docs")
}

func TestDocs_LongLinesWrapToTheWindowAndNothingIsLost(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keyShftG)
	for _, line := range strings.Split(screen(m), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("a line is %d columns wide in a 50 column window: %q", w, line)
		}
	}
	// Scroll up through the wrapped tail: the end of the long line is there.
	found := false
	for i := 0; i < 30 && !found; i++ {
		found = strings.Contains(screen(m), "THE END OF THE LONG LINE")
		m = press(m, keyK)
	}
	if !found {
		t.Error("the end of the wrapped line never appeared")
	}
}

func TestDocs_TheHeaderStaysSoARunningSessionStaysVisible(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := press(r.docsModel(docsText()), keyShftH)
	wantScreen(t, "countdown while reading", m, "Focus", "30:00", "Track docs")
}

func TestDocs_SearchCountsMatchesIgnoringCaseAndStepsThroughThem(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keySlash)
	m = typeText(m, "fish")
	wantScreen(t, "prompt", m, "Search: fish")
	m = press(m, keyEnter)
	wantScreen(t, "first of three", m, `Search "fish": 1/3`, "In fish an unquoted")

	m = press(m, keyN)
	wantScreen(t, "second", m, `Search "fish": 2/3`, "FISH")
	m = press(m, keyN, keyN)
	wantScreen(t, "wraps to the first", m, `Search "fish": 1/3`)
	m = press(m, keyShftN)
	wantScreen(t, "previous wraps to the last", m, `Search "fish": 3/3`, "A fish and a long tail")
}

func TestDocs_ANoMatchSearchSaysSoAndNStaysPut(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keySlash)
	m = typeText(m, "zebra")
	m = press(m, keyEnter, keyN)
	wantScreen(t, "no match", m, `No match for "zebra"`)
}

func TestDocs_NWithoutASearchPointsToSlash(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keyN)
	wantScreen(t, "no search", m, "Press / to search")
}

func TestDocs_EscClearsTheSearchThenGoesBack(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD")
	m := press(r.docsModel(docsText()), keyShftH, keySlash)
	m = typeText(m, "fish")
	m = press(m, keyEnter)
	wantScreen(t, "searching", m, `Search "fish"`)

	m = press(m, keyEsc)
	wantNoText(t, "search cleared", m, "Search")
	wantScreen(t, "still in the docs", m, "line", "docs")
	m = press(m, keyEsc)
	wantScreen(t, "back in the list", m, "write the PRD")
}

func TestDocs_AnEmptySearchClearsIt(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keySlash)
	m = typeText(m, "fish")
	m = press(m, keyEnter, keySlash)
	wantScreen(t, "the prompt starts with the last search", m, "Search: fish")
	for range "fish" {
		m = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m = press(m, keyEnter)
	wantNoText(t, "cleared", m, `Search "fish"`)
}

func TestDocs_EscInTheSearchPromptCancelsItAndStaysInTheDocs(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keySlash)
	m = typeText(m, "fish")
	m = press(m, keyEsc)
	wantNoText(t, "prompt closed", m, "Search: fish")
	wantScreen(t, "still in the docs", m, "Track docs")
	wantNoText(t, "nothing searched", m, `Search "fish"`)
}

func TestDocs_LettersAreTextInTheSearchPrompt(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keySlash)
	m = typeText(m, "qnNgGH")
	wantScreen(t, "typed", m, "Search: qnNgGH", "Track docs")
}

func TestDocs_MatchesAreHighlightedAndTheCurrentOneStandsOut(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keySlash)
	if got := styleOf(t, m, "In fish an unquoted"); got.reverse {
		t.Fatalf("text is highlighted before any search: %+v", got)
	}
	m = typeText(m, "fish")
	m = press(m, keyEnter)
	if got := styleOf(t, m, "fish"); !got.reverse || !got.bold || !got.underline {
		t.Errorf("the current match = %+v, want reverse, bold and underlined", got)
	}
	m = press(m, keyEsc)
	if got := styleOf(t, m, "fish"); got.reverse || got.bold {
		t.Errorf("the highlight stayed after the search was cleared: %+v", got)
	}
}

func TestDocs_AnExternalChangeAndTicksDoNotMoveTheText(t *testing.T) {
	r := newRig(t)
	m := press(r.docsModel(docsText()), keyShftH, keyJ, keyJ, keyJ)
	before := screen(m)
	r.unfile(t, "arrives while reading")
	m = send(m, tui.TickMsg{})
	if screen(m) != before && !strings.Contains(screen(m), "Unfiled notes: 1") {
		t.Errorf("the docs moved:\n%s", screen(m))
	}
	wantNoText(t, "still scrolled", m, "Track docs")
}

func TestDocs_TheFullHelpListsTheirKeys(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH)
	wantNoHints(t, "short footer", m, "g/G top/bottom")
	wantHints(t, "short footer", m, "/ search", "esc back")
	m = press(m, keyHelp)
	wantHints(t, "full help", m, "/ search", "n/N next/prev", "space/b page", "g/G top/bottom", "esc back")
}

func TestDocs_ResizingRewrapsAndKeepsTheFooter(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH, keyShftG)
	m = send(m, tea.WindowSizeMsg{Width: 30, Height: 10})
	lines := strings.Split(screen(m), "\n")
	if len(lines) > 10 {
		t.Errorf("%d lines in a 10 line window", len(lines))
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 30 {
			t.Errorf("too wide for 30 columns: %q", line)
		}
	}
	wantScreen(t, "footer", m, "/ search")
}

func TestDocs_QQuitsAndCtrlCQuitsFromTheSearchPrompt(t *testing.T) {
	m := press(newRig(t).docsModel(docsText()), keyShftH)
	if _, cmd := m.Update(keyQ); cmd == nil || !isQuit(cmd) {
		t.Error("q in the docs should quit")
	}
	m = press(m, keySlash)
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil || !isQuit(cmd) {
		t.Error("ctrl+c in the search prompt should quit")
	}
}

func isQuit(cmd tea.Cmd) bool {
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
