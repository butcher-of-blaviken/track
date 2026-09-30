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

// Report summarises the window of a period around the clock's now. A session's
// time is clipped to the window, so one that crosses midnight counts in both
// days, and a running one counts up to now.
func (t *Tracker) Report(ctx context.Context, p Period) (Report, error) {
	now := t.clock.Now()
	from, to := windowFor(p, now)
	r := Report{Period: p, From: from, To: to}

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
	for _, s := range sessions {
		if !s.StartedAt.Before(from) && s.StartedAt.Before(to) && s.SkippedBreak > 0 {
			r.Overrides++
		}
		start, end := s.StartedAt, s.StartedAt.Add(s.Elapsed(now))
		if start.Before(from) {
			start = from
		}
		if end.After(to) {
			end = to
		}
		clipped := end.Sub(start)
		if clipped <= 0 {
			continue
		}
		r.Focused += clipped
		r.Sessions++
		perTask[s.TaskID] += clipped
	}

	perTag := map[string]time.Duration{}
	var untagged time.Duration
	for id, focused := range perTask {
		task := byID[id]
		r.Tasks = append(r.Tasks, TaskTime{Task: task, Focused: focused})
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
	return r, nil
}
