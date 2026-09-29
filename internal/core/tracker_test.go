package core_test

import (
	"context"
	"errors"
	"slices"
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
	return newRigWithPolicy(t, testPolicy)
}

var testPolicy = core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4}

func newRigWithPolicy(t *testing.T, policy core.BreakPolicy) *rig {
	t.Helper()
	store := memory.New()
	fake := clock.NewFake(sessionStart)
	tracker, err := core.NewTracker(store, fake, policy)
	if err != nil {
		t.Fatal(err)
	}
	return &rig{tracker: tracker, store: store, clock: fake}
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
	got, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
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
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(29 * time.Minute)
	for _, id := range []core.TaskID{task, other} {
		if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); !errors.Is(err, core.ErrSessionRunning) {
			t.Errorf("StartSession(task %d) error = %v, want ErrSessionRunning", id, err)
		}
	}
	if n := len(r.sessions(t)); n != 1 {
		t.Errorf("%d sessions stored, want 1", n)
	}
}

func TestStartSession_IsAllowedOnceThePreviousSessionAndItsBreakAreOver(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(30*time.Minute + testPolicy.Short) // exactly the end of the Break
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
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
			if _, err := r.tracker.StartSession(ctx, tt.task, tt.planned, core.StartOptions{}); !errors.Is(err, tt.want) {
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
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
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
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
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
	r.clock.Advance(testPolicy.Short) // let the completed session's Break end first
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
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
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(15 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}

	second, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
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

// completeSession starts a 30m session and lets it run to its end and through
// its Break, so the next start is not locked.
func (r *rig) completeSession(t *testing.T, task core.TaskID) core.FocusSession {
	t.Helper()
	got, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	r.clock.Advance(30*time.Minute + got.BreakDuration)
	return got
}

func TestStartSession_PlansALongBreakEveryNthCompletedSession(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)

	var long []bool
	var lengths []time.Duration
	for range 5 {
		s := r.completeSession(t, task)
		long = append(long, s.LongBreak)
		lengths = append(lengths, s.BreakDuration)
	}
	wantLong := []bool{false, false, false, true, false}
	wantLengths := []time.Duration{10 * time.Minute, 10 * time.Minute, 10 * time.Minute, 20 * time.Minute, 10 * time.Minute}
	if !slices.Equal(long, wantLong) || !slices.Equal(lengths, wantLengths) {
		t.Errorf("long = %v, lengths = %v; want %v, %v", long, lengths, wantLong, wantLengths)
	}

	// The plan is stored on the session, so it survives later config changes.
	stored := r.sessions(t)
	if !stored[3].LongBreak || stored[3].BreakDuration != 20*time.Minute {
		t.Errorf("stored 4th session = %+v, want the long Break recorded", stored[3])
	}
}

func TestStartSession_EarlyStopsDoNotAdvanceTheLongBreakCount(t *testing.T) {
	r := newRigWithPolicy(t, core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 2})
	task := r.addTask(t, core.StateActive)

	r.completeSession(t, task) // completed #1
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(5 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil { // stopped early: not counted
		t.Fatal(err)
	}

	third, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !third.LongBreak || third.BreakDuration != 20*time.Minute {
		t.Errorf("third session = %+v, want the long Break (second completed session)", third)
	}
}

func TestNewTracker_RejectsAnInvalidBreakPolicy(t *testing.T) {
	bad := []core.BreakPolicy{
		{Short: 0, Long: 20 * time.Minute, LongEvery: 4},
		{Short: 10 * time.Minute, Long: 0, LongEvery: 4},
		{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 0},
		{Short: -time.Minute, Long: 20 * time.Minute, LongEvery: 4},
	}
	for _, policy := range bad {
		if _, err := core.NewTracker(memory.New(), clock.NewFake(sessionStart), policy); !errors.Is(err, core.ErrInvalidBreakPolicy) {
			t.Errorf("NewTracker(%+v) error = %v, want ErrInvalidBreakPolicy", policy, err)
		}
	}
}

func TestStartSession_SoftLockDuringABreak(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(34 * time.Minute) // 4m into the 10m Break: 6m left

	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); !errors.Is(err, core.ErrBreakActive) {
		t.Fatalf("StartSession during a Break error = %v, want ErrBreakActive", err)
	}
	if n := len(r.sessions(t)); n != 1 {
		t.Fatalf("%d sessions stored after a locked start, want 1", n)
	}

	got, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{OverrideBreak: true})
	if err != nil {
		t.Fatalf("overridden StartSession: %v", err)
	}
	if got.SkippedBreak != 6*time.Minute {
		t.Errorf("SkippedBreak = %v, want 6m", got.SkippedBreak)
	}
	stored := r.sessions(t)
	if len(stored) != 2 || stored[1].SkippedBreak != 6*time.Minute {
		t.Errorf("stored sessions = %+v, want the override recorded on the second", stored)
	}
}

func TestStartSession_OverrideWithNoBreakRecordsNothing(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)

	got, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{OverrideBreak: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.SkippedBreak != 0 {
		t.Errorf("SkippedBreak = %v, want 0 when there was no Break to skip", got.SkippedBreak)
	}
}

func TestStartSession_NoLockAfterAnEarlyStopBecauseNoBreakFollows(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(10 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Errorf("StartSession right after an early stop: %v", err)
	}
}
