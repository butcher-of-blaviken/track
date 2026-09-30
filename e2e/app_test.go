//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestThePickerStartsAnExistingTaskAndCreatesANewOneInline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "write the PRD", `write the PRD`)
	addTaskByKeyboard(tm, "fix the build", `fix the build`)

	// Pick an existing Task by a fragment of its title.
	tm.press("C-p")
	tm.waitFor(`Start:`, wait)
	tm.typeText("prd")
	tm.waitUntil("only the match to be listed", wait, func(s string) bool {
		return strings.Contains(s, "write the PRD") && !strings.Contains(s, "fix the build")
	})
	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+write the PRD`, wait)
	tm.press("x")
	tm.waitFor(`Hand-off for: write the PRD`, wait)
	skipHandoff(tm)
	tm.waitFor(`Idle`, wait) // the resume prompt is not offered mid-run

	// Nothing matches, so the picker offers to create the Task and start it.
	tm.press("C-p")
	tm.typeText("zzz brand new ##docs")
	tm.waitFor(`Create "zzz brand new ##docs"`, wait)
	tm.press("Enter")
	tm.waitFor(`Focus\s+\d\d:\d\d\s+zzz brand new`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestSlashFiltersTheListUntilEscClearsIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "write the PRD", `write the PRD`)
	addTaskByKeyboard(tm, "fix the build", `fix the build`)

	tm.press("/")
	tm.waitFor(`Find:`, wait)
	tm.typeText("build")
	tm.press("Enter")
	tm.waitUntil("the filtered list", wait, func(s string) bool {
		return strings.Contains(s, "Filter: build") && strings.Contains(s, "fix the build") && !strings.Contains(s, "write the PRD")
	})

	tm.press("Escape")
	tm.waitUntil("the whole list again", wait, func(s string) bool {
		return !strings.Contains(s, "Filter:") && strings.Contains(s, "write the PRD") && strings.Contains(s, "fix the build")
	})
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

// seedTask creates a Task in the given state, as another process sharing the
// database would.
func seedTask(t *testing.T, dir, title string, state core.State) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Update(context.Background(), func(tx core.Tx) error {
		_, err := tx.CreateTask(core.Task{Title: title, State: state, CreatedAt: time.Now()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsATaskByItsTagAndHashRestrictsToTags(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "write the PRD ##docs", `write the PRD\s+#docs`)
	addTaskByKeyboard(tm, "docs cleanup", `docs cleanup`)
	addTaskByKeyboard(tm, "fix the build", `fix the build`)

	tm.press("/")
	tm.waitFor(`Find:`, wait)
	tm.typeText("#docs")
	tm.waitUntil("only the tagged Task", wait, func(s string) bool {
		return strings.Contains(s, "write the PRD") && !strings.Contains(s, "docs cleanup") && !strings.Contains(s, "fix the build")
	})
	tm.press("Escape")
	tm.waitUntil("the picker to close", wait, func(s string) bool { return !strings.Contains(s, "Find:") })
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestTabTogglesDoneAndArchivedTasksIntoTheList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedTask(t, dir, "shipped it", core.StateDone)
	seedTask(t, dir, "gave up", core.StateArchived)
	seedTask(t, dir, "in progress", core.StateActive)
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitUntil("only the Active Task", wait, func(s string) bool {
		return strings.Contains(s, "in progress") && !strings.Contains(s, "shipped it") && !strings.Contains(s, "gave up")
	})

	tm.press("Tab")
	tm.waitFor(`Showing: all tasks`, wait)
	tm.waitFor(`shipped it \(done\)`, wait)
	tm.waitFor(`gave up \(archived\)`, wait)

	tm.press("Tab")
	tm.waitUntil("Active only again", wait, func(s string) bool {
		return !strings.Contains(s, "Showing:") && !strings.Contains(s, "shipped it")
	})
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

// seedTaskWithNote creates an Active Task with a filed note, as another
// process sharing the database would.
func seedTaskWithNote(t *testing.T, dir, title, note string) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Update(context.Background(), func(tx core.Tx) error {
		id, err := tx.CreateTask(core.Task{Title: title, State: core.StateActive, CreatedAt: time.Now()})
		if err != nil {
			return err
		}
		_, err = tx.CreateNote(core.Note{TaskID: id, Text: note, CreatedAt: time.Now()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsATaskByItsNoteAndShowsTheMatchingLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedTaskWithNote(t, dir, "write the PRD", "left off at the parser")
	seedTaskWithNote(t, dir, "fix the build", "waiting on CI")
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`write the PRD`, wait)
	if s := tm.screen(); strings.Contains(s, "left off at the parser") {
		t.Errorf("a note is shown before searching:\n%s", s)
	}

	tm.press("/")
	tm.waitFor(`Find:`, wait)
	tm.typeText("parser")
	tm.waitUntil("the note line under its Task", wait, func(s string) bool {
		return strings.Contains(s, "write the PRD") && strings.Contains(s, "left off at the parser") && !strings.Contains(s, "fix the build")
	})
	tm.press("Escape")
	tm.waitUntil("the picker to close", wait, func(s string) bool { return !strings.Contains(s, "Find:") })
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

// storedTaskState reads a Task's state from the database, as another process would.
func storedTaskState(t *testing.T, dir, title string) core.State {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	tasks, err := store.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Title == title {
			return task.State
		}
	}
	t.Fatalf("no task titled %q", title)
	return 0
}

func TestMarkATaskDoneReopenItAndArchiveIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedTask(t, dir, "finish me", core.StateActive)
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`finish me`, wait)

	tm.press("d")
	tm.waitFor(`Marked done: finish me`, wait)
	tm.waitUntil("the Task to leave the list", wait, func(s string) bool {
		return !strings.Contains(s, "finish me (done)") && strings.Contains(s, "No tasks")
	})

	tm.press("Tab")
	tm.waitFor(`finish me \(done\)`, wait)
	tm.press("u")
	tm.waitFor(`Reopened: finish me`, wait)
	tm.waitUntil("the marker to go", wait, func(s string) bool { return !strings.Contains(s, "(done)") })

	tm.press("D")
	tm.waitFor(`finish me \(archived\)`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
	if got := storedTaskState(t, dir, "finish me"); got != core.StateArchived {
		t.Errorf("stored state = %v, want Archived", got)
	}
}

// storedNoteState returns where the note with this text is filed: its Task's
// title, or "" if it is still unfiled.
func storedNoteState(t *testing.T, dir, text string) (found bool, taskTitle string) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	notes, err := store.Notes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.Text != text {
			continue
		}
		if n.TaskID == 0 {
			return true, ""
		}
		task, err := store.Task(ctx, n.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		return true, task.Title
	}
	return false, ""
}

func TestCaptureNotesAndFileThemFromTheInbox(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedTask(t, dir, "existing work", core.StateActive)
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`existing work`, wait)

	// Capture one note, then turn it into a Task.
	tm.press("n")
	tm.waitFor(`Note:`, wait)
	tm.typeText("check the retry logic")
	tm.press("Enter")
	tm.waitFor(`Unfiled notes: 1 \(i to file\)`, wait)
	tm.press("i")
	tm.waitFor(`check the retry logic`, wait)
	tm.press("c")
	tm.waitFor(`Created task: check the retry logic`, wait)
	tm.waitFor(`Inbox is empty`, wait)
	tm.press("Escape")
	tm.waitUntil("the new Task on the list and no unfiled count", wait, func(s string) bool {
		return strings.Contains(s, "existing work") && !strings.Contains(s, "Inbox is empty") && !strings.Contains(s, "Unfiled notes")
	})

	// Capture another and file it onto the existing Task through the picker.
	tm.press("n")
	tm.waitFor(`Note:`, wait)
	tm.typeText("belongs to existing")
	tm.press("Enter")
	tm.waitFor(`Unfiled notes: 1`, wait)
	tm.press("i")
	tm.waitFor(`belongs to existing`, wait)
	tm.press("f")
	tm.waitFor(`Filing: "belongs to existing"`, wait)
	tm.typeText("existing")
	tm.press("Enter")
	tm.waitFor(`Filed onto: existing work`, wait)
	tm.press("Escape")
	tm.waitUntil("the list without an unfiled count", wait, func(s string) bool {
		return strings.Contains(s, "existing work") && !strings.Contains(s, "Inbox is empty") && !strings.Contains(s, "Unfiled notes")
	})
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}

	if found, title := storedNoteState(t, dir, "check the retry logic"); !found || title != "check the retry logic" {
		t.Errorf("first note filed on %q (found %v), want its own new Task", title, found)
	}
	if found, title := storedNoteState(t, dir, "belongs to existing"); !found || title != "existing work" {
		t.Errorf("second note filed on %q (found %v), want \"existing work\"", title, found)
	}
}

// seedTaskWithHistory creates an Active Task with one completed 30-minute
// session two hours ago and a note, as another process sharing the database would.
func seedTaskWithHistory(t *testing.T, dir, title, note string) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	started := time.Now().Add(-2 * time.Hour)
	if err := store.Update(context.Background(), func(tx core.Tx) error {
		id, err := tx.CreateTask(core.Task{Title: title, State: core.StateActive, CreatedAt: started})
		if err != nil {
			return err
		}
		if _, err = tx.CreateSession(core.FocusSession{TaskID: id, StartedAt: started, PlannedDuration: 30 * time.Minute, BreakDuration: 10 * time.Minute}); err != nil {
			return err
		}
		_, err = tx.CreateNote(core.Note{TaskID: id, Text: note, CreatedAt: started.Add(30 * time.Minute)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheDetailViewShowsFocusedTimeAndTheLogAndTakesANote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedTaskWithHistory(t, dir, "write the PRD", "left off at the parser")
	tm := launch(t, nil, "--data-dir", dir)
	// The session ended long ago with its hand-off pending, then the resume prompt follows.
	tm.waitFor(`Hand-off for: write the PRD`, wait)
	skipHandoff(tm)
	tm.waitFor(`Resume "write the PRD"`, wait)
	tm.press("n")
	tm.waitUntil("the resume prompt to close", wait, func(s string) bool { return !strings.Contains(s, "Resume") })

	tm.press("l")
	tm.waitFor(`Focused: 30m over 1 session`, wait)
	tm.waitFor(`\d{4}-\d\d-\d\d \d\d:\d\d  left off at the parser`, wait)

	tm.press("n")
	tm.waitFor(`Note:`, wait)
	tm.typeText("added from the detail")
	tm.press("Enter")
	tm.waitUntil("the new note above the old one", wait, func(s string) bool {
		a, b := strings.Index(s, "added from the detail"), strings.Index(s, "left off at the parser")
		return a >= 0 && b >= 0 && a < b
	})
	tm.press("Escape")
	tm.waitUntil("the list", wait, func(s string) bool { return !strings.Contains(s, "Focused:") })
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
	if found, title := storedNoteState(t, dir, "added from the detail"); !found || title != "write the PRD" {
		t.Errorf("note filed on %q (found %v), want \"write the PRD\"", title, found)
	}
}

// seedRecentWork creates a Task with the given Tags and a session that started
// minutes ago, as another process sharing the database would. override marks
// the session as having skipped part of a Break.
func seedRecentWork(t *testing.T, dir, title string, tags []string, override bool) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	started := time.Now().Add(-10 * time.Minute)
	stopped := started.Add(5 * time.Minute)
	var skipped time.Duration
	if override {
		skipped = 3 * time.Minute
	}
	if err := store.Update(context.Background(), func(tx core.Tx) error {
		id, err := tx.CreateTask(core.Task{Title: title, State: core.StateActive, Tags: tags, CreatedAt: started})
		if err != nil {
			return err
		}
		_, err = tx.CreateSession(core.FocusSession{TaskID: id, StartedAt: started, PlannedDuration: 30 * time.Minute, StoppedAt: &stopped, BreakDuration: 10 * time.Minute, SkippedBreak: skipped})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheReportShowsTimePerTaskAndTagAndBreakOverrides(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedRecentWork(t, dir, "write the PRD", []string{"docs"}, true)
	seedRecentWork(t, dir, "fix the build", nil, false)
	tm := launch(t, nil, "--data-dir", dir)
	// Both sessions stopped early, so hand-off prompts and the resume prompt come first.
	tm.waitFor(`Hand-off for`, wait)
	skipHandoff(tm)
	tm.waitUntil("the resume prompt or the list", wait, func(s string) bool { return strings.Contains(s, "Resume") || strings.Contains(s, "r report") })
	if strings.Contains(tm.screen(), "Resume") {
		tm.press("n")
		tm.waitUntil("the prompt to close", wait, func(s string) bool { return !strings.Contains(s, "Resume") })
	}

	tm.press("r")
	tm.waitFor(`Report: Today`, wait)
	tm.waitFor(`Focused: 10m over 2 sessions`, wait)
	tm.waitFor(`Break overrides: 1`, wait)
	tm.waitFor(`By task`, wait)
	tm.waitFor(`By tag`, wait)
	tm.waitFor(`#docs`, wait)
	tm.waitFor(`\(untagged\)`, wait)

	tm.press("Tab")
	tm.waitFor(`Report: This week`, wait)
	tm.press("Escape")
	tm.waitUntil("the list", wait, func(s string) bool { return !strings.Contains(s, "Report:") })
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestQuestionMarkTogglesTheFullHelpInEachView(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedTaskWithHistory(t, dir, "write the PRD", "left off")
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`Hand-off for`, wait)
	skipHandoff(tm)
	tm.waitFor(`Resume "write the PRD"`, wait)
	tm.press("n")
	tm.waitUntil("the resume prompt to close", wait, func(s string) bool { return !strings.Contains(s, "Resume") })

	// The short footer is short; the full help lists the keys it leaves out.
	tm.waitFor(`\? help`, wait)
	if s := tm.screen(); strings.Contains(s, "archive") {
		t.Errorf("the short footer lists the archive key:\n%s", s)
	}
	tm.press("?")
	tm.waitFor(`D\s+archive`, wait)
	tm.waitFor(`u\s+reopen`, wait)
	tm.waitFor(`ctrl\+p\s+pick`, wait)
	tm.press("?")
	tm.waitUntil("the help to close", wait, func(s string) bool { return !strings.Contains(s, "archive") })

	// Each view has its own keys in its help, and leaving the view closes it.
	tm.press("l")
	tm.waitFor(`Focused:`, wait)
	tm.press("?")
	tm.waitFor(`n {2,}note`, wait) // only the full help pads its columns; the short footer has "n note"
	tm.press("Escape")             // closes the help first
	tm.waitUntil("the help closed but still in the detail", wait, func(s string) bool {
		return strings.Contains(s, "Focused:") && strings.Contains(s, "n note • j/k scroll")
	})
	tm.press("Escape")
	tm.waitUntil("the list", wait, func(s string) bool { return !strings.Contains(s, "Focused:") })
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheConfigFileSetsTheFocusDuration(t *testing.T) {
	t.Parallel()
	cfg := writeConfig(t, "focus_duration = \"20m\"\n")
	tm := launch(t, nil, "--data-dir", t.TempDir(), "--config", cfg)
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "configured work", `configured work`)
	tm.press("Enter")
	tm.waitFor(`Focus\s+(20:00|19:5\d)\s+configured work`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestABrokenConfigStopsTheAppWithAClearMessage(t *testing.T) {
	t.Parallel()
	cfg := writeConfig(t, "focus_duration = \"soon\"\n")
	tm := launch(t, nil, "--data-dir", t.TempDir(), "--config", cfg)
	if status := tm.exitStatus(wait); status == 0 {
		t.Error("exit status = 0, want a failure")
	}
	s := tm.screen()
	for _, want := range []string{"config", "focus_duration", "not a duration"} { // the path wraps in an 80-column pane
		if !strings.Contains(s, want) {
			t.Errorf("the message lacks %q:\n%s", want, s)
		}
	}
}

func TestAMissingNamedConfigFileIsAnError(t *testing.T) {
	t.Parallel()
	tm := launch(t, nil, "--data-dir", t.TempDir(), "--config", filepath.Join(t.TempDir(), "nope.toml"))
	if status := tm.exitStatus(wait); status == 0 {
		t.Error("exit status = 0, want a failure")
	}
}

func TestTheFocusDurationFlagSetsTheCountdown(t *testing.T) {
	t.Parallel()
	tm := launch(t, nil, "--data-dir", t.TempDir(), "--focus-duration", "20m")
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "flagged work", `flagged work`)
	tm.press("Enter")
	tm.waitFor(`Focus\s+(20:00|19:5\d)\s+flagged work`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestAFlagBeatsTheConfigFile(t *testing.T) {
	t.Parallel()
	cfg := writeConfig(t, "focus_duration = \"45m\"\n")
	tm := launch(t, nil, "--data-dir", t.TempDir(), "--config", cfg, "--focus-duration", "20m")
	tm.waitFor(`No tasks`, wait)
	addTaskByKeyboard(tm, "flagged work", `flagged work`)
	tm.press("Enter")
	tm.waitFor(`Focus\s+(20:00|19:5\d)\s+flagged work`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestARunningSessionFromAnotherSettingShowsTheMismatchNotice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedRunningSessionOf(t, dir, 45*time.Minute)
	tm := launch(t, nil, "--data-dir", dir, "--focus-duration", "20m")
	tm.waitFor(`This session is 45m; the current setting is 20m`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestABadFlagValueStopsTheAppNamingTheFlag(t *testing.T) {
	t.Parallel()
	// Run without a terminal: the message is longer than an 80-column pane.
	code, _, stderr := runTrack(t, "--data-dir", t.TempDir(), "--focus-duration", "soon")
	if code != 2 {
		t.Errorf("exit status = %d, want 2", code)
	}
	for _, want := range []string{"focus-duration", "not a duration"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the message lacks %q:\n%s", want, stderr)
		}
	}
}

func TestHelpListsTheFlagsAndExitsCleanly(t *testing.T) {
	t.Parallel()
	tm := launch(t, nil, "--help")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
	s := tm.screen()
	for _, want := range []string{"--focus-duration", "--break-duration", "--long-break-duration", "--long-break-interval"} {
		if !strings.Contains(s, want) {
			t.Errorf("the help lacks %q:\n%s", want, s)
		}
	}
}

// runTrack runs the real binary without a terminal, for the subcommands, and
// returns its exit code and what it printed.
func runTrack(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatalf("running track: %v", err)
	}
	return code, out.String(), errOut.String()
}

func TestAddAndNoteFromTheCommandLineAppearInTheApp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if code, stdout, stderr := runTrack(t, "--data-dir", dir, "add", "write the PRD ##docs"); code != 0 || !strings.Contains(stdout, "Added task 1: write the PRD  #docs") {
		t.Fatalf("add: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runTrack(t, "--data-dir", dir, "note", "check the retry logic"); code != 0 || !strings.Contains(stdout, "Saved note 1") {
		t.Fatalf("note: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`write the PRD\s+#docs`, wait)
	tm.waitFor(`Unfiled notes: 1`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestSubcommandExitCodes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		args []string
		want int
	}{
		"add without text":  {[]string{"add"}, 2},
		"note without text": {[]string{"note"}, 2},
		"unknown command":   {[]string{"frobnicate"}, 2},
		"tags-only add":     {[]string{"add", "##docs"}, 1},
		"blank note":        {[]string{"note", " "}, 1},
		"help":              {[]string{"--help"}, 0},
	} {
		if code, _, _ := runTrack(t, append([]string{"--data-dir", dir}, tc.args...)...); code != tc.want {
			t.Errorf("%s: exit %d, want %d", name, code, tc.want)
		}
	}
}

func TestExportWorksWhileTheAppIsRunningASession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runTrack(t, "--data-dir", dir, "add", "write the PRD ##docs")
	runTrack(t, "--data-dir", dir, "note", "check the retry logic")

	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`write the PRD`, wait)
	tm.press("s")
	tm.waitFor(`29:5\d|30:00`, wait)

	code, stdout, stderr := runTrack(t, "--data-dir", dir, "export")
	if code != 0 {
		t.Fatalf("export JSON: exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{`"title": "write the PRD"`, `"outcome": "running"`, `"text": "check the retry logic"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("JSON lacks %s:\n%s", want, stdout)
		}
	}

	out := filepath.Join(t.TempDir(), "track.md")
	if code, _, stderr := runTrack(t, "--data-dir", dir, "export", "--format", "markdown", "--output", out); code != 0 {
		t.Fatalf("export Markdown: exit %d, stderr %q", code, stderr)
	}
	md, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## write the PRD", "Tags: #docs", "## Inbox", "> check the retry logic"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("Markdown lacks %q:\n%s", want, md)
		}
	}

	// The running app is undisturbed.
	tm.waitFor(`29:\d\d|30:00`, wait)
}

func TestARunningAppPicksUpWhatTheCommandLineWritesOnItsNextTick(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`No tasks`, wait)

	// No key is pressed from here on: only the app's own tick can show these.
	if code, _, stderr := runTrack(t, "--data-dir", dir, "add", "written while open ##live"); code != 0 {
		t.Fatalf("add: exit %d, stderr %q", code, stderr)
	}
	tm.waitFor(`written while open\s+#live`, wait)

	for i := 1; i <= 2; i++ {
		if code, _, stderr := runTrack(t, "--data-dir", dir, "note", "a note while open"); code != 0 {
			t.Fatalf("note: exit %d, stderr %q", code, stderr)
		}
		tm.waitFor(`Unfiled notes: `+strconv.Itoa(i), wait)
	}

	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

func TestVersionNamesTheBinaryAndItsVersion(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--version"}, {"version"}} {
		code, stdout, stderr := runTrack(t, args...)
		if code != 0 || !regexp.MustCompile(`^track \S+`).MatchString(stdout) || stderr != "" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestTheDocsOpenInTheAppWithHAndCanBeSearched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runTrack(t, "--data-dir", dir, "add", "read the docs")
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`read the docs`, wait)

	tm.press("H")
	tm.waitFor(`# Track`, wait)
	tm.press("/")
	tm.typeText("fish")
	tm.press("Enter")
	tm.waitFor(`Search "fish": 1/\d+`, wait)
	tm.press("n")
	tm.waitFor(`Search "fish": 2/\d+`, wait)

	tm.press("Escape") // clears the search
	tm.waitUntil("the search cleared", wait, func(s string) bool { return !strings.Contains(s, `Search "fish"`) })
	tm.press("Escape") // back to the list
	tm.waitFor(`read the docs`, wait)
	tm.press("q")
	if status := tm.exitStatus(wait); status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
}

var sgrSeq = regexp.MustCompile(`^\x1b\[([0-9;:]*)m`)

// sgrParams splits the parameters of one SGR sequence.
func sgrParams(seq string) []int {
	var out []int
	for _, p := range strings.FieldsFunc(seq, func(r rune) bool { return r == ';' || r == ':' }) {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// sgrAt returns the SGR parameters in force on the first character of text in a
// styled capture (tmux capture-pane -e), following resets the way a terminal does.
func sgrAt(t *testing.T, styled, text string) map[int]bool {
	t.Helper()
	params := map[int]bool{}
	for i := 0; i < len(styled); {
		if m := sgrSeq.FindStringSubmatch(styled[i:]); m != nil {
			nums := sgrParams(m[1])
			if len(nums) == 0 {
				params = map[int]bool{}
			}
			for _, n := range nums {
				if n == 0 {
					params = map[int]bool{}
				} else {
					params[n] = true
				}
			}
			i += len(m[0])
			continue
		}
		if strings.HasPrefix(styled[i:], text) {
			return params
		}
		i++
	}
	t.Fatalf("%q is not on the styled screen:\n%q", text, styled)
	return nil
}

func TestColoursReachARealTerminal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runTrack(t, "--data-dir", dir, "add", "read the docs ##reading")
	runTrack(t, "--data-dir", dir, "note", "a loose thought")
	tm := launch(t, nil, "--data-dir", dir)
	tm.waitFor(`> read the docs`, wait)

	styled := tm.styledScreen()
	if p := sgrAt(t, styled, "> read the docs"); !p[7] || !p[34] {
		t.Errorf("the selected row has SGR %v, want reverse (7) and blue (34):\n%q", p, styled)
	}
	if p := sgrAt(t, styled, "Unfiled notes"); !p[33] {
		t.Errorf("the unfiled-notes line has SGR %v, want yellow (33)", p)
	}
	if p := sgrAt(t, styled, "Idle"); !p[2] {
		t.Errorf("Idle has SGR %v, want faint (2)", p)
	}

	tm.press("s")
	tm.waitFor(`Focus\s+\d\d:\d\d`, wait)
	if p := sgrAt(t, tm.styledScreen(), "Focus"); !p[35] || !p[1] {
		t.Errorf("Focus has SGR %v, want bold (1) magenta (35)", p)
	}
}

func TestNoColorLeavesNoColourCodes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runTrack(t, "--data-dir", dir, "add", "read the docs ##reading")
	runTrack(t, "--data-dir", dir, "note", "a loose thought")
	tm := launch(t, []string{"NO_COLOR=1"}, "--data-dir", dir)
	tm.waitFor(`> read the docs`, wait)
	tm.press("s")
	tm.waitFor(`Focus\s+\d\d:\d\d`, wait)

	styled := tm.styledScreen()
	for _, seq := range regexp.MustCompile(`\x1b\[([0-9;:]*)m`).FindAllStringSubmatch(styled, -1) {
		for _, n := range sgrParams(seq[1]) {
			if (n >= 30 && n <= 38) || (n >= 40 && n <= 48) || (n >= 90 && n <= 97) || (n >= 100 && n <= 107) {
				t.Fatalf("NO_COLOR is set but the screen has colour code %d:\n%q", n, styled)
			}
		}
	}
	// The layout is still readable in plain text.
	plain := tm.screen()
	for _, want := range []string{"> read the docs", "#reading", "Unfiled notes: 1", "Focus"} {
		if !strings.Contains(plain, want) {
			t.Errorf("the plain screen lacks %q:\n%s", want, plain)
		}
	}
}
