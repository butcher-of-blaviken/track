package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keyL = tea.KeyPressMsg{Code: 'l', Text: "l"}
	keyH = tea.KeyPressMsg{Code: 'h', Text: "h"}
)

func stamp(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

func TestDetail_LOpensTheCursorTasksDetailAndEscOrHGoesBack(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD ##docs")
	m := booted(r.newModel())

	m = press(m, keyL)
	wantScreen(t, "detail", m, "write the PRD", "#docs", "Focused: none yet", "Notes", "No notes yet. Press n to add one.", "esc back")
	for _, back := range []tea.KeyPressMsg{keyEsc, keyH} {
		m = press(m, back)
		wantNoText(t, "back on the list", m, "Focused:")
		wantScreen(t, "back on the list", m, "write the PRD")
		m = press(m, keyL)
		wantScreen(t, "reopened", m, "Focused:")
	}
}

func TestDetail_TheLogIsNewestFirstWithTimestamps(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "some task")
	first, _ := r.tracker.AddNote(ctx, task.ID, "first thought")
	r.clock.Advance(3 * time.Hour)
	second, _ := r.tracker.AddNote(ctx, task.ID, "second thought")
	if _, err := r.tracker.AddUnfiledNote(ctx, "not on this task"); err != nil {
		t.Fatal(err)
	}
	m := press(booted(r.newModel()), keyL)

	wantScreen(t, "log", m, stamp(first.CreatedAt)+"  first thought", stamp(second.CreatedAt)+"  second thought")
	wantNoText(t, "log", m, "not on this task")
	if indexOf(t, m, "second thought") > indexOf(t, m, "first thought") {
		t.Errorf("want the newest note first:\n%s", screen(m))
	}
}

func TestDetail_MarksANonActiveTask(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "finished work")
	r.setState(t, task, core.StateDone)
	m := booted(r.newModel())
	m = press(m, keyTab, keyL)
	wantScreen(t, "done task", m, "finished work (done)")
}

func TestDetail_ShowsFocusedTimeThatCountsUpAndOnlyForThisTask(t *testing.T) {
	r := newRig(t)
	other := r.addTask(t, "other")
	r.startSession(t) // a Task called "task", 30m
	m := booted(r.newModel())
	r.clock.Advance(5 * time.Minute)
	m = send(m, tui.TickMsg{})

	// Newest first: "task" is the top row.
	m = press(m, keyL)
	wantScreen(t, "running", m, "Focused: 5m over 1 session")
	r.clock.Advance(5 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "counting up", m, "Focused: 10m over 1 session")
	r.clock.Advance(3 * time.Hour)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "capped", m, "Focused: 30m over 1 session")

	m = press(m, keyEsc, keyJ, keyL)
	wantScreen(t, "the other task", m, "other", "Focused: none yet")
	_ = other
}

func TestDetail_FocusedTimeRoundsToMinutesAndShowsHours(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, "worked on").ID
	for range 3 { // 3 x 30m
		if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(40 * time.Minute)
	}
	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(20*time.Second + 0)
	if _, err := r.tracker.StopSession(ctx); err != nil { // 20s more
		t.Fatal(err)
	}
	m := press(booted(r.newModel()), keyEsc, keyEsc, keyL) // skip the hand-off, decline the resume prompt
	wantScreen(t, "hours", m, "Focused: 1h 30m over 4 sessions")
}

func TestDetail_NAddsANoteToThisTask(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "some task")
	m := press(booted(r.newModel()), keyL, keyN)
	wantScreen(t, "prompt", m, "Note:", "enter save", "esc cancel")

	m = press(m, keyEnter)
	wantScreen(t, "empty", m, "Write a note")
	m = typeText(m, "remember   this")
	m = press(m, keyEnter)

	wantScreen(t, "back in the detail with the note", m, "Focused:", "remember this")
	wantNoText(t, "prompt closed", m, "Note:")
	log, _ := r.tracker.TaskNotes(ctx, task.ID)
	if len(log) != 1 || log[0].Text != "remember this" {
		t.Errorf("log = %+v, want the one normalised note", log)
	}
	if n := r.unfiledCount(t); n != 0 {
		t.Errorf("%d unfiled notes, want the note filed on the Task", n)
	}
}

func TestDetail_EscFromTheNotePromptStaysInTheDetail(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "some task")
	m := press(booted(r.newModel()), keyL, keyN)
	m = typeText(m, "unsent")
	m = press(m, keyEsc)
	wantScreen(t, "detail", m, "Focused:")
	wantNoText(t, "prompt", m, "unsent")
}

func TestDetail_SStartsASessionAndLeavesTheDetail(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "some task")
	m := press(booted(r.newModel()), keyL, keyS)
	wantFocusLine(t, "panel", m, "30:00", "some task")
	wantNoText(t, "left the detail", m, "Notes")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
}

func TestDetail_SOnAnInactiveTaskShowsTheNotice(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "finished work")
	r.setState(t, task, core.StateDone)
	m := booted(r.newModel())
	m = press(m, keyTab, keyL, keyS)
	wantScreen(t, "notice", m, "not active")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions, want 0", n)
	}
}

func TestDetail_SDuringABreakAsksFirst(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.addTask(t, "next up")
	m := booted(r.newModel())
	r.clock.Set(epoch.Add(sessionEnd + 4*time.Minute))
	m = send(m, tui.TickMsg{})
	m = press(m, keyEsc) // the hand-off
	m = press(m, keyL, keyS)
	wantScreen(t, "confirmation", m, "Start anyway")
}

func TestDetail_ANoteAddedElsewhereAppearsOnTheNextTick(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "some task")
	m := press(booted(r.newModel()), keyL)
	wantScreen(t, "empty", m, "No notes yet")
	if _, err := r.tracker.AddNote(ctx, task.ID, "from another process"); err != nil {
		t.Fatal(err)
	}
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after the tick", m, "from another process")
	wantNoText(t, "after the tick", m, "No notes yet")
}

func TestDetail_ALongLogScrollsAndNeverOutgrowsTheTerminal(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "busy task")
	for i := range 30 {
		if _, err := r.tracker.AddNote(ctx, task.ID, "note number "+string(rune('A'+i%26))+string(rune('a'+i/26))); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(time.Minute)
	}
	const height = 14
	m := booted(r.newModel())
	m = send(m, tea.WindowSizeMsg{Width: 70, Height: height})
	m = press(m, keyL)
	newest, oldest := "note number Db", "note number Aa"
	wantScreen(t, "top", m, "Focused:", newest)
	wantNoText(t, "top", m, oldest)

	for range 40 {
		m = press(m, keyJ)
		if n := len(strings.Split(screen(m), "\n")); n > height {
			t.Fatalf("%d lines on a %d-line terminal:\n%s", n, height, screen(m))
		}
	}
	wantScreen(t, "scrolled to the end", m, "Focused:", oldest)
	wantNoText(t, "scrolled to the end", m, newest)

	for range 40 {
		m = press(m, keyK)
	}
	wantScreen(t, "back at the top", m, newest)
}

func TestDetail_LDoesNothingOnAnEmptyListAndUsesTheFilteredCursorTask(t *testing.T) {
	m := press(booted(newRig(t).newModel()), keyL)
	wantNoText(t, "empty list", m, "Notes")

	r := newRig(t)
	r.addTask(t, "alpha")
	r.addTask(t, "beta")
	m = booted(r.newModel())
	m = press(m, keySlash)
	m = typeText(m, "alp")
	m = press(m, keyEnter, keyL)
	wantScreen(t, "filtered", m, "alpha", "Focused:")
	wantNoText(t, "filtered", m, "beta")
}

func TestDetail_FootersFitTheTerminal(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "some task with a fairly long title to push the line past narrow widths")
	for width := 12; width <= 60; width += 4 {
		m := booted(r.newModel())
		m = send(m, tea.WindowSizeMsg{Width: width, Height: 12})
		for name, keys := range map[string][]tea.KeyPressMsg{"detail": {keyL}, "note": {keyL, keyN}} {
			view := press(m, keys...)
			for i, line := range strings.Split(view.View().Content, "\n") {
				if w := ansi.StringWidth(line); w > width {
					t.Errorf("%s at width %d: line %d is %d cells: %q", name, width, i, w, line)
				}
			}
		}
	}
}
