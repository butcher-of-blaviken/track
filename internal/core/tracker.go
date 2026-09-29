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
	// ErrTaskNotActive is returned when starting a session on a Done or Archived Task.
	ErrTaskNotActive = errors.New("task is not active")
	// ErrInvalidDuration is returned for a planned duration that is not positive.
	ErrInvalidDuration = errors.New("planned duration must be positive")
)

// Tracker runs the Focus session lifecycle over a Store, reading time from an
// injected clock.
type Tracker struct {
	store Store
	clock clock.Clock
}

// NewTracker returns a Tracker.
func NewTracker(store Store, clock clock.Clock) *Tracker {
	return &Tracker{store: store, clock: clock}
}

// StartSession starts a Focus session on a Task, recording the current time
// and the planned duration.
func (t *Tracker) StartSession(ctx context.Context, taskID TaskID, planned time.Duration) (FocusSession, error) {
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
		// Only one session runs at a time, so the latest is the only candidate.
		switch latest, err := tx.LatestSession(); {
		case err == nil && latest.Outcome(now) == OutcomeRunning:
			return ErrSessionRunning
		case err != nil && !errors.Is(err, ErrNotFound):
			return err
		}
		session = FocusSession{TaskID: taskID, StartedAt: now, PlannedDuration: planned}
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
