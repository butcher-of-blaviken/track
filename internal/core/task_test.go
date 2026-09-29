package core_test

import (
	"testing"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestNewTask_StartsActive(t *testing.T) {
	task := core.NewTask("write the PRD")
	if got := task.State; got != core.StateActive {
		t.Errorf("State = %v, want Active", got)
	}
	if got := task.Title; got != "write the PRD" {
		t.Errorf("Title = %q, want %q", got, "write the PRD")
	}
}

func TestTask_DoneMovesActiveToDone(t *testing.T) {
	task := core.NewTask("x")
	if err := task.Done(); err != nil {
		t.Fatalf("Done() error: %v", err)
	}
	if got := task.State; got != core.StateDone {
		t.Errorf("State = %v, want Done", got)
	}
}

// taskIn returns a Task brought to the given state through the public API.
func taskIn(t *testing.T, state core.State) *core.Task {
	t.Helper()
	task := core.NewTask("x")
	switch state {
	case core.StateDone:
		if err := task.Done(); err != nil {
			t.Fatal(err)
		}
	case core.StateArchived:
		if err := task.Archive(); err != nil {
			t.Fatal(err)
		}
	}
	return task
}

func TestTask_Transitions(t *testing.T) {
	const (
		done    = "Done"
		archive = "Archive"
		reopen  = "Reopen"
	)
	apply := map[string]func(*core.Task) error{
		done:    (*core.Task).Done,
		archive: (*core.Task).Archive,
		reopen:  (*core.Task).Reopen,
	}
	tests := []struct {
		name string
		from core.State
		op   string
		want core.State // ignored when wantErr
		// wantErr means the transition is illegal and the state must not change.
		wantErr bool
	}{
		{"active to done", core.StateActive, done, core.StateDone, false},
		{"active to archived", core.StateActive, archive, core.StateArchived, false},
		{"done reopens to active", core.StateDone, reopen, core.StateActive, false},
		{"done to archived", core.StateDone, archive, core.StateArchived, false},
		{"archived reopens to active", core.StateArchived, reopen, core.StateActive, false},
		{"done to done is an error", core.StateDone, done, 0, true},
		{"archived to archived is an error", core.StateArchived, archive, 0, true},
		{"reopening an active task is an error", core.StateActive, reopen, 0, true},
		{"archived cannot go straight to done", core.StateArchived, done, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := taskIn(t, tt.from)
			err := apply[tt.op](task)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("%s from state %v returned no error", tt.op, tt.from)
				}
				if got := task.State; got != tt.from {
					t.Errorf("state changed to %v despite error, want %v", got, tt.from)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := task.State; got != tt.want {
				t.Errorf("state = %v, want %v", got, tt.want)
			}
		})
	}
}
