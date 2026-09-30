package core

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidTransition is returned when a Task is moved to a state it cannot
// move to from where it is.
var ErrInvalidTransition = errors.New("invalid task state change")

// State is where a Task is in its lifecycle.
type State int

const (
	// StateActive tasks are shown by default.
	StateActive State = iota
	// StateDone tasks are finished; hidden by default but searchable and reopenable.
	StateDone
	// StateArchived tasks were abandoned; hidden unless explicitly requested.
	StateArchived
)

// TaskID identifies a Task. It is assigned by the store; the zero value means
// the Task has not been saved yet.
type TaskID int64

// Task is a unit of work being tracked.
type Task struct {
	ID    TaskID
	Title string // Tag markers already stripped
	State State
	// Tags are the Task's Tag names, in first-use casing.
	Tags      []string
	CreatedAt time.Time
	// DoneAt is when the Task was marked done, set while it is Done and nil
	// otherwise. A Task finished before Track recorded this has none.
	DoneAt *time.Time
}

// NewTask returns an unsaved Active Task with the given title.
func NewTask(title string) *Task {
	return &Task{Title: title, State: StateActive}
}

// Done marks the Task as finished. Only an Active Task can be finished.
func (t *Task) Done() error {
	return t.transition(StateDone, StateActive)
}

// Archive marks the Task as abandoned. Active and Done Tasks can be archived.
func (t *Task) Archive() error {
	return t.transition(StateArchived, StateActive, StateDone)
}

// Reopen makes a Done or Archived Task Active again.
func (t *Task) Reopen() error {
	return t.transition(StateActive, StateDone, StateArchived)
}

// transition moves the Task to state to if it is currently in one of from,
// and otherwise leaves it untouched and returns an error.
func (t *Task) transition(to State, from ...State) error {
	for _, f := range from {
		if t.State == f {
			t.State = to
			return nil
		}
	}
	return fmt.Errorf("%w: cannot move task from %v to %v", ErrInvalidTransition, t.State, to)
}

func (s State) String() string {
	switch s {
	case StateActive:
		return "Active"
	case StateDone:
		return "Done"
	case StateArchived:
		return "Archived"
	}
	return fmt.Sprintf("State(%d)", int(s))
}
