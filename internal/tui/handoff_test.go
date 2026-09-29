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

var keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

func (r *rig) handoffPending(t *testing.T) bool {
	t.Helper()
	snap, err := r.tracker.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return snap.HandoffPending
}

func (r *rig) taskNotes(t *testing.T) []core.Note {
	t.Helper()
	notes, err := r.store.Notes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func wantNoPrompt(t *testing.T, what string, m tea.Model) {
	t.Helper()
	if strings.Contains(screen(m), "Hand-off for") {
		t.Errorf("%s: the hand-off prompt is open:\n%s", what, screen(m))
	}
}

// endedModel is a model watching a session that completed a while ago.
func endedModel(t *testing.T, r *rig) tea.Model {
	t.Helper()
	r.startSession(t)
	m := booted(r.newModel())
	r.clock.Set(epoch.Add(sessionEnd + time.Minute))
	return send(m, tui.TickMsg{})
}

func TestHandoff_OpensWhenASessionCompletes(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())

	m, _ = tickAt(t, r, m, sessionEnd-time.Second)
	wantNoPrompt(t, "one second before the end", m)

	m, _ = tickAt(t, r, m, sessionEnd)
	wantScreen(t, "at the end", m, "Hand-off for: task", "Note:")
}

func TestHandoff_OpensAfterAnEarlyStop(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "some work")
	m := booted(r.newModel())
	m = press(m, keyS)
	wantNoPrompt(t, "while running", m)

	r.clock.Advance(10 * time.Minute)
	m = press(m, keyX)
	wantScreen(t, "after stopping", m, "Hand-off for: some work", "Note:")
}

func TestHandoff_OpensOnLaunchForASessionThatEndedWhileAway(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.clock.Set(epoch.Add(sessionEnd + 3*time.Minute))
	m := booted(r.newModel())
	wantScreen(t, "on launch", m, "Hand-off for: task", "ended 3m ago")
}

func TestHandoff_DoesNotOpenWhenNothingIsPending(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	wantNoPrompt(t, "empty store", m)

	m = endedModel(t, r)
	m = press(m, keyEsc)
	m = send(m, tui.TickMsg{})
	wantNoPrompt(t, "after it was skipped", m)
}

func TestHandoff_EnterSavesTheNoteOnTheSessionsTask(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)

	m = typeText(m, "left  off at the parser")
	m = press(m, keyEnter)

	notes := r.taskNotes(t)
	if len(notes) != 1 || notes[0].Text != "left off at the parser" || notes[0].TaskID == 0 || notes[0].SessionID == 0 {
		t.Fatalf("stored notes = %+v, want one filed, session-linked, normalised note", notes)
	}
	wantNoPrompt(t, "after saving", m)
	if strings.Contains(screen(m), "Hand-off due") {
		t.Errorf("hand-off still due after saving:\n%s", screen(m))
	}
	m = send(m, tui.TickMsg{})
	wantNoPrompt(t, "on the next tick", m)
}

func TestHandoff_EscSkipsWithoutANoteAndStaysSkipped(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)

	m = press(m, keyEsc)
	if n := len(r.taskNotes(t)); n != 0 {
		t.Errorf("%d notes stored after skipping, want 0", n)
	}
	if r.handoffPending(t) {
		t.Error("hand-off still pending after skipping")
	}
	wantNoPrompt(t, "after skipping", m)
	for range 3 {
		m = send(m, tui.TickMsg{})
	}
	wantNoPrompt(t, "on later ticks", m)
	if strings.Contains(screen(m), "Hand-off due") {
		t.Errorf("hand-off still due after skipping:\n%s", screen(m))
	}
}

func TestHandoff_EmptyEnterKeepsThePromptOpenAndDoesNotSkip(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)

	m = press(m, keyEnter)
	wantScreen(t, "after an empty enter", m, "Hand-off for", "Write a note, or press esc to skip")
	if !r.handoffPending(t) {
		t.Fatal("an empty enter resolved the hand-off")
	}

	m = typeText(m, "second try")
	m = press(m, keyEnter)
	wantNoPrompt(t, "after a real note", m)
	if notes := r.taskNotes(t); len(notes) != 1 || notes[0].Text != "second try" {
		t.Errorf("stored notes = %+v, want the second try", notes)
	}
}

func TestHandoff_WaitsForTheAddPromptThenOpens(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m = press(m, keyA)
	m = typeText(m, "half typed")

	m, _ = tickAt(t, r, m, sessionEnd)
	wantNoPrompt(t, "while adding a task", m)
	wantScreen(t, "the add prompt is untouched", m, "Add task:", "half typed")

	m = press(m, keyEsc)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after the add prompt closed", m, "Hand-off for: task")
}

func TestHandoff_ClosesWhenResolvedElsewhere(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)
	wantScreen(t, "open", m, "Hand-off for")

	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	m = send(m, tui.TickMsg{})
	wantNoPrompt(t, "after another process skipped it", m)
}

func TestHandoff_KeysAreTextAndOnlyCtrlCQuits(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)

	m = typeText(m, "quit j k a s x")
	wantScreen(t, "typed text", m, "quit j k a s x")
	if n := len(r.storedSessions(t)); n != 1 {
		t.Errorf("%d sessions after typing s, want 1", n)
	}

	if _, cmd := m.Update(keyCtrlC); cmd == nil {
		t.Error("ctrl+c returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
	if !r.handoffPending(t) {
		t.Error("quitting resolved the hand-off; it should stay pending for the next launch")
	}
}

func TestHandoff_FooterShowsTheKeysAndNothingOverflows(t *testing.T) {
	r := newRig(t)
	m := endedModel(t, r)
	wantScreen(t, "footer", m, "enter save", "esc skip")

	m = typeText(m, "a rather long note to make the prompt line overflow a narrow terminal")
	for width := 8; width <= 80; width += 6 {
		m = send(m, tea.WindowSizeMsg{Width: width, Height: 24})
		for i, line := range strings.Split(screen(m), "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("width %d: line %d is %d cells: %q", width, i, w, line)
			}
		}
	}
}

func TestHandoff_TheFirstKeySilencesTheBellAndIsStillTyped(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m, n := tickAt(t, r, m, sessionEnd)
	if n != 1 {
		t.Fatalf("rang %d, want 1", n)
	}
	wantScreen(t, "banner and prompt together", m, "Focus complete", "Hand-off for")

	m = typeText(m, "zq")
	if strings.Contains(screen(m), "Focus complete") {
		t.Errorf("the banner survived a key:\n%s", screen(m))
	}
	wantScreen(t, "the key was typed", m, "zq")
	if _, n = tickAt(t, r, m, sessionEnd+30*time.Second); n != 0 {
		t.Errorf("rang %d after being silenced, want 0", n)
	}
}
