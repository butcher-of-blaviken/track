package core

import (
	"context"
	"errors"
	"time"
)

// Phase is what the user should currently be doing.
type Phase int

const (
	// PhaseIdle means no session is running and no Break is in progress.
	PhaseIdle Phase = iota
	// PhaseFocus means a Focus session is running.
	PhaseFocus
	// PhaseBreak means a completed session's Break is in progress.
	PhaseBreak
)

// Snapshot is the state of the timer at one moment, derived from the store and
// the clock. It is a pure read: taking one changes nothing.
type Snapshot struct {
	Phase Phase
	// At is the clock reading the snapshot was derived at.
	At time.Time
	// Session is the latest session, running or ended, or nil if there is none.
	Session *FocusSession
	// Remaining is the time left in the current phase: until the planned end in
	// Focus, until the Break ends in Break, and zero when Idle.
	Remaining time.Duration
	// HandoffPending reports that the latest session has ended and its
	// hand-off is unresolved. It is independent of Phase: after a completed
	// session the Break runs while the hand-off is still due.
	HandoffPending bool
}

// Snapshot derives the current Snapshot.
func (t *Tracker) Snapshot(ctx context.Context) (Snapshot, error) {
	now := t.clock.Now()
	snap := Snapshot{Phase: PhaseIdle, At: now}
	latest, err := t.store.LatestSession(ctx)
	if errors.Is(err, ErrNotFound) {
		return snap, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	snap.Session = &latest
	switch {
	case latest.Outcome(now) == OutcomeRunning:
		snap.Phase = PhaseFocus
		snap.Remaining = min(latest.PlannedEnd().Sub(now), latest.PlannedDuration)
	default:
		snap.HandoffPending = latest.HandoffAt == nil
		if remaining := latest.BreakRemaining(now); remaining > 0 {
			snap.Phase = PhaseBreak
			snap.Remaining = remaining
		}
	}
	return snap, nil
}
