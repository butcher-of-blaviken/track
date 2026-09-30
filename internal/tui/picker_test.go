package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keySlash = tea.KeyPressMsg{Code: '/', Text: "/"}
	keyCtrlP = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	keyCtrlN = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
)

// pickerRig has three Active Tasks, newest first on the list:
// "prd review", "fix the build", "write the PRD".
func pickerRig(t *testing.T) (*rig, tea.Model) {
	t.Helper()
	r := newRig(t)
	r.addTask(t, "write the PRD")
	r.addTask(t, "fix the build")
	r.addTask(t, "prd review")
	return r, booted(r.newModel())
}

// rowsOf returns the Task rows on the screen, in order.
func rowsOf(m tea.Model) []string {
	var rows []string
	for _, line := range strings.Split(screen(m), "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		for _, title := range []string{"write the PRD", "fix the build", "prd review"} {
			if strings.HasPrefix(trimmed, title) {
				rows = append(rows, title)
			}
		}
	}
	return rows
}

func wantRows(t *testing.T, what string, m tea.Model, want ...string) {
	t.Helper()
	got := rowsOf(m)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s: rows = %v, want %v\n%s", what, got, want, screen(m))
	}
}

func TestPicker_SlashAndCtrlPOpenItAndTypingNarrowsAndRanksTheList(t *testing.T) {
	for name, open := range map[string]tea.KeyPressMsg{"slash": keySlash, "ctrl+p": keyCtrlP} {
		t.Run(name, func(t *testing.T) {
			_, m := pickerRig(t)
			m = press(m, open)
			wantRows(t, "empty query", m, "prd review", "fix the build", "write the PRD")

			m = typeText(m, "prd")
			wantRows(t, "prd", m, "prd review", "write the PRD")
			wantSelected(t, "best match first", m, "prd review")

			m = typeText(m, " wr")
			wantRows(t, "prd wr", m, "write the PRD")
		})
	}
}

func TestPicker_LettersAreTextIncludingTheListKeys(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keySlash)
	m = typeText(m, "jksxq")
	wantScreen(t, "typed", m, "jksxq")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions after typing s, want 0", n)
	}
}

func TestPicker_ArrowsAndCtrlNPMoveTheHighlightAndStopAtTheEnds(t *testing.T) {
	_, m := pickerRig(t)
	m = press(m, keySlash)
	wantSelected(t, "start", m, "prd review")
	m = press(m, keyDown)
	wantSelected(t, "down", m, "fix the build")
	m = press(m, keyCtrlN, keyCtrlN, keyCtrlN)
	wantSelected(t, "clamped at the end", m, "write the PRD")
	m = press(m, keyCtrlP)
	wantSelected(t, "ctrl+p", m, "fix the build")
	m = press(m, keyUp, keyUp, keyUp)
	wantSelected(t, "clamped at the start", m, "prd review")
}

func TestPicker_FilterEnterKeepsTheListFilteredEscClearsIt(t *testing.T) {
	_, m := pickerRig(t)
	m = press(m, keySlash)
	m = typeText(m, "prd")
	m = press(m, keyDown, keyEnter)

	wantRows(t, "applied", m, "prd review", "write the PRD")
	wantScreen(t, "header", m, "Filter: prd")
	wantSelected(t, "cursor on the chosen Task", m, "write the PRD")

	m = press(m, keyEsc)
	wantRows(t, "cleared", m, "prd review", "fix the build", "write the PRD")
	if strings.Contains(screen(m), "Filter:") {
		t.Errorf("the filter line is still shown:\n%s", screen(m))
	}
	wantSelected(t, "cursor stays on the Task", m, "write the PRD")
}

func TestPicker_JAndKMoveWithinAFilteredListAndSStartsTheCursorTask(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keySlash)
	m = typeText(m, "prd")
	m = press(m, keyEnter)
	m = press(m, keyJ)
	wantSelected(t, "j", m, "write the PRD")
	m = press(m, keyJ)
	wantSelected(t, "clamped", m, "write the PRD")
	m = press(m, keyS)
	wantFocusLine(t, "panel", m, "30:00", "write the PRD")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
}

func TestPicker_EscInThePickerCancelsAndLeavesTheListAsItWas(t *testing.T) {
	_, m := pickerRig(t)
	m = press(m, keyJ) // "fix the build"
	m = press(m, keySlash)
	m = typeText(m, "prd")
	m = press(m, keyEsc)
	wantRows(t, "after cancelling", m, "prd review", "fix the build", "write the PRD")
	wantSelected(t, "cursor is back", m, "fix the build")
	if strings.Contains(screen(m), "Filter:") {
		t.Errorf("a filter was applied by cancelling:\n%s", screen(m))
	}

	// With a filter already applied, cancelling a new picker keeps that filter.
	m = press(m, keySlash)
	m = typeText(m, "prd")
	m = press(m, keyEnter)
	m = press(m, keySlash)
	m = typeText(m, "zzz")
	m = press(m, keyEsc)
	wantRows(t, "previous filter kept", m, "prd review", "write the PRD")
}

func TestPicker_FilterWithNoMatchesSaysSoAndEnterDoesNothing(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keySlash)
	m = typeText(m, "zzz")
	wantScreen(t, "no matches", m, "No matches")
	if strings.Contains(screen(m), "Create") {
		t.Errorf("filter mode offers to create:\n%s", screen(m))
	}
	m = press(m, keyEnter)
	wantScreen(t, "still open", m, "No matches", "zzz")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions, want 0", n)
	}
}

func TestPicker_StartEnterStartsTheHighlightedTask(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keyCtrlP)
	m = typeText(m, "build")
	m = press(m, keyEnter)
	wantFocusLine(t, "panel", m, "30:00", "fix the build")
	if strings.Contains(screen(m), "Start:") {
		t.Errorf("the picker is still open:\n%s", screen(m))
	}
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
}

func TestPicker_StartDuringABreakAsksForConfirmation(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.addTask(t, "other")
	m := booted(r.newModel())
	r.clock.Set(epoch.Add(sessionEnd + 4*time.Minute))
	m = send(m, tui.TickMsg{})
	m = press(m, keyEsc) // hand-off
	m = press(m, keyCtrlP)
	m = typeText(m, "other")
	m = press(m, keyEnter)
	wantScreen(t, "confirmation", m, "Start anyway")
	m = press(m, keyY)
	wantFocusLine(t, "panel", m, "30:00", "other")
	if s := r.storedSessions(t); len(s) != 2 || s[1].SkippedBreak <= 0 {
		t.Errorf("sessions = %+v, want an override recorded", s)
	}
}

func TestPicker_StartWhileASessionRunsShowsTheNotice(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "one")
	r.addTask(t, "two")
	m := booted(r.newModel())
	m = press(m, keyS)
	m = press(m, keyCtrlP)
	m = typeText(m, "one")
	m = press(m, keyEnter)
	wantScreen(t, "notice", m, "already running")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
}

func TestPicker_StartOffersToCreateWhenNothingMatchesAndEnterCreatesAndStarts(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keyCtrlP)
	m = typeText(m, "brand new ##docs")
	wantScreen(t, "create row", m, `Create "brand new ##docs"`)
	m = press(m, keyEnter)

	wantFocusLine(t, "panel", m, "30:00", "brand new")
	tasks, err := r.tracker.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range tasks {
		if task.Title == "brand new" && len(task.Tags) == 1 && task.Tags[0] == "docs" {
			found = true
		}
	}
	if !found {
		t.Errorf("tasks = %+v, want a \"brand new\" Task tagged docs", tasks)
	}
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
}

func TestPicker_CreateIsNotOfferedWhenSomethingMatches(t *testing.T) {
	_, m := pickerRig(t)
	m = press(m, keyCtrlP)
	m = typeText(m, "prd")
	if strings.Contains(screen(m), "Create") {
		t.Errorf("create offered with matches:\n%s", screen(m))
	}
}

func TestPicker_CreateWithOnlyTagsShowsTheTitleErrorAndCreatesNothing(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keyCtrlP)
	m = typeText(m, "##onlytag")
	m = press(m, keyEnter)
	wantScreen(t, "error", m, "needs a title")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions, want 0", n)
	}
	tasks, _ := r.tracker.Tasks(ctx)
	if len(tasks) != 3 {
		t.Errorf("%d tasks, want the original 3", len(tasks))
	}
}

func TestPicker_CreateDuringABreakCreatesTheTaskThenAsks(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	r.clock.Set(epoch.Add(sessionEnd + 4*time.Minute))
	m = send(m, tui.TickMsg{})
	m = press(m, keyEsc)
	m = press(m, keyCtrlP)
	m = typeText(m, "zzz fresh")
	m = press(m, keyEnter)
	wantScreen(t, "confirmation", m, "Start anyway")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Fatalf("%d sessions before confirming, want 1", n)
	}
	m = press(m, keyY)
	wantFocusLine(t, "panel", m, "30:00", "zzz fresh")
}

func TestPicker_ShowsATaskAddedElsewhereAndKeepsTheHighlightOnItsTask(t *testing.T) {
	r, m := pickerRig(t)
	m = press(m, keySlash)
	m = press(m, keyDown) // "fix the build"
	r.addTask(t, "written elsewhere")
	m = send(m, tui.TickMsg{})
	wantScreen(t, "new task listed", m, "written elsewhere")
	wantSelected(t, "highlight stays", m, "fix the build")
}

func TestPicker_TheHandoffWaitsUntilItCloses(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m = press(m, keySlash)
	m = typeText(m, "ta")
	m, _ = tickAt(t, r, m, sessionEnd)
	wantNoPrompt(t, "while searching", m)
	wantScreen(t, "picker intact", m, "ta")
	m = press(m, keyEsc)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after closing", m, "Hand-off for")
}

func TestPicker_FooterShowsItsKeysAndNothingOverflows(t *testing.T) {
	_, m := pickerRig(t)
	m = press(m, keySlash)
	wantScreen(t, "filter footer", m, "enter", "esc")
	m = press(m, keyEsc, keyCtrlP)
	wantScreen(t, "start footer", m, "enter start", "esc")
}

func TestPicker_CtrlCQuits(t *testing.T) {
	_, m := pickerRig(t)
	m = press(m, keySlash)
	if _, cmd := m.Update(keyCtrlC); cmd == nil {
		t.Error("ctrl+c returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
}
