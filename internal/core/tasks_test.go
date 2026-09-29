package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestAddTask_SplitsTitleAndTagsAndStampsTheCreationTime(t *testing.T) {
	r := newRig(t)
	r.clock.Advance(2 * time.Hour)

	task, err := r.tracker.AddTask(ctx, "finishing up auth for service x ##PROJ-123 ##backend")
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if task.ID == 0 || task.Title != "finishing up auth for service x" || task.State != core.StateActive {
		t.Errorf("task = %+v, want an Active task with the tags stripped from its title", task)
	}
	if want := []string{"PROJ-123", "backend"}; !slices.Equal(task.Tags, want) {
		t.Errorf("Tags = %v, want %v", task.Tags, want)
	}
	if want := sessionStart.Add(2 * time.Hour); !task.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want the clock's %v", task.CreatedAt, want)
	}

	stored, err := r.store.Task(ctx, task.ID)
	if err != nil || stored.Title != task.Title || !slices.Equal(stored.Tags, task.Tags) {
		t.Errorf("stored task = %+v, %v; want what AddTask returned", stored, err)
	}
}

func TestAddTask_ReturnsTagsInTheirFirstUseCasing(t *testing.T) {
	r := newRig(t)
	if _, err := r.tracker.AddTask(ctx, "one ##Auth"); err != nil {
		t.Fatal(err)
	}
	second, err := r.tracker.AddTask(ctx, "two ##auth")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Auth"}; !slices.Equal(second.Tags, want) {
		t.Errorf("Tags = %v, want %v (first-use casing)", second.Tags, want)
	}
}

func TestAddTask_EmptyTitleIsRejectedAndStoresNothing(t *testing.T) {
	r := newRig(t)
	for _, text := range []string{"", "   ", "##only ##tags"} {
		if _, err := r.tracker.AddTask(ctx, text); !errors.Is(err, core.ErrEmptyTitle) {
			t.Errorf("AddTask(%q) error = %v, want ErrEmptyTitle", text, err)
		}
	}
	if all, _ := r.store.Tasks(ctx); len(all) != 0 {
		t.Errorf("%d tasks stored after rejected adds, want 0", len(all))
	}
}

func TestTasks_FiltersByStateAndListsNewestFirst(t *testing.T) {
	r := newRig(t)
	add := func(text string) core.Task {
		t.Helper()
		task, err := r.tracker.AddTask(ctx, text)
		if err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(time.Minute)
		return task
	}
	setState := func(task core.Task, change func(*core.Task) error) {
		t.Helper()
		err := r.store.Update(ctx, func(tx core.Tx) error {
			cur, err := tx.Task(task.ID)
			if err != nil {
				return err
			}
			if err := change(&cur); err != nil {
				return err
			}
			return tx.SaveTask(cur)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	titles := func(tasks []core.Task, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, task := range tasks {
			out = append(out, task.Title)
		}
		return out
	}

	add("oldest")
	done, archived := add("done"), add("archived")
	add("newest")
	setState(done, (*core.Task).Done)
	setState(archived, (*core.Task).Archive)

	if got, want := titles(r.tracker.Tasks(ctx, core.StateActive)), []string{"newest", "oldest"}; !slices.Equal(got, want) {
		t.Errorf("Active = %v, want %v", got, want)
	}
	if got, want := titles(r.tracker.Tasks(ctx, core.StateDone, core.StateArchived)), []string{"archived", "done"}; !slices.Equal(got, want) {
		t.Errorf("Done+Archived = %v, want %v", got, want)
	}
	if got, want := titles(r.tracker.Tasks(ctx)), []string{"newest", "archived", "done", "oldest"}; !slices.Equal(got, want) {
		t.Errorf("no states (all) = %v, want %v", got, want)
	}
}
