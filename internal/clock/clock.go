// Package clock provides an injectable source of time so timers and
// reconciliation can be tested without sleeping.
package clock

import (
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
