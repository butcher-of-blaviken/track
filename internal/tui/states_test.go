package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
)

var (
	keyD     = tea.KeyPressMsg{Code: 'd', Text: "d"}
	keyShftD = tea.KeyPressMsg{Code: 'D', Text: "D", Mod: tea.ModShift}
	keyU     = tea.KeyPressMsg{Code: 'u', Text: "u"}
)

func (r *rig) stateOf(t *testing.T, id core.TaskID) core.State {
	t.Helper()
	task, err := r.tracker.Task(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return task.State
}

// threeTasks are "gamma" (newest, first on the list), "beta" and "alpha".
func threeTasks(t *testing.T) (*rig, []core.Task, tea.Model) {
	t.Helper()
	r := newRig(t)
	alpha := r.addTask(t, "alpha")
	beta := r.addTask(t, "beta")
	gamma := r.addTask(t, "gamma")
	return r, []core.Task{alpha, beta, gamma}, booted(r.newModel())
}

func TestStates_DMarksTheCursorTaskDoneAndItLeavesTheActiveList(t *testing.T) {
	r, tasks, m := threeTasks(t)
	m = press(m, keyJ) // beta
	wantSelected(t, "before", m, "beta")

	m = press(m, keyD)
	if got := r.stateOf(t, tasks[1].ID); got != core.StateDone {
		t.Fatalf("beta state = %v, want Done", got)
	}
	wantScreen(t, "notice", m, "Marked done: beta", "tab shows it", "u reopens")
	wantNoText(t, "list", m, "beta (done)")
	wantSelected(t, "cursor moves to the neighbour", m, "alpha")

	m = press(m, keyTab)
	wantScreen(t, "all scope", m, "beta (done)")
}

func TestStates_TheCursorFallsBackToThePreviousTaskAtTheEnd(t *testing.T) {
	_, _, m := threeTasks(t)
	m = press(m, keyJ, keyJ) // alpha, last
	m = press(m, keyD)
	wantSelected(t, "last row finished", m, "beta")
}

func TestStates_ShiftDArchivesAndUReopens(t *testing.T) {
	r, tasks, m := threeTasks(t)
	m = press(m, keyShftD) // gamma
	if got := r.stateOf(t, tasks[2].ID); got != core.StateArchived {
		t.Fatalf("gamma state = %v, want Archived", got)
	}
	wantScreen(t, "notice", m, "Archived: gamma", "u reopens")

	m = press(m, keyTab)
	wantScreen(t, "shown with the scope on", m, "gamma (archived)")
	m = press(m, keyU) // cursor is on the first Active row, so move to gamma
	wantScreen(t, "u on an Active task", m, "Already active")

	// Move the cursor to gamma (last) and reopen it.
	m = press(m, keyJ, keyJ, keyU)
	if got := r.stateOf(t, tasks[2].ID); got != core.StateActive {
		t.Fatalf("gamma state = %v, want Active", got)
	}
	wantScreen(t, "reopened", m, "Reopened: gamma")
	wantNoText(t, "marker", m, "gamma (archived)")
}

func TestStates_DoneTasksCanBeArchivedAndReopened(t *testing.T) {
	r, tasks, m := threeTasks(t)
	m = press(m, keyD, keyTab) // gamma done; scope on
	m = press(m, keyJ, keyJ, keyJ)
	wantSelected(t, "gamma (done) is last", m, "gamma (done)")
	m = press(m, keyShftD)
	if got := r.stateOf(t, tasks[2].ID); got != core.StateArchived {
		t.Errorf("state = %v, want Archived", got)
	}
	press(m, keyU)
	if got := r.stateOf(t, tasks[2].ID); got != core.StateActive {
		t.Errorf("state = %v, want Active", got)
	}
}

func TestStates_WrongStatePressesShowANoticeAndChangeNothing(t *testing.T) {
	r := newRig(t)
	archived := r.addTask(t, "archived one")
	done := r.addTask(t, "done one")
	r.addTask(t, "active one")
	r.setState(t, archived, core.StateArchived)
	r.setState(t, done, core.StateDone)
	m := booted(r.newModel())
	m = press(m, keyTab) // active one, done one, archived one

	m = press(m, keyU)
	wantScreen(t, "u on Active", m, "Already active")

	m = press(m, keyJ, keyD)
	wantScreen(t, "d on Done", m, "Already done")

	m = press(m, keyJ, keyD)
	wantScreen(t, "d on Archived", m, "Reopen it first (u)")
	m = press(m, keyShftD)
	wantScreen(t, "D on Archived", m, "Already archived")

	if got := r.stateOf(t, done.ID); got != core.StateDone {
		t.Errorf("done state = %v", got)
	}
	if got := r.stateOf(t, archived.ID); got != core.StateArchived {
		t.Errorf("archived state = %v", got)
	}
}

func TestStates_ARunningSessionsTaskCannotBeFinishedOrArchived(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "focus here")
	m := booted(r.newModel())
	m = press(m, keyS)
	wantFocusLine(t, "running", m, "30:00", "focus here")

	m = press(m, keyD)
	wantScreen(t, "d", m, "Stop the running session first (x)")
	m = press(m, keyShftD)
	wantScreen(t, "D", m, "Stop the running session first (x)")
	if got := r.stateOf(t, task.ID); got != core.StateActive {
		t.Errorf("state = %v, want it left Active", got)
	}
}

func TestStates_ActOnTheCursorTaskOfAFilteredList(t *testing.T) {
	r, tasks, m := threeTasks(t)
	m = press(m, keySlash)
	m = typeText(m, "alp")
	m = press(m, keyEnter)
	press(m, keyD)
	if got := r.stateOf(t, tasks[0].ID); got != core.StateDone {
		t.Errorf("alpha state = %v, want Done", got)
	}
	if got := r.stateOf(t, tasks[2].ID); got != core.StateActive {
		t.Errorf("gamma state = %v, want untouched", got)
	}
}

func TestStates_LettersAreTextInThePicker(t *testing.T) {
	r, tasks, m := threeTasks(t)
	m = press(m, keySlash)
	m = typeText(m, "dDu")
	wantScreen(t, "typed", m, "dDu")
	for _, task := range tasks {
		if got := r.stateOf(t, task.ID); got != core.StateActive {
			t.Errorf("%s state = %v, want untouched", task.Title, got)
		}
	}
}

func TestStates_NothingHappensOnAnEmptyList(t *testing.T) {
	m := booted(newRig(t).newModel())
	m = press(m, keyD, keyShftD, keyU)
	wantNoText(t, "empty list", m, "Marked")
}

func TestStates_FooterShowsDoneAndNothingOverflows(t *testing.T) {
	_, _, m := threeTasks(t)
	wantScreen(t, "footer", m, "d done")
	if strings.Contains(screen(m), "D archive") {
		t.Errorf("the footer lists the archive key; only d is advertised:\n%s", screen(m))
	}
}
