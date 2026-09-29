package core

import (
	"errors"
	"time"
)

// ErrInvalidBellPolicy is returned for a bell interval that is not positive or
// a negative repeat count.
var ErrInvalidBellPolicy = errors.New("bell interval must be positive and repeats must not be negative")

// BellPolicy says how often a bell repeats and how many times. A bell rings
// once when its event happens and then Repeats more times, one Interval apart.
type BellPolicy struct {
	Interval time.Duration
	Repeats  int
}

// defaultBellPolicy is 11 rings over five minutes: the first, plus 10 repeats.
var defaultBellPolicy = BellPolicy{Interval: 30 * time.Second, Repeats: 10}

// BellKind is which event a bell announces.
type BellKind int

const (
	// BellSessionEnd announces that a Focus session completed.
	BellSessionEnd BellKind = iota
	// BellBreakEnd announces that a Break is over.
	BellBreakEnd
)

func (k BellKind) String() string {
	switch k {
	case BellSessionEnd:
		return "SessionEnd"
	case BellBreakEnd:
		return "BreakEnd"
	}
	return "BellKind(?)"
}

// Bell is the bell that should currently be ringing. The core keeps no
// acknowledgement state: the caller remembers how many rings it has already
// made for a given (Session, Kind) and whether the user silenced it, and rings
// again when Scheduled exceeds that count.
type Bell struct {
	// Session is the session whose end, or whose Break's end, this bell announces.
	Session SessionID
	Kind    BellKind
	// Scheduled is how many rings should have happened by now, counting from 1.
	Scheduled int
}

// Option customises a Tracker.
type Option func(*Tracker) error

// WithBellPolicy overrides the default bell policy.
func WithBellPolicy(p BellPolicy) Option {
	return func(t *Tracker) error {
		if p.Interval <= 0 || p.Repeats < 0 {
			return ErrInvalidBellPolicy
		}
		t.bell = p
		return nil
	}
}

// bellFor returns the bell that should be ringing for the latest session at
// now, or nil. Only a completed session rings: the user ends an early stop
// themselves. The latest event that has started wins, so the Break-end bell
// takes over from the session-end one, and an event whose window has passed is
// silent.
func (p BellPolicy) bellFor(session FocusSession, now time.Time) *Bell {
	if session.Outcome(now) != OutcomeCompleted {
		return nil
	}
	kind, at := BellSessionEnd, session.PlannedEnd()
	if session.BreakDuration > 0 {
		if breakEnd := at.Add(session.BreakDuration); !now.Before(breakEnd) {
			kind, at = BellBreakEnd, breakEnd
		}
	}
	scheduled := int(now.Sub(at)/p.Interval) + 1
	if scheduled > 1+p.Repeats {
		return nil
	}
	return &Bell{Session: session.ID, Kind: kind, Scheduled: scheduled}
}
