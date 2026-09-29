// Package clock provides an injectable source of time so timers and
// reconciliation can be tested without sleeping.
package clock

import (
	"errors"
	"math"
	"sync"
	"time"
)

// Clock reports the current time.
type Clock interface {
	Now() time.Time
}

// Fake is a Clock that only moves when told to.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake whose Now() is start.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

// Now returns the fake's current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the fake forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set jumps the fake to t, which may be earlier than the current time.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// System is the real wall clock.
type System struct{}

// Now returns the current time.
func (System) Now() time.Time { return time.Now() }

// ErrInvalidScale is returned for a scale factor that is not a positive, finite number.
var ErrInvalidScale = errors.New("clock scale factor must be positive and finite")

// Scaled is a Clock that runs at a multiple of its base clock's speed, starting
// from the base's time at creation. It exists so headless runs can watch a
// 30-minute session pass in seconds.
type Scaled struct {
	base   Clock
	start  time.Time
	factor float64
}

// NewScaled returns a Scaled clock that starts at base.Now() and then runs
// factor times as fast as base.
func NewScaled(base Clock, factor float64) (*Scaled, error) {
	if factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return nil, ErrInvalidScale
	}
	return &Scaled{base: base, start: base.Now(), factor: factor}, nil
}

// Now returns the scaled current time.
func (s *Scaled) Now() time.Time {
	elapsed := s.base.Now().Sub(s.start)
	return s.start.Add(time.Duration(float64(elapsed) * s.factor))
}
