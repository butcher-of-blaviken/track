package tui_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
	"github.com/butcher-of-blaviken/track/internal/store/sqlite"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

// The running app has no file watcher: it sees what another process wrote the
// next time it ticks. These tests write through a second Tracker, as `track
// add` would, and then tick.

// A backend opens the store the TUI runs on and the store of "another
// process". With memory they are one store; with SQLite they are separate
// connections to one file.
type backend struct {
	name string
	open func(t *testing.T) (ui, other core.Store)
}

var backends = []backend{
	{"memory", func(*testing.T) (core.Store, core.Store) { s := memory.New(); return s, s }},
	{"sqlite", func(t *testing.T) (core.Store, core.Store) {
		path := filepath.Join(t.TempDir(), "track.db")
		open := func() core.Store {
			s, err := sqlite.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		}
		return open(), open()
	}},
}

func eachBackend(t *testing.T, run func(t *testing.T, r *rig, other *rig)) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			ui, other := b.open(t)
			run(t, newRigOn(t, ui), newRigOn(t, other))
		})
	}
}

func TestExternal_ATaskAddedElsewhereAppearsOnTheNextTickAndTheCursorStays(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		r.addTask(t, "alpha")
		r.addTask(t, "beta")
		m := booted(r.newModel())
		wantSelected(t, "before", m, "beta")
		m = press(m, keyJ)
		wantSelected(t, "moved", m, "alpha")

		other.clock.Advance(time.Hour)
		other.addTask(t, "from the command line")
		wantNoText(t, "not before the tick", m, "from the command line")

		m = send(m, tui.TickMsg{})
		wantScreen(t, "after the tick", m, "from the command line")
		wantSelected(t, "cursor stays on its task", m, "alpha")
	})
}

func TestExternal_AnUnfiledNoteAddedElsewhereRaisesTheCount(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		m := booted(r.newModel())
		wantNoText(t, "none yet", m, "Unfiled")
		other.unfile(t, "one")
		m = send(m, tui.TickMsg{})
		wantScreen(t, "one", m, "Unfiled notes: 1")
		other.unfile(t, "two")
		m = send(m, tui.TickMsg{})
		wantScreen(t, "two", m, "Unfiled notes: 2")
	})
}

func TestExternal_AnOpenInboxGainsANoteAndKeepsItsCursor(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		r.unfile(t, "first note")
		r.unfile(t, "second note")
		m := press(booted(r.newModel()), keyI, keyJ)
		wantScreen(t, "second is selected", m, "second note")

		other.clock.Advance(time.Hour)
		other.unfile(t, "third note")
		m = send(m, tui.TickMsg{})
		wantScreen(t, "new note listed", m, "third note", "second note", "first note")
		press(m, keyC) // create a task from the selected note
		tasks, err := r.tracker.Tasks(ctx)
		if err != nil || len(tasks) != 1 || tasks[0].Title != "second note" {
			t.Errorf("the cursor should have stayed on the second note; tasks = %+v, %v", tasks, err)
		}
	})
}

func TestExternal_AFilterShowsNewMatchesAndHidesNewMisses(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		r.addTask(t, "write docs")
		m := booted(r.newModel())
		m = press(m, keySlash)
		m = typeText(m, "docs")
		m = press(m, keyEnter)

		other.addTask(t, "review docs")
		other.addTask(t, "unrelated chore")
		m = send(m, tui.TickMsg{})
		wantScreen(t, "match", m, "review docs", "write docs")
		wantNoText(t, "miss", m, "unrelated chore")
	})
}

func TestExternal_ATaskFinishedElsewhereLeavesTheListAndTheCursorMovesOn(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		r.addTask(t, "alpha")
		beta := r.addTask(t, "beta")
		r.addTask(t, "gamma")
		m := booted(r.newModel())
		m = press(m, keyJ) // beta
		wantSelected(t, "on beta", m, "beta")

		if _, err := other.tracker.MarkDone(ctx, beta.ID); err != nil {
			t.Fatal(err)
		}
		m = send(m, tui.TickMsg{})
		wantNoText(t, "beta gone", m, "beta")
		wantSelected(t, "the cursor moves to the neighbour", m, "alpha")
	})
}

func TestExternal_AnOpenDetailShowsANoteFiledElsewhere(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		task := r.addTask(t, "write the PRD")
		note := other.unfile(t, "left off at section two")
		m := press(booted(r.newModel()), keyL)
		wantScreen(t, "detail", m, "No notes yet")

		if _, err := other.tracker.FileNote(ctx, note.ID, task.ID); err != nil {
			t.Fatal(err)
		}
		m = send(m, tui.TickMsg{})
		wantScreen(t, "filed note", m, "left off at section two")
	})
}

func TestExternal_ATickDoesNotDisturbAHalfTypedTask(t *testing.T) {
	eachBackend(t, func(t *testing.T, r, other *rig) {
		m := press(booted(r.newModel()), keyA)
		m = typeText(m, "half typ")
		other.addTask(t, "from elsewhere")
		m = send(m, tui.TickMsg{})
		wantScreen(t, "typing kept", m, "half typ", "from elsewhere")
		m = typeText(m, "ed")
		m = press(m, keyEnter)
		wantScreen(t, "saved", m, "half typed", "from elsewhere")
	})
}

// flakyStore fails the snapshot read while down is set.
type flakyStore struct {
	core.Store
	down *atomic.Bool
}

func (f flakyStore) LatestSession(ctx context.Context) (core.FocusSession, error) {
	if f.down.Load() {
		return core.FocusSession{}, errors.New("database is locked")
	}
	return f.Store.LatestSession(ctx)
}

func TestExternal_AFailedReadKeepsTheLastScreenAndTheNextTickRecovers(t *testing.T) {
	down := &atomic.Bool{}
	r := newRigOn(t, flakyStore{memory.New(), down})
	r.addTask(t, "alpha")
	m := booted(r.newModel())

	down.Store(true)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "error shown over the last good screen", m, "database is locked", "alpha")

	down.Store(false)
	r.addTask(t, "beta")
	m = send(m, tui.TickMsg{})
	wantNoText(t, "error cleared", m, "database is locked")
	wantScreen(t, "caught up", m, "alpha", "beta")
}
