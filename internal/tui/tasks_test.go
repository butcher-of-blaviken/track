package tui_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

// addTask creates a Task through the tracker, as another process would, and
// moves the clock on so later Tasks are strictly newer.
func (r *rig) addTask(t *testing.T, text string) core.Task {
	t.Helper()
	task, err := r.tracker.AddTask(ctx, text)
	if err != nil {
		t.Fatalf("AddTask(%q): %v", text, err)
	}
	r.clock.Advance(time.Minute)
	return task
}

func press(m tea.Model, keys ...tea.KeyPressMsg) tea.Model {
	for _, k := range keys {
		m = send(m, k)
	}
	return m
}

func typeText(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

var (
	keyA     = tea.KeyPressMsg{Code: 'a', Text: "a"}
	keyJ     = tea.KeyPressMsg{Code: 'j', Text: "j"}
	keyK     = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

var selectedRow = regexp.MustCompile(`(?m)^ *> (.*)$`)

// selected returns the text of the row with the cursor on it.
func selected(t *testing.T, m tea.Model) string {
	t.Helper()
	found := selectedRow.FindAllStringSubmatch(ansi.Strip(screen(m)), -1)
	if len(found) != 1 {
		t.Fatalf("want exactly one selected row, found %d in:\n%s", len(found), screen(m))
	}
	return found[0][1]
}

func wantSelected(t *testing.T, what string, m tea.Model, title string) {
	t.Helper()
	if got := selected(t, m); !strings.Contains(got, title) {
		t.Errorf("%s: selected row is %q, want it to contain %q", what, got, title)
	}
}

func TestList_EmptyStateHintsAtAdd(t *testing.T) {
	m := booted(newRig(t).newModel())
	wantScreen(t, "no tasks", m, "No tasks", "a add")
}

func TestList_ShowsActiveTasksNewestFirstWithTagChipsAndACursor(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "oldest task")
	r.addTask(t, "middle task ##Docs")
	r.addTask(t, "newest task ##PROJ-123 ##backend")
	m := booted(r.newModel())

	got := ansi.Strip(screen(m))
	newest, middle, oldest := strings.Index(got, "newest task"), strings.Index(got, "middle task"), strings.Index(got, "oldest task")
	if newest < 0 || newest >= middle || middle >= oldest {
		t.Errorf("tasks are not listed newest first:\n%s", got)
	}
	wantScreen(t, "chips", m, "#PROJ-123", "#backend", "#Docs")
	if strings.Contains(got, "##") {
		t.Errorf("tags should show as #tag, not the ## input marker:\n%s", got)
	}
	wantSelected(t, "initial cursor", m, "newest task")
}

func TestList_OnlyActiveTasksAreShown(t *testing.T) {
	r := newRig(t)
	done := r.addTask(t, "finished task")
	r.addTask(t, "live task")
	if err := r.store.Update(ctx, func(tx core.Tx) error {
		task, err := tx.Task(done.ID)
		if err != nil {
			return err
		}
		if err := task.Done(); err != nil {
			return err
		}
		return tx.SaveTask(task)
	}); err != nil {
		t.Fatal(err)
	}
	m := booted(r.newModel())
	wantScreen(t, "active", m, "live task")
	if strings.Contains(screen(m), "finished task") {
		t.Errorf("a Done task is listed by default:\n%s", screen(m))
	}
}

func TestList_CursorMovesWithJKAndArrowsAndClampsAtTheEnds(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "third")
	r.addTask(t, "second")
	r.addTask(t, "first")
	m := booted(r.newModel())
	wantSelected(t, "start", m, "first")

	m = press(m, keyUp) // clamps at the top
	wantSelected(t, "up at the top", m, "first")
	m = press(m, keyJ)
	wantSelected(t, "j", m, "second")
	m = press(m, keyDown)
	wantSelected(t, "down", m, "third")
	m = press(m, keyJ, keyDown) // clamps at the bottom
	wantSelected(t, "past the bottom", m, "third")
	m = press(m, keyK)
	wantSelected(t, "k", m, "second")
	m = press(m, keyUp)
	wantSelected(t, "up", m, "first")
}

func TestAdd_CreatesTheTaskWithTagsAndSelectsIt(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "existing")
	m := booted(r.newModel())

	m = press(m, keyA)
	wantScreen(t, "prompt open", m, "Add task")
	m = typeText(m, "finishing up auth ##PROJ-123")
	m = press(m, keyEnter)

	if strings.Contains(screen(m), "Add task") {
		t.Errorf("the prompt is still open after submitting:\n%s", screen(m))
	}
	wantScreen(t, "new task", m, "finishing up auth", "#PROJ-123")
	wantSelected(t, "cursor", m, "finishing up auth")

	tasks, err := r.tracker.Tasks(ctx, core.StateActive)
	if err != nil || len(tasks) != 2 || tasks[0].Title != "finishing up auth" || len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "PROJ-123" {
		t.Errorf("stored tasks = %+v, %v; want the new Task stored with its title and Tag", tasks, err)
	}
}

func TestAdd_EscCancelsWithoutCreatingAnything(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	m = press(m, keyA)
	m = typeText(m, "never mind")
	m = press(m, keyEsc)

	if strings.Contains(screen(m), "Add task") || strings.Contains(screen(m), "never mind") {
		t.Errorf("the prompt or its text is still on screen after Esc:\n%s", screen(m))
	}
	if tasks, _ := r.tracker.Tasks(ctx); len(tasks) != 0 {
		t.Errorf("%d tasks stored after cancelling, want 0", len(tasks))
	}
	// A fresh prompt starts empty.
	m = press(m, keyA)
	if strings.Contains(screen(m), "never mind") {
		t.Errorf("the cancelled text came back:\n%s", screen(m))
	}
}

func TestAdd_EmptySubmissionShowsAnErrorAndKeepsThePromptOpen(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	for _, text := range []string{"", "##only ##tags"} {
		m = press(m, keyA)
		m = typeText(m, text)
		m = press(m, keyEnter)
		wantScreen(t, fmt.Sprintf("submitting %q", text), m, "Add task", "title")
		m = press(m, keyEsc)
	}
	if tasks, _ := r.tracker.Tasks(ctx); len(tasks) != 0 {
		t.Errorf("%d tasks stored after empty submissions, want 0", len(tasks))
	}

	m = press(m, keyA)
	m = typeText(m, "oops")
	m = press(m, keyEnter)
	wantScreen(t, "a real title", m, "oops")
}

func TestAdd_KeysAreTypedIntoThePromptNotActedOn(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "one")
	r.addTask(t, "two")
	m := booted(r.newModel())
	m = press(m, keyA)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("q quit the program while the prompt was open")
		}
	}
	m = typeText(m, "quick jab")
	wantScreen(t, "typed text", m, "quick jab")
	m = press(m, keyEsc)
	wantSelected(t, "j and k typed in the prompt must not have moved the cursor", m, "two")

	m = press(m, keyA)
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c returned no command while the prompt was open")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit while the prompt was open")
	}
}

func TestRefresh_PicksUpTasksAddedElsewhereAndKeepsTheCursorOnItsTask(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "older")
	r.addTask(t, "newer")
	m := booted(r.newModel())
	m = press(m, keyJ)
	wantSelected(t, "before", m, "older")

	r.addTask(t, "added by another process") // newest, so it lands above the cursor
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after the tick", m, "added by another process")
	wantSelected(t, "the cursor follows its task, not its row number", m, "older")
}

func TestRefresh_ClampsTheCursorWhenItsTaskDisappears(t *testing.T) {
	r := newRig(t)
	oldest := r.addTask(t, "goes away")
	r.addTask(t, "stays")
	m := booted(r.newModel())
	m = press(m, keyJ)
	wantSelected(t, "before", m, "goes away")

	if err := r.store.Update(ctx, func(tx core.Tx) error {
		task, err := tx.Task(oldest.ID)
		if err != nil {
			return err
		}
		if err := task.Archive(); err != nil {
			return err
		}
		return tx.SaveTask(task)
	}); err != nil {
		t.Fatal(err)
	}
	m = send(m, tui.TickMsg{})
	wantSelected(t, "after the selected task was archived elsewhere", m, "stays")
}

func TestList_ScrollsToKeepTheCursorVisibleOnAShortTerminal(t *testing.T) {
	r := newRig(t)
	for i := 1; i <= 30; i++ {
		r.addTask(t, fmt.Sprintf("task number %02d", i)) // 30 is the newest, listed first
	}
	m := booted(r.newModel())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 12})
	wantScreen(t, "top of the list", m, "task number 30")

	for range 29 {
		m = press(m, keyJ)
	}
	wantSelected(t, "bottom of the list", m, "task number 01")
	if strings.Contains(screen(m), "task number 30") {
		t.Errorf("the top of the list is still shown after scrolling to the bottom:\n%s", screen(m))
	}
	if got := strings.Count(screen(m), "\n") + 1; got > 12 {
		t.Errorf("the screen is %d lines, want at most the terminal's 12:\n%s", got, screen(m))
	}

	for range 29 {
		m = press(m, keyK)
	}
	wantSelected(t, "back at the top", m, "task number 30")
}

// cellUnderCursor returns the character the terminal cursor is drawn over.
func cellUnderCursor(t *testing.T, m tea.Model) string {
	t.Helper()
	v := m.View()
	if v.Cursor == nil {
		t.Fatalf("no cursor on screen:\n%s", v.Content)
	}
	lines := strings.Split(ansi.Strip(v.Content), "\n")
	if v.Cursor.Y >= len(lines) {
		t.Fatalf("cursor row %d is off the %d-line screen:\n%s", v.Cursor.Y, len(lines), v.Content)
	}
	cells := []rune(lines[v.Cursor.Y])
	if v.Cursor.X >= len(cells) {
		return " " // past the end of the line: nothing is covered
	}
	return string(cells[v.Cursor.X])
}

func TestAdd_TheEmptyPromptShowsItsWholeHintAndTheCursorCoversNoText(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(m, keyA)

	wantScreen(t, "empty prompt", m, "Add task:", "what are you working on?", "##tag")
	if got := cellUnderCursor(t, m); got != " " {
		t.Errorf("the cursor covers %q, want a blank cell so no hint text is hidden", got)
	}
}

func TestAdd_LongInputScrollsAndTheCursorStaysOnScreen(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = send(m, tea.WindowSizeMsg{Width: 30, Height: 10})
	m = press(m, keyA)
	m = typeText(m, "a task title that is much longer than the terminal is wide")

	v := m.View()
	if v.Cursor == nil || v.Cursor.X >= 30 {
		t.Fatalf("cursor = %+v, want it inside the 30-column terminal", v.Cursor)
	}
	wantScreen(t, "the end of the text stays visible", m, "terminal is wide")
	for i, line := range strings.Split(v.Content, "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Errorf("line %d is %d cells wide, want at most 30: %q", i, w, line)
		}
	}
}

// promptLine returns the raw (still styled) line holding the add prompt.
func promptLine(t *testing.T, m tea.Model) string {
	t.Helper()
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(ansi.Strip(line), "Add task:") {
			return line
		}
	}
	t.Fatalf("no prompt line on screen:\n%s", screen(m))
	return ""
}

func TestAdd_TheHintIsStyledAsAPromptAndTypedTextIsNot(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(m, keyA)

	if line := promptLine(t, m); ansi.Strip(line) == line {
		t.Errorf("the hint carries no styling, want it greyed out: %q", line)
	}

	m = typeText(m, "real text")
	if line := promptLine(t, m); ansi.Strip(line) != line {
		t.Errorf("typed text is styled, want it plain: %q", line)
	}
}

func TestFooter_ShowsTheKeysForTheCurrentMode(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	wantScreen(t, "list mode", m, "enter start", "x stop", "a add", "j/k move", "q quit")

	m = press(m, keyA)
	wantScreen(t, "prompt mode", m, "enter add", "esc cancel", "ctrl+c quit")
	for _, listKey := range []string{"x stop", "j/k move", "q quit", "a add"} {
		if strings.Contains(screen(m), listKey) {
			t.Errorf("the prompt footer still advertises the list key %q:\n%s", listKey, screen(m))
		}
	}

	m = press(m, keyEsc)
	wantScreen(t, "back in list mode", m, "enter start", "x stop")
}

func TestFooter_TruncatesToTheTerminalWidth(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = send(m, tea.WindowSizeMsg{Width: 24, Height: 12})
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(line); w > 24 {
			t.Errorf("line %d is %d cells wide, want at most 24: %q", i, w, line)
		}
	}
	wantScreen(t, "the first keys survive", m, "enter start")
}

func TestFooter_NeverCutsAHintInHalf(t *testing.T) {
	hints := map[string]bool{"enter start": true, "x stop": true, "a add": true, "d done": true, "/ find": true, "tab scope": true, "j/k move": true, "q quit": true}
	for width := 12; width <= 60; width++ {
		m := booted(newRig(t).newModel())
		m = send(m, tea.WindowSizeMsg{Width: width, Height: 12})
		for _, line := range strings.Split(screen(m), "\n") {
			if !strings.Contains(line, "enter start") {
				continue
			}
			for _, part := range strings.Split(line, "•") {
				hint := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), "…"))
				if !hints[hint] {
					t.Errorf("width %d: footer %q contains a partial hint %q", width, line, hint)
				}
			}
		}
	}
}
