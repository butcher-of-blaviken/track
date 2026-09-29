package main

import (
	"testing"

	"github.com/butcher-of-blaviken/track/internal/clock"
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
