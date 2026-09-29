package core_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func (r *rig) snapshot(t *testing.T) core.Snapshot {
	t.Helper()
	snap, err := r.tracker.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return snap
}

func TestSnapshot_EmptyStoreIsIdle(t *testing.T) {
	r := newRig(t)
	snap := r.snapshot(t)
	if snap.Phase != core.PhaseIdle || snap.Session != nil || snap.HandoffPending || snap.Remaining != 0 {
		t.Errorf("snapshot = %+v, want Idle with no session and no hand-off", snap)
	}
	if !snap.At.Equal(r.clock.Now()) {
		t.Errorf("At = %v, want %v", snap.At, r.clock.Now())
	}
}

func TestSnapshot_RunningSessionIsFocusWithTimeRemaining(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	started, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(15 * time.Minute)
	snap := r.snapshot(t)
	if snap.Phase != core.PhaseFocus || snap.Remaining != 15*time.Minute || snap.HandoffPending {
		t.Errorf("snapshot = %+v, want Focus with 15m remaining and no hand-off pending", snap)
	}
	if snap.Session == nil || snap.Session.ID != started.ID {
		t.Errorf("Session = %+v, want the running session %d", snap.Session, started.ID)
	}
}

func TestSnapshot_CompletedSessionStartsABreakWithAPendingHandoff(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(30 * time.Minute) // exactly the planned end
	if snap := r.snapshot(t); snap.Phase != core.PhaseBreak || snap.Remaining != 10*time.Minute || !snap.HandoffPending {
		t.Errorf("at completion: %+v, want Break with 10m remaining and hand-off pending", snap)
	}

	r.clock.Advance(4 * time.Minute)
	if snap := r.snapshot(t); snap.Phase != core.PhaseBreak || snap.Remaining != 6*time.Minute || !snap.HandoffPending {
		t.Errorf("mid-Break: %+v, want Break with 6m remaining and hand-off pending", snap)
	}

	r.clock.Advance(6 * time.Minute) // the Break ends
	if snap := r.snapshot(t); snap.Phase != core.PhaseIdle || snap.Remaining != 0 || !snap.HandoffPending {
		t.Errorf("after the Break: %+v, want Idle with the hand-off still pending", snap)
	}
}

func TestSnapshot_StoppedEarlyIsIdleWithAPendingHandoff(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(10 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}

	snap := r.snapshot(t)
	if snap.Phase != core.PhaseIdle || snap.Remaining != 0 || !snap.HandoffPending {
		t.Errorf("snapshot = %+v, want Idle (no Break after an early stop) with the hand-off pending", snap)
	}
	if snap.Session == nil || snap.Session.Outcome(snap.At) != core.OutcomeStoppedEarly {
		t.Errorf("Session = %+v, want the stopped session", snap.Session)
	}
}

func TestSnapshot_ResolvingTheHandoffClearsTheFlagButNotTheBreak(t *testing.T) {
	for name, resolve := range map[string]func(*testing.T, *rig){
		"a note": func(t *testing.T, r *rig) {
			if _, err := r.tracker.AddHandoffNote(ctx, "left off at the parser"); err != nil {
				t.Fatal(err)
			}
		},
		"skipping": func(t *testing.T, r *rig) {
			if err := r.tracker.SkipHandoff(ctx); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			task := r.addTask(t, core.StateActive)
			if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
				t.Fatal(err)
			}
			r.clock.Advance(34 * time.Minute) // 4m into the Break

			resolve(t, r)
			if snap := r.snapshot(t); snap.HandoffPending || snap.Phase != core.PhaseBreak || snap.Remaining != 6*time.Minute {
				t.Errorf("snapshot = %+v, want Break with 6m left and no hand-off pending", snap)
			}
		})
	}
}

func TestSnapshot_ReconcilesAfterTimeAway(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(5 * time.Hour) // the app was closed the whole time
	snap := r.snapshot(t)
	if snap.Phase != core.PhaseIdle || !snap.HandoffPending {
		t.Fatalf("snapshot = %+v, want Idle with the hand-off pending", snap)
	}
	if snap.Session.Outcome(snap.At) != core.OutcomeCompleted || snap.Session.Elapsed(snap.At) != 30*time.Minute {
		t.Errorf("session outcome %v, elapsed %v; want Completed, 30m (capped)", snap.Session.Outcome(snap.At), snap.Session.Elapsed(snap.At))
	}
	ended, ok := snap.Session.EndedAt(snap.At)
	if want := sessionStart.Add(30 * time.Minute); !ok || !ended.Equal(want) {
		t.Errorf("EndedAt = %v, %v; want %v (when it actually ended, not when the app noticed)", ended, ok, want)
	}
}

func TestSnapshot_IsDeterministicAndReadOnly(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(31 * time.Minute)
	sessionsBefore, notesBefore := r.sessions(t), r.notes(t)

	first, second := r.snapshot(t), r.snapshot(t)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("snapshots at the same time differ: %+v vs %+v", first, second)
	}
	r.clock.Advance(time.Minute)
	if later := r.snapshot(t); later.Remaining >= first.Remaining {
		t.Errorf("Remaining did not shrink as time passed: %v then %v", first.Remaining, later.Remaining)
	}
	if !reflect.DeepEqual(r.sessions(t), sessionsBefore) || !reflect.DeepEqual(r.notes(t), notesBefore) {
		t.Error("taking snapshots changed the stored data")
	}
}

func TestSnapshot_AClockSetBeforeTheStartIsFocusWithRemainingCapped(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	r.clock.Set(sessionStart.Add(-time.Hour)) // the wall clock was adjusted backwards
	snap := r.snapshot(t)
	if snap.Phase != core.PhaseFocus || snap.Remaining != 30*time.Minute {
		t.Errorf("snapshot = %+v, want Focus with Remaining capped at the planned 30m", snap)
	}
}
