package tui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

// history gives a task a completed session, then one stopped after 12 minutes,
// then one still running 5 minutes in. It returns them oldest first.
func (r *rig) history(t *testing.T, task core.TaskID) []core.FocusSession {
	t.Helper()
	must := func(s core.FocusSession, err error) core.FocusSession {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	start := func() core.FocusSession {
		return must(r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}))
	}
	a := start()
	r.clock.Advance(30 * time.Minute)
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(11 * time.Minute)
	b := start()
	r.clock.Advance(12 * time.Minute)
	must(r.tracker.StopSession(ctx))
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	c := start()
	r.clock.Advance(5 * time.Minute)
	return []core.FocusSession{a, b, c}
}

func detailOf(r *rig, w, h int) tea.Model {
	return press(send(booted(r.newModel()), tea.WindowSizeMsg{Width: w, Height: h}), keyL)
}

func TestDetailHistory_EachSessionShowsItsOutcomeNewestFirst(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD")
	s := r.history(t, task.ID)
	m := detailOf(r, 90, 30)

	wantScreen(t, "sessions", m,
		"Sessions",
		stamp(s[2].StartedAt)+"  5m of 30m  running",
		stamp(s[1].StartedAt)+"  12m of 30m  stopped early",
		stamp(s[0].StartedAt)+"  30m  completed",
	)
	if indexOf(t, m, "running") >= indexOf(t, m, "stopped early") || indexOf(t, m, "stopped early") >= indexOf(t, m, "completed") {
		t.Errorf("want newest first:\n%s", screen(m))
	}
	wantScreen(t, "focused total", m, "Focused: 47m over 3 sessions")
}

func TestDetailHistory_OutcomesAreColouredAndTimesMuted(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD")
	s := r.history(t, task.ID)
	m := detailOf(r, 90, 30)
	if got := styleOf(t, m, "running"); got.fg != ansiMagenta {
		t.Errorf("running = %+v, want magenta", got)
	}
	if got := styleOf(t, m, "stopped early"); got.fg != ansiYellow {
		t.Errorf("stopped early = %+v, want yellow", got)
	}
	if got := styleOf(t, m, "completed"); got.fg != ansiGreen {
		t.Errorf("completed = %+v, want green", got)
	}
	if got := styleOf(t, m, stamp(s[0].StartedAt)); !got.faint {
		t.Errorf("session time = %+v, want faint", got)
	}
}

func TestDetailHistory_ARunningSessionCountsUpOnEachTick(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD")
	r.history(t, task.ID)
	m := detailOf(r, 90, 30)
	wantScreen(t, "before", m, "5m of 30m  running")
	r.clock.Advance(4 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after", m, "9m of 30m  running")
}

func TestDetailHistory_OnlyTheLatestFiveShowWithAnEarlierCount(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "busy")
	for range 7 {
		if _, err := r.tracker.StartSession(ctx, task.ID, 30*time.Minute, core.StartOptions{}); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(30 * time.Minute)
		if err := r.tracker.SkipHandoff(ctx); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(21 * time.Minute) // past even a long Break
	}
	// Launching Idle after a session offers to resume it; decline, then open the detail.
	m := send(booted(r.newModel()), tea.WindowSizeMsg{Width: 90, Height: 40})
	m = press(m, keyN, keyL)
	if got := strings.Count(screen(m), "completed"); got != 5 {
		t.Errorf("%d sessions shown, want 5:\n%s", got, screen(m))
	}
	wantScreen(t, "earlier", m, "+2 earlier")
	if got := styleOf(t, m, "+2 earlier"); !got.faint {
		t.Errorf("earlier count = %+v, want faint", got)
	}
}

func TestDetailHistory_NoSessionsMeansNoSessionsSection(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "fresh")
	m := detailOf(r, 90, 30)
	wantNoText(t, "no sessions", m, "Sessions")
	wantScreen(t, "notes remain", m, "Notes", "No notes yet")
}

func TestDetailHistory_HandoffNotesAreMarkedAndOtherNotesAreNot(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD")
	if _, err := r.tracker.StartSession(ctx, task.ID, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(31 * time.Minute)
	hand, err := r.tracker.AddHandoffNote(ctx, "left off at section 2")
	if err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	plain, err := r.tracker.AddNote(ctx, task.ID, "an ad hoc thought")
	if err != nil {
		t.Fatal(err)
	}
	m := detailOf(r, 90, 30)
	wantScreen(t, "hand-off", m, stamp(hand.CreatedAt)+"  hand-off  left off at section 2")
	wantScreen(t, "ad hoc", m, stamp(plain.CreatedAt)+"  an ad hoc thought")
	if got := styleOf(t, m, "hand-off  left"[:8]); got.fg != ansiYellow {
		t.Errorf("hand-off marker = %+v, want yellow", got)
	}
}

func TestDetailHistory_ShowsTheStateAndWhenTheTaskWasCreated(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "finished work")
	r.setState(t, task, core.StateDone)
	m := press(send(booted(r.newModel()), tea.WindowSizeMsg{Width: 90, Height: 30}), keyTab, keyL)
	wantScreen(t, "state line", m, "State: Done", "Created: "+stamp(task.CreatedAt))
}

func TestDetailHistory_SessionsAndNotesScrollTogetherAndNeverOutgrowTheTerminal(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "busy task")
	r.history(t, task.ID)
	for i := range 20 {
		if _, err := r.tracker.AddNote(ctx, task.ID, fmt.Sprintf("note %02d", i)); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(time.Minute)
	}
	const height = 14
	m := detailOf(r, 70, height)
	wantScreen(t, "top", m, "Focused:", "Sessions", "running")
	wantNoText(t, "top", m, "note 00")

	for range 60 {
		m = press(m, keyJ)
		if n := len(strings.Split(screen(m), "\n")); n > height {
			t.Fatalf("%d lines on a %d-line terminal:\n%s", n, height, screen(m))
		}
	}
	wantScreen(t, "end", m, "Focused:", "note 00")
	wantNoText(t, "end", m, "running")
	for range 60 {
		m = press(m, keyK)
	}
	wantScreen(t, "top again", m, "Sessions", "running")
}
