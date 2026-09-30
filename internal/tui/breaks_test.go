package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keyY = tea.KeyPressMsg{Code: 'y', Text: "y"}
	keyN = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

// inBreak is a model 4m into the 10m Break of a completed session, with the
// hand-off skipped so the list has the keyboard.
func inBreak(t *testing.T, r *rig) tea.Model {
	t.Helper()
	r.startSession(t)
	m := booted(r.newModel())
	r.clock.Set(epoch.Add(sessionEnd + 4*time.Minute))
	m = send(m, tui.TickMsg{})
	return press(m, keyEsc)
}

func wantNoConfirm(t *testing.T, what string, m tea.Model) {
	t.Helper()
	if strings.Contains(screen(m), "Start anyway") {
		t.Errorf("%s: the Break confirmation is open:\n%s", what, screen(m))
	}
}

func TestBreakOverride_StartAsksFirstAndStartsNothing(t *testing.T) {
	for name, key := range map[string]tea.KeyPressMsg{"s": keyS, "enter": keyEnter} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			m := inBreak(t, r)
			m = press(m, key)
			wantScreen(t, "confirmation", m, "Start anyway", "06:00 left", "y", "n")
			if n := len(r.storedSessions(t)); n != 1 {
				t.Errorf("%d sessions stored, want only the first", n)
			}
		})
	}
}

func TestBreakOverride_DeclineKeepsTheBreak(t *testing.T) {
	for name, key := range map[string]tea.KeyPressMsg{"n": keyN, "esc": keyEsc} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			m := inBreak(t, r)
			m = press(m, keyS, key)
			wantNoConfirm(t, "after declining", m)
			wantScreen(t, "still on the Break", m, "Break")
			if n := len(r.storedSessions(t)); n != 1 {
				t.Errorf("%d sessions stored, want only the first", n)
			}
		})
	}
}

func TestBreakOverride_ConfirmStartsAndRecordsTheSkippedBreak(t *testing.T) {
	r := newRig(t)
	m := inBreak(t, r)
	m = press(m, keyS, keyY)

	wantNoConfirm(t, "after confirming", m)
	wantFocusLine(t, "panel", m, "30:00", "task")
	sessions := r.storedSessions(t)
	if len(sessions) != 2 || sessions[1].SkippedBreak != 6*time.Minute {
		t.Fatalf("stored sessions = %+v, want a second one that skipped 6m of Break", sessions)
	}
}

func TestBreakOverride_OtherKeysAreIgnored(t *testing.T) {
	r := newRig(t)
	m := inBreak(t, r)
	m = press(m, keyS, keyJ, keyA, keyX, keyS, keyEnter)
	wantScreen(t, "still asking", m, "Start anyway")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions stored, want only the first", n)
	}
}

func TestBreakOverride_CtrlCQuits(t *testing.T) {
	r := newRig(t)
	m := press(inBreak(t, r), keyS)
	if _, cmd := m.Update(keyCtrlC); cmd == nil {
		t.Error("ctrl+c returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
}

func TestBreakOverride_ConfirmingAfterTheBreakEndedStartsNormally(t *testing.T) {
	r := newRig(t)
	m := inBreak(t, r)
	m = press(m, keyS)
	r.clock.Set(epoch.Add(sessionEnd + 11*time.Minute))
	m = press(m, keyY)

	sessions := r.storedSessions(t)
	if len(sessions) != 2 || sessions[1].SkippedBreak != 0 {
		t.Fatalf("stored sessions = %+v, want a second one that skipped nothing", sessions)
	}
	wantNoConfirm(t, "after confirming", m)
}

func TestBreakOverride_HandoffPromptKeepsTheKeyboardUntilResolved(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)
	m = press(m, keyS)
	wantNoConfirm(t, "while the hand-off prompt is open", m)
	wantScreen(t, "s is typed", m, "Hand-off for", "s")

	m = press(m, keyEsc)
	m = press(m, keyS)
	wantScreen(t, "after skipping", m, "Start anyway")
}

func TestBreakOverride_FooterShowsTheKeys(t *testing.T) {
	r := newRig(t)
	m := press(inBreak(t, r), keyS)
	wantScreen(t, "footer", m, "y start", "n cancel")
}
