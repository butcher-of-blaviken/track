package core

import (
	"fmt"
	"time"
)

// SessionID identifies a Focus session. It is assigned by the store.
type SessionID int64

// Outcome is how a Focus session stands at a given moment.
type Outcome int

const (
	// OutcomeRunning means the session has not reached its planned end and was not stopped.
	OutcomeRunning Outcome = iota
	// OutcomeCompleted means the session ran its full planned length.
	OutcomeCompleted
	// OutcomeStoppedEarly means the session was ended by the user before its planned end.
	OutcomeStoppedEarly
)

func (o Outcome) String() string {
	switch o {
	case OutcomeRunning:
		return "Running"
	case OutcomeCompleted:
		return "Completed"
	case OutcomeStoppedEarly:
		return "StoppedEarly"
	}
	return fmt.Sprintf("Outcome(%d)", int(o))
}

// FocusSession is one timed block of work on a single Task.
//
// Completion is derived from the clock, never stored: a session is Completed
// once its planned end has passed and it was not stopped.
type FocusSession struct {
	ID     SessionID
	TaskID TaskID
	// StartedAt is when the session began.
	StartedAt time.Time
	// PlannedDuration is recorded at start and never rewritten.
	PlannedDuration time.Duration
	// StoppedAt is set only when the user ended the session early.
	StoppedAt *time.Time
	// BreakDuration is the Break this session earns if it completes, planned
	// at start like PlannedDuration so later config changes do not alter it.
	BreakDuration time.Duration
	// LongBreak reports whether BreakDuration is a long Break.
	LongBreak bool
	// SkippedBreak is the Break time that was left when this session was
	// started during a Break with the user's confirmation; zero otherwise.
	SkippedBreak time.Duration
}

// PlannedEnd is when the session completes if it is not stopped.
func (s FocusSession) PlannedEnd() time.Time { return s.StartedAt.Add(s.PlannedDuration) }

// Outcome reports whether the session is running, completed or stopped early at now.
func (s FocusSession) Outcome(now time.Time) Outcome {
	switch {
	case s.StoppedAt != nil:
		return OutcomeStoppedEarly
	case !now.Before(s.PlannedEnd()):
		return OutcomeCompleted
	}
	return OutcomeRunning
}

// Elapsed is the time actually spent in the session at now, capped at its planned length.
func (s FocusSession) Elapsed(now time.Time) time.Duration {
	end := now
	if s.StoppedAt != nil {
		end = *s.StoppedAt
	}
	return min(max(end.Sub(s.StartedAt), 0), s.PlannedDuration)
}

// BreakRemaining is how much of the Break earned by this session is left at
// now. It is zero while the session runs, after an early stop (no Break
// follows), and once the Break has ended.
func (s FocusSession) BreakRemaining(now time.Time) time.Duration {
	if s.Outcome(now) != OutcomeCompleted {
		return 0
	}
	return max(s.PlannedEnd().Add(s.BreakDuration).Sub(now), 0)
}
