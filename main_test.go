package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/config"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/notify"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestClockFromEnv_DefaultsToTheSystemClock(t *testing.T) {
	c, err := clockFromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(clock.System); !ok {
		t.Errorf("clock = %T, want clock.System", c)
	}
}

func TestClockFromEnv_TimeScaleGivesAScaledClock(t *testing.T) {
	c, err := clockFromEnv(env(map[string]string{"TRACK_TIME_SCALE": "120"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*clock.Scaled); !ok {
		t.Errorf("clock = %T, want *clock.Scaled", c)
	}
}

func TestClockFromEnv_RejectsABadTimeScale(t *testing.T) {
	for _, bad := range []string{"fast", "0", "-3", "NaN", "1x"} {
		if _, err := clockFromEnv(env(map[string]string{"TRACK_TIME_SCALE": bad})); err == nil {
			t.Errorf("TRACK_TIME_SCALE=%q returned no error", bad)
		}
	}
}

func writeConfigFile(t *testing.T, dir, text string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSettings_TheDefaultConfigIsOptionalAndFoundInTheConfigDir(t *testing.T) {
	xdg := t.TempDir()
	getenv := env(map[string]string{"XDG_CONFIG_HOME": xdg})

	got, err := loadSettings("", getenv)
	if err != nil || got != config.Defaults() {
		t.Fatalf("no config file: %+v, %v; want the defaults", got, err)
	}

	if err := os.MkdirAll(filepath.Join(xdg, "track"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfigFile(t, filepath.Join(xdg, "track"), `focus_duration = "50m"`)
	got, err = loadSettings("", getenv)
	if err != nil || got.FocusDuration != 50*time.Minute {
		t.Errorf("with a config file: %+v, %v; want a 50m focus duration", got, err)
	}
}

func TestLoadSettings_TheConfigFlagNamesTheFile(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), `break_duration = "3m"`)
	got, err := loadSettings(path, env(nil))
	if err != nil || got.BreakDuration != 3*time.Minute {
		t.Errorf("got %+v, %v; want a 3m Break", got, err)
	}
}

func TestLoadSettings_ANamedFileMustExistAndABrokenOneIsRefused(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.toml")
	if _, err := loadSettings(missing, env(nil)); err == nil {
		t.Error("a missing --config file returned no error")
	}
	broken := writeConfigFile(t, t.TempDir(), `focus_duration = "soon"`)
	if _, err := loadSettings(broken, env(nil)); err == nil || !strings.Contains(err.Error(), "focus_duration") {
		t.Errorf("err = %v, want one naming focus_duration", err)
	}
}

func TestNewTracker_TheSettingsDriveTheBreakPolicy(t *testing.T) {
	s := config.Defaults()
	s.BreakDuration = 7 * time.Minute
	s.LongBreakDuration = 15 * time.Minute
	s.LongBreakInterval = 2
	start := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	clk := clock.NewFake(start)
	tracker, err := newTracker(memory.New(), clk, s)
	if err != nil {
		t.Fatal(err)
	}
	task, err := tracker.AddTask(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}

	var breaks []time.Duration
	for range 2 {
		session, err := tracker.StartSession(context.Background(), task.ID, 30*time.Minute, core.StartOptions{})
		if err != nil {
			t.Fatal(err)
		}
		breaks = append(breaks, session.BreakDuration)
		clk.Advance(30*time.Minute + session.BreakDuration)
	}
	if breaks[0] != 7*time.Minute || breaks[1] != 15*time.Minute {
		t.Errorf("Break durations = %v, want a 7m short Break then the 15m long one (every 2nd)", breaks)
	}
}

func TestNewTracker_TheSettingsDriveTheBellPolicy(t *testing.T) {
	s := config.Defaults()
	s.BellInterval = 10 * time.Second
	s.BellRepeats = 2
	start := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	clk := clock.NewFake(start)
	tracker, err := newTracker(memory.New(), clk, s)
	if err != nil {
		t.Fatal(err)
	}
	task, _ := tracker.AddTask(context.Background(), "work")
	if _, err := tracker.StartSession(context.Background(), task.ID, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		after time.Duration
		want  int
	}{{0, 1}, {10 * time.Second, 2}, {20 * time.Second, 3}, {5 * time.Minute, 0}} { // 0: the repeats are used up
		clk.Set(start.Add(30*time.Minute + tc.after))
		snap, err := tracker.Snapshot(context.Background())
		switch {
		case err != nil:
			t.Fatal(err)
		case tc.want == 0 && snap.Bell != nil:
			t.Errorf("%v after the end: bell %+v, want none left", tc.after, snap.Bell)
		case tc.want > 0 && (snap.Bell == nil || snap.Bell.Scheduled != tc.want):
			t.Errorf("%v after the end: bell %+v, want %d rings due", tc.after, snap.Bell, tc.want)
		}
	}
}

func TestParseArgs_NoFlagsSetsNothing(t *testing.T) {
	opts, err := parseArgs(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.overrides != (config.Overrides{}) || opts.dataDir != "" || opts.configFile != "" {
		t.Errorf("opts = %+v, want everything unset", opts)
	}
}

func TestParseArgs_TheFourDurationFlagsInEveryForm(t *testing.T) {
	for _, args := range [][]string{
		{"--focus-duration", "20m", "--break-duration", "3m", "--long-break-duration", "15m", "--long-break-interval", "2"},
		{"-focus-duration", "20m", "-break-duration", "3m", "-long-break-duration", "15m", "-long-break-interval", "2"},
		{"--focus-duration=20m", "--break-duration=3m", "--long-break-duration=15m", "--long-break-interval=2"},
	} {
		opts, err := parseArgs(args, io.Discard)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		o := opts.overrides
		if o.FocusDuration == nil || *o.FocusDuration != 20*time.Minute ||
			o.BreakDuration == nil || *o.BreakDuration != 3*time.Minute ||
			o.LongBreakDuration == nil || *o.LongBreakDuration != 15*time.Minute ||
			o.LongBreakInterval == nil || *o.LongBreakInterval != 2 {
			t.Errorf("%v: overrides = %+v", args, o)
		}
	}
}

func TestParseArgs_OnlyTheFlagsGivenAreSet(t *testing.T) {
	opts, err := parseArgs([]string{"--break-duration", "3m", "--data-dir", "/d", "--config", "/c.toml"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	o := opts.overrides
	if o.BreakDuration == nil || o.FocusDuration != nil || o.LongBreakDuration != nil || o.LongBreakInterval != nil {
		t.Errorf("overrides = %+v, want only the Break duration", o)
	}
	if opts.dataDir != "/d" || opts.configFile != "/c.toml" {
		t.Errorf("opts = %+v", opts)
	}
}

func TestParseArgs_BadValuesAreRefusedNamingTheFlag(t *testing.T) {
	for name, tc := range map[string]struct {
		args      []string
		flag, why string
	}{
		"not a duration":    {[]string{"--focus-duration", "soon"}, "focus-duration", "not a duration"},
		"zero duration":     {[]string{"--break-duration", "0s"}, "break-duration", "positive"},
		"negative duration": {[]string{"--long-break-duration=-1m"}, "long-break-duration", "positive"},
		"zero interval":     {[]string{"--long-break-interval", "0"}, "long-break-interval", "at least 1"},
		"not an integer":    {[]string{"--long-break-interval", "four"}, "long-break-interval", "integer"},
		"missing value":     {[]string{"--focus-duration"}, "focus-duration", "argument"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseArgs(tc.args, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.flag) || !strings.Contains(err.Error(), tc.why) {
				t.Errorf("err = %v, want one naming -%s and saying %q", err, tc.flag, tc.why)
			}
		})
	}
}

func TestParseArgs_AnUnknownFlagIsRefused(t *testing.T) {
	if _, err := parseArgs([]string{"--focus"}, io.Discard); err == nil {
		t.Error("an unknown flag returned no error")
	}
}

func TestParseArgs_HelpListsEveryFlagWithItsDefault(t *testing.T) {
	var out bytes.Buffer
	_, err := parseArgs([]string{"--help"}, &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	for _, want := range []string{
		"--focus-duration", "--break-duration", "--long-break-duration", "--long-break-interval",
		"--data-dir", "--config", "30m", "10m", "20m", "config.toml",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("usage lacks %q:\n%s", want, out.String())
		}
	}
}

func TestSettingsFor_FlagsBeatTheConfigFileWhichBeatsTheDefaults(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), "focus_duration = \"45m\"\nbreak_duration = \"7m\"\n")
	opts, err := parseArgs([]string{"--config", path, "--focus-duration", "20m"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got, err := opts.settings(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.FocusDuration != 20*time.Minute {
		t.Errorf("focus = %v, want the flag's 20m over the file's 45m", got.FocusDuration)
	}
	if got.BreakDuration != 7*time.Minute {
		t.Errorf("break = %v, want the file's 7m (no flag)", got.BreakDuration)
	}
	if got.LongBreakDuration != config.Defaults().LongBreakDuration {
		t.Errorf("long break = %v, want the default", got.LongBreakDuration)
	}
}

func TestSettingsFor_ABrokenConfigStillStopsStartupWhateverTheFlagsSay(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), `focus_duration = "soon"`)
	opts, err := parseArgs([]string{"--config", path, "--focus-duration", "20m"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opts.settings(env(nil)); err == nil || !strings.Contains(err.Error(), "focus_duration") {
		t.Errorf("err = %v, want the config error", err)
	}
}

func TestRealMain_HelpSucceeds(t *testing.T) {
	if code := realMain([]string{"--help"}, env(nil), io.Discard, io.Discard); code != 0 {
		t.Errorf("realMain(--help) = %d, want 0", code)
	}
}

// The config package lists the themes so that it need not import the UI; this
// keeps that list and the UI's palettes the same.
func TestThemesAreTheDefaultAndEveryBuiltInPalette(t *testing.T) {
	want := append([]string{config.ThemeDefault}, tui.PaletteNames()...)
	if strings.Join(config.Themes, " ") != strings.Join(want, " ") {
		t.Errorf("config.Themes = %v, want %v", config.Themes, want)
	}
	for _, name := range config.Themes {
		if name == config.ThemeDefault {
			continue
		}
		if p, ok := tui.BuiltinPalette(name); !ok || themePalette(name, colorprofile.TrueColor) != p {
			t.Errorf("themePalette(%q) is not the built-in palette", name)
		}
	}
	if themePalette(config.ThemeDefault, colorprofile.TrueColor) != tui.DefaultPalette() {
		t.Error("the default theme is not the default palette")
	}
}

func TestNotifierFor_OnlyDesktopAsksForOneAndAnUnsupportedPlatformShowsNothing(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows", "plan9"} {
		if got := notifierFor(config.NotificationsOff, goos); got != (notify.Nop{}) {
			t.Errorf("off on %s: notifier = %T, want none", goos, got)
		}
	}
	if got := notifierFor(config.NotificationsDesktop, "darwin"); got == (notify.Nop{}) {
		t.Error("desktop on macOS shows nothing")
	} else if _, ok := got.(*notify.MacOS); !ok {
		t.Errorf("desktop on macOS: notifier = %T, want *notify.MacOS", got)
	}
	if got := notifierFor(config.NotificationsDesktop, "linux"); got == (notify.Nop{}) {
		t.Error("desktop on Linux shows nothing")
	} else if _, ok := got.(*notify.Linux); !ok {
		t.Errorf("desktop on Linux: notifier = %T, want *notify.Linux", got)
	}
	if got := notifierFor(config.NotificationsDesktop, "plan9"); got != (notify.Nop{}) {
		t.Errorf("desktop on a platform with none: notifier = %T, want none, and no error", got)
	}
}
