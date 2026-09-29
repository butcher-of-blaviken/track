package core

import "fmt"

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

// Task is a unit of work being tracked.
type Task struct {
	title string
	state State
}

// NewTask returns an Active Task with the given title.
func NewTask(title string) *Task {
	return &Task{title: title, state: StateActive}
}

// Title returns the Task's title, with any Tag markers already stripped.
func (t *Task) Title() string { return t.title }

// State returns the Task's current state.
func (t *Task) State() State { return t.state }

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
		if t.state == f {
			t.state = to
			return nil
		}
	}
	return fmt.Errorf("cannot move task from %v to %v", t.state, to)
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
