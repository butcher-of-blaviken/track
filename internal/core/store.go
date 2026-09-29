package core

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when an entity does not exist in the store.
var ErrNotFound = errors.New("not found")

// Store persists the domain. Reads are plain methods; every write goes through
// Update so that multi-step changes commit all together or not at all.
//
// The store owns Tag canonicalization: Tags are case-insensitive, and every Task
// sees the casing from the Tag's first use.
type Store interface {
	// Tasks returns all Tasks in every state, ordered by ID ascending.
	// Filtering and search happen in the core.
	Tasks(ctx context.Context) ([]Task, error)
	// Task returns the Task with the given ID, or ErrNotFound.
	Task(ctx context.Context, id TaskID) (Task, error)
	// Sessions returns all Focus sessions, ordered by ID ascending.
	Sessions(ctx context.Context) ([]FocusSession, error)
	// Notes returns all Notes, filed and unfiled, ordered by ID ascending.
	Notes(ctx context.Context) ([]Note, error)
	// Update runs fn in a transaction. If fn returns an error, none of its
	// writes are applied and that error is returned.
	Update(ctx context.Context, fn func(Tx) error) error
}

// Tx is the write path of a transaction. It sees its own writes.
type Tx interface {
	// Task returns the Task with the given ID, or ErrNotFound.
	Task(id TaskID) (Task, error)
	// CreateTask saves t as a new Task, ignoring t.ID, and returns its ID.
	CreateTask(t Task) (TaskID, error)
	// SaveTask updates the title, state and Tags of an existing Task, or
	// returns ErrNotFound.
	SaveTask(t Task) error
	// CreateSession saves s as a new Focus session, ignoring s.ID, and returns
	// its ID. It returns ErrNotFound if s.TaskID is not an existing Task.
	CreateSession(s FocusSession) (SessionID, error)
	// SaveSession records how a session ended: it persists StoppedAt and
	// HandoffAt only. The other fields, including the Break plan, are
	// immutable, so changes to them are ignored.
	// It returns ErrNotFound if the session does not exist.
	SaveSession(s FocusSession) error
	// LatestSession returns the session with the highest ID, or ErrNotFound if
	// there are none. Only one session runs at a time, so the latest is the
	// only one that can be running.
	LatestSession() (FocusSession, error)
	// CreateNote saves n as a new Note, ignoring n.ID, and returns its ID. It
	// returns ErrNotFound if n.TaskID or n.SessionID is set but does not exist.
	CreateNote(n Note) (NoteID, error)
	// Note returns the Note with the given ID, or ErrNotFound.
	Note(id NoteID) (Note, error)
	// SaveNote files a Note onto a Task: it persists TaskID only, and the
	// other fields are immutable. It returns ErrNotFound if the note or the
	// Task does not exist.
	SaveNote(n Note) error
	// CompletedSessionCount counts the sessions that ran their full planned
	// length by now: not stopped early, and past their planned end.
	CompletedSessionCount(now time.Time) (int, error)
}
