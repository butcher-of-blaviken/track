package core_test

import (
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
)

// reportZone is deliberately not UTC, so the windows are checked in local time.
var reportZone = time.FixedZone("EEST", 3*60*60)

// A Wednesday; the week starts on Monday 28 September.
var reportNow = time.Date(2026, 9, 30, 15, 0, 0, 0, reportZone)

type reportRig struct {
	*rig
}

func newReportRig(t *testing.T) *reportRig {
	t.Helper()
	store := memory.New()
	fake := clock.NewFake(reportNow)
	tracker, err := core.NewTracker(store, fake, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	return &reportRig{&rig{tracker: tracker, store: store, clock: fake}}
}

// task saves an Active Task with the given Tags.
func (r *reportRig) task(t *testing.T, title string, tags ...string) core.TaskID {
	t.Helper()
	var id core.TaskID
	if err := r.store.Update(ctx, func(tx core.Tx) error {
		var err error
		id, err = tx.CreateTask(core.Task{Title: title, State: core.StateActive, Tags: tags, CreatedAt: reportNow})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// session saves a 30m session that started at start. It was stopped after
// stoppedAfter, if that is not zero, and skipped that much Break.
func (r *reportRig) session(t *testing.T, task core.TaskID, start time.Time, stoppedAfter, skipped time.Duration) {
	t.Helper()
	s := core.FocusSession{TaskID: task, StartedAt: start, PlannedDuration: 30 * time.Minute, BreakDuration: 10 * time.Minute, SkippedBreak: skipped}
	if stoppedAfter > 0 {
		stopped := start.Add(stoppedAfter)
		s.StoppedAt = &stopped
	}
	if err := r.store.Update(ctx, func(tx core.Tx) error {
		_, err := tx.CreateSession(s)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 9, day, hour, minute, 0, 0, reportZone)
}

func (r *reportRig) report(t *testing.T, p core.Period) core.Report {
	t.Helper()
	got, err := r.tracker.Report(ctx, p)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	return got
}

func TestReport_TodayIsTheLocalCalendarDayAndTheWeekStartsOnMonday(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "work")
	r.session(t, id, at(30, 9, 0), 0, 0) // today, 30m
	r.session(t, id, at(29, 9, 0), 0, 0) // yesterday (Tuesday), 30m
	r.session(t, id, at(28, 9, 0), 0, 0) // Monday, 30m
	r.session(t, id, at(27, 9, 0), 0, 0) // Sunday: last week
	r.session(t, id, at(1, 9, 0), 0, 0)  // long ago

	today, week := r.report(t, core.PeriodToday), r.report(t, core.PeriodWeek)
	if today.Focused != 30*time.Minute || today.Sessions != 1 {
		t.Errorf("today = %v over %d, want 30m over 1", today.Focused, today.Sessions)
	}
	if week.Focused != 90*time.Minute || week.Sessions != 3 {
		t.Errorf("week = %v over %d, want 1h30m over 3", week.Focused, week.Sessions)
	}
	if !today.From.Equal(at(30, 0, 0)) || !today.To.Equal(at(31, 0, 0)) {
		t.Errorf("today window = %v to %v", today.From, today.To)
	}
	if !week.From.Equal(at(28, 0, 0)) || !week.To.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, reportZone)) {
		t.Errorf("week window = %v to %v, want Monday 28 Sep to Monday 5 Oct", week.From, week.To)
	}
}

func TestReport_OnAMondayTheWeekIsJustThatDay(t *testing.T) {
	r := newReportRig(t)
	r.clock.Set(at(28, 12, 0))
	id := r.task(t, "work")
	r.session(t, id, at(28, 9, 0), 0, 0)
	r.session(t, id, at(27, 9, 0), 0, 0) // Sunday
	if got := r.report(t, core.PeriodWeek); got.Focused != 30*time.Minute {
		t.Errorf("week on a Monday = %v, want 30m", got.Focused)
	}
}

func TestReport_ClipsASessionThatCrossesMidnight(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "late night")
	r.session(t, id, at(29, 23, 50), 0, 0) // 23:50 to 00:20

	if got := r.report(t, core.PeriodToday); got.Focused != 20*time.Minute {
		t.Errorf("today = %v, want the 20m after midnight", got.Focused)
	}
	r.clock.Set(at(29, 23, 55))
	if got := r.report(t, core.PeriodToday); got.Focused != 5*time.Minute {
		t.Errorf("yesterday's view of it = %v, want the 5m so far", got.Focused)
	}
}

func TestReport_ClipsASessionThatCrossesTheWeekStart(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "sunday night")
	r.session(t, id, at(27, 23, 45), 0, 0) // Sunday 23:45 to Monday 00:15
	if got := r.report(t, core.PeriodWeek); got.Focused != 15*time.Minute {
		t.Errorf("week = %v, want the 15m after Monday began", got.Focused)
	}
}

func TestReport_CountsStoppedEarlyAndRunningSessionsAndCapsTheRunningOne(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "mixed")
	r.session(t, id, at(30, 9, 0), 12*time.Minute, 0) // stopped after 12m
	r.session(t, id, at(30, 14, 50), 0, 0)            // running, 10m so far
	if got := r.report(t, core.PeriodToday); got.Focused != 22*time.Minute || got.Sessions != 2 {
		t.Errorf("got %v over %d, want 22m over 2", got.Focused, got.Sessions)
	}

	r.clock.Set(at(30, 20, 0)) // the running session is long over: capped at 30m
	if got := r.report(t, core.PeriodToday); got.Focused != 42*time.Minute {
		t.Errorf("got %v, want 12m + 30m", got.Focused)
	}
}

func TestReport_SumsPerTaskLargestFirstWithTiesByID(t *testing.T) {
	r := newReportRig(t)
	small := r.task(t, "small")
	big := r.task(t, "big")
	tieA := r.task(t, "tie a")
	tieB := r.task(t, "tie b")
	r.session(t, small, at(30, 8, 0), 10*time.Minute, 0)
	r.session(t, big, at(30, 9, 0), 0, 0)
	r.session(t, big, at(30, 10, 0), 0, 0)
	r.session(t, tieB, at(30, 11, 0), 20*time.Minute, 0)
	r.session(t, tieA, at(30, 12, 0), 20*time.Minute, 0)

	got := r.report(t, core.PeriodToday)
	var titles []string
	for _, tt := range got.Tasks {
		titles = append(titles, tt.Task.Title)
	}
	want := []string{"big", "tie a", "tie b", "small"}
	if len(titles) != len(want) {
		t.Fatalf("tasks = %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("tasks = %v, want %v", titles, want)
		}
	}
	if got.Tasks[0].Focused != 60*time.Minute {
		t.Errorf("big = %v, want 1h", got.Tasks[0].Focused)
	}
}

func TestReport_SumsPerTagCountingATaskInEachOfItsTagsWithAnUntaggedRow(t *testing.T) {
	r := newReportRig(t)
	both := r.task(t, "both", "docs", "Backend")
	docsOnly := r.task(t, "docs only", "docs")
	bare := r.task(t, "bare")
	r.session(t, both, at(30, 9, 0), 20*time.Minute, 0)
	r.session(t, docsOnly, at(30, 10, 0), 10*time.Minute, 0)
	r.session(t, bare, at(30, 11, 0), 5*time.Minute, 0)

	got := r.report(t, core.PeriodToday)
	if got.Focused != 35*time.Minute {
		t.Errorf("total = %v, want 35m (tag rows may add up to more)", got.Focused)
	}
	want := []core.TagTime{
		{Tag: "docs", Focused: 30 * time.Minute},
		{Tag: "Backend", Focused: 20 * time.Minute},
		{Untagged: true, Focused: 5 * time.Minute},
	}
	if len(got.Tags) != len(want) {
		t.Fatalf("tags = %+v, want %+v", got.Tags, want)
	}
	for i := range want {
		if got.Tags[i] != want[i] {
			t.Errorf("tags[%d] = %+v, want %+v", i, got.Tags[i], want[i])
		}
	}
}

func TestReport_HasNoUntaggedRowWhenEverythingIsTagged(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "tagged", "docs")
	r.session(t, id, at(30, 9, 0), 0, 0)
	for _, tag := range r.report(t, core.PeriodToday).Tags {
		if tag.Untagged {
			t.Errorf("unexpected untagged row: %+v", tag)
		}
	}
}

func TestReport_CountsBreakOverridesByWhenTheySessionStarted(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "rushed")
	r.session(t, id, at(30, 9, 0), 0, 4*time.Minute) // override today
	r.session(t, id, at(30, 11, 0), 0, 0)            // no override
	r.session(t, id, at(29, 9, 0), 0, 6*time.Minute) // override yesterday
	r.session(t, id, at(20, 9, 0), 0, 6*time.Minute) // override before this week

	if got := r.report(t, core.PeriodToday); got.Overrides != 1 {
		t.Errorf("today overrides = %d, want 1", got.Overrides)
	}
	if got := r.report(t, core.PeriodWeek); got.Overrides != 2 {
		t.Errorf("week overrides = %d, want 2", got.Overrides)
	}
}

func TestReport_IncludesDoneAndArchivedTasksAndIsEmptyWithNoSessions(t *testing.T) {
	r := newReportRig(t)
	empty := r.report(t, core.PeriodToday)
	if empty.Focused != 0 || empty.Sessions != 0 || empty.Overrides != 0 || len(empty.Tasks) != 0 || len(empty.Tags) != 0 {
		t.Errorf("empty report = %+v, want zeros", empty)
	}

	var done core.TaskID
	if err := r.store.Update(ctx, func(tx core.Tx) error {
		var err error
		done, err = tx.CreateTask(core.Task{Title: "finished", State: core.StateDone, CreatedAt: reportNow})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r.session(t, done, at(30, 9, 0), 0, 0)
	got := r.report(t, core.PeriodToday)
	if len(got.Tasks) != 1 || got.Tasks[0].Task.State != core.StateDone {
		t.Errorf("tasks = %+v, want the Done Task", got.Tasks)
	}
}
