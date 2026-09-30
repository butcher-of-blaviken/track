package tui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var keyHelp = tea.KeyPressMsg{Code: '?', Text: "?"}

// lineWith is the index of the screen line containing text, or -1.
func lineWith(m tea.Model, text string) int {
	for i, line := range strings.Split(screen(m), "\n") {
		if strings.Contains(line, text) {
			return i
		}
	}
	return -1
}

// hintRE matches "key label" as the aligned columns print it, with any run of
// spaces between the two.
func hintRE(hint string) *regexp.Regexp {
	key, label, _ := strings.Cut(hint, " ")
	return regexp.MustCompile(regexp.QuoteMeta(key) + `\s+` + regexp.QuoteMeta(label))
}

func wantHints(t *testing.T, what string, m tea.Model, hints ...string) {
	t.Helper()
	for _, h := range hints {
		if !hintRE(h).MatchString(screen(m)) {
			t.Errorf("%s: screen lacks the hint %q:\n%s", what, h, screen(m))
		}
	}
}

func wantNoHints(t *testing.T, what string, m tea.Model, hints ...string) {
	t.Helper()
	for _, h := range hints {
		if hintRE(h).MatchString(screen(m)) {
			t.Errorf("%s: screen has the hint %q:\n%s", what, h, screen(m))
		}
	}
}

func sizedModel(r *rig, width, height int) tea.Model {
	return send(booted(r.newModel()), tea.WindowSizeMsg{Width: width, Height: height})
}

func TestHelp_TheListFooterIsShortAndHintsAtTheFullHelp(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	m := sizedModel(r, 80, 24)
	wantScreen(t, "short footer", m, "enter start • x stop • a add • j/k move • ? help • q quit")
}

func TestHelp_QuestionMarkShowsEveryListKeyInStackedColumns(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	m := press(sizedModel(r, 100, 30), keyHelp)
	wantHints(t, "full help", m,
		"enter start", "x stop", "a add", "n note", "d done", "D archive", "u reopen",
		"l open", "r report", "i inbox", "/ find", "ctrl+p pick", "tab scope", "j/k move", "? help", "q quit")
	if a, b := lineWith(m, "enter start"), lineWith(m, "stop"); a < 0 || b <= a {
		t.Errorf("want bindings stacked vertically (start on line %d, stop on line %d):\n%s", a, b, screen(m))
	}
	if lineWith(m, "enter start") != lineWith(m, "open") {
		t.Errorf("want the columns side by side:\n%s", screen(m))
	}
}

func TestHelp_QuestionMarkOrEscClosesItAndOtherKeysKeepWorking(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "alpha")
	r.addTask(t, "beta")
	m := sizedModel(r, 100, 30)

	m = press(m, keyHelp)
	wantHints(t, "open", m, "D archive")
	m = press(m, keyJ)
	wantHints(t, "still open after j", m, "D archive")
	wantSelected(t, "j moved", m, "alpha")

	m = press(m, keyHelp)
	wantNoHints(t, "closed by ?", m, "D archive")

	m = press(m, keyHelp, keyEsc)
	wantNoHints(t, "closed by esc", m, "D archive")
}

func TestHelp_EscClosesTheHelpBeforeItLeavesTheView(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	m := press(sizedModel(r, 100, 30), keyL, keyHelp)
	wantScreen(t, "detail help", m, "Focused:")
	wantHints(t, "detail help", m, "n note", "j/k scroll")
	m = press(m, keyEsc)
	wantScreen(t, "help closed, still in the detail", m, "Focused:", "n note • j/k scroll")
	m = press(m, keyEsc)
	wantNoText(t, "left the detail", m, "Focused:")
}

func TestHelp_EachViewShowsOnlyItsOwnKeys(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	r.unfile(t, "a note")
	cases := map[string]struct {
		open tea.KeyPressMsg
		want []string
		not  []string
	}{
		"inbox":  {keyI, []string{"enter file", "c new task", "j/k move", "esc back", "? help", "q quit"}, []string{"d done", "r report", "tab period"}},
		"detail": {keyL, []string{"enter start", "n note", "j/k scroll", "esc back", "? help", "q quit"}, []string{"d done", "r report", "c new task"}},
		"report": {keyR, []string{"tab period", "j/k scroll", "esc back", "? help", "q quit"}, []string{"d done", "c new task", "n note"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := press(sizedModel(r, 100, 30), tc.open, keyHelp)
			wantHints(t, name, m, tc.want...)
			wantNoHints(t, name, m, tc.not...)
		})
	}
}

func TestHelp_LeavingTheViewClosesIt(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	m := press(sizedModel(r, 100, 30), keyHelp, keyL)
	wantNoHints(t, "in the detail", m, "D archive")
	m = press(m, keyEsc)
	wantNoHints(t, "back on the list", m, "D archive")
	wantScreen(t, "short footer", m, "? help")
}

func TestHelp_QuestionMarkIsTextInThePrompts(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	r.unfile(t, "a note")
	for name, open := range map[string][]tea.KeyPressMsg{
		"add":    {keyA},
		"note":   {keyN},
		"picker": {keySlash},
		"file":   {keyI, keyF},
	} {
		t.Run(name, func(t *testing.T) {
			m := press(sizedModel(r, 100, 30), open...)
			m = typeText(m, "why?")
			wantScreen(t, name, m, "why?")
			wantNoHints(t, name, m, "D archive")
		})
	}
}

func TestHelp_TheHandoffPromptTakesQuestionMarkAsText(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)
	m = typeText(m, "?")
	wantScreen(t, "typed", m, "Hand-off for")
	wantNoHints(t, "no help", m, "D archive")
}

func TestHelp_FitsEveryTerminalSizeAndKeepsARowForTheList(t *testing.T) {
	r := newRig(t)
	for i := range 8 {
		r.addTask(t, "task number "+string(rune('a'+i)))
	}
	for width := 16; width <= 100; width += 6 {
		for height := 8; height <= 32; height += 3 {
			m := press(sizedModel(r, width, height), keyHelp)
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) > height {
				t.Fatalf("%dx%d: %d lines", width, height, len(lines))
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w > width {
					t.Fatalf("%dx%d: line %d is %d cells: %q", width, height, i, w, ansi.Strip(line))
				}
			}
			if height >= 12 && !strings.Contains(screen(m), "> ") {
				t.Errorf("%dx%d: the list lost its cursor row:\n%s", width, height, screen(m))
			}
		}
	}
}

func TestHelp_TheShortFooterKeepsQuitAtUsualWidths(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	for _, width := range []int{60, 80, 100} {
		wantScreen(t, "width", sizedModel(r, width, 24), "q quit")
	}
}

func TestHelp_ListRowsShrinkWhileOpenAndComeBack(t *testing.T) {
	r := newRig(t)
	for i := range 10 {
		r.addTask(t, "row "+string(rune('a'+i)))
	}
	m := sizedModel(r, 100, 14)
	before := strings.Count(screen(m), "row ")
	m = press(m, keyHelp)
	open := strings.Count(screen(m), "row ")
	m = press(m, keyHelp)
	after := strings.Count(screen(m), "row ")
	if open >= before || after != before {
		t.Errorf("rows before %d, with help open %d, after %d; want fewer while open and the same after", before, open, after)
	}
}

func TestHelp_AKeyPressedWithTheHelpOpenInTheReportStillScrolls(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "worked")
	r.work(t, task.ID, epoch.Add(time.Hour), 10*time.Minute, core.StartOptions{})
	r.clock.Set(epoch.Add(5 * time.Hour))
	m := press(sizedModel(r, 100, 30), keyEsc, keyR, keyHelp)
	wantScreen(t, "open in the report", m, "Report: Today")
	wantHints(t, "open in the report", m, "tab period")
	m = send(m, tui.TickMsg{})
	wantScreen(t, "survives a tick", m, "Report: Today")
	wantHints(t, "survives a tick", m, "tab period")
}
