# Desktop notifications from the UI layer, with osascript and D-Bus

When a Focus session or its Break ends, Track already rings the terminal bell. The `notifications` config key (`off` by default, or `desktop`) adds a notification from the operating system's own notification system, on the first ring of each event, so a person notices when Track is not the window in front of them. The bell is unchanged and always rings.

- **It is a `Notifier` in the UI layer.** A small interface (`internal/notify`) is injected like the tick and the theme, and called from where the bell is rung. The core stays free of notification code and goroutines (ADR 0002), and the pull-based design means no timer is needed: the UI already learns when an event is due.
- **macOS uses `osascript`.** It is on every Mac, so nothing has to be installed. The title, body and sound are passed as arguments after `--`, never put in the script, so no text can become AppleScript.
- **Linux calls `org.freedesktop.Notifications` over D-Bus with godbus.** That interface is what GNOME, KDE Plasma, XFCE, dunst and mako implement, so it does not depend on the distribution, or on `notify-send` being installed. Track finds the session bus itself (`$DBUS_SESSION_BUS_ADDRESS`, then `$XDG_RUNTIME_DIR/bus`) and never uses godbus's discovery, which would run `dbus-launch` and start a bus daemon, and which also looks in a hardcoded `/run/user/<uid>/bus` whatever the environment says.
- **First ring only.** The bell repeats every 30 seconds for five minutes; a banner that repeated would be a nuisance, and a missed one is covered by the bell.
- **The text is generic.** Banners show on the lock screen and in Notification Center, so a Task's title stays out of them.
- **Failure is quiet, and checkable.** A notification that cannot be shown (no `osascript`, no session bus, no server, a timeout) is an error the UI ignores, with a 10 second limit so a stuck helper cannot linger. `track notify-test` is how a person finds out why, because on macOS it fails silently.
- **One additive config key,** optional with a default, so a config from an earlier v1 behaves as before (ADR 0003). More values can be added later.

## Considered Options

- **A native macOS notification (UNUserNotificationCenter):** it would show as Track, not Script Editor, but needs a signed app bundle, which a `go install` binary cannot be.
- **`terminal-notifier` or `alerter`:** they would give a better identity than Script Editor, but are third-party tools to install, and `terminal-notifier` had no release for nine years before 2026. They can be preferred when present, later.
- **Terminal escape sequences (OSC 9, 777, 99):** the terminal turns them into a notification, and they are the only thing that works over SSH. But whether a terminal supports them cannot be detected, macOS Terminal.app does not, and tmux needs `allow-passthrough`. Worth adding as an opt-in later; it is not a replacement.
- **A library such as beeep:** it builds its AppleScript by interpolating text, has no control over the sound, and brings Windows dependencies. A thin package of our own is smaller and can be tested.
- **Running `notify-send` on Linux:** simpler, but it needs the `libnotify` package, which desktop distributions usually have and minimal installs do not. Talking D-Bus directly has no such dependency.
- **A background daemon,** or notifying when the app is closed: a non-goal in the PRD, and not needed to notify while the app runs.
- **Notifying on every ring,** or including the Task's title: rejected above.

## Consequences

On macOS the banner is attributed to Script Editor, which has to be allowed to send notifications once; until it is, nothing shows and nothing says so, except `track notify-test`. On Linux the sound is a hint that desktops honour inconsistently. The Linux notifier is tested against a real `dbus-daemon` with a fake notification server (in CI, which fails if the daemon is missing), which checks the exact call it sends, but not how a real desktop draws or sounds it. The macOS notifier is tested with a fake command runner and by hand. Other platforms get nothing. Nothing is shown while the app is closed.
