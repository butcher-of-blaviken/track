package tui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keyS = tea.KeyPressMsg{Code: 's', Text: "s"}
	keyX = tea.KeyPressMsg{Code: 'x', Text: "x"}
)

func (r *rig) storedSessions(t *testing.T) []core.FocusSession {
	t.Helper()
	got, err := r.store.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// wantFocusLine checks the panel's phase line names the Task being focused on.
func wantFocusLine(t *testing.T, what string, m tea.Model, countdown, title string) {
	t.Helper()
	re := regexp.MustCompile(`Focus\s+` + regexp.QuoteMeta(countdown) + `\s+` + regexp.QuoteMeta(title))
	if !re.MatchString(screen(m)) {
		t.Errorf("%s: no line matches %v:\n%s", what, re, screen(m))
	}
}

func TestStart_SAndEnterStartASessionOnTheSelectedTask(t *testing.T) {
	for name, key := range map[string]tea.KeyPressMsg{"s": keyS, "enter": keyEnter} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.addTask(t, "the other one")
			r.addTask(t, "write the PRD ##docs")
			m := booted(r.newModel())
			// Newest first, so the newest is selected; move to the older one.
			m = press(m, keyJ)
			wantSelected(t, "before starting", m, "the other one")

			m = press(m, key)
			wantFocusLine(t, "panel", m, "30:00", "the other one")

			sessions := r.storedSessions(t)
			if len(sessions) != 1 || sessions[0].PlannedDuration != 30*time.Minute {
				t.Fatalf("stored sessions = %+v, want one 30m session", sessions)
			}
			task, err := r.tracker.Task(ctx, sessions[0].TaskID)
			if err != nil || task.Title != "the other one" {
				t.Errorf("session is on task %+v, %v; want the selected \"the other one\"", task, err)
			}
		})
	}
}

func TestStart_UsesTheConfiguredFocusDuration(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "deep work")
	m := booted(tui.New(r.tracker, tui.WithTick(noTick), tui.WithFocusDuration(45*time.Minute)))
	m = press(m, keyS)
	wantFocusLine(t, "panel", m, "45:00", "deep work")
	if got := r.storedSessions(t); len(got) != 1 || got[0].PlannedDuration != 45*time.Minute {
		t.Errorf("stored sessions = %+v, want one 45m session", got)
	}
}

func TestStart_WithNoTasksDoesNothing(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	m = press(m, keyS, keyEnter)
	wantScreen(t, "still idle", m, "Idle")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions stored with no tasks, want 0", n)
	}
}

func TestStop_XEndsTheSessionEarlyAndTheHandoffIsDue(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "some work")
	m := booted(r.newModel())
	m = press(m, keyS)
	r.clock.Advance(10 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "running", m, "Focus", "20:00")

	m = press(m, keyX)
	wantScreen(t, "after stopping", m, "Idle", "Hand-off due")
	if strings.Contains(screen(m), "Break") {
		t.Errorf("an early stop must not start a Break:\n%s", screen(m))
	}
	sessions := r.storedSessions(t)
	if len(sessions) != 1 || sessions[0].Outcome(r.clock.Now()) != core.OutcomeStoppedEarly || sessions[0].Elapsed(r.clock.Now()) != 10*time.Minute {
		t.Errorf("stored sessions = %+v, want one stopped early after 10m", sessions)
	}
}

func TestStart_BlockedStartsShowANoticeAndCreateNothing(t *testing.T) {
	t.Run("while a session is running", func(t *testing.T) {
		r := newRig(t)
		r.addTask(t, "first")
		m := booted(r.newModel())
		m = press(m, keyS, keyJ, keyS)
		wantScreen(t, "notice", m, "already running", "(x)")
		if n := len(r.storedSessions(t)); n != 1 {
			t.Errorf("%d sessions stored, want only the first", n)
		}
	})

	t.Run("during a Break", func(t *testing.T) {
		r := newRig(t)
		r.addTask(t, "first")
		m := booted(r.newModel())
		m = press(m, keyS)
		r.clock.Advance(34 * time.Minute) // 4m into the 10m Break
		m = send(m, tui.TickMsg{})
		m = press(m, keyEsc) // skip the hand-off prompt that opened, to reach the list
		m = press(m, keyS)
		wantScreen(t, "notice", m, "break is in progress", "06:00 left")
		if n := len(r.storedSessions(t)); n != 1 {
			t.Errorf("%d sessions stored, want only the first", n)
		}
	})

	t.Run("stopping with nothing running", func(t *testing.T) {
		m := booted(newRig(t).newModel())
		m = press(m, keyX)
		wantScreen(t, "notice", m, "Nothing to stop")
	})
}

func TestNotice_ClearsOnTheNextKey(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = press(m, keyX)
	wantScreen(t, "notice shown", m, "Nothing to stop")
	m = press(m, keyJ)
	if strings.Contains(screen(m), "Nothing to stop") {
		t.Errorf("the notice is still shown after another key:\n%s", screen(m))
	}
}

func TestRefresh_ShowsASessionStartedByAnotherProcessWithItsTask(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "started elsewhere")
	m := booted(r.newModel())
	wantScreen(t, "before", m, "Idle")

	if _, err := r.tracker.StartSession(ctx, task.ID, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(5 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantFocusLine(t, "after the tick", m, "25:00", "started elsewhere")
}

func TestPanel_ShowsTheUnfiledNoteCountOnlyWhenThereAreSome(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	if strings.Contains(screen(m), "Unfiled") {
		t.Errorf("unfiled count shown with none:\n%s", screen(m))
	}

	for _, text := range []string{"check the retry logic", "ask about the schema"} {
		if _, err := r.tracker.AddUnfiledNote(ctx, text); err != nil {
			t.Fatal(err)
		}
	}
	m = send(m, tui.TickMsg{})
	wantScreen(t, "two unfiled", m, "Unfiled notes: 2")
}
