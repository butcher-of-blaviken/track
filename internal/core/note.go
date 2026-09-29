package core

import "time"

// NoteID identifies a Note. It is assigned by the store.
type NoteID int64

// Note is a one-line entry in a Task's log: a Hand-off note written when a
// Focus session ends, or an ad-hoc note. A Note with no Task is an Unfiled
// note, waiting to be filed onto one.
type Note struct {
	ID NoteID
	// TaskID is 0 for an Unfiled note.
	TaskID TaskID
	// SessionID is set only for a Hand-off note, to the session it was written for.
	SessionID SessionID
	Text      string
	// CreatedAt is when the note was written. It is kept when the note is filed later.
	CreatedAt time.Time
}
