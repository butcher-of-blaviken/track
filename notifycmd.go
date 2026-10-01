package main

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/butcher-of-blaviken/track/internal/config"
	"github.com/butcher-of-blaviken/track/internal/notify"
)

// notifyTestTimeout is how long `track notify-test` waits for the notifier. It
// is a variable so a test need not wait it out.
var notifyTestTimeout = 10 * time.Second

// runNotifyTestCommand is `track notify-test` for this platform. It needs neither
// the database nor the config file, and works whatever the notifications setting
// is, so it can be tried before turning them on.
func runNotifyTestCommand(words []string, stdout, stderr io.Writer) int {
	return runNotifyTest(words, notifierFor(config.NotificationsDesktop, runtime.GOOS), runtime.GOOS, stdout, stderr)
}

// runNotifyTest sends one notification through n and says what happened. On macOS
// a blocked notification fails silently (osascript exits 0 and shows nothing), so
// what it prints when it works is the checklist for when nothing appears.
func runNotifyTest(words []string, n notify.Notifier, goos string, stdout, stderr io.Writer) int {
	if len(words) > 0 {
		_, _ = fmt.Fprintf(stderr, "track notify-test: unexpected argument %q\nRun 'track --help' for usage.\n", words[0])
		return exitUsage
	}
	name := notify.NameOf(n)
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "track notify-test: Track has no desktop notifications for %s yet, so only the terminal bell is used.\n", goos)
		return exitFailure
	}

	ctx, cancel := context.WithTimeout(context.Background(), notifyTestTimeout)
	defer cancel()
	if err := n.Notify(ctx, notify.Event{Title: "Track", Body: "This is a test notification from Track."}); err != nil {
		_, _ = fmt.Fprintf(stderr, "track notify-test: could not send a notification with %s: %v\n", name, err)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "Sent a test notification with %s.\n", name)
	_, _ = io.WriteString(stdout, notifyTestAdvice(goos))
	return 0
}

// notifyTestAdvice is what to check, for a platform whose notifier cannot tell
// whether the notification was shown.
func notifyTestAdvice(goos string) string {
	switch goos {
	case "darwin":
		return `osascript exited normally, but macOS gives no way to tell whether it showed the
banner. Look at your screen: you should see a notification titled "Track" and
hear the Glass sound.

If nothing appeared:
  - Open System Settings > Notifications > Script Editor and turn on Allow
    Notifications. Track sends them with osascript, so macOS shows them as coming
    from Script Editor.
  - Check that a Focus mode or Do Not Disturb is not hiding them.
  - Run track notify-test again.

To get these when a Focus session or a Break ends, set notifications = "desktop"
in the config file.
`
	case "linux":
		return `The notification server accepted it, so a banner should have appeared. The sound is
only a hint, and desktops differ in whether they play it.

If nothing appeared:
  - Check that Do Not Disturb (or your desktop's equivalent) is not on, and that
    notifications are allowed in your desktop's settings.
  - Track needs a notification server on your D-Bus session bus. GNOME, KDE Plasma
    and XFCE have one; on a minimal window manager install dunst or mako.
  - Over SSH or in a headless session there is no session bus, so nothing can be
    shown there; the terminal bell still rings.
  - Run track notify-test again.

To get these when a Focus session or a Break ends, set notifications = "desktop"
in the config file.
`
	}
	return ""
}
