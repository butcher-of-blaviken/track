//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
func seedRunningSession(t *testing.T, dir string) { seedRunningSessionOf(t, dir, 30*time.Minute) }

// seedRunningSessionOf is seedRunningSession with a chosen planned duration.
func seedRunningSessionOf(t *testing.T, dir string, planned time.Duration) {
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
	if _, err := tracker.StartSession(ctx, task, planned, core.StartOptions{}); err != nil {
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
	t.Parallel()
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

	skipHandoff(tm) // the hand-off prompt has the keyboard; skip it to quit with q
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestAddATaskFromTheKeyboardAndItSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)

	tm.press("a")
	tm.waitFor(`Add task`, wait)
	tm.typeText("finishing up auth ##PROJ-123")
	tm.press("Enter")

	screen := tm.waitFor(`finishing up auth\s+#PROJ-123`, wait)
	if strings.Contains(screen, "##") {
		t.Errorf("the tag should show as #PROJ-123, not with the ## marker:\n%s", screen)
	}
	if strings.Contains(screen, "Add task") {
		t.Errorf("the prompt should close after adding:\n%s", screen)
	}

	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}

	again := launch(t, nil, "--data-dir", dir)
	again.waitFor(`finishing up auth\s+#PROJ-123`, wait)
}

// addTaskByKeyboard adds a Task through the prompt and waits for it to show up.
func addTaskByKeyboard(tm *term, text, expect string) {
	tm.t.Helper()
	tm.press("a")
	tm.waitFor(`Add task`, wait)
	tm.typeText(text)
	tm.press("Enter")
	tm.waitFor(expect, wait)
}

func TestStartAndStopASessionFromTheKeyboard(t *testing.T) {
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "write the PRD", `write the PRD`)

	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+write the PRD`, wait)

	tm.press("s") // a second start while one is running is refused, with a notice
	tm.waitFor(`already running`, wait)

	tm.press("x")
	stopped := tm.waitFor(`Idle`, wait)
	if !strings.Contains(stopped, "Hand-off due") {
		t.Errorf("stopping early should leave the hand-off due:\n%s", stopped)
	}
	if strings.Contains(stopped, "Break") {
		t.Errorf("stopping early should not start a Break:\n%s", stopped)
	}

	skipHandoff(tm) // the hand-off prompt has the keyboard; skip it to quit with q
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestAStartedSessionRunsIntoABreakWithTheHandoffDue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir) // 30m passes in ~15s
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "long haul", `long haul`)
	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+long haul`, wait)

	onBreak := tm.waitFor(`Break\s+\d+:\d\d`, 40*time.Second)
	if !strings.Contains(onBreak, "Hand-off due") {
		t.Errorf("the Break screen does not show the hand-off due:\n%s", onBreak)
	}
	skipHandoff(tm) // the hand-off prompt has the keyboard; skip it to quit with q
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestBellsRepeatAndBannersMarkTheEndOfASessionAndItsBreak(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedRunningSession(t, dir)
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir) // 30m passes in ~15s
	tm.record()

	tm.waitFor(`Focus\s+\d\d:\d\d`, wait)
	if n := tm.bells(); n != 0 {
		t.Errorf("rang %d bells while just focusing, want 0", n)
	}

	tm.waitFor(`Focus complete`, 40*time.Second)
	tm.waitForBells(1, wait)
	tm.waitForBells(2, wait) // repeats while nobody presses a key

	onBreakEnd := tm.waitFor(`Break over`, 40*time.Second)
	if strings.Contains(onBreakEnd, "Focus complete") {
		t.Errorf("the session-end banner should give way to the Break-over one:\n%s", onBreakEnd)
	}
	before := tm.bells()
	tm.waitForBells(before+1, wait) // the Break-over bell is a new event, so it rings again
}

func TestAKeyPressSilencesTheBellAndDismissesTheBanner(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedRunningSession(t, dir)
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir)
	tm.record()

	tm.waitFor(`Focus complete`, 40*time.Second)
	tm.waitForBells(1, wait)

	tm.press("j") // any key
	tm.waitUntil("the banner to go", wait, func(s string) bool { return !strings.Contains(s, "Focus complete") })
	time.Sleep(300 * time.Millisecond) // let a refresh already in flight land
	silenced := tm.bells()

	// Well past the point where the next ring would have been due.
	time.Sleep(3 * time.Second)
	if n := tm.bells(); n != silenced {
		t.Errorf("rang %d more bells after the key press, want none", n-silenced)
	}
	if s := tm.screen(); !strings.Contains(s, "Hand-off due") {
		t.Errorf("the lasting state should remain after the banner goes:\n%s", s)
	}
}

// storedNotes reads the notes in the database, as another process would.
func storedNotes(t *testing.T, dir string) []core.Note {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	notes, err := store.Notes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func TestTheHandoffPromptOpensWhenASessionEndsAndSavesANote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedRunningSession(t, dir)
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir) // 30m passes in ~15s

	tm.waitFor(`Focus\s+\d\d:\d\d`, wait)
	tm.waitFor(`Hand-off for: seeded`, 40*time.Second)

	tm.typeText("left off at the parser")
	tm.press("Enter")
	tm.waitUntil("the prompt and the due marker to go", wait, func(s string) bool {
		return !strings.Contains(s, "Hand-off for") && !strings.Contains(s, "Hand-off due")
	})

	notes := storedNotes(t, dir)
	if len(notes) != 1 || notes[0].Text != "left off at the parser" || notes[0].TaskID == 0 {
		t.Errorf("stored notes = %+v, want one filed note with the typed text", notes)
	}

	tm.press("q") // the keyboard is back on the list
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestEscSkipsTheHandoffAfterAnEarlyStop(t *testing.T) {
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "write the PRD", `write the PRD`)
	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+write the PRD`, wait)

	tm.press("x")
	tm.waitFor(`Hand-off for: write the PRD`, wait)

	tm.press("Escape")
	tm.waitUntil("the prompt and the due marker to go", wait, func(s string) bool {
		return strings.Contains(s, "Idle") && !strings.Contains(s, "Hand-off for") && !strings.Contains(s, "Hand-off due")
	})
	if notes := storedNotes(t, dir); len(notes) != 0 {
		t.Errorf("stored notes = %+v after skipping, want none", notes)
	}

	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

// skipHandoff presses Esc on the hand-off prompt and waits for it to close. The
// wait matters: an Esc followed at once by another key is read as Alt+key.
func skipHandoff(tm *term) {
	tm.t.Helper()
	tm.press("Escape")
	tm.waitUntil("the hand-off prompt to close", wait, func(s string) bool { return !strings.Contains(s, "Hand-off for") })
}

// TestFullCycleFromTheKeyboard is the whole product loop through the real
// binary with nothing seeded: add a Task, focus, be told it is over, leave a
// hand-off note, take the Break, start again, and find it all still there after
// a restart that offers to resume. At 120x a 30m session takes ~15s and its 10m Break ~5s.
func TestFullCycleFromTheKeyboard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir)
	tm.record()
	tm.waitFor(`No tasks`, wait)

	// Add a Task and start focusing on it.
	addTaskByKeyboard(tm, "write the PRD ##docs", `write the PRD\s+#docs`)
	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+write the PRD`, wait)
	if n := tm.bells(); n != 0 {
		t.Errorf("rang %d bells while just focusing, want 0", n)
	}

	// The session ends: the banner, the bell and the hand-off prompt arrive together.
	ended := tm.waitFor(`Hand-off for: write the PRD`, 40*time.Second)
	if !strings.Contains(ended, "Focus complete") {
		t.Errorf("the session-end banner should show with the prompt:\n%s", ended)
	}
	tm.waitForBells(1, wait)

	// Leave the note; the prompt and the due marker go and the Break carries on.
	tm.typeText("left off at the parser")
	tm.press("Enter")
	onBreak := tm.waitUntil("the Break with the prompt and due marker gone", wait, func(s string) bool {
		return seconds(s, "Break") >= 0 && !strings.Contains(s, "Hand-off for") && !strings.Contains(s, "Hand-off due")
	})
	if strings.Contains(onBreak, "Focus complete") {
		t.Errorf("typing should have silenced the session-end banner:\n%s", onBreak)
	}

	// The Break ends and says so.
	over := tm.waitFor(`Break over`, 40*time.Second)
	if !strings.Contains(over, "Idle") || strings.Contains(over, "Hand-off") {
		t.Errorf("after the Break the app should be Idle with nothing due:\n%s", over)
	}

	// Any key silences it, and Enter starts the next session.
	tm.press("j")
	tm.waitUntil("the banner to go", wait, func(s string) bool { return !strings.Contains(s, "Break over") })
	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+write the PRD`, wait)

	// Stop it early, skip its hand-off, and quit.
	tm.press("x")
	tm.waitFor(`Hand-off for: write the PRD`, wait)
	skipHandoff(tm)
	tm.waitFor(`Idle`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}

	// Everything survives a restart: the Task with its tag, and just the one note.
	again := launch(t, nil, "--data-dir", dir)
	again.waitFor(`write the PRD\s+#docs`, wait)

	// The restart offers to resume, with the note from the earlier session, and
	// yes starts a session on that Task.
	again.waitFor(`Resume "write the PRD`, wait)
	again.waitFor(`Last note .*left off at the parser`, wait)
	again.press("y")
	again.waitFor(`Focus\s+\d\d:\d\d\s+write the PRD`, wait)
	if notes := storedNotes(t, dir); len(notes) != 1 || notes[0].Text != "left off at the parser" || notes[0].TaskID == 0 || notes[0].SessionID == 0 {
		t.Errorf("stored notes = %+v, want the one typed note, filed and linked to its session", notes)
	}
}

// storedSessions reads the sessions in the database, as another process would.
func storedSessions(t *testing.T, dir string) []core.FocusSession {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	sessions, err := store.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

// TestStartingDuringABreakAsksFirstAndRecordsTheOverride declines once, then
// confirms. The 10m Break lasts ~5s at 120x, so the keys go in without pauses.
func TestStartingDuringABreakAsksFirstAndRecordsTheOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, []string{"TRACK_TIME_SCALE=120"}, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "long haul", `long haul`)
	tm.press("Enter")
	tm.waitFor(`Break\s+\d+:\d\d`, 40*time.Second)
	skipHandoff(tm)

	// Declining keeps the Break.
	tm.press("Enter")
	tm.waitFor(`Start anyway`, wait)
	tm.press("n")
	tm.waitUntil("the confirmation to close", wait, func(s string) bool {
		return !strings.Contains(s, "Start anyway") && seconds(s, "Break") >= 0
	})

	// Confirming starts a session.
	tm.press("Enter")
	tm.waitFor(`Start anyway`, wait)
	tm.press("y")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+long haul`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}

	sessions := storedSessions(t, dir)
	if len(sessions) != 2 || sessions[1].SkippedBreak <= 0 {
		t.Errorf("stored sessions = %+v, want a second one that skipped some Break", sessions)
	}
}

func TestASessionRecordedWithAnotherDurationShowsAMismatchNotice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedRunningSessionOf(t, dir, 45*time.Minute)
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`This session is 45m; the current setting is 30m`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}
