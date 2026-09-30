package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var keyV = tea.KeyPressMsg{Code: 'v', Text: "v"}

// splitRig has two tasks, "alpha" and "beta" (on top, under the cursor), each
// with a note of its own.
func splitRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	alpha, beta := r.addTask(t, "alpha"), r.addTask(t, "beta")
	if _, err := r.tracker.AddNote(ctx, alpha.ID, "alpha note"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.AddNote(ctx, beta.ID, "beta note"); err != nil {
		t.Fatal(err)
	}
	return r
}

func splitModel(r *rig, w, h int) tea.Model { return sized(booted(r.newSplitModel()), w, h) }

// edgeOf is how the top-left corner of the panel with this title is styled.
func edgeOf(t *testing.T, m tea.Model, title string) attrs {
	t.Helper()
	for _, line := range styledLines(raw(m)) {
		var b strings.Builder
		for _, c := range line {
			b.WriteRune(c.r)
		}
		if at := strings.Index(b.String(), "╭─ "+title); at >= 0 {
			return line[len([]rune(b.String()[:at]))].attrs
		}
	}
	t.Fatalf("no panel titled %q:\n%s", title, screen(m))
	return attrs{}
}

func isSplit(m tea.Model) bool { return strings.Contains(screen(m), "╭─ Tasks") }

func TestSplit_ShowsTheTasksTheInboxAndTheCursorTasksDetailAtOnce(t *testing.T) {
	r := splitRig(t)
	m := splitModel(r, 120, 30)
	wantScreen(t, "panels", m, "╭─ Tasks", "╭─ Inbox", "╭─ Detail", "beta", "alpha", "beta note", "Inbox is empty.")
	wantNoText(t, "the other task's detail", m, "alpha note")
	wantSelected(t, "cursor", m, "beta")
}

func TestSplit_EveryLineFillsTheTerminalExactly(t *testing.T) {
	r := splitRig(t)
	r.unfile(t, "one note")
	r.unfile(t, "another note with quite a lot more words in it than the panel will ever have room for")
	for _, size := range [][2]int{{100, 16}, {100, 24}, {120, 30}, {140, 40}, {200, 50}, {101, 17}} {
		m := splitModel(r, size[0], size[1])
		if !isSplit(m) {
			t.Fatalf("%dx%d: not the split layout:\n%s", size[0], size[1], screen(m))
		}
		lines := strings.Split(raw(m), "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines\n%s", size[0], size[1], len(lines), screen(m))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Errorf("%dx%d: a line is %d wide: %q", size[0], size[1], w, ansi.Strip(line))
			}
		}
		// The panel rows reach both edges: every body line starts and ends on a border.
		for _, line := range strings.Split(screen(m), "\n") {
			if panelRow := strings.ContainsAny(line[:min(len(line), 3)], "│╭╰"); panelRow && ansi.StringWidth(line) != size[0] {
				t.Errorf("%dx%d: a panel row is %d wide, want the whole width: %q", size[0], size[1], ansi.StringWidth(line), line)
			}
		}
	}
}

func TestSplit_StaysOneSinglePaneWhenThereIsNoRoom(t *testing.T) {
	r := splitRig(t)
	for name, m := range map[string]tea.Model{
		"too narrow":    splitModel(r, 99, 30),
		"too short":     splitModel(r, 120, 15),
		"size unknown":  booted(r.newSplitModel()),
		"a small term.": splitModel(r, 80, 24),
	} {
		if strings.Contains(screen(m), "╭") {
			t.Errorf("%s: drew panels:\n%s", name, screen(m))
		}
		wantScreen(t, name, m, "beta", "alpha")
	}
	if !isSplit(splitModel(r, 100, 16)) {
		t.Error("100x16 is the smallest split layout, and it was not drawn")
	}
}

func TestSplit_TheModeDecidesWhichPanelHasTheKeyboard(t *testing.T) {
	r := splitRig(t)
	r.unfile(t, "a stray thought")
	m := splitModel(r, 120, 30)

	focused := func(m tea.Model, title string) bool {
		edge := edgeOf(t, m, title)
		return edge.fg == ansiBlue && !edge.faint
	}
	dim := func(m tea.Model, title string) bool {
		edge := edgeOf(t, m, title)
		return edge.faint && edge.fg == -1
	}
	check := func(what string, m tea.Model, on string, off ...string) {
		t.Helper()
		if !focused(m, on) {
			t.Errorf("%s: %s is not the focused panel:\n%s", what, on, screen(m))
		}
		for _, title := range off {
			if !dim(m, title) {
				t.Errorf("%s: %s is not dimmed:\n%s", what, title, screen(m))
			}
		}
	}
	check("list", m, "Tasks", "Inbox", "Detail")
	check("inbox", press(m, keyI), "Inbox", "Tasks", "Note")
	check("detail", press(m, keyL), "Detail", "Tasks", "Inbox")
	check("back from the detail", press(m, keyL, keyEsc), "Tasks", "Inbox", "Detail")
	check("back from the inbox", press(m, keyI, keyEsc), "Tasks", "Inbox", "Detail")
}

func TestSplit_TheDetailFollowsTheTasksCursor(t *testing.T) {
	r := splitRig(t)
	m := splitModel(r, 120, 30)
	wantScreen(t, "beta under the cursor", m, "beta note")

	m = press(m, keyJ)
	wantSelected(t, "moved", m, "alpha")
	wantScreen(t, "alpha's detail", m, "alpha note")
	wantNoText(t, "beta's detail", m, "beta note")

	m = press(m, keyK)
	wantScreen(t, "back on beta", m, "beta note")
	wantNoText(t, "alpha's", m, "alpha note")
}

func TestSplit_TheDetailPaneScrollsWhileTheTasksCursorStays(t *testing.T) {
	r := splitRig(t)
	for i := 0; i < 30; i++ {
		if _, err := r.tracker.AddNote(ctx, 2, fmt.Sprintf("filler %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	m := splitModel(r, 120, 24)
	m = press(m, keyL)
	wantScreen(t, "newest note first", m, "filler 29")
	m = press(m, keyJ, keyJ, keyJ)
	wantNoText(t, "scrolled past", m, "filler 29")
	wantSelected(t, "the Tasks cursor stayed", m, "beta")
	m = press(m, keyEsc)
	wantSelected(t, "after esc", m, "beta")
}

func TestSplit_TheInboxPanelListsNotesAndACollapsedStripWhenEmpty(t *testing.T) {
	r := splitRig(t)
	m := splitModel(r, 120, 30)
	lines := strings.Split(screen(m), "\n")
	inbox := -1
	for i, l := range lines {
		if strings.Contains(l, "╭─ Inbox ") {
			inbox = i
		}
	}
	if inbox < 0 || !strings.HasPrefix(lines[inbox+2], "╰") {
		t.Errorf("an empty inbox is not a three line strip:\n%s", screen(m))
	}

	r.unfile(t, "first stray")
	r.unfile(t, "second stray")
	m = splitModel(r, 120, 30)
	wantScreen(t, "notes", m, "╭─ Inbox (2)", "first stray", "second stray")
	wantNoText(t, "strip", m, "Inbox is empty.")
	wantSelected(t, "the one cursor is in the Tasks panel", m, "beta")

	m = press(m, keyI, keyEsc)
	wantScreen(t, "the notes are still there on the way back", m, "first stray", "second stray")
}

func TestSplit_TheLeftColumnIsAboutFortyPercentBetweenFortyAndFiftySixColumns(t *testing.T) {
	r := splitRig(t)
	for width, want := range map[int]int{100: 40, 120: 48, 130: 52, 140: 56, 200: 56} {
		m := splitModel(r, width, 30)
		for _, line := range strings.Split(screen(m), "\n") {
			if strings.Contains(line, "╭─ Tasks") {
				if got := strings.Index(line, "╮"); ansi.StringWidth(line[:got])+1 != want {
					t.Errorf("width %d: the left column is %d wide, want %d", width, ansi.StringWidth(line[:got])+1, want)
				}
			}
		}
	}
}

func TestSplit_ScrollingTheDetailDoesNotCarryOverToTheNextTask(t *testing.T) {
	r := splitRig(t)
	for i := 0; i < 30; i++ {
		for id := 1; id <= 2; id++ {
			if _, err := r.tracker.AddNote(ctx, core.TaskID(id), fmt.Sprintf("filler %02d", i)); err != nil {
				t.Fatal(err)
			}
		}
	}
	m := press(splitModel(r, 120, 24), keyL, keyJ, keyJ, keyJ, keyEsc)
	wantNoText(t, "scrolled", m, "filler 29")
	m = press(m, keyJ)
	wantScreen(t, "the next task from its top", m, "filler 29")
}

func TestSplit_TheInboxPaneShowsTheSelectedNoteInFullWithItsKeys(t *testing.T) {
	r := splitRig(t)
	r.unfile(t, "short one")
	r.unfile(t, "the long one "+strings.Repeat("and on and on ", 12)+"THE END")
	m := press(splitModel(r, 120, 30), keyI)
	wantScreen(t, "first note", m, "╭─ Note", "short one", "enter: file", "c: new task")
	wantNoText(t, "second note's end", m, "THE END")

	m = press(m, keyJ)
	wantScreen(t, "second note, wrapped, not cut off", m, "THE END")
}

func TestSplit_FilingANoteFromThePanelStillWorks(t *testing.T) {
	r := splitRig(t)
	r.unfile(t, "file me")
	m := press(splitModel(r, 120, 30), keyI, keyC)
	wantScreen(t, "created", m, "Created task: file me")
	if got := r.unfiledCount(t); got != 0 {
		t.Errorf("unfiled = %d, want 0", got)
	}
	wantScreen(t, "still the split layout", m, "╭─ Inbox", "Inbox is empty.")
}

func TestSplit_VTogglesBetweenTheSplitAndASinglePane(t *testing.T) {
	r := splitRig(t)
	m := splitModel(r, 120, 30)

	m = press(m, keyV)
	if isSplit(m) || strings.Contains(screen(m), "╭") {
		t.Errorf("v did not leave the split layout:\n%s", screen(m))
	}
	wantScreen(t, "the plain list", m, "beta", "alpha")
	wantSelected(t, "cursor", m, "beta")
	wantNoText(t, "no detail beside it", m, "beta note")

	m = press(m, keyV)
	wantScreen(t, "split again", m, "╭─ Tasks", "beta note")

	m = press(m, keyL, keyV)
	wantScreen(t, "the detail is a full screen in a single pane", m, "beta note", "State:")
	wantNoText(t, "panels", m, "╭")
	m = press(m, keyV)
	wantScreen(t, "and the detail pane in the split", m, "╭─ Detail", "beta note")

	r.unfile(t, "stray")
	m = press(splitModel(r, 120, 30), keyI, keyV)
	wantNoText(t, "panels", m, "╭")
	wantScreen(t, "the inbox as a full screen", m, "stray", "enter file")
}

func TestSplit_VSaysWhyWhenTheWindowIsTooSmall(t *testing.T) {
	r := splitRig(t)
	m := press(splitModel(r, 80, 24), keyV)
	wantScreen(t, "notice", m, "too small for the split layout", "100×16")
	wantNoText(t, "panels", m, "╭")
	// The refusal is remembered nowhere: growing the window shows the split.
	m = sized(m, 120, 30)
	wantScreen(t, "grown", m, "╭─ Tasks")
}

func TestSplit_TheSingleLayoutSettingStartsWithoutPanelsAndVTurnsThemOn(t *testing.T) {
	r := splitRig(t)
	m := sized(booted(tui.New(r.tracker, tui.WithTick(noTick), tui.WithLayout("single"))), 120, 30)
	wantNoText(t, "single", m, "╭")
	m = press(m, keyV)
	wantScreen(t, "split", m, "╭─ Tasks", "beta note")

	for _, layout := range []string{"auto", "split", ""} {
		m := sized(booted(tui.New(r.tracker, tui.WithTick(noTick), tui.WithLayout(layout))), 120, 30)
		wantScreen(t, "layout "+layout, m, "╭─ Tasks")
	}
}

func TestSplit_ThePromptsTheReportAndTheDocsTakeTheWholeScreen(t *testing.T) {
	r := splitRig(t)
	m := splitModel(r, 120, 30)
	for name, keys := range map[string][]tea.KeyPressMsg{
		"add prompt":  {keyA},
		"note prompt": {keyN},
		"picker":      {keySlash},
		"report":      {keyR},
	} {
		got := press(m, keys...)
		wantNoText(t, name, got, "╭─ Detail")
	}
	m = press(m, keyA)
	m = press(m, keyEsc)
	wantScreen(t, "back from the prompt", m, "╭─ Tasks", "╭─ Detail")
}

func TestSplit_AFilterNarrowsTheTasksPanelAndTheDetailFollows(t *testing.T) {
	r := splitRig(t)
	m := splitModel(r, 120, 30)
	m = press(m, keySlash)
	m = typeText(m, "alp")
	m = press(m, keyEnter)
	wantScreen(t, "filtered", m, "╭─ Tasks", "Filter: alp", "alpha note")
	wantNoText(t, "beta", m, "beta note")
}

func TestSplit_OnlyAsManyTasksAsFitWithAFullInboxBesideThem(t *testing.T) {
	r := splitRig(t)
	for i := 0; i < 12; i++ {
		r.unfile(t, fmt.Sprintf("stray %02d", i))
	}
	m := splitModel(r, 100, 16)
	wantScreen(t, "tasks still show", m, "beta", "alpha", "╭─ Inbox (12)")
	if got := strings.Count(screen(m), "\n") + 1; got != 16 {
		t.Errorf("%d lines in a 16 line terminal", got)
	}
}
