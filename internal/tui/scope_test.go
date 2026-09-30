package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var keyTab = tea.KeyPressMsg{Code: tea.KeyTab}

// setState moves a Task to state straight in the store, as another process
// would; the TUI has no key for it yet.
func (r *rig) setState(t *testing.T, task core.Task, state core.State) {
	t.Helper()
	if err := r.store.Update(ctx, func(tx core.Tx) error {
		task.State = state
		return tx.SaveTask(task)
	}); err != nil {
		t.Fatal(err)
	}
}

// scopeRig has an Active Task, a Done one and an Archived one; the Archived
// one was created last.
func scopeRig(t *testing.T) (*rig, tea.Model) {
	t.Helper()
	r := newRig(t)
	r.addTask(t, "active work ##docs")
	done := r.addTask(t, "done work")
	archived := r.addTask(t, "archived work")
	r.setState(t, done, core.StateDone)
	r.setState(t, archived, core.StateArchived)
	return r, booted(r.newModel())
}

func indexOf(t *testing.T, m tea.Model, text string) int {
	t.Helper()
	i := strings.Index(screen(m), text)
	if i < 0 {
		t.Fatalf("screen lacks %q:\n%s", text, screen(m))
	}
	return i
}

func TestTags_SearchFindsATaskByTagName(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD ##docs")
	r.addTask(t, "fix the build")
	m := booted(r.newModel())
	m = press(m, keySlash)
	m = typeText(m, "docs")
	wantScreen(t, "tag match", m, "write the PRD")
	if strings.Contains(screen(m), "fix the build") {
		t.Errorf("an untagged Task matched:\n%s", screen(m))
	}
}

func TestTags_HashFiltersByTagOnly(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD ##docs")
	r.addTask(t, "docs cleanup")
	m := booted(r.newModel())
	m = press(m, keySlash)
	m = typeText(m, "#docs")
	wantScreen(t, "tagged", m, "write the PRD")
	if strings.Contains(screen(m), "docs cleanup") {
		t.Errorf("a Task with docs only in its title matched #docs:\n%s", screen(m))
	}
}

func TestScope_TabShowsDoneAndArchivedAfterTheActiveOnesAndTabAgainHidesThem(t *testing.T) {
	_, m := scopeRig(t)
	if strings.Contains(screen(m), "done work") || strings.Contains(screen(m), "Showing") {
		t.Fatalf("Done or Archived shown by default:\n%s", screen(m))
	}

	m = press(m, keyTab)
	wantScreen(t, "all", m, "Showing: all tasks", "active work", "done work (done)", "archived work (archived)")
	if indexOf(t, m, "active work") >= indexOf(t, m, "done work") || indexOf(t, m, "done work") >= indexOf(t, m, "archived work") {
		t.Errorf("want Active, then Done, then Archived:\n%s", screen(m))
	}

	m = press(m, keyTab)
	if s := screen(m); strings.Contains(s, "done work") || strings.Contains(s, "archived work") || strings.Contains(s, "Showing") {
		t.Errorf("Done or Archived still shown after toggling back:\n%s", s)
	}
}

func TestScope_TheCursorStaysOnItsTaskAcrossAToggle(t *testing.T) {
	_, m := scopeRig(t)
	m = press(m, keyTab)
	wantSelected(t, "start", m, "active work")
	m = press(m, keyTab)
	wantSelected(t, "back", m, "active work")
}

func TestScope_PickerSearchesOnlyTheScopeAndTabWidensIt(t *testing.T) {
	_, m := scopeRig(t)
	m = press(m, keySlash)
	m = typeText(m, "done")
	wantScreen(t, "active scope", m, "No matches")

	m = press(m, keyTab)
	wantScreen(t, "all scope", m, "done work (done)")
	if strings.Contains(screen(m), "No matches") {
		t.Errorf("still no matches after widening:\n%s", screen(m))
	}
	m = press(m, keyTab)
	wantScreen(t, "active again", m, "No matches")
}

func TestScope_StartingADoneOrArchivedTaskShowsTheNoticeAndStartsNothing(t *testing.T) {
	r, m := scopeRig(t)
	m = press(m, keyTab, keyJ) // "done work"
	wantSelected(t, "cursor", m, "done work")
	m = press(m, keyS)
	wantScreen(t, "notice", m, "not active")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions, want 0", n)
	}

	// Through the picker too.
	m = press(m, keyCtrlP)
	m = typeText(m, "archived")
	m = press(m, keyEnter)
	wantScreen(t, "picker notice", m, "not active")
	if n := len(r.storedSessions(t)); n != 0 {
		t.Errorf("%d sessions, want 0", n)
	}
}

func TestScope_AnInactiveTaskDoesNotAskAboutTheBreak(t *testing.T) {
	r, m := scopeRig(t)
	r.startSession(t)
	r.clock.Set(epoch.Add(sessionEnd))
	m = send(m, tui.TickMsg{})
	m = press(m, keyEsc)
	m = press(m, keyTab, keyJ)
	m = press(m, keyS)
	if strings.Contains(screen(m), "Start anyway") {
		t.Errorf("asked to override the Break for a Task that cannot start:\n%s", screen(m))
	}
}

func TestScope_FooterShowsTheTabHint(t *testing.T) {
	_, m := scopeRig(t)
	m = press(m, keyHelp)
	wantHints(t, "list help", m, "tab scope")
	m = press(m, keyHelp)
	m = press(m, keySlash)
	wantScreen(t, "picker footer", m, "tab")
}
