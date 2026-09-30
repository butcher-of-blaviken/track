package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/sqlite"
	"github.com/butcher-of-blaviken/track/internal/store/storetest"
)

// dbPath returns a database path in a fresh temp dir. The directory name has
// a space, like macOS's "Application Support", to exercise path escaping.
func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "with space", "track.db")
}

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) core.Store {
		s, err := sqlite.Open(dbPath(t))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}

func mustOpen(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTask(title string) core.Task {
	return core.Task{Title: title, State: core.StateActive, CreatedAt: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)}
}

func TestData_SurvivesCloseAndReopen(t *testing.T) {
	path := dbPath(t)
	ctx := context.Background()
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var id core.TaskID
	if err := s.Update(ctx, func(tx core.Tx) error {
		task := newTask("persist me")
		task.Tags = []string{"PROJ-1"}
		var err error
		id, err = tx.CreateTask(task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := mustOpen(t, path)
	got, err := reopened.Task(ctx, id)
	if err != nil {
		t.Fatalf("Task(%d) after reopen: %v", id, err)
	}
	if got.Title != "persist me" || !reflect.DeepEqual(got.Tags, []string{"PROJ-1"}) {
		t.Errorf("after reopen got %+v", got)
	}
}

func TestTwoHandles_SeeEachOthersWrites(t *testing.T) {
	path := dbPath(t)
	ctx := context.Background()
	a, b := mustOpen(t, path), mustOpen(t, path)

	var id core.TaskID
	if err := a.Update(ctx, func(tx core.Tx) error {
		var err error
		id, err = tx.CreateTask(newTask("from a"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Task(ctx, id); err != nil || got.Title != "from a" {
		t.Errorf("handle b Task(%d) = %+v, %v; want the task written by a", id, got, err)
	}
}

func TestTwoHandles_ConcurrentWritersDoNotFail(t *testing.T) {
	path := dbPath(t)
	ctx := context.Background()
	const perWriter = 25

	// Both handles open at the same moment on a fresh file, so migrations race too.
	stores := make([]*sqlite.Store, 2)
	var wg sync.WaitGroup
	errs := make(chan error, 2*perWriter+2)
	for i := range stores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := sqlite.Open(path)
			if err != nil {
				errs <- err
				return
			}
			stores[i] = s
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Open: %v", err)
	}
	for _, s := range stores {
		t.Cleanup(func() { _ = s.Close() })
	}

	errs = make(chan error, 2*perWriter)
	for w, s := range stores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < perWriter; n++ {
				err := s.Update(ctx, func(tx core.Tx) error {
					_, err := tx.CreateTask(newTask(fmt.Sprintf("w%d-%d", w, n)))
					return err
				})
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}
	all, err := stores[0].Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2*perWriter {
		t.Errorf("got %d tasks, want %d", len(all), 2*perWriter)
	}
}

func TestOpen_RefusesDatabaseFromNewerVersion(t *testing.T) {
	path := dbPath(t)
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a database migrated by a newer build of Track.
	raw, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (999, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := sqlite.Open(path); !errors.Is(err, sqlite.ErrSchemaTooNew) {
		t.Errorf("Open error = %v, want ErrSchemaTooNew", err)
	}
}

func TestStartSession_ConcurrentStartsFromTwoHandlesAllowExactlyOne(t *testing.T) {
	path := dbPath(t)
	ctx := context.Background()
	a, b := mustOpen(t, path), mustOpen(t, path)

	var task core.TaskID
	if err := a.Update(ctx, func(tx core.Tx) error {
		var err error
		task, err = tx.CreateTask(newTask("contended"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	fake := clock.NewFake(time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC))
	policy := core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4}
	var trackers []*core.Tracker
	for _, s := range []*sqlite.Store{a, b} {
		tr, err := core.NewTracker(s, fake, policy)
		if err != nil {
			t.Fatal(err)
		}
		trackers = append(trackers, tr)
	}

	const perHandle = 5
	results := make(chan error, len(trackers)*perHandle)
	var wg sync.WaitGroup
	for _, tr := range trackers {
		for range perHandle {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tr.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
				results <- err
			}()
		}
	}
	wg.Wait()
	close(results)

	var started, rejected int
	for err := range results {
		switch {
		case err == nil:
			started++
		case errors.Is(err, core.ErrSessionRunning):
			rejected++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if started != 1 || rejected != len(trackers)*perHandle-1 {
		t.Errorf("started %d, rejected %d; want exactly 1 started", started, rejected)
	}
	if all, err := a.Sessions(ctx); err != nil || len(all) != 1 {
		t.Errorf("Sessions = %v, %v; want exactly one", all, err)
	}
}

func TestSnapshot_SeesASessionStartedThroughAnotherHandle(t *testing.T) {
	path := dbPath(t)
	ctx := context.Background()
	a, b := mustOpen(t, path), mustOpen(t, path)
	policy := core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4}
	fake := clock.NewFake(time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC))
	starter, err := core.NewTracker(a, fake, policy)
	if err != nil {
		t.Fatal(err)
	}
	watcher, err := core.NewTracker(b, fake, policy)
	if err != nil {
		t.Fatal(err)
	}

	var task core.TaskID
	if err := a.Update(ctx, func(tx core.Tx) error {
		var err error
		task, err = tx.CreateTask(newTask("shared"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if snap, err := watcher.Snapshot(ctx); err != nil || snap.Phase != core.PhaseIdle {
		t.Fatalf("before the start: %+v, %v; want Idle", snap, err)
	}
	if _, err := starter.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	fake.Advance(10 * time.Minute)
	snap, err := watcher.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Phase != core.PhaseFocus || snap.Remaining != 20*time.Minute {
		t.Errorf("snapshot through the other handle = %+v, want Focus with 20m remaining", snap)
	}
}

// The running app reads on every tick while `track add` and `track note` write
// from other processes. A read must never fail or see a half-written state.
func TestTwoHandles_ReadsDuringWritesNeverFailAndNeverGoBackwards(t *testing.T) {
	path := dbPath(t)
	reader, writer := mustOpen(t, path), mustOpen(t, path)
	const writes = 60
	ctx := context.Background()

	var wg sync.WaitGroup
	writeErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			err := writer.Update(ctx, func(tx core.Tx) error {
				if _, err := tx.CreateTask(core.Task{Title: fmt.Sprintf("task %d", i), State: core.StateActive, CreatedAt: time.Now()}); err != nil {
					return err
				}
				_, err := tx.CreateNote(core.Note{Text: fmt.Sprintf("note %d", i), CreatedAt: time.Now()})
				return err
			})
			if err != nil {
				writeErr <- err
				return
			}
		}
	}()

	tasks, notes := 0, 0
	for tasks < writes || notes < writes {
		ts, err := reader.Tasks(ctx)
		if err != nil {
			t.Fatalf("Tasks during writes: %v", err)
		}
		n, err := reader.UnfiledNoteCount(ctx)
		if err != nil {
			t.Fatalf("UnfiledNoteCount during writes: %v", err)
		}
		if _, err := reader.LatestSession(ctx); err != nil && !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("LatestSession during writes: %v", err)
		}
		if len(ts) < tasks || n < notes {
			t.Fatalf("a read went backwards: tasks %d -> %d, notes %d -> %d", tasks, len(ts), notes, n)
		}
		tasks, notes = len(ts), n
		select {
		case err := <-writeErr:
			t.Fatalf("write failed: %v", err)
		default:
		}
	}
	wg.Wait()
}
