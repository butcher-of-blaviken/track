// Command track is a terminal app for tracking tasks, timing focused work
// on them, and leaving hand-off notes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

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

// options are what the command line asks for.
type options struct {
	dataDir    string
	configFile string
	overrides  config.Overrides
}

// parseArgs reads the flags, writing usage and errors to stderr. It returns
// flag.ErrHelp for -h and --help.
func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("track", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }

	fs.StringVar(&opts.dataDir, "data-dir", "", "")
	fs.StringVar(&opts.configFile, "config", "", "")
	duration := func(name string, into **time.Duration) {
		fs.Func(name, "", func(text string) error {
			d, err := config.ParseDuration(text)
			if err != nil {
				return err
			}
			*into = &d
			return nil
		})
	}
	duration("focus-duration", &opts.overrides.FocusDuration)
	duration("break-duration", &opts.overrides.BreakDuration)
	duration("long-break-duration", &opts.overrides.LongBreakDuration)
	fs.Func("long-break-interval", "", func(text string) error {
		n, err := config.ParseInterval(text)
		if err != nil {
			return err
		}
		opts.overrides.LongBreakInterval = &n
		return nil
	})

	err := fs.Parse(args)
	return opts, err
}

// usage prints the help text, with the defaults taken from config.Defaults.
func usage(w io.Writer) {
	d := config.Defaults()
	_, _ = fmt.Fprintf(w, `Usage: track [flags]

Flags:
  --focus-duration d       length of a Focus session (default %s)
  --break-duration d       length of a short Break (default %s)
  --long-break-duration d  length of a long Break (default %s)
  --long-break-interval n  every nth completed session earns a long Break (default %d)
  --data-dir dir           directory for Track's data (default: the platform data directory)
  --config file            config file to read (default: config.toml in the platform config directory)

Durations are written like 25m or 1h30m. Flags apply to this run only and override
the config file, which overrides the defaults.
`, d.FocusDuration, d.BreakDuration, d.LongBreakDuration, d.LongBreakInterval)
}

// settings is the config file over the defaults, with the flags over that.
func (o options) settings(getenv func(string) string) (config.Settings, error) {
	s, err := loadSettings(o.configFile, getenv)
	if err != nil {
		return config.Settings{}, err
	}
	return s.Apply(o.overrides), nil
}

func run(args []string, getenv func(string) string) error {
	opts, err := parseArgs(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	settings, err := opts.settings(getenv)
	if err != nil {
		return err
	}

	dir, err := datadir.Resolve(opts.dataDir)
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
