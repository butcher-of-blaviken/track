//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/sqlite"
)

const wait = 10 * time.Second

func TestStartsIdleOnAnIsolatedDataDirAndQuitsCleanly(t *testing.T) {
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)

	tm.waitFor(`Idle`, wait)
	if _, err := os.Stat(filepath.Join(dir, "track.db")); err != nil {
		t.Errorf("the database was not created in the isolated data dir: %v", err)
	}

	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

// seedRunningSession creates a Task and starts a 30-minute session on it, as
// another process sharing the same database would.
func seedRunningSession(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	tracker, err := core.NewTracker(store, clock.System{}, core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4})
	if err != nil {
		t.Fatal(err)
	}
	var task core.TaskID
	if err := store.Update(ctx, func(tx core.Tx) error {
		var err error
		task, err = tx.CreateTask(core.Task{Title: "seeded", State: core.StateActive, CreatedAt: time.Now()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
}

var countdown = regexp.MustCompile(`(Focus|Break)\s+(\d+):(\d\d)`)

// seconds returns the countdown shown on screen, or -1 if there is none.
func seconds(screen, label string) int {
	m := countdown.FindStringSubmatch(screen)
	if m == nil || m[1] != label {
		return -1
	}
	mins, _ := strconv.Atoi(m[2])
	secs, _ := strconv.Atoi(m[3])
	return mins*60 + secs
}

func TestShowsTheFocusToBreakToIdleCycleAsTimePasses(t *testing.T) {
	dir := t.TempDir()
	seedRunningSession(t, dir)
	// 120x: the 30m session ends after ~15s and its 10m Break after ~5s more.
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir)

	first := tm.waitUntil("a Focus countdown", wait, func(s string) bool { return seconds(s, "Focus") >= 0 })
	start := seconds(first, "Focus")
	if start > 30*60 {
		t.Errorf("Focus countdown starts at %d s, want at most 30:00", start)
	}

	tm.waitUntil("the Focus countdown to shrink", wait, func(s string) bool {
		n := seconds(s, "Focus")
		return n >= 0 && n < start
	})

	onBreak := tm.waitUntil("the Break, with the hand-off due", 40*time.Second, func(s string) bool {
		return seconds(s, "Break") >= 0
	})
	if !regexp.MustCompile(`Hand-off due`).MatchString(onBreak) {
		t.Errorf("the Break screen does not show the hand-off due:\n%s", onBreak)
	}

	idle := tm.waitFor(`Idle`, 30*time.Second)
	if !regexp.MustCompile(`Hand-off due`).MatchString(idle) {
		t.Errorf("the hand-off should still be due after the Break:\n%s", idle)
	}

	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}
