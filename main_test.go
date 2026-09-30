package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/config"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
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
