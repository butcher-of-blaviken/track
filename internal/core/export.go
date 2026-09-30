package core

import (
	"context"
	"slices"
	"time"
)

// Export is everything the store holds, for taking it out of the app.
type Export struct {
	// ExportedAt is the clock reading the export was made at.
	ExportedAt time.Time
	// Tasks are the Tasks in every state, ordered by ID.
	Tasks []ExportTask
	// Unfiled are the notes not yet filed onto a Task, oldest first.
	Unfiled []Note
}

// ExportTask is a Task with its Focus sessions and its log.
type ExportTask struct {
	Task Task
	// Focused is the total time spent in the Task's sessions, counted as in TaskDetail.
	Focused time.Duration
	// Sessions are ordered by ID; Notes are oldest first. Neither is ever nil.
	Sessions []FocusSession
	Notes    []Note
}

// Export reads the whole store. It is read-only.
func (t *Tracker) Export(ctx context.Context) (Export, error) {
	now := t.clock.Now()
	tasks, err := t.store.Tasks(ctx)
	if err != nil {
		return Export{}, err
	}
	sessions, err := t.store.Sessions(ctx)
	if err != nil {
		return Export{}, err
	}
	notes, err := t.NotesByTask(ctx)
	if err != nil {
		return Export{}, err
	}
	unfiled, err := t.UnfiledNotes(ctx)
	if err != nil {
		return Export{}, err
	}

	e := Export{ExportedAt: now, Tasks: make([]ExportTask, 0, len(tasks)), Unfiled: unfiled}
	slices.SortFunc(tasks, func(a, b Task) int { return int(a.ID - b.ID) })
	for _, task := range tasks {
		et := ExportTask{Task: task, Sessions: []FocusSession{}, Notes: []Note{}}
		et.Notes = append(et.Notes, notes[task.ID]...)
		for _, s := range sessions {
			if s.TaskID == task.ID {
				et.Sessions = append(et.Sessions, s)
				et.Focused += s.Elapsed(now)
			}
		}
		e.Tasks = append(e.Tasks, et)
	}
	return e, nil
}
