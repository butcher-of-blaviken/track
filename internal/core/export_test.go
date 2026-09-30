package core_test

import (
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestExport_HoldsEveryTaskInEveryStateWithItsSessionsAndNotes(t *testing.T) {
	r := newRig(t)
	active := r.addTask(t, core.StateActive)
	done := r.addTask(t, core.StateDone)
	archived := r.addTask(t, core.StateArchived)
	r.completeSession(t, active) // 30m
	if _, err := r.tracker.AddNote(ctx, done, "shipped"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.AddUnfiledNote(ctx, "loose thought"); err != nil {
		t.Fatal(err)
	}

	e, err := r.tracker.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !e.ExportedAt.Equal(r.clock.Now()) {
		t.Errorf("ExportedAt = %v, want now", e.ExportedAt)
	}
	if len(e.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3", len(e.Tasks))
	}
	byID := map[core.TaskID]core.ExportTask{}
	for i, et := range e.Tasks {
		byID[et.Task.ID] = et
		if i > 0 && e.Tasks[i-1].Task.ID > et.Task.ID {
			t.Errorf("tasks are not ordered by ID")
		}
	}
	if got := byID[active]; len(got.Sessions) != 1 || got.Focused != 30*time.Minute {
		t.Errorf("active: %d sessions, focused %v; want 1 and 30m", len(got.Sessions), got.Focused)
	}
	if got := byID[done]; got.Task.State != core.StateDone || len(got.Notes) != 1 || got.Notes[0].Text != "shipped" {
		t.Errorf("done: %+v", got)
	}
	if got := byID[archived]; got.Task.State != core.StateArchived || got.Sessions == nil || got.Notes == nil {
		t.Errorf("archived should have empty, non-nil lists: %+v", got)
	}
	if len(e.Unfiled) != 1 || e.Unfiled[0].Text != "loose thought" {
		t.Errorf("unfiled = %+v", e.Unfiled)
	}
}

func TestExport_ARunningSessionCountsOnlyUpToItsPlannedEnd(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(3 * time.Hour)
	e, err := r.tracker.Export(ctx)
	if err != nil || e.Tasks[0].Focused != 30*time.Minute {
		t.Errorf("focused %v, %v; want 30m", e.Tasks[0].Focused, err)
	}
}

func TestExport_AnEmptyStoreGivesEmptyNonNilLists(t *testing.T) {
	e, err := newRig(t).tracker.Export(ctx)
	if err != nil || e.Tasks == nil || e.Unfiled == nil || len(e.Tasks)+len(e.Unfiled) != 0 {
		t.Errorf("export = %+v, %v", e, err)
	}
}
