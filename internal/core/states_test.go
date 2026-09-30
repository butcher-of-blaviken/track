package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func (r *rig) stateOf(t *testing.T, id core.TaskID) core.State {
	t.Helper()
	task, err := r.tracker.Task(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return task.State
}

func TestTaskStates_MarkDoneArchiveAndReopenMoveTheTask(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateActive)

	if task, err := r.tracker.MarkDone(ctx, id); err != nil || task.State != core.StateDone {
		t.Fatalf("MarkDone = %+v, %v; want Done", task, err)
	}
	if got := r.stateOf(t, id); got != core.StateDone {
		t.Errorf("stored state = %v, want Done", got)
	}
	if task, err := r.tracker.ReopenTask(ctx, id); err != nil || task.State != core.StateActive {
		t.Fatalf("ReopenTask = %+v, %v; want Active", task, err)
	}
	if task, err := r.tracker.ArchiveTask(ctx, id); err != nil || task.State != core.StateArchived {
		t.Fatalf("ArchiveTask = %+v, %v; want Archived", task, err)
	}
	if task, err := r.tracker.ReopenTask(ctx, id); err != nil || task.State != core.StateActive {
		t.Fatalf("ReopenTask from Archived = %+v, %v; want Active", task, err)
	}
	// A Done Task can be archived too.
	if _, err := r.tracker.MarkDone(ctx, id); err != nil {
		t.Fatal(err)
	}
	if task, err := r.tracker.ArchiveTask(ctx, id); err != nil || task.State != core.StateArchived {
		t.Fatalf("ArchiveTask from Done = %+v, %v; want Archived", task, err)
	}
}

func TestTaskStates_KeepTitleTagsSessionsAndNotes(t *testing.T) {
	r := newRig(t)
	task, err := r.tracker.AddTask(ctx, "write the PRD ##docs")
	if err != nil {
		t.Fatal(err)
	}
	r.completeSession(t, task.ID)
	if _, err := r.tracker.AddNote(ctx, task.ID, "left off at the parser"); err != nil {
		t.Fatal(err)
	}

	done, err := r.tracker.MarkDone(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Title != "write the PRD" || !slices.Equal(done.Tags, []string{"docs"}) {
		t.Errorf("done = %+v, want the title and Tags kept", done)
	}
	if n := len(r.sessions(t)); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
	if notes, err := r.tracker.TaskNotes(ctx, task.ID); err != nil || len(notes) != 1 {
		t.Errorf("notes = %+v, %v; want the one note", notes, err)
	}
}

func TestTaskStates_InvalidTransitionsChangeNothing(t *testing.T) {
	r := newRig(t)
	active := r.addTask(t, core.StateActive)
	done := r.addTask(t, core.StateDone)
	archived := r.addTask(t, core.StateArchived)

	for name, tc := range map[string]struct {
		do   func(core.TaskID) (core.Task, error)
		id   core.TaskID
		want core.State
	}{
		"done on Done":        {func(id core.TaskID) (core.Task, error) { return r.tracker.MarkDone(ctx, id) }, done, core.StateDone},
		"done on Archived":    {func(id core.TaskID) (core.Task, error) { return r.tracker.MarkDone(ctx, id) }, archived, core.StateArchived},
		"archive on Archived": {func(id core.TaskID) (core.Task, error) { return r.tracker.ArchiveTask(ctx, id) }, archived, core.StateArchived},
		"reopen on Active":    {func(id core.TaskID) (core.Task, error) { return r.tracker.ReopenTask(ctx, id) }, active, core.StateActive},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.do(tc.id); !errors.Is(err, core.ErrInvalidTransition) {
				t.Errorf("err = %v, want ErrInvalidTransition", err)
			}
			if got := r.stateOf(t, tc.id); got != tc.want {
				t.Errorf("state = %v, want it left as %v", got, tc.want)
			}
		})
	}
}

func TestTaskStates_AnUnknownTaskIsNotFound(t *testing.T) {
	r := newRig(t)
	for name, do := range map[string]func() error{
		"done":    func() error { _, err := r.tracker.MarkDone(ctx, 99); return err },
		"archive": func() error { _, err := r.tracker.ArchiveTask(ctx, 99); return err },
		"reopen":  func() error { _, err := r.tracker.ReopenTask(ctx, 99); return err },
	} {
		if err := do(); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestTaskStates_ATaskWithARunningSessionCannotBeFinishedOrArchived(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(10 * time.Minute)

	if _, err := r.tracker.MarkDone(ctx, id); !errors.Is(err, core.ErrSessionRunning) {
		t.Errorf("MarkDone err = %v, want ErrSessionRunning", err)
	}
	if _, err := r.tracker.ArchiveTask(ctx, id); !errors.Is(err, core.ErrSessionRunning) {
		t.Errorf("ArchiveTask err = %v, want ErrSessionRunning", err)
	}
	if got := r.stateOf(t, id); got != core.StateActive {
		t.Errorf("state = %v, want it left Active", got)
	}
}

func TestTaskStates_AllowedOnceTheSessionEndedOrForAnotherTask(t *testing.T) {
	t.Run("stopped early", func(t *testing.T) {
		r := newRig(t)
		id := r.addTask(t, core.StateActive)
		if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(10 * time.Minute)
		if _, err := r.tracker.StopSession(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := r.tracker.MarkDone(ctx, id); err != nil {
			t.Errorf("MarkDone after stopping: %v", err)
		}
		// The hand-off is still due and can still be written.
		if _, err := r.tracker.AddHandoffNote(ctx, "wrapped up"); err != nil {
			t.Errorf("AddHandoffNote on a Done Task: %v", err)
		}
	})

	t.Run("completed", func(t *testing.T) {
		r := newRig(t)
		id := r.addTask(t, core.StateActive)
		r.completeSession(t, id)
		if _, err := r.tracker.ArchiveTask(ctx, id); err != nil {
			t.Errorf("ArchiveTask after completing: %v", err)
		}
	})

	t.Run("another Task is running", func(t *testing.T) {
		r := newRig(t)
		running := r.addTask(t, core.StateActive)
		other := r.addTask(t, core.StateActive)
		if _, err := r.tracker.StartSession(ctx, running, 30*time.Minute, core.StartOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.tracker.MarkDone(ctx, other); err != nil {
			t.Errorf("MarkDone on a different Task: %v", err)
		}
	})
}

func TestTaskStates_AReopenedTaskCanBeStartedAgain(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateActive)
	if _, err := r.tracker.MarkDone(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); !errors.Is(err, core.ErrTaskNotActive) {
		t.Fatalf("start on Done: err = %v, want ErrTaskNotActive", err)
	}
	if _, err := r.tracker.ReopenTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Errorf("start after reopening: %v", err)
	}
	if active, _ := r.tracker.Tasks(ctx, core.StateActive); len(active) != 1 {
		t.Errorf("Active tasks = %+v, want the reopened one", active)
	}
}
