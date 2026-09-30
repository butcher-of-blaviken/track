package core

import (
	"context"
	"slices"
	"time"
)

// TaskDetail is everything the detail view shows about one Task.
type TaskDetail struct {
	Task Task
	// Focused is the total time spent in the Task's Focus sessions so far,
	// counting a running session up to now and never past its planned end.
	Focused time.Duration
	// Sessions is how many Focus sessions the Task has had, running or not.
	Sessions int
	// History is those sessions, newest first, never nil. Ask each for its
	// Outcome and Elapsed at At.
	History []FocusSession
	// At is the clock reading the detail was derived at.
	At time.Time
	// Notes is the Task's log, oldest first.
	Notes []Note
}

// TaskDetail returns the Task's detail, or ErrNotFound. It works in every state.
func (t *Tracker) TaskDetail(ctx context.Context, id TaskID) (TaskDetail, error) {
	task, err := t.store.Task(ctx, id)
	if err != nil {
		return TaskDetail{}, err
	}
	sessions, err := t.store.Sessions(ctx)
	if err != nil {
		return TaskDetail{}, err
	}
	now := t.clock.Now()
	d := TaskDetail{Task: task, At: now, History: []FocusSession{}}
	for _, s := range sessions { // by ID ascending
		if s.TaskID == id {
			d.Focused += s.Elapsed(now)
			d.Sessions++
			d.History = append(d.History, s)
		}
	}
	slices.Reverse(d.History)
	if d.Notes, err = t.TaskNotes(ctx, id); err != nil {
		return TaskDetail{}, err
	}
	return d, nil
}
