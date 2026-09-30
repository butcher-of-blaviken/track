package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestTaskDetail_SumsFocusedTimeOverCompletedStoppedAndRunningSessions(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateActive)

	r.completeSession(t, id) // 30m, then its Break

	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(12 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil { // 12m, stopped early: no Break
		t.Fatal(err)
	}

	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(5 * time.Minute) // running: 5m so far

	d, err := r.tracker.TaskDetail(ctx, id)
	if err != nil {
		t.Fatalf("TaskDetail: %v", err)
	}
	if want := (30 + 12 + 5) * time.Minute; d.Focused != want || d.Sessions != 3 {
		t.Errorf("focused %v over %d sessions, want %v over 3", d.Focused, d.Sessions, want)
	}
}

func TestTaskDetail_ARunningSessionCountsOnlyUpToItsPlannedEnd(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(3 * time.Hour)
	d, err := r.tracker.TaskDetail(ctx, id)
	if err != nil || d.Focused != 30*time.Minute {
		t.Errorf("focused %v, %v; want 30m", d.Focused, err)
	}
}

func TestTaskDetail_LeavesOutOtherTasksSessions(t *testing.T) {
	r := newRig(t)
	mine := r.addTask(t, core.StateActive)
	other := r.addTask(t, core.StateActive)
	r.completeSession(t, other)

	d, err := r.tracker.TaskDetail(ctx, mine)
	if err != nil || d.Focused != 0 || d.Sessions != 0 {
		t.Errorf("detail = %+v, %v; want no focus time", d, err)
	}
}

func TestTaskDetail_ReturnsTheTaskAndItsLogOldestFirst(t *testing.T) {
	r := newRig(t)
	task, err := r.tracker.AddTask(ctx, "write the PRD ##docs")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := r.tracker.AddNote(ctx, task.ID, "first")
	r.clock.Advance(time.Minute)
	second, _ := r.tracker.AddNote(ctx, task.ID, "second")
	if _, err := r.tracker.AddUnfiledNote(ctx, "not mine"); err != nil {
		t.Fatal(err)
	}

	d, err := r.tracker.TaskDetail(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Task.Title != "write the PRD" || len(d.Task.Tags) != 1 {
		t.Errorf("task = %+v", d.Task)
	}
	if got := ids(d.Notes); len(got) != 2 || got[0] != first.ID || got[1] != second.ID {
		t.Errorf("note ids = %v, want [%d %d]", got, first.ID, second.ID)
	}
}

func TestTaskDetail_WorksForDoneTasksAndRejectsUnknownOnes(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateDone)
	if d, err := r.tracker.TaskDetail(ctx, id); err != nil || d.Task.State != core.StateDone {
		t.Errorf("done task: %+v, %v", d, err)
	}
	if _, err := r.tracker.TaskDetail(ctx, 99); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown task: err = %v, want ErrNotFound", err)
	}
}

func TestTaskDetail_ListsTheTasksSessionsNewestFirstAndNotOtherTasks(t *testing.T) {
	r := newRig(t)
	id, other := r.addTask(t, core.StateActive), r.addTask(t, core.StateActive)

	first := r.completeSession(t, id) // 30m, then its Break
	r.clock.Advance(time.Hour)
	if _, err := r.tracker.StartSession(ctx, other, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(40 * time.Minute)
	second, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(5 * time.Minute) // still running

	d, err := r.tracker.TaskDetail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.History) != 2 || d.History[0].ID != second.ID || d.History[1].ID != first.ID {
		t.Fatalf("history = %+v, want the running session then the completed one", d.History)
	}
	if !d.At.Equal(r.clock.Now()) {
		t.Errorf("At = %v, want now", d.At)
	}
	if d.History[0].Outcome(d.At) != core.OutcomeRunning || d.History[1].Outcome(d.At) != core.OutcomeCompleted {
		t.Errorf("outcomes at At: %v, %v", d.History[0].Outcome(d.At), d.History[1].Outcome(d.At))
	}
}

func TestTaskDetail_HistoryIsEmptyNotNilForATaskWithNoSessions(t *testing.T) {
	r := newRig(t)
	d, err := r.tracker.TaskDetail(ctx, r.addTask(t, core.StateActive))
	if err != nil || d.History == nil || len(d.History) != 0 {
		t.Errorf("history = %#v, %v", d.History, err)
	}
}
