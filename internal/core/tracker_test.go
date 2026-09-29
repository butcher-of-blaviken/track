package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
)

var ctx = context.Background()

// rig is a Tracker wired to an in-memory store and a fake clock.
type rig struct {
	tracker *core.Tracker
	store   core.Store
	clock   *clock.Fake
}

func newRig(t *testing.T) *rig {
	t.Helper()
	store := memory.New()
	fake := clock.NewFake(sessionStart)
	return &rig{tracker: core.NewTracker(store, fake), store: store, clock: fake}
}

// addTask saves a Task in the given state and returns its ID.
func (r *rig) addTask(t *testing.T, state core.State) core.TaskID {
	t.Helper()
	var id core.TaskID
	err := r.store.Update(ctx, func(tx core.Tx) error {
		var err error
		id, err = tx.CreateTask(core.Task{Title: "task", State: state, CreatedAt: sessionStart})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) sessions(t *testing.T) []core.FocusSession {
	t.Helper()
	got, err := r.store.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestStartSession_RecordsStartAndPlannedDuration(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)

	r.clock.Advance(5 * time.Minute)
	got, err := r.tracker.StartSession(ctx, task, 30*time.Minute)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if got.ID == 0 || got.TaskID != task || got.StoppedAt != nil {
		t.Errorf("unexpected session: %+v", got)
	}
	if want := sessionStart.Add(5 * time.Minute); !got.StartedAt.Equal(want) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want)
	}
	if got.PlannedDuration != 30*time.Minute {
		t.Errorf("PlannedDuration = %v, want 30m", got.PlannedDuration)
	}

	// The planned duration is stored as given and does not change as time passes.
	r.clock.Advance(2 * time.Hour)
	stored := r.sessions(t)
	if len(stored) != 1 || stored[0].PlannedDuration != 30*time.Minute {
		t.Errorf("stored sessions = %+v, want one with a 30m plan", stored)
	}
}

func TestStartSession_RejectsASecondSessionWhileOneIsRunning(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	other := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(29 * time.Minute)
	for _, id := range []core.TaskID{task, other} {
		if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute); !errors.Is(err, core.ErrSessionRunning) {
			t.Errorf("StartSession(task %d) error = %v, want ErrSessionRunning", id, err)
		}
	}
	if n := len(r.sessions(t)); n != 1 {
		t.Errorf("%d sessions stored, want 1", n)
	}
}

func TestStartSession_IsAllowedOnceThePreviousSessionCompleted(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(30 * time.Minute) // exactly the planned end
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Errorf("StartSession after completion: %v", err)
	}
	if n := len(r.sessions(t)); n != 2 {
		t.Errorf("%d sessions stored, want 2", n)
	}
}

func TestStartSession_Validation(t *testing.T) {
	r := newRig(t)
	active := r.addTask(t, core.StateActive)
	done := r.addTask(t, core.StateDone)
	archived := r.addTask(t, core.StateArchived)

	tests := []struct {
		name    string
		task    core.TaskID
		planned time.Duration
		want    error
	}{
		{"a Done Task cannot be focused on", done, time.Minute, core.ErrTaskNotActive},
		{"an Archived Task cannot be focused on", archived, time.Minute, core.ErrTaskNotActive},
		{"an unknown Task is not found", 9999, time.Minute, core.ErrNotFound},
		{"zero duration is invalid", active, 0, core.ErrInvalidDuration},
		{"negative duration is invalid", active, -time.Minute, core.ErrInvalidDuration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := r.tracker.StartSession(ctx, tt.task, tt.planned); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
	if n := len(r.sessions(t)); n != 0 {
		t.Errorf("%d sessions stored after rejected starts, want 0", n)
	}
}

func TestStopSession_EndsTheRunningSessionEarlyAndKeepsElapsedTime(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(12 * time.Minute)
	got, err := r.tracker.StopSession(ctx)
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	now := r.clock.Now()
	if got.Outcome(now) != core.OutcomeStoppedEarly || got.Elapsed(now) != 12*time.Minute {
		t.Errorf("stopped session: outcome %v, elapsed %v; want StoppedEarly, 12m", got.Outcome(now), got.Elapsed(now))
	}

	// Persisted, and stable as time passes.
	r.clock.Advance(3 * time.Hour)
	stored := r.sessions(t)
	if len(stored) != 1 || stored[0].Outcome(r.clock.Now()) != core.OutcomeStoppedEarly || stored[0].Elapsed(r.clock.Now()) != 12*time.Minute {
		t.Errorf("stored sessions = %+v, want one stopped early after 12m", stored)
	}
}

func TestStopSession_WithNothingRunningIsAnError(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)

	if _, err := r.tracker.StopSession(ctx); !errors.Is(err, core.ErrNoSessionRunning) {
		t.Errorf("before any session: error = %v, want ErrNoSessionRunning", err)
	}

	// A session past its planned end has already completed; it is not stopped early.
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(30*time.Minute + 5*time.Second)
	if _, err := r.tracker.StopSession(ctx); !errors.Is(err, core.ErrNoSessionRunning) {
		t.Errorf("after completion: error = %v, want ErrNoSessionRunning", err)
	}
	if got := r.sessions(t)[0]; got.Outcome(r.clock.Now()) != core.OutcomeCompleted {
		t.Errorf("outcome = %v, want Completed", got.Outcome(r.clock.Now()))
	}

	// A session that was already stopped cannot be stopped again.
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.StopSession(ctx); !errors.Is(err, core.ErrNoSessionRunning) {
		t.Errorf("after stopping: error = %v, want ErrNoSessionRunning", err)
	}
}

func TestStartSession_AfterAnEarlyStopBeginsAFreshFullLengthSession(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(15 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}

	second, err := r.tracker.StartSession(ctx, task, 30*time.Minute)
	if err != nil {
		t.Fatalf("StartSession after early stop: %v", err)
	}
	if second.Outcome(r.clock.Now()) != core.OutcomeRunning || second.Elapsed(r.clock.Now()) != 0 || second.PlannedDuration != 30*time.Minute {
		t.Errorf("second session = %+v, want a fresh running 30m session", second)
	}
	all := r.sessions(t)
	if len(all) != 2 || all[0].Elapsed(r.clock.Now()) != 15*time.Minute {
		t.Errorf("sessions = %+v, want the first kept at 15m elapsed", all)
	}
}
