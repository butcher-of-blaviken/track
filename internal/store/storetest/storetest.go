// Package storetest is a contract test suite that every core.Store
// implementation must pass.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

var created = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)

// Run runs the contract suite. newStore must return a fresh, empty store.
func Run(t *testing.T, newStore func(t *testing.T) core.Store) {
	t.Helper()
	tests := []struct {
		name string
		run  func(t *testing.T, s core.Store)
	}{
		{"CreateThenReadRoundTrips", createThenReadRoundTrips},
		{"SaveTaskPersistsChanges", saveTaskPersistsChanges},
		{"MissingTaskIsNotFound", missingTaskIsNotFound},
		{"TagsShareFirstUseCasing", tagsShareFirstUseCasing},
		{"UpdateIsAtomic", updateIsAtomic},
		{"TasksListsAllStatesInIDOrder", tasksListsAllStatesInIDOrder},
		{"TagCasingFoldsUnicode", tagCasingFoldsUnicode},
		{"SessionsRoundTripInIDOrder", sessionsRoundTripInIDOrder},
		{"SaveSessionPersistsOnlyStoppedAt", saveSessionPersistsOnlyStoppedAt},
		{"LatestSessionIsTheHighestID", latestSessionIsTheHighestID},
		{"SessionForUnknownTaskIsNotFound", sessionForUnknownTaskIsNotFound},
		{"CompletedSessionCountCountsOnlyCompleted", completedSessionCountCountsOnlyCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { tt.run(t, newStore(t)) })
	}
}

// createTask saves a new Task in its own transaction and returns its ID.
func createTask(t *testing.T, s core.Store, task core.Task) core.TaskID {
	t.Helper()
	var id core.TaskID
	err := s.Update(context.Background(), func(tx core.Tx) error {
		var err error
		id, err = tx.CreateTask(task)
		return err
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return id
}

func createThenReadRoundTrips(t *testing.T, s core.Store) {
	ctx := context.Background()
	idA := createTask(t, s, core.Task{Title: "auth for service x", State: core.StateActive, Tags: []string{"PROJ-123"}, CreatedAt: created})
	idB := createTask(t, s, core.Task{Title: "write docs", State: core.StateDone, CreatedAt: created.Add(time.Hour)})

	if idA == 0 || idB == 0 || idA == idB {
		t.Fatalf("IDs must be non-zero and unique, got %d and %d", idA, idB)
	}
	got, err := s.Task(ctx, idA)
	if err != nil {
		t.Fatalf("Task(%d): %v", idA, err)
	}
	if got.ID != idA || got.Title != "auth for service x" || got.State != core.StateActive ||
		!reflect.DeepEqual(got.Tags, []string{"PROJ-123"}) || !got.CreatedAt.Equal(created) {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func saveTaskPersistsChanges(t *testing.T, s core.Store) {
	ctx := context.Background()
	id := createTask(t, s, core.Task{Title: "old", State: core.StateActive, CreatedAt: created})

	err := s.Update(ctx, func(tx core.Tx) error {
		task, err := tx.Task(id)
		if err != nil {
			return err
		}
		task.Title = "new"
		task.State = core.StateDone
		task.Tags = []string{"writing"}
		return tx.SaveTask(task)
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := s.Task(ctx, id)
	if err != nil {
		t.Fatalf("Task(%d): %v", id, err)
	}
	if got.Title != "new" || got.State != core.StateDone || !reflect.DeepEqual(got.Tags, []string{"writing"}) {
		t.Errorf("changes not persisted: %+v", got)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt changed to %v, want %v", got.CreatedAt, created)
	}
}

func missingTaskIsNotFound(t *testing.T, s core.Store) {
	ctx := context.Background()
	const missing core.TaskID = 9999
	if _, err := s.Task(ctx, missing); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Store.Task error = %v, want ErrNotFound", err)
	}
	err := s.Update(ctx, func(tx core.Tx) error {
		if _, err := tx.Task(missing); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("Tx.Task error = %v, want ErrNotFound", err)
		}
		return tx.SaveTask(core.Task{ID: missing, Title: "ghost"})
	})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("SaveTask error = %v, want ErrNotFound", err)
	}
}

func tagsShareFirstUseCasing(t *testing.T, s core.Store) {
	ctx := context.Background()
	createTask(t, s, core.Task{Title: "a", Tags: []string{"Auth"}, CreatedAt: created})
	idB := createTask(t, s, core.Task{Title: "b", Tags: []string{"auth", "Extra"}, CreatedAt: created})
	idC := createTask(t, s, core.Task{Title: "c", CreatedAt: created})

	// A later Task that reuses a Tag via SaveTask is canonicalized too.
	err := s.Update(ctx, func(tx core.Tx) error {
		task, err := tx.Task(idC)
		if err != nil {
			return err
		}
		task.Tags = []string{"EXTRA", "AUTH"}
		return tx.SaveTask(task)
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	for id, want := range map[core.TaskID][]string{
		idB: {"Auth", "Extra"},
		idC: {"Extra", "Auth"},
	} {
		got, err := s.Task(ctx, id)
		if err != nil {
			t.Fatalf("Task(%d): %v", id, err)
		}
		if !reflect.DeepEqual(got.Tags, want) {
			t.Errorf("Task %d tags = %v, want %v", id, got.Tags, want)
		}
	}
}

func updateIsAtomic(t *testing.T, s core.Store) {
	ctx := context.Background()
	existing := createTask(t, s, core.Task{Title: "keep", State: core.StateActive, CreatedAt: created})

	// A failing transaction leaves no trace, including Tags it introduced.
	var createdInFailed core.TaskID
	boom := fmt.Errorf("boom")
	err := s.Update(ctx, func(tx core.Tx) error {
		var err error
		if createdInFailed, err = tx.CreateTask(core.Task{Title: "doomed", Tags: []string{"Zed"}, CreatedAt: created}); err != nil {
			return err
		}
		task, err := tx.Task(existing)
		if err != nil {
			return err
		}
		task.State = core.StateArchived
		if err := tx.SaveTask(task); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Update error = %v, want %v", err, boom)
	}
	if _, err := s.Task(ctx, createdInFailed); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Task created in failed Update is visible: err = %v", err)
	}
	if got, _ := s.Task(ctx, existing); got.State != core.StateActive {
		t.Errorf("failed Update changed state to %v, want Active", got.State)
	}
	later := createTask(t, s, core.Task{Title: "later", Tags: []string{"zed"}, CreatedAt: created})
	if got, _ := s.Task(ctx, later); !reflect.DeepEqual(got.Tags, []string{"zed"}) {
		t.Errorf("rolled-back Tag leaked: tags = %v, want [zed]", got.Tags)
	}

	// A successful transaction commits all its writes together.
	var a, b core.TaskID
	err = s.Update(ctx, func(tx core.Tx) error {
		var err error
		if a, err = tx.CreateTask(core.Task{Title: "a", CreatedAt: created}); err != nil {
			return err
		}
		b, err = tx.CreateTask(core.Task{Title: "b", CreatedAt: created})
		return err
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, id := range []core.TaskID{a, b} {
		if _, err := s.Task(ctx, id); err != nil {
			t.Errorf("Task(%d) after commit: %v", id, err)
		}
	}
}

func tasksListsAllStatesInIDOrder(t *testing.T, s core.Store) {
	ctx := context.Background()
	if got, err := s.Tasks(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Tasks on empty store = %v, %v; want none", got, err)
	}
	ids := []core.TaskID{
		createTask(t, s, core.Task{Title: "active", State: core.StateActive, CreatedAt: created}),
		createTask(t, s, core.Task{Title: "done", State: core.StateDone, CreatedAt: created}),
		createTask(t, s, core.Task{Title: "archived", State: core.StateArchived, CreatedAt: created}),
	}
	got, err := s.Tasks(ctx)
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(got) != len(ids) {
		t.Fatalf("Tasks returned %d, want %d", len(got), len(ids))
	}
	for i, task := range got {
		if task.ID != ids[i] {
			t.Errorf("Tasks()[%d].ID = %d, want %d", i, task.ID, ids[i])
		}
	}
	if got[1].State != core.StateDone || got[2].State != core.StateArchived {
		t.Errorf("states = %v, %v; want Done, Archived", got[1].State, got[2].State)
	}
}

func tagCasingFoldsUnicode(t *testing.T, s core.Store) {
	createTask(t, s, core.Task{Title: "a", Tags: []string{"Ünï"}, CreatedAt: created})
	id := createTask(t, s, core.Task{Title: "b", Tags: []string{"ünï"}, CreatedAt: created})
	got, err := s.Task(context.Background(), id)
	if err != nil {
		t.Fatalf("Task(%d): %v", id, err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"Ünï"}) {
		t.Errorf("tags = %v, want [Ünï]", got.Tags)
	}
}

// createSession saves a new session in its own transaction and returns its ID.
func createSession(t *testing.T, s core.Store, session core.FocusSession) core.SessionID {
	t.Helper()
	var id core.SessionID
	err := s.Update(context.Background(), func(tx core.Tx) error {
		var err error
		id, err = tx.CreateSession(session)
		return err
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return id
}

func sessionsRoundTripInIDOrder(t *testing.T, s core.Store) {
	ctx := context.Background()
	task := createTask(t, s, core.Task{Title: "t", CreatedAt: created})
	stopped := created.Add(12 * time.Minute)
	a := createSession(t, s, core.FocusSession{TaskID: task, StartedAt: created, PlannedDuration: 30 * time.Minute})
	b := createSession(t, s, core.FocusSession{
		TaskID: task, StartedAt: created.Add(time.Hour), PlannedDuration: 25 * time.Minute, StoppedAt: &stopped,
		BreakDuration: 20 * time.Minute, LongBreak: true, SkippedBreak: 7 * time.Minute,
	})
	if a == 0 || b == 0 || a == b {
		t.Fatalf("session IDs must be non-zero and unique, got %d and %d", a, b)
	}

	got, err := s.Sessions(ctx)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(got) != 2 || got[0].ID != a || got[1].ID != b {
		t.Fatalf("Sessions = %+v, want IDs [%d %d]", got, a, b)
	}
	first, second := got[0], got[1]
	if first.TaskID != task || !first.StartedAt.Equal(created) || first.PlannedDuration != 30*time.Minute || first.StoppedAt != nil {
		t.Errorf("first session mismatch: %+v", first)
	}
	if second.PlannedDuration != 25*time.Minute || second.StoppedAt == nil || !second.StoppedAt.Equal(stopped) {
		t.Errorf("second session mismatch: %+v", second)
	}
	if first.BreakDuration != 0 || first.LongBreak || first.SkippedBreak != 0 {
		t.Errorf("first session Break fields = %v/%v/%v, want zero values", first.BreakDuration, first.LongBreak, first.SkippedBreak)
	}
	if second.BreakDuration != 20*time.Minute || !second.LongBreak || second.SkippedBreak != 7*time.Minute {
		t.Errorf("second session Break fields = %v/%v/%v, want 20m/true/7m", second.BreakDuration, second.LongBreak, second.SkippedBreak)
	}
}

func saveSessionPersistsOnlyStoppedAt(t *testing.T, s core.Store) {
	ctx := context.Background()
	task := createTask(t, s, core.Task{Title: "t", CreatedAt: created})
	id := createSession(t, s, core.FocusSession{TaskID: task, StartedAt: created, PlannedDuration: 30 * time.Minute})

	stopped := created.Add(10 * time.Minute)
	err := s.Update(ctx, func(tx core.Tx) error {
		return tx.SaveSession(core.FocusSession{
			ID: id, TaskID: 999, StartedAt: created.Add(time.Hour), PlannedDuration: time.Hour, StoppedAt: &stopped,
		})
	})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	all, err := s.Sessions(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("Sessions = %+v, %v", all, err)
	}
	got := all[0]
	if got.StoppedAt == nil || !got.StoppedAt.Equal(stopped) {
		t.Errorf("StoppedAt = %v, want %v", got.StoppedAt, stopped)
	}
	if got.TaskID != task || !got.StartedAt.Equal(created) || got.PlannedDuration != 30*time.Minute {
		t.Errorf("immutable fields changed: %+v", got)
	}

	err = s.Update(ctx, func(tx core.Tx) error { return tx.SaveSession(core.FocusSession{ID: 9999}) })
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("SaveSession on a missing session error = %v, want ErrNotFound", err)
	}
}

func latestSessionIsTheHighestID(t *testing.T, s core.Store) {
	ctx := context.Background()
	err := s.Update(ctx, func(tx core.Tx) error {
		if _, err := tx.LatestSession(); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("LatestSession with none = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	task := createTask(t, s, core.Task{Title: "t", CreatedAt: created})
	createSession(t, s, core.FocusSession{TaskID: task, StartedAt: created, PlannedDuration: time.Minute})
	err = s.Update(ctx, func(tx core.Tx) error {
		id, err := tx.CreateSession(core.FocusSession{TaskID: task, StartedAt: created.Add(time.Hour), PlannedDuration: 2 * time.Minute})
		if err != nil {
			return err
		}
		got, err := tx.LatestSession() // sees this transaction's own write
		if err != nil {
			return err
		}
		if got.ID != id || got.PlannedDuration != 2*time.Minute {
			t.Errorf("LatestSession = %+v, want the session just created (ID %d)", got, id)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sessionForUnknownTaskIsNotFound(t *testing.T, s core.Store) {
	err := s.Update(context.Background(), func(tx core.Tx) error {
		_, err := tx.CreateSession(core.FocusSession{TaskID: 9999, StartedAt: created, PlannedDuration: time.Minute})
		return err
	})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("CreateSession for a missing Task error = %v, want ErrNotFound", err)
	}
	if all, err := s.Sessions(context.Background()); err != nil || len(all) != 0 {
		t.Errorf("Sessions = %v, %v; want none", all, err)
	}
}

func completedSessionCountCountsOnlyCompleted(t *testing.T, s core.Store) {
	task := createTask(t, s, core.Task{Title: "t", CreatedAt: created})
	stopped := created.Add(time.Minute)
	// Completed by created+2h: started 09:00 for 30m.
	createSession(t, s, core.FocusSession{TaskID: task, StartedAt: created, PlannedDuration: 30 * time.Minute})
	// Stopped early.
	createSession(t, s, core.FocusSession{TaskID: task, StartedAt: created.Add(time.Hour), PlannedDuration: 30 * time.Minute, StoppedAt: &stopped})
	// Still running at created+2h: started 09:00+1h50m for 30m.
	createSession(t, s, core.FocusSession{TaskID: task, StartedAt: created.Add(110 * time.Minute), PlannedDuration: 30 * time.Minute})

	count := func(now time.Time) int {
		t.Helper()
		var n int
		err := s.Update(context.Background(), func(tx core.Tx) error {
			var err error
			n, err = tx.CompletedSessionCount(now)
			return err
		})
		if err != nil {
			t.Fatalf("CompletedSessionCount: %v", err)
		}
		return n
	}
	if got := count(created.Add(29 * time.Minute)); got != 0 {
		t.Errorf("count before the first session ends = %d, want 0", got)
	}
	if got := count(created.Add(30 * time.Minute)); got != 1 {
		t.Errorf("count exactly at the first planned end = %d, want 1", got)
	}
	if got := count(created.Add(2 * time.Hour)); got != 1 {
		t.Errorf("count with one stopped early and one running = %d, want 1", got)
	}
	if got := count(created.Add(3 * time.Hour)); got != 2 {
		t.Errorf("count once the third has also completed = %d, want 2", got)
	}
}
