package core_test

import (
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

var sessionStart = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)

func TestFocusSession_OutcomeAndElapsed(t *testing.T) {
	stoppedAt := sessionStart.Add(12 * time.Minute)
	running := core.FocusSession{StartedAt: sessionStart, PlannedDuration: 30 * time.Minute}
	stopped := core.FocusSession{StartedAt: sessionStart, PlannedDuration: 30 * time.Minute, StoppedAt: &stoppedAt}

	tests := []struct {
		name        string
		session     core.FocusSession
		now         time.Time
		wantOutcome core.Outcome
		wantElapsed time.Duration
	}{
		{"just started", running, sessionStart, core.OutcomeRunning, 0},
		{"midway", running, sessionStart.Add(10 * time.Minute), core.OutcomeRunning, 10 * time.Minute},
		{"one second before the planned end", running, sessionStart.Add(30*time.Minute - time.Second), core.OutcomeRunning, 30*time.Minute - time.Second},
		{"exactly at the planned end is completed", running, sessionStart.Add(30 * time.Minute), core.OutcomeCompleted, 30 * time.Minute},
		{"long after the planned end stays capped", running, sessionStart.Add(3 * time.Hour), core.OutcomeCompleted, 30 * time.Minute},
		{"a clock before the start reads as zero", running, sessionStart.Add(-time.Hour), core.OutcomeRunning, 0},
		{"stopped early keeps the time it ran", stopped, stoppedAt, core.OutcomeStoppedEarly, 12 * time.Minute},
		{"stopped early is stable as time passes", stopped, sessionStart.Add(5 * time.Hour), core.OutcomeStoppedEarly, 12 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.Outcome(tt.now); got != tt.wantOutcome {
				t.Errorf("Outcome = %v, want %v", got, tt.wantOutcome)
			}
			if got := tt.session.Elapsed(tt.now); got != tt.wantElapsed {
				t.Errorf("Elapsed = %v, want %v", got, tt.wantElapsed)
			}
		})
	}
}

func TestFocusSession_BreakRemaining(t *testing.T) {
	stoppedAt := sessionStart.Add(12 * time.Minute)
	completing := core.FocusSession{StartedAt: sessionStart, PlannedDuration: 30 * time.Minute, BreakDuration: 10 * time.Minute}
	stopped := completing
	stopped.StoppedAt = &stoppedAt
	end := sessionStart.Add(30 * time.Minute)

	tests := []struct {
		name    string
		session core.FocusSession
		now     time.Time
		want    time.Duration
	}{
		{"none while the session is running", completing, sessionStart.Add(29 * time.Minute), 0},
		{"the full Break right at completion", completing, end, 10 * time.Minute},
		{"counts down", completing, end.Add(4 * time.Minute), 6 * time.Minute},
		{"zero exactly when the Break ends", completing, end.Add(10 * time.Minute), 0},
		{"zero long after the Break", completing, end.Add(5 * time.Hour), 0},
		{"none after an early stop", stopped, stoppedAt.Add(time.Minute), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.BreakRemaining(tt.now); got != tt.want {
				t.Errorf("BreakRemaining = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFocusSession_EndedAt(t *testing.T) {
	stoppedAt := sessionStart.Add(12 * time.Minute)
	planned := core.FocusSession{StartedAt: sessionStart, PlannedDuration: 30 * time.Minute}
	stopped := planned
	stopped.StoppedAt = &stoppedAt

	tests := []struct {
		name    string
		session core.FocusSession
		now     time.Time
		want    time.Time
		wantOK  bool
	}{
		{"not ended while running", planned, sessionStart.Add(10 * time.Minute), time.Time{}, false},
		{"a completed session ended at its planned end", planned, sessionStart.Add(5 * time.Hour), sessionStart.Add(30 * time.Minute), true},
		{"exactly at the planned end", planned, sessionStart.Add(30 * time.Minute), sessionStart.Add(30 * time.Minute), true},
		{"a stopped session ended when it was stopped", stopped, sessionStart.Add(5 * time.Hour), stoppedAt, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.session.EndedAt(tt.now)
			if ok != tt.wantOK || !got.Equal(tt.want) {
				t.Errorf("EndedAt = %v, %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
