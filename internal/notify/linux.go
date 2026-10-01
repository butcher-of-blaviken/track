package notify

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// DefaultLinuxSound is the sound a notification asks for: a name from the
// freedesktop sound naming specification, which the sound theme maps to a file.
const DefaultLinuxSound = "alarm-clock-elapsed"

// ErrNoBus is returned when there is no D-Bus session bus to talk to, as in a
// headless session or over SSH.
var ErrNoBus = errors.New("no D-Bus session bus")

// ErrNoServer is returned when the session bus is there but no notification
// server is running on it.
var ErrNoServer = errors.New("no notification server is running")

// Linux shows a notification by calling org.freedesktop.Notifications.Notify on
// the D-Bus session bus. Every mainstream desktop (GNOME, KDE Plasma, XFCE, and
// the dunst and mako daemons) implements that interface, so this does not depend
// on the distribution, nor on notify-send being installed. It compiles
// everywhere, so it can be tested anywhere; only main chooses it, only on Linux.
//
// It never starts a bus: godbus would run dbus-launch if it found none, which on
// a headless machine spawns a daemon as a side effect of a notification. No bus,
// or no server on it, is an error for the caller to ignore, and the bell rings.
//
// The sound is a hint, and servers differ in whether they honour it, so it is
// best effort.
type Linux struct {
	// Sound is the name of a sound from the sound theme, sent as the sound-name
	// hint. Empty sends none.
	Sound string

	// address is the bus to use, for tests; empty means the user's session bus.
	address string
}

// NewLinux is a Linux notifier that asks for DefaultLinuxSound.
func NewLinux() *Linux { return &Linux{Sound: DefaultLinuxSound} }

// Name implements Named.
func (*Linux) Name() string { return "D-Bus (org.freedesktop.Notifications)" }

const (
	notificationsDest = "org.freedesktop.Notifications"
	notificationsPath = "/org/freedesktop/Notifications"
	notificationsCall = "org.freedesktop.Notifications.Notify"
	linuxAppName      = "Track"
	expireDefault     = -1 // expire_timeout: let the server decide
)

// Notify implements Notifier.
func (n *Linux) Notify(ctx context.Context, e Event) error {
	conn, err := n.connect(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoBus, err)
	}
	defer func() { _ = conn.Close() }()

	hints := map[string]dbus.Variant{}
	if n.Sound != "" {
		hints["sound-name"] = dbus.MakeVariant(n.Sound)
	}
	call := conn.Object(notificationsDest, notificationsPath).CallWithContext(ctx, notificationsCall, 0,
		linuxAppName, uint32(0), "", e.Title, e.Body, []string{}, hints, int32(expireDefault))
	if call.Err != nil {
		// The connection lives only as long as ctx, so when ctx ends the call can
		// fail as a closed connection first. The deadline is the real cause.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%s: %w", notificationsDest, ctxErr)
		}
		var dbusErr dbus.Error
		if errors.As(call.Err, &dbusErr) && (dbusErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || dbusErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner") {
			return fmt.Errorf("%w: %w", ErrNoServer, call.Err)
		}
		return fmt.Errorf("%s: %w", notificationsDest, call.Err)
	}
	return nil
}

// connect opens, authenticates and greets a connection to the bus, without ever
// launching one.
func (n *Linux) connect(ctx context.Context) (*dbus.Conn, error) {
	var conn *dbus.Conn
	var err error
	if n.address != "" {
		conn, err = dbus.Dial(n.address, dbus.WithContext(ctx))
	} else {
		conn, err = dbus.SessionBusPrivateNoAutoStartup(dbus.WithContext(ctx))
	}
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
