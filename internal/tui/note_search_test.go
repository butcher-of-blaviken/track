package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

// noteRig has two Active Tasks and a note on the older one.
func noteRig(t *testing.T) (*rig, tea.Model) {
	t.Helper()
	r := newRig(t)
	prd := r.addTask(t, "write the PRD ##docs")
	r.addTask(t, "fix the build")
	if _, err := r.tracker.AddNote(ctx, prd.ID, "left off at the parser"); err != nil {
		t.Fatal(err)
	}
	return r, booted(r.newModel())
}

func TestNoteSearch_FindsATaskByANoteAndShowsTheNoteLineWithItsAge(t *testing.T) {
	_, m := noteRig(t)
	wantNoText(t, "before searching", m, "left off")

	m = press(m, keySlash)
	m = typeText(m, "parser")
	wantScreen(t, "note match", m, "write the PRD", "↳ left off at the parser", "(just now)")
	wantNoText(t, "the other Task", m, "fix the build")

	// The note line sits directly under its row.
	lines := strings.Split(screen(m), "\n")
	for i, line := range lines {
		if strings.Contains(line, "write the PRD") {
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "↳ left off at the parser") {
				t.Errorf("the note line does not follow its row:\n%s", screen(m))
			}
		}
	}
}

func TestNoteSearch_NoNoteLineWhenTheTitleMatched(t *testing.T) {
	_, m := noteRig(t)
	m = press(m, keySlash)
	m = typeText(m, "prd")
	wantScreen(t, "title match", m, "write the PRD")
	wantNoText(t, "title match", m, "↳")
}

func TestNoteSearch_AnAppliedFilterKeepsTheNoteLines(t *testing.T) {
	_, m := noteRig(t)
	m = press(m, keySlash)
	m = typeText(m, "parser")
	m = press(m, keyEnter)
	wantScreen(t, "applied", m, "Filter: parser", "↳ left off at the parser")
	m = press(m, keyEsc)
	wantNoText(t, "cleared", m, "↳")
}

func TestNoteSearch_ANoteAddedElsewhereShowsUpOnTheNextTick(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "some task")
	m := booted(r.newModel())
	m = press(m, keySlash)
	m = typeText(m, "widget")
	wantScreen(t, "no match yet", m, "No matches")

	if _, err := r.tracker.AddNote(ctx, task.ID, "rewrite the widget"); err != nil {
		t.Fatal(err)
	}
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after the tick", m, "some task", "↳ rewrite the widget")
}

func TestNoteSearch_TheNoteLineStaysWithItsRowWhileScrolling(t *testing.T) {
	r := newRig(t)
	for i := range 12 {
		task := r.addTask(t, "task "+string(rune('a'+i)))
		if _, err := r.tracker.AddNote(ctx, task.ID, "shared phrase "+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	const height = 11
	m := booted(r.newModel())
	m = send(m, tea.WindowSizeMsg{Width: 60, Height: height})
	m = press(m, keySlash)
	m = typeText(m, "shared phrase")
	for step := range 12 {
		lines := strings.Split(screen(m), "\n")
		if len(lines) > height {
			t.Fatalf("step %d: %d lines on a %d-line terminal:\n%s", step, len(lines), height, screen(m))
		}
		found := false
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), ">") {
				found = true
				if i+1 >= len(lines) || !strings.Contains(lines[i+1], "↳ shared phrase") {
					t.Errorf("step %d: the selected row has no note line under it:\n%s", step, screen(m))
				}
			}
		}
		if !found {
			t.Fatalf("step %d: the selected row is off screen:\n%s", step, screen(m))
		}
		m = press(m, keyDown)
	}
}

func wantNoText(t *testing.T, what string, m tea.Model, text string) {
	t.Helper()
	if strings.Contains(screen(m), text) {
		t.Errorf("%s: the screen contains %q:\n%s", what, text, screen(m))
	}
}
