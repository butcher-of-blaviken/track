// Command track is a terminal app for tracking tasks, timing focused work
// on them, and leaving hand-off notes.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/datadir"
	"github.com/butcher-of-blaviken/track/internal/store/sqlite"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "track:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("track", flag.ContinueOnError)
	dataDir := fs.String("data-dir", "", "directory for Track's data (default: the platform data directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir, err := datadir.Resolve(*dataDir)
	if err != nil {
		return err
	}
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	clk, err := clockFromEnv(getenv)
	if err != nil {
		return err
	}
	// Defaults for now; the config file and flags arrive with the config tickets.
	tracker, err := core.NewTracker(store, clk, core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4})
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(tui.New(tracker)).Run()
	return err
}

// clockFromEnv returns the real clock, or, for headless test runs, a clock
// running TRACK_TIME_SCALE times faster so a 30-minute session passes in
// seconds. The variable is deliberately undocumented: it is a testing aid.
func clockFromEnv(getenv func(string) string) (clock.Clock, error) {
	raw := getenv("TRACK_TIME_SCALE")
	if raw == "" {
		return clock.System{}, nil
	}
	factor, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("TRACK_TIME_SCALE %q: %w", raw, err)
	}
	return clock.NewScaled(clock.System{}, factor)
}
