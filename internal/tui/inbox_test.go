package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keyI = tea.KeyPressMsg{Code: 'i', Text: "i"}
	keyC = tea.KeyPressMsg{Code: 'c', Text: "c"}
	keyF = tea.KeyPressMsg{Code: 'f', Text: "f"}
)

// unfile writes an Unfiled note and moves the clock on a minute.
func (r *rig) unfile(t *testing.T, text string) core.Note {
	t.Helper()
	n, err := r.tracker.AddUnfiledNote(ctx, text)
	if err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	return n
}

func (r *rig) unfiledCount(t *testing.T) int {
	t.Helper()
	n, err := r.tracker.UnfiledNoteCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInbox_ThePanelShowsTheCountWithAHintAndINeedsSomethingToOpen(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	wantNoText(t, "no notes", m, "Unfiled")
	m = press(m, keyI)
	wantScreen(t, "i with nothing to file", m, "No unfiled notes")

	r.unfile(t, "one")
	r.unfile(t, "two")
	m = send(m, tui.TickMsg{})
	wantScreen(t, "count", m, "Unfiled notes: 2 (i to file)")
}

func TestInbox_ListsTheNotesOldestFirstWithTheirAgesAndEscGoesBack(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "some task")
	r.unfile(t, "first note")
	r.unfile(t, "second note")
	r.clock.Advance(2 * time.Hour)
	m := booted(r.newModel())

	m = press(m, keyI)
	wantScreen(t, "inbox", m, "first note", "second note", "2h 2m ago", "enter file", "c new task", "esc back")
	if indexOf(t, m, "first note") > indexOf(t, m, "second note") {
		t.Errorf("want the oldest note first:\n%s", screen(m))
	}
	wantNoText(t, "the task list", m, "some task")
	wantSelected(t, "cursor", m, "first note")
	m = press(m, keyJ)
	wantSelected(t, "moved", m, "second note")

	m = press(m, keyEsc)
	wantScreen(t, "back on the list", m, "some task")
	wantNoText(t, "back on the list", m, "first note")

	m = press(m, keyI, keyI) // i opens it, and i closes it
	wantScreen(t, "toggled closed", m, "some task")
}

func TestInbox_CCreatesATaskFromTheNoteAndFilesIt(t *testing.T) {
	r := newRig(t)
	r.unfile(t, "check the retry logic ##backend")
	r.unfile(t, "second one")
	m := booted(r.newModel())
	m = press(m, keyI, keyC)

	wantScreen(t, "notice", m, "Created task: check the retry logic")
	wantNoText(t, "gone from the inbox", m, "check the retry logic ##backend")
	wantSelected(t, "next note", m, "second one")
	if n := r.unfiledCount(t); n != 1 {
		t.Errorf("%d unfiled notes, want 1", n)
	}
	tasks, _ := r.tracker.Tasks(ctx)
	if len(tasks) != 1 || tasks[0].Title != "check the retry logic" || len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "backend" {
		t.Fatalf("tasks = %+v, want the new tagged Task", tasks)
	}
	if log, _ := r.tracker.TaskNotes(ctx, tasks[0].ID); len(log) != 1 {
		t.Errorf("log = %+v, want the filed note", log)
	}

	m = press(m, keyEsc)
	wantScreen(t, "on the list", m, "check the retry logic", "#backend")
}

func TestInbox_ATagsOnlyNoteCannotBecomeATask(t *testing.T) {
	r := newRig(t)
	r.unfile(t, "##idea")
	m := booted(r.newModel())
	m = press(m, keyI, keyC)
	wantScreen(t, "error", m, "needs a title", "##idea")
	if n := r.unfiledCount(t); n != 1 {
		t.Errorf("%d unfiled notes, want it kept", n)
	}
}

func TestInbox_EmptyingItSaysSo(t *testing.T) {
	r := newRig(t)
	r.unfile(t, "only one")
	m := booted(r.newModel())
	m = press(m, keyI, keyC)
	wantScreen(t, "emptied", m, "Inbox is empty")
	m = press(m, keyC, keyF, keyEnter) // nothing to act on
	wantScreen(t, "still fine", m, "Inbox is empty")
}

func TestInbox_FilePicksATaskAndFilesTheNoteOnIt(t *testing.T) {
	for name, open := range map[string]tea.KeyPressMsg{"f": keyF, "enter": keyEnter} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			alpha := r.addTask(t, "alpha")
			r.addTask(t, "beta")
			note := r.unfile(t, "belongs to alpha")
			m := booted(r.newModel())
			m = press(m, keyI, open)
			wantScreen(t, "picker", m, `Filing: "belongs to alpha"`, "File:", "alpha", "beta")

			m = typeText(m, "alp")
			wantNoText(t, "narrowed", m, "beta")
			m = press(m, keyEnter)

			wantScreen(t, "back in the inbox", m, "Filed onto: alpha", "Inbox is empty")
			log, _ := r.tracker.TaskNotes(ctx, alpha.ID)
			if len(log) != 1 || log[0].ID != note.ID {
				t.Errorf("alpha's log = %+v, want the filed note", log)
			}
			if n := r.unfiledCount(t); n != 0 {
				t.Errorf("%d unfiled notes, want 0", n)
			}
		})
	}
}

func TestInbox_EscFromThePickerLeavesTheNoteUnfiledInTheInbox(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "alpha")
	r.unfile(t, "stay put")
	m := booted(r.newModel())
	m = press(m, keyI, keyF)
	m = typeText(m, "alp")
	m = press(m, keyEsc)
	wantScreen(t, "inbox again", m, "stay put", "enter file")
	wantNoText(t, "picker", m, "Filing:")
	if n := r.unfiledCount(t); n != 1 {
		t.Errorf("%d unfiled notes, want 1", n)
	}
}

func TestInbox_TheFilePickerNeverOffersToCreateAndTabWidensItsScope(t *testing.T) {
	r := newRig(t)
	done := r.addTask(t, "finished work")
	r.setState(t, done, core.StateDone)
	r.unfile(t, "a note")
	m := booted(r.newModel())
	m = press(m, keyI, keyF)
	m = typeText(m, "finished")
	wantScreen(t, "active scope", m, "No matches")
	wantNoText(t, "create row", m, "Create")

	m = press(m, keyTab)
	wantScreen(t, "all scope", m, "finished work (done)")
}

func TestInbox_ANoteAddedElsewhereAppearsAndTheCursorStaysOnItsNote(t *testing.T) {
	r := newRig(t)
	r.unfile(t, "first")
	r.unfile(t, "second")
	m := booted(r.newModel())
	m = press(m, keyI, keyJ)
	wantSelected(t, "on second", m, "second")

	r.unfile(t, "third")
	m = send(m, tui.TickMsg{})
	wantScreen(t, "third listed", m, "third")
	wantSelected(t, "cursor stays", m, "second")
}

func TestNote_NCapturesAnUnfiledNoteFromTheList(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	m = press(m, keyN)
	wantScreen(t, "prompt", m, "Note:", "enter", "esc")

	m = press(m, keyEnter)
	wantScreen(t, "empty enter", m, "Write a note")
	if n := r.unfiledCount(t); n != 0 {
		t.Fatalf("%d notes after an empty enter, want 0", n)
	}

	m = typeText(m, "buy   milk")
	m = press(m, keyEnter)
	wantScreen(t, "saved", m, "Saved to the inbox", "Unfiled notes: 1")
	wantNoText(t, "prompt closed", m, "Note:")
	notes, _ := r.tracker.UnfiledNotes(ctx)
	if len(notes) != 1 || notes[0].Text != "buy milk" {
		t.Errorf("notes = %+v, want the one normalised note", notes)
	}
}

func TestNote_EscCancelsAndKeysAreText(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	m = press(m, keyN)
	m = typeText(m, "quit jk snx")
	wantScreen(t, "typed", m, "quit jk snx")
	m = press(m, keyEsc)
	wantNoText(t, "cancelled", m, "Note:")
	if n := r.unfiledCount(t); n != 0 {
		t.Errorf("%d notes, want 0", n)
	}
	if _, cmd := press(m, keyN).Update(keyCtrlC); cmd == nil {
		t.Error("ctrl+c returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
}

func TestNote_TheHandoffWaitsForTheNotePrompt(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m = press(m, keyN)
	m = typeText(m, "half typed")
	m, _ = tickAt(t, r, m, sessionEnd)
	wantNoPrompt(t, "while writing a note", m)
	wantScreen(t, "untouched", m, "half typed")
	m = press(m, keyEsc)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after", m, "Hand-off for")
}

func TestInbox_FootersFitTheTerminal(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "alpha")
	r.unfile(t, "a note that is quite a bit longer than the little terminal is wide")
	for width := 12; width <= 60; width += 4 {
		m := booted(r.newModel())
		m = send(m, tea.WindowSizeMsg{Width: width, Height: 12})
		for name, keys := range map[string][]tea.KeyPressMsg{"inbox": {keyI}, "picker": {keyI, keyF}, "note": {keyN}} {
			view := press(m, keys...)
			for i, line := range strings.Split(view.View().Content, "\n") {
				if w := ansi.StringWidth(line); w > width {
					t.Errorf("%s at width %d: line %d is %d cells: %q", name, width, i, w, line)
				}
			}
		}
	}
}
