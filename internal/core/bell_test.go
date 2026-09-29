package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
)

func newRigWithBell(t *testing.T, bell core.BellPolicy) *rig {
	t.Helper()
	store := memory.New()
	fake := clock.NewFake(sessionStart)
	tracker, err := core.NewTracker(store, fake, testPolicy, core.WithBellPolicy(bell))
	if err != nil {
		t.Fatal(err)
	}
	return &rig{tracker: tracker, store: store, clock: fake}
}

// startAndRunTo starts a 30m session (10m Break) and moves the clock to offset
// after its planned end.
func (r *rig) startAndRunTo(t *testing.T, task core.TaskID, afterEnd time.Duration) core.FocusSession {
	t.Helper()
	s, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.clock.Set(s.PlannedEnd().Add(afterEnd))
	return s
}

func (r *rig) bell(t *testing.T) *core.Bell { t.Helper(); return r.snapshot(t).Bell }

func wantBell(t *testing.T, what string, got *core.Bell, session core.SessionID, kind core.BellKind, scheduled int) {
	t.Helper()
	if got == nil || got.Session != session || got.Kind != kind || got.Scheduled != scheduled {
		t.Errorf("%s: bell = %+v, want session %d, kind %v, scheduled %d", what, got, session, kind, scheduled)
	}
}

func wantNoBell(t *testing.T, what string, got *core.Bell) {
	t.Helper()
	if got != nil {
		t.Errorf("%s: bell = %+v, want none", what, got)
	}
}

func TestBell_NothingRingsWhileFocusingBeforeAnyEndOrAfterAnEarlyStop(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	wantNoBell(t, "empty store", r.bell(t))

	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(29 * time.Minute)
	wantNoBell(t, "before the planned end", r.bell(t))

	r.clock.Advance(-9 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	wantNoBell(t, "just after stopping early", r.bell(t))
	r.clock.Advance(time.Hour)
	wantNoBell(t, "long after stopping early", r.bell(t))
}

func TestBell_SessionEndRepeatsOnTheIntervalForTheFirstRingPlusRepeats(t *testing.T) {
	r := newRig(t) // defaults: every 30s, 10 repeats, so 11 rings
	task := r.addTask(t, core.StateActive)
	s := r.startAndRunTo(t, task, 0)

	tests := []struct {
		afterEnd time.Duration
		want     int // 0 = no bell
	}{
		{0, 1},
		{29 * time.Second, 1},
		{30 * time.Second, 2},
		{5 * 30 * time.Second, 6},
		{10 * 30 * time.Second, 11},           // the last ring
		{11*30*time.Second - time.Second, 11}, // still inside the last interval
		{11 * 30 * time.Second, 0},            // window over
		{time.Hour, 0},
	}
	for _, tt := range tests {
		r.clock.Set(s.PlannedEnd().Add(tt.afterEnd))
		if tt.want == 0 {
			wantNoBell(t, tt.afterEnd.String(), r.bell(t))
		} else {
			wantBell(t, tt.afterEnd.String(), r.bell(t), s.ID, core.BellSessionEnd, tt.want)
		}
	}
}

func TestBell_BreakEndRingsWhenTheBreakIsOver(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	s := r.startAndRunTo(t, task, 0)
	breakEnd := s.PlannedEnd().Add(s.BreakDuration)

	r.clock.Set(breakEnd.Add(-time.Second))
	wantNoBell(t, "just before the Break ends (the session-end window is long over)", r.bell(t))
	r.clock.Set(breakEnd)
	wantBell(t, "at the Break's end", r.bell(t), s.ID, core.BellBreakEnd, 1)
	r.clock.Set(breakEnd.Add(30 * time.Second))
	wantBell(t, "one interval later", r.bell(t), s.ID, core.BellBreakEnd, 2)
	r.clock.Set(breakEnd.Add(11 * 30 * time.Second))
	wantNoBell(t, "after the Break-end window", r.bell(t))
}

func TestBell_BreakEndTakesOverFromAWindowThatIsStillOpen(t *testing.T) {
	r := newRigWithBell(t, core.BellPolicy{Interval: time.Minute, Repeats: 20}) // window 21m > the 10m Break
	task := r.addTask(t, core.StateActive)
	s := r.startAndRunTo(t, task, 9*time.Minute)
	wantBell(t, "still inside the Break", r.bell(t), s.ID, core.BellSessionEnd, 10)

	r.clock.Set(s.PlannedEnd().Add(s.BreakDuration))
	wantBell(t, "at the Break's end", r.bell(t), s.ID, core.BellBreakEnd, 1)
}

func TestBell_AfterTimeAwayOnlyARecentEventStillRings(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	s := r.startAndRunTo(t, task, 40*time.Second) // reopened 40s after the end
	wantBell(t, "40s after the end", r.bell(t), s.ID, core.BellSessionEnd, 2)

	r.clock.Advance(5 * time.Hour)
	wantNoBell(t, "hours later the event is stale and silent", r.bell(t))
}

func TestBell_StartingOverTheBreakSilencesItsBellAndTheNewSessionOwnsTheNext(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	first := r.startAndRunTo(t, task, 4*time.Minute) // 4m into the Break
	second, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{OverrideBreak: true})
	if err != nil {
		t.Fatal(err)
	}

	r.clock.Set(first.PlannedEnd().Add(first.BreakDuration)) // when the skipped Break would have ended
	wantNoBell(t, "the skipped Break's end", r.bell(t))

	r.clock.Set(second.PlannedEnd())
	wantBell(t, "the new session's end", r.bell(t), second.ID, core.BellSessionEnd, 1)
}

func TestBell_ZeroRepeatsRingsOnce(t *testing.T) {
	r := newRigWithBell(t, core.BellPolicy{Interval: 30 * time.Second, Repeats: 0})
	task := r.addTask(t, core.StateActive)
	s := r.startAndRunTo(t, task, 0)
	wantBell(t, "at the end", r.bell(t), s.ID, core.BellSessionEnd, 1)
	r.clock.Set(s.PlannedEnd().Add(30 * time.Second))
	wantNoBell(t, "one interval later", r.bell(t))
}

func TestWithBellPolicy_RejectsAnInvalidPolicy(t *testing.T) {
	for _, bell := range []core.BellPolicy{
		{Interval: 0, Repeats: 10},
		{Interval: -time.Second, Repeats: 10},
		{Interval: 30 * time.Second, Repeats: -1},
	} {
		_, err := core.NewTracker(memory.New(), clock.NewFake(sessionStart), testPolicy, core.WithBellPolicy(bell))
		if !errors.Is(err, core.ErrInvalidBellPolicy) {
			t.Errorf("WithBellPolicy(%+v) error = %v, want ErrInvalidBellPolicy", bell, err)
		}
	}
}
