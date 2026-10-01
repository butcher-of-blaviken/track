package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultMacOSSound is the sound a notification plays: a file in
// /System/Library/Sounds, named without its extension.
const DefaultMacOSSound = "Glass"

// MacOS shows a notification with osascript, which every Mac has, so there is
// nothing to install. It compiles everywhere, so it can be tested anywhere; only
// main chooses it, and only on macOS.
//
// The banner is attributed to Script Editor, not to Track or the terminal, and
// macOS shows it only if Script Editor is allowed to send notifications (System
// Settings > Notifications). If it is not, osascript still exits 0 and shows
// nothing, which no code here can detect: that is what `track notify-test` is for.
// Focus and Do Not Disturb may hide it too.
type MacOS struct {
	// Sound is the name of a system sound to play with the banner. Empty plays none.
	Sound string

	run runner
}

// runner runs a command and returns its combined output. It is a seam so tests
// need no Mac.
type runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// NewMacOS is a MacOS that plays DefaultMacOSSound.
func NewMacOS() *MacOS { return &MacOS{Sound: DefaultMacOSSound, run: execRunner} }

// Notify implements Notifier.
func (n *MacOS) Notify(ctx context.Context, e Event) error {
	run := n.run
	if run == nil {
		run = execRunner
	}
	out, err := run(ctx, "osascript", n.args(e)...)
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("osascript: %w: %s", err, msg)
		}
		return fmt.Errorf("osascript: %w", err)
	}
	return nil
}

// args are osascript's arguments. The script is fixed text, and the title, body
// and sound reach it as arguments, which AppleScript hands to `on run` as plain
// strings: text in a Task or a setting can therefore never become script.
func (n *MacOS) args(e Event) []string {
	display := "display notification (item 2 of argv) with title (item 1 of argv)"
	args := []string{e.Title, e.Body}
	if n.Sound != "" {
		display += " sound name (item 3 of argv)"
		args = append(args, n.Sound)
	}
	return append([]string{"-e", "on run argv", "-e", display, "-e", "end run", "--"}, args...)
}
