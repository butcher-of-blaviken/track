package core

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

var (
	// ErrEmptyNote is returned for a note with no text once whitespace is trimmed.
	ErrEmptyNote = errors.New("note is empty")
	// ErrNoteAlreadyFiled is returned when filing a note that already belongs to a Task.
	ErrNoteAlreadyFiled = errors.New("note is already filed")
	// ErrNoHandoffPending is returned when there is no ended session waiting for a hand-off.
	ErrNoHandoffPending = errors.New("no hand-off is pending")
)

// normalizeNote makes text one line: whitespace runs, including newlines,
// become single spaces, and the ends are trimmed.
func normalizeNote(text string) (string, error) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "", ErrEmptyNote
	}
	return text, nil
}

// AddHandoffNote writes the Hand-off note for the latest session, which must
// have ended (completed or stopped early) with its hand-off still pending.
func (t *Tracker) AddHandoffNote(ctx context.Context, text string) (Note, error) {
	text, err := normalizeNote(text)
	if err != nil {
		return Note{}, err
	}
	var note Note
	err = t.store.Update(ctx, func(tx Tx) error {
		now := t.clock.Now()
		session, err := pendingHandoff(tx, now)
		if err != nil {
			return err
		}
		note = Note{TaskID: session.TaskID, SessionID: session.ID, Text: text, CreatedAt: now}
		if note.ID, err = tx.CreateNote(note); err != nil {
			return err
		}
		session.HandoffAt = &now
		return tx.SaveSession(session)
	})
	return note, err
}

// pendingHandoff returns the latest session if it has ended and its hand-off is
// still pending, and ErrNoHandoffPending otherwise.
func pendingHandoff(tx Tx, now time.Time) (FocusSession, error) {
	session, err := tx.LatestSession()
	if errors.Is(err, ErrNotFound) {
		return FocusSession{}, ErrNoHandoffPending
	}
	if err != nil {
		return FocusSession{}, err
	}
	if session.Outcome(now) == OutcomeRunning || session.HandoffAt != nil {
		return FocusSession{}, ErrNoHandoffPending
	}
	return session, nil
}

// SkipHandoff resolves the pending hand-off without writing a note.
func (t *Tracker) SkipHandoff(ctx context.Context) error {
	return t.store.Update(ctx, func(tx Tx) error {
		now := t.clock.Now()
		session, err := pendingHandoff(tx, now)
		if err != nil {
			return err
		}
		session.HandoffAt = &now
		return tx.SaveSession(session)
	})
}

// AddNote writes an ad-hoc note on a Task, in any state, with no session link.
func (t *Tracker) AddNote(ctx context.Context, taskID TaskID, text string) (Note, error) {
	return t.addNote(ctx, taskID, text)
}

// AddUnfiledNote writes a note with no Task, to be filed onto one later.
func (t *Tracker) AddUnfiledNote(ctx context.Context, text string) (Note, error) {
	return t.addNote(ctx, 0, text)
}

func (t *Tracker) addNote(ctx context.Context, taskID TaskID, text string) (Note, error) {
	text, err := normalizeNote(text)
	if err != nil {
		return Note{}, err
	}
	note := Note{TaskID: taskID, Text: text, CreatedAt: t.clock.Now()}
	err = t.store.Update(ctx, func(tx Tx) error {
		var err error
		note.ID, err = tx.CreateNote(note)
		return err
	})
	return note, err
}

// FileNote attaches an Unfiled note to a Task. The note keeps the time it was written.
func (t *Tracker) FileNote(ctx context.Context, noteID NoteID, taskID TaskID) (Note, error) {
	var note Note
	err := t.store.Update(ctx, func(tx Tx) error {
		var err error
		if note, err = tx.Note(noteID); err != nil {
			return err
		}
		if note.TaskID != 0 {
			return ErrNoteAlreadyFiled
		}
		note.TaskID = taskID
		return tx.SaveNote(note)
	})
	return note, err
}

// CreateTaskFromNote turns an Unfiled note into a new Active Task and files the
// note onto it, in one step. The note's text is the Task's title, with ##tag
// markers becoming Tags as in AddTask, so a note of only tags is refused with
// ErrEmptyTitle. The note keeps the time it was written.
func (t *Tracker) CreateTaskFromNote(ctx context.Context, noteID NoteID) (Task, error) {
	var task Task
	err := t.store.Update(ctx, func(tx Tx) error {
		note, err := tx.Note(noteID)
		if err != nil {
			return err
		}
		if note.TaskID != 0 {
			return ErrNoteAlreadyFiled
		}
		title, tags, err := ParseTaskText(note.Text)
		if err != nil {
			return err
		}
		id, err := tx.CreateTask(Task{Title: title, State: StateActive, Tags: tags, CreatedAt: t.clock.Now()})
		if err != nil {
			return err
		}
		note.TaskID = id
		if err := tx.SaveNote(note); err != nil {
			return err
		}
		task, err = tx.Task(id) // re-read: the store canonicalizes Tag casing
		return err
	})
	return task, err
}

// TaskNotes returns a Task's log, ordered by when each note was written.
func (t *Tracker) TaskNotes(ctx context.Context, taskID TaskID) ([]Note, error) {
	if _, err := t.store.Task(ctx, taskID); err != nil {
		return nil, err
	}
	return t.notesWhere(ctx, func(n Note) bool { return n.TaskID == taskID })
}

// UnfiledNotes returns the notes not yet filed onto a Task, oldest first.
func (t *Tracker) UnfiledNotes(ctx context.Context) ([]Note, error) {
	return t.notesWhere(ctx, func(n Note) bool { return n.TaskID == 0 })
}

// NotesByTask returns every filed note grouped by its Task, each Task's log
// ordered by when its notes were written. Unfiled notes are left out. It reads
// all notes once, unlike TaskNotes per Task.
func (t *Tracker) NotesByTask(ctx context.Context) (map[TaskID][]Note, error) {
	filed, err := t.notesWhere(ctx, func(n Note) bool { return n.TaskID != 0 })
	if err != nil {
		return nil, err
	}
	out := map[TaskID][]Note{}
	for _, n := range filed {
		out[n.TaskID] = append(out[n.TaskID], n)
	}
	return out, nil
}

func (t *Tracker) notesWhere(ctx context.Context, keep func(Note) bool) ([]Note, error) {
	all, err := t.store.Notes(ctx)
	if err != nil {
		return nil, err
	}
	out := []Note{}
	for _, n := range all {
		if keep(n) {
			out = append(out, n)
		}
	}
	slices.SortStableFunc(out, func(a, b Note) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}
