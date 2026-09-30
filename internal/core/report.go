package core

import (
	"cmp"
	"context"
	"slices"
	"time"
)

// Period is the window a Report covers.
type Period int

const (
	// PeriodToday is the local calendar day of the clock.
	PeriodToday Period = iota
	// PeriodWeek is the local calendar week, starting on Monday.
	PeriodWeek
)

// TaskTime is the focused time spent on one Task in a Report's window.
type TaskTime struct {
	Task    Task
	Focused time.Duration
	// Sessions is how many Focus sessions spent time on the Task in the window.
	Sessions int
}

// TaskNotes is the notes written on one Task in a Report's window, oldest first.
type TaskNotes struct {
	Task  Task
	Notes []Note
}

// TagTime is the focused time spent on the Tasks carrying one Tag. A Task with
// several Tags counts towards each, so these can add up to more than the total.
// Untagged, with an empty Tag, is the time on Tasks with no Tags.
type TagTime struct {
	Tag      string
	Focused  time.Duration
	Untagged bool
}

// Report is what was worked on in a window, in every Task state.
type Report struct {
	// Period is the period asked for; it is not set by Day.
	Period   Period
	From, To time.Time // the window, [From, To)
	Focused  time.Duration
	// Sessions is how many Focus sessions spent time in the window.
	Sessions int
	// Overrides is how many sessions that started in the window overrode a Break.
	Overrides int
	// Tasks and Tags are largest first, ties by Task ID and by Tag name.
	Tasks []TaskTime
	Tags  []TagTime
	// Notes are the notes written in the window onto Tasks, by Task ID, whether or
	// not the Task had any focused time. A note keeps the time it was written when
	// it is filed later.
	Notes []TaskNotes
	// Unfiled are the notes written in the window that belong to no Task yet.
	Unfiled []Note
	// Finished are the Tasks marked done in the window and still Done, earliest
	// first. A Task finished before Track recorded when is never listed.
	Finished []Task
}

// windowFor returns the window of a period around now, in now's location.
func windowFor(p Period, now time.Time) (time.Time, time.Time) {
	y, m, d := now.Date()
	from := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	if p == PeriodWeek {
		from = from.AddDate(0, 0, -((int(from.Weekday()) + 6) % 7)) // back to Monday
		return from, from.AddDate(0, 0, 7)
	}
	return from, from.AddDate(0, 0, 1)
}

// focusedIn is the time the session spent focusing inside [from, to) at now: its
// elapsed time, which stops at the planned end, clipped to the window.
func (s FocusSession) focusedIn(from, to, now time.Time) time.Duration {
	start, end := s.StartedAt, s.StartedAt.Add(s.Elapsed(now))
	if start.Before(from) {
		start = from
	}
	if end.After(to) {
		end = to
	}
	return end.Sub(start)
}

// DaySummary is the total for the local calendar day of the clock.
type DaySummary struct {
	Focused  time.Duration
	Sessions int
}

// Today sums the day's Focus sessions the way Report does, without the per-Task
// and per-Tag tables, so a header can show it cheaply.
func (t *Tracker) Today(ctx context.Context) (DaySummary, error) {
	now := t.clock.Now()
	from, to := windowFor(PeriodToday, now)
	sessions, err := t.store.Sessions(ctx)
	if err != nil {
		return DaySummary{}, err
	}
	var d DaySummary
	for _, s := range sessions {
		if clipped := s.focusedIn(from, to, now); clipped > 0 {
			d.Focused += clipped
			d.Sessions++
		}
	}
	return d, nil
}

// Report summarises the window of a period around the clock's now. A session's
// time is clipped to the window, so one that crosses midnight counts in both
// days, and a running one counts up to now.
func (t *Tracker) Report(ctx context.Context, p Period) (Report, error) {
	from, to := windowFor(p, t.clock.Now())
	r, err := t.reportWindow(ctx, from, to)
	r.Period = p
	return r, err
}

// Now is the Tracker's clock reading, so callers name "today" the way reports do.
func (t *Tracker) Now() time.Time { return t.clock.Now() }

// Day is the Report of the local calendar day that contains the given time, in
// that time's location, which may be any day, not only today.
func (t *Tracker) Day(ctx context.Context, day time.Time) (Report, error) {
	y, m, d := day.Date()
	from := time.Date(y, m, d, 0, 0, 0, 0, day.Location())
	return t.reportWindow(ctx, from, from.AddDate(0, 0, 1))
}

func (t *Tracker) reportWindow(ctx context.Context, from, to time.Time) (Report, error) {
	now := t.clock.Now()
	r := Report{From: from, To: to}

	tasks, err := t.store.Tasks(ctx)
	if err != nil {
		return Report{}, err
	}
	byID := make(map[TaskID]Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	sessions, err := t.store.Sessions(ctx)
	if err != nil {
		return Report{}, err
	}

	perTask := map[TaskID]time.Duration{}
	sessionsOn := map[TaskID]int{}
	for _, s := range sessions {
		if !s.StartedAt.Before(from) && s.StartedAt.Before(to) && s.SkippedBreak > 0 {
			r.Overrides++
		}
		clipped := s.focusedIn(from, to, now)
		if clipped <= 0 {
			continue
		}
		r.Focused += clipped
		r.Sessions++
		perTask[s.TaskID] += clipped
		sessionsOn[s.TaskID]++
	}

	perTag := map[string]time.Duration{}
	var untagged time.Duration
	for id, focused := range perTask {
		task := byID[id]
		r.Tasks = append(r.Tasks, TaskTime{Task: task, Focused: focused, Sessions: sessionsOn[id]})
		if len(task.Tags) == 0 {
			untagged += focused
		}
		for _, tag := range task.Tags {
			perTag[tag] += focused
		}
	}
	slices.SortFunc(r.Tasks, func(a, b TaskTime) int {
		return cmp.Or(cmp.Compare(b.Focused, a.Focused), cmp.Compare(a.Task.ID, b.Task.ID))
	})
	for tag, focused := range perTag {
		r.Tags = append(r.Tags, TagTime{Tag: tag, Focused: focused})
	}
	slices.SortFunc(r.Tags, func(a, b TagTime) int {
		return cmp.Or(cmp.Compare(b.Focused, a.Focused), cmp.Compare(a.Tag, b.Tag))
	})
	if untagged > 0 {
		r.Tags = append(r.Tags, TagTime{Untagged: true, Focused: untagged})
	}

	notes, err := t.notesWhere(ctx, func(n Note) bool { return !n.CreatedAt.Before(from) && n.CreatedAt.Before(to) })
	if err != nil {
		return Report{}, err
	}
	perTaskNotes := map[TaskID][]Note{}
	for _, n := range notes {
		if n.TaskID == 0 {
			r.Unfiled = append(r.Unfiled, n)
			continue
		}
		perTaskNotes[n.TaskID] = append(perTaskNotes[n.TaskID], n)
	}
	for id, list := range perTaskNotes {
		r.Notes = append(r.Notes, TaskNotes{Task: byID[id], Notes: list})
	}
	slices.SortFunc(r.Notes, func(a, b TaskNotes) int { return cmp.Compare(a.Task.ID, b.Task.ID) })

	for _, task := range tasks {
		if task.State == StateDone && task.DoneAt != nil && !task.DoneAt.Before(from) && task.DoneAt.Before(to) {
			r.Finished = append(r.Finished, task)
		}
	}
	slices.SortStableFunc(r.Finished, func(a, b Task) int { return cmp.Or(a.DoneAt.Compare(*b.DoneAt), cmp.Compare(a.ID, b.ID)) })
	return r, nil
}
