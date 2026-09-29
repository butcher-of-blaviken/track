package core

import (
	"context"
	"errors"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
)

var (
	// ErrSessionRunning is returned when starting a session while another is running.
	ErrSessionRunning = errors.New("a focus session is already running")
	// ErrNoSessionRunning is returned when there is no running session to act on.
	ErrNoSessionRunning = errors.New("no focus session is running")
	// ErrBreakActive is returned when starting a session during a Break without overriding it.
	ErrBreakActive = errors.New("a break is in progress")
	// ErrTaskNotActive is returned when starting a session on a Done or Archived Task.
	ErrTaskNotActive = errors.New("task is not active")
	// ErrInvalidDuration is returned for a planned duration that is not positive.
	ErrInvalidDuration = errors.New("planned duration must be positive")
	// ErrInvalidBreakPolicy is returned for a BreakPolicy with a non-positive field.
	ErrInvalidBreakPolicy = errors.New("break policy durations and interval must be positive")
)

// BreakPolicy says how long Breaks are and how often one is long.
type BreakPolicy struct {
	Short time.Duration
	Long  time.Duration
	// LongEvery makes every Nth completed session earn the long Break.
	LongEvery int
}

// StartOptions modify how a session is started.
type StartOptions struct {
	// OverrideBreak starts the session even though a Break is running. The
	// caller is responsible for having the user confirm this first.
	OverrideBreak bool
}

// Tracker runs the Focus session lifecycle over a Store, reading time from an
// injected clock.
type Tracker struct {
	store  Store
	clock  clock.Clock
	policy BreakPolicy
}

// NewTracker returns a Tracker, or ErrInvalidBreakPolicy if policy has a
// non-positive field.
func NewTracker(store Store, clock clock.Clock, policy BreakPolicy) (*Tracker, error) {
	if policy.Short <= 0 || policy.Long <= 0 || policy.LongEvery <= 0 {
		return nil, ErrInvalidBreakPolicy
	}
	return &Tracker{store: store, clock: clock, policy: policy}, nil
}

// StartSession starts a Focus session on a Task, recording the current time,
// the planned duration and the Break the session will earn.
func (t *Tracker) StartSession(ctx context.Context, taskID TaskID, planned time.Duration, opts StartOptions) (FocusSession, error) {
	if planned <= 0 {
		return FocusSession{}, ErrInvalidDuration
	}
	var session FocusSession
	err := t.store.Update(ctx, func(tx Tx) error {
		now := t.clock.Now()
		task, err := tx.Task(taskID)
		if err != nil {
			return err
		}
		if task.State != StateActive {
			return ErrTaskNotActive
		}
		// Only one session runs at a time, so the latest is the only one that can
		// be running or have a Break in progress.
		var skipped time.Duration
		switch latest, err := tx.LatestSession(); {
		case err != nil && !errors.Is(err, ErrNotFound):
			return err
		case err != nil:
			// No sessions yet.
		case latest.Outcome(now) == OutcomeRunning:
			return ErrSessionRunning
		case latest.BreakRemaining(now) > 0:
			if !opts.OverrideBreak {
				return ErrBreakActive
			}
			skipped = latest.BreakRemaining(now)
		}
		completed, err := tx.CompletedSessionCount(now)
		if err != nil {
			return err
		}
		session = FocusSession{TaskID: taskID, StartedAt: now, PlannedDuration: planned, BreakDuration: t.policy.Short, SkippedBreak: skipped}
		// Every LongEvery-th completed session earns the long Break. Early stops
		// are not counted and no Break follows them.
		if (completed+1)%t.policy.LongEvery == 0 {
			session.BreakDuration, session.LongBreak = t.policy.Long, true
		}
		id, err := tx.CreateSession(session)
		session.ID = id
		return err
	})
	return session, err
}

// StopSession ends the running Focus session early, keeping the time elapsed.
// It returns ErrNoSessionRunning if there is nothing to stop, including when the
// latest session already reached its planned end: that session completed.
func (t *Tracker) StopSession(ctx context.Context) (FocusSession, error) {
	var session FocusSession
	err := t.store.Update(ctx, func(tx Tx) error {
		now := t.clock.Now()
		latest, err := tx.LatestSession()
		if errors.Is(err, ErrNotFound) {
			return ErrNoSessionRunning
		}
		if err != nil {
			return err
		}
		if latest.Outcome(now) != OutcomeRunning {
			return ErrNoSessionRunning
		}
		latest.StoppedAt = &now
		if err := tx.SaveSession(latest); err != nil {
			return err
		}
		session = latest
		return nil
	})
	return session, err
}
