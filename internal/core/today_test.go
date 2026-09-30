package core_test

import (
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func (r *reportRig) today(t *testing.T) core.DaySummary {
	t.Helper()
	got, err := r.tracker.Today(ctx)
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	return got
}

func TestToday_IsTheFocusedTimeAndSessionsOfTheLocalDay(t *testing.T) {
	r := newReportRig(t)
	id := r.task(t, "work")
	r.session(t, id, at(30, 9, 0), 0, 0)               // 30m
	r.session(t, id, at(30, 10, 0), 12*time.Minute, 0) // stopped after 12m
	r.session(t, id, at(29, 9, 0), 0, 0)               // yesterday
	if got := r.today(t); got.Focused != 42*time.Minute || got.Sessions != 2 {
		t.Errorf("today = %v over %d, want 42m over 2", got.Focused, got.Sessions)
	}
}

func TestToday_AgreesWithTheReportForTheSameDay(t *testing.T) {
	r := newReportRig(t)
	a, b := r.task(t, "a", "x"), r.task(t, "b")
	r.session(t, a, at(29, 23, 50), 0, 0) // crosses midnight: 20m today
	r.session(t, b, at(30, 9, 0), 0, 0)   // 30m
	r.session(t, a, at(30, 14, 50), 0, 0) // running: capped, 10m so far
	report := r.report(t, core.PeriodToday)
	got := r.today(t)
	if got.Focused != report.Focused || got.Sessions != report.Sessions {
		t.Errorf("Today = %v over %d but Report = %v over %d", got.Focused, got.Sessions, report.Focused, report.Sessions)
	}
	if got.Focused != 60*time.Minute || got.Sessions != 3 {
		t.Errorf("today = %v over %d, want 1h over 3", got.Focused, got.Sessions)
	}
}

func TestToday_IsEmptyWithNoSessions(t *testing.T) {
	if got := newReportRig(t).today(t); got.Focused != 0 || got.Sessions != 0 {
		t.Errorf("today = %+v, want nothing", got)
	}
}
