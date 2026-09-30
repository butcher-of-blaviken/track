// Command track is a terminal app for tracking tasks, timing focused work
// on them, and leaving hand-off notes.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/config"
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
	configFile := fs.String("config", "", "config file to read (default: config.toml in the platform config directory, if present)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	settings, err := loadSettings(*configFile, getenv)
	if err != nil {
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
	tracker, err := newTracker(store, clk, settings)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(tui.New(tracker, tui.WithFocusDuration(settings.FocusDuration))).Run()
	return err
}

// loadSettings reads the config file over the defaults. A file named with
// --config must exist; the default one is optional.
func loadSettings(configFlag string, getenv func(string) string) (config.Settings, error) {
	home, _ := os.UserHomeDir()
	path, required := config.ResolveFrom(configFlag, config.Env{Getenv: getenv, GOOS: runtime.GOOS, Home: home})
	return config.Load(path, required)
}

// newTracker builds the Tracker with the Break and bell policies the settings
// describe.
func newTracker(store core.Store, clk clock.Clock, s config.Settings) (*core.Tracker, error) {
	policy := core.BreakPolicy{Short: s.BreakDuration, Long: s.LongBreakDuration, LongEvery: s.LongBreakInterval}
	return core.NewTracker(store, clk, policy, core.WithBellPolicy(core.BellPolicy{Interval: s.BellInterval, Repeats: s.BellRepeats}))
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
