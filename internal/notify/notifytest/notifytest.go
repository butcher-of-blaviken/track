// Package notifytest has a fake notify.Notifier for tests.
package notifytest

import (
	"context"
	"sync"

	"github.com/butcher-of-blaviken/track/internal/notify"
)

// Recorder is a notify.Notifier that records what it was asked to show.
type Recorder struct {
	// Err is returned by every Notify.
	Err error

	mu        sync.Mutex
	events    []notify.Event
	deadlines []bool
}

// Notify implements notify.Notifier.
func (r *Recorder) Notify(ctx context.Context, e notify.Event) error {
	_, hasDeadline := ctx.Deadline()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	r.deadlines = append(r.deadlines, hasDeadline)
	return r.Err
}

// Events is every Event shown so far, oldest first.
func (r *Recorder) Events() []notify.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notify.Event(nil), r.events...)
}

// AllHadDeadlines is whether every call came with a context that has a deadline.
func (r *Recorder) AllHadDeadlines() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.deadlines {
		if !d {
			return false
		}
	}
	return true
}

// Hang is a notify.Notifier that blocks until its context is done, the way a
// stuck helper process would, and then returns the context's error.
type Hang struct{}

// Notify implements notify.Notifier.
func (Hang) Notify(ctx context.Context, _ notify.Event) error {
	<-ctx.Done()
	return ctx.Err()
}
