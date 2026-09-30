package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

// endedLongAgo is a rig whose only session completed and whose Break is over,
// with the hand-off left as the caller sets it. It returns the session's Task.
func endedLongAgo(t *testing.T, r *rig) core.Task {
	t.Helper()
	s := r.startSession(t)
	r.clock.Set(epoch.Add(sessionEnd + time.Hour))
	task, err := r.tracker.Task(ctx, s.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func wantNoResume(t *testing.T, what string, m tea.Model) {
	t.Helper()
	if strings.Contains(screen(m), "Resume") {
		t.Errorf("%s: the resume prompt is open:\n%s", what, screen(m))
	}
}

func TestResume_OffersTheLastTaskOnLaunchAndYStartsIt(t *testing.T) {
	r := newRig(t)
	endedLongAgo(t, r)
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	m := booted(r.newModel())
	wantScreen(t, "on launch", m, `Resume "task"?`)

	m = press(m, keyY)
	wantNoResume(t, "after y", m)
	wantFocusLine(t, "panel", m, "30:00", "task")
	if n := len(r.storedSessions(t)); n != 2 {
		t.Errorf("%d sessions stored, want 2", n)
	}
}

func TestResume_NAndEscDismissItForTheRun(t *testing.T) {
	for name, key := range map[string]tea.KeyPressMsg{"n": keyN, "esc": keyEsc} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			endedLongAgo(t, r)
			if err := r.tracker.SkipHandoff(ctx); err != nil {
				t.Fatal(err)
			}
			m := booted(r.newModel())
			m = press(m, key)
			wantNoResume(t, "after dismissing", m)
			for range 3 {
				m = send(m, tui.TickMsg{})
			}
			wantNoResume(t, "on later ticks", m)
			if n := len(r.storedSessions(t)); n != 1 {
				t.Errorf("%d sessions stored, want only the first", n)
			}
		})
	}
}

func TestResume_ShowsTheLastNoteOfTheTask(t *testing.T) {
	r := newRig(t)
	task := endedLongAgo(t, r)
	r.clock.Set(epoch.Add(sessionEnd + 10*time.Minute))
	for _, text := range []string{"first thought", "left off at the parser"} {
		if _, err := r.tracker.AddNote(ctx, task.ID, text); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(time.Minute)
	}
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	r.clock.Set(epoch.Add(sessionEnd + time.Hour))
	m := booted(r.newModel())
	wantScreen(t, "prompt", m, `Resume "task"?`, "left off at the parser", "ago")
	if strings.Contains(screen(m), "first thought") {
		t.Errorf("an older note is shown:\n%s", screen(m))
	}
}

func TestResume_WithoutNotesShowsNoNoteLine(t *testing.T) {
	r := newRig(t)
	endedLongAgo(t, r)
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	m := booted(r.newModel())
	wantScreen(t, "prompt", m, `Resume "task"?`)
	if strings.Contains(screen(m), "Last note") {
		t.Errorf("a note line is shown with no notes:\n%s", screen(m))
	}
}

func TestResume_WaitsForTheHandoffThenShowsTheSavedNote(t *testing.T) {
	r := newRig(t)
	endedLongAgo(t, r)
	m := booted(r.newModel())
	wantScreen(t, "hand-off first", m, "Hand-off for: task")
	wantNoResume(t, "while the hand-off is open", m)

	m = typeText(m, "picked up here")
	m = press(m, keyEnter)
	wantScreen(t, "after saving", m, `Resume "task"?`, "picked up here")
}

func TestResume_AfterSkippingTheHandoffShowsNoNewNote(t *testing.T) {
	r := newRig(t)
	endedLongAgo(t, r)
	m := booted(r.newModel())
	m = press(m, keyEsc)
	wantScreen(t, "after skipping", m, `Resume "task"?`)
}

func TestResume_NotOfferedWhenThereIsNothingToResume(t *testing.T) {
	t.Run("no sessions", func(t *testing.T) {
		r := newRig(t)
		r.addTask(t, "fresh")
		wantNoResume(t, "empty", booted(r.newModel()))
	})

	t.Run("a session is running", func(t *testing.T) {
		r := newRig(t)
		r.startSession(t)
		wantNoResume(t, "running", booted(r.newModel()))
	})

	t.Run("a Break is in progress", func(t *testing.T) {
		r := newRig(t)
		r.startSession(t)
		r.clock.Set(epoch.Add(sessionEnd + 4*time.Minute))
		if err := r.tracker.SkipHandoff(ctx); err != nil {
			t.Fatal(err)
		}
		m := booted(r.newModel())
		wantNoResume(t, "on a Break", m)
		r.clock.Set(epoch.Add(sessionEnd + 11*time.Minute))
		m = send(m, tui.TickMsg{})
		wantNoResume(t, "once the Break ends", m)
	})

	t.Run("the Task is no longer active", func(t *testing.T) {
		r := newRig(t)
		task := endedLongAgo(t, r)
		if err := r.tracker.SkipHandoff(ctx); err != nil {
			t.Fatal(err)
		}
		if err := r.store.Update(ctx, func(tx core.Tx) error {
			task.State = core.StateDone
			return tx.SaveTask(task)
		}); err != nil {
			t.Fatal(err)
		}
		wantNoResume(t, "done task", booted(r.newModel()))
	})
}

func TestResume_KeysOtherThanTheAnswersAreIgnored(t *testing.T) {
	r := newRig(t)
	endedLongAgo(t, r)
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	m := booted(r.newModel())
	m = press(m, keyJ, keyA, keyX, keyS, keyEnter)
	wantScreen(t, "still asking", m, "Resume")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions stored, want only the first", n)
	}
	if _, cmd := m.Update(keyCtrlC); cmd == nil {
		t.Error("ctrl+c returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
}

func TestResume_FooterShowsTheKeys(t *testing.T) {
	r := newRig(t)
	endedLongAgo(t, r)
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	wantScreen(t, "footer", booted(r.newModel()), "y resume", "n cancel")
}
