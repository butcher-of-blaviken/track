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
	os.Exit(realMain(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// Exit codes: a usage mistake is distinct from a failure to do what was asked.
const (
	exitFailure = 1
	exitUsage   = 2
)

// realMain is the whole command line, returning the exit code: it opens the TUI
// when no subcommand is given and otherwise runs the subcommand.
func realMain(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "track: %v\nRun 'track --help' for usage.\n", err)
		return exitUsage
	case len(opts.rest) == 0:
		return report(runTUI(opts, getenv), stderr)
	}
	if opts.rest[0] == "export" {
		return runExport(opts.rest[1:], opts, getenv, stdout, stderr)
	}
	cmd, ok := findCommand(opts.rest[0])
	if !ok {
		_, _ = fmt.Fprintf(stderr, "track: unknown command %q (commands: add, note, export)\nRun 'track --help' for usage.\n", opts.rest[0])
		return exitUsage
	}
	return runCommand(cmd, opts.rest[1:], opts, getenv, stdout, stderr)
}

// report prints an error as the command line's failure and returns its exit code.
func report(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "track:", err)
	return exitFailure
}

// options are what the command line asks for.
type options struct {
	dataDir    string
	configFile string
	overrides  config.Overrides
	// rest is what follows the flags: a subcommand and its words, or nothing.
	rest []string
}

// parseArgs reads the flags, writing usage and errors to stderr. It returns
// flag.ErrHelp for -h and --help.
func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("track", flag.ContinueOnError)
	// The flag package would print the error and then the whole usage, which is
	// longer than a terminal and pushes the error off the top. Errors go back to
	// the caller instead, and only -h prints the usage.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

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
	if errors.Is(err, flag.ErrHelp) {
		usage(stderr)
	}
	opts.rest = fs.Args()
	return opts, err
}

// usage prints the help text, with the defaults taken from config.Defaults.
func usage(w io.Writer) {
	d := config.Defaults()
	_, _ = fmt.Fprintf(w, `Usage:
  track [flags]             open the app
  track add <text>          create a Task; ##tag words become Tags
  track note <text>         save a note to the inbox, to file onto a Task later
  track export              write everything as JSON or Markdown (-f, -o FILE)

Flags (they go before the subcommand: track --data-dir DIR add <text>):
  --focus-duration d       length of a Focus session (default %s)
  --break-duration d       length of a short Break (default %s)
  --long-break-duration d  length of a long Break (default %s)
  --long-break-interval n  every nth completed session earns a long Break (default %d)
  --data-dir dir           directory for Track's data (default: the platform data directory)
  --config file            config file to read (default: config.toml in the platform config directory)

Durations are written like 25m or 1h30m. Flags apply to this run only and override
the config file, which overrides the defaults. The text of add and note may be
several words; put it in quotes, and use -- before text that starts with a dash.
In fish an unquoted ##tag starts a comment, so quote it.
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

// app is the opened database and the Tracker over it.
type app struct {
	tracker *core.Tracker
	close   func() error
}

// openApp opens the database in the data directory and builds the Tracker from
// the settings and the clock.
func openApp(dataDirFlag string, getenv func(string) string, settings config.Settings) (*app, error) {
	dir, err := datadir.Resolve(dataDirFlag)
	if err != nil {
		return nil, err
	}
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		return nil, err
	}
	clk, err := clockFromEnv(getenv)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	tracker, err := newTracker(store, clk, settings)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return &app{tracker: tracker, close: store.Close}, nil
}

// runTUI opens the app.
func runTUI(opts options, getenv func(string) string) error {
	settings, err := opts.settings(getenv)
	if err != nil {
		return err
	}
	a, err := openApp(opts.dataDir, getenv, settings)
	if err != nil {
		return err
	}
	defer func() { _ = a.close() }()

	_, err = tea.NewProgram(tui.New(a.tracker, tui.WithFocusDuration(settings.FocusDuration))).Run()
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
