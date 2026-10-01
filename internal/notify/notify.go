// Package notify raises a system notification when a Focus session or a Break
// ends, in addition to the terminal bell. It knows nothing about the domain: the
// UI says what to announce, and an operating system's implementation, one per
// platform, shows it.
package notify

import "context"

// Event is one notification: what to show, with no task text in it, because
// banners show on the lock screen and in the notification centre.
type Event struct {
	Title string
	Body  string
}

// Notifier shows an Event. An implementation must give up when ctx is done, and
// must not panic or exit: a notification that cannot be shown is an error for the
// caller to ignore, since the bell still rings.
type Notifier interface {
	Notify(ctx context.Context, e Event) error
}

// Nop is a Notifier that shows nothing: the default, and the notifier on a
// platform that has none.
type Nop struct{}

// Notify implements Notifier.
func (Nop) Notify(context.Context, Event) error { return nil }
