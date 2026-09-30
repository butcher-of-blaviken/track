package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

// modelWithSetting is a model whose configured focus duration is d.
func (r *rig) modelWithSetting(d time.Duration) tea.Model {
	return tui.New(r.tracker, tui.WithTick(noTick), tui.WithFocusDuration(d))
}

func wantNoMismatch(t *testing.T, what string, m tea.Model) {
	t.Helper()
	if strings.Contains(screen(m), "current setting") {
		t.Errorf("%s: the duration notice is shown:\n%s", what, screen(m))
	}
}

func TestDurationNotice_ShowsBothValuesWhenTheyDiffer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setting time.Duration
		want    string
	}{
		{"setting is longer", 45 * time.Minute, "This session is 30m; the current setting is 45m."},
		{"setting is shorter", 20 * time.Minute, "This session is 30m; the current setting is 20m."},
		{"hours and minutes", 90 * time.Minute, "This session is 30m; the current setting is 1h30m."},
		{"whole hours", time.Hour, "This session is 30m; the current setting is 1h."},
		{"seconds", 90 * time.Second, "This session is 30m; the current setting is 1m30s."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.startSession(t)
			wantScreen(t, tc.name, booted(r.modelWithSetting(tc.setting)), tc.want)
		})
	}
}

func TestDurationNotice_NotShownWhenTheyMatch(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	wantNoMismatch(t, "matching", booted(r.modelWithSetting(30*time.Minute)))
}

func TestDurationNotice_NotShownForASessionStartedFromTheTUI(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "task")
	m := booted(r.modelWithSetting(45 * time.Minute))
	m = press(m, keyS)
	wantFocusLine(t, "panel", m, "45:00", "task")
	wantNoMismatch(t, "own session", m)
}

func TestDurationNotice_OnlyWhileTheSessionRuns(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		r := newRig(t)
		r.addTask(t, "task")
		wantNoMismatch(t, "no session", booted(r.modelWithSetting(45*time.Minute)))
	})

	t.Run("after stopping", func(t *testing.T) {
		r := newRig(t)
		r.startSession(t)
		m := booted(r.modelWithSetting(45 * time.Minute))
		wantScreen(t, "running", m, "current setting")
		m = press(m, keyX)
		wantNoMismatch(t, "stopped", m)
	})

	t.Run("in the Break", func(t *testing.T) {
		r := newRig(t)
		r.startSession(t)
		m := booted(r.modelWithSetting(45 * time.Minute))
		r.clock.Set(epoch.Add(sessionEnd + time.Minute))
		m = send(m, tui.TickMsg{})
		wantScreen(t, "on the Break", m, "Break")
		wantNoMismatch(t, "on the Break", m)
	})
}

func TestDurationNotice_DoesNotRewriteTheRecordedDuration(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.modelWithSetting(45 * time.Minute))
	m = send(m, tui.TickMsg{})
	_ = m
	if sessions := r.storedSessions(t); len(sessions) != 1 || sessions[0].PlannedDuration != 30*time.Minute {
		t.Errorf("stored sessions = %+v, want the one 30m session untouched", sessions)
	}
}
