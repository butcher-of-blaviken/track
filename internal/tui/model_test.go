package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/memory"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	ctx   = context.Background()
	epoch = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
)

type rig struct {
	tracker *core.Tracker
	store   core.Store
	clock   *clock.Fake
}

func newRig(t *testing.T) *rig { return newRigOn(t, memory.New()) }

func newRigOn(t *testing.T, store core.Store) *rig {
	t.Helper()
	fake := clock.NewFake(epoch)
	tracker, err := core.NewTracker(store, fake, core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{tracker: tracker, store: store, clock: fake}
}

// startSession creates a Task and starts a 30m session on it.
func (r *rig) startSession(t *testing.T) core.FocusSession {
	t.Helper()
	var task core.TaskID
	if err := r.store.Update(ctx, func(tx core.Tx) error {
		var err error
		task, err = tx.CreateTask(core.Task{Title: "task", State: core.StateActive, CreatedAt: epoch})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// noTick stops the model scheduling its own ticks, so tests drive time
// explicitly with TickMsg and never wait on a real timer.
func noTick() tea.Cmd { return nil }

// newModel is the single-pane layout, which most tests are about; newSplitModel
// is the one that shows the panels side by side when there is room.
func (r *rig) newModel() tea.Model {
	return tui.New(r.tracker, tui.WithTick(noTick), tui.WithLayout("single"))
}

func (r *rig) newSplitModel() tea.Model { return tui.New(r.tracker, tui.WithTick(noTick)) }

// collect runs a command and returns the messages it produces, flattening batches.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collect(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// send delivers msg to the model and then everything its commands produce.
func send(m tea.Model, msg tea.Msg) tea.Model {
	m, cmd := m.Update(msg)
	for _, produced := range collect(cmd) {
		m = send(m, produced)
	}
	return m
}

// booted returns a model that has run Init and processed the result.
func booted(m tea.Model) tea.Model {
	for _, msg := range collect(m.Init()) {
		m = send(m, msg)
	}
	return m
}

// screen is what the user reads: the view with styling removed, so tests match
// on words and not on colours.
func screen(m tea.Model) string { return ansi.Strip(m.View().Content) }

func wantScreen(t *testing.T, what string, m tea.Model, parts ...string) {
	t.Helper()
	got := screen(m)
	for _, part := range parts {
		if !strings.Contains(got, part) {
			t.Errorf("%s: screen lacks %q:\n%s", what, part, got)
		}
	}
}

func TestView_ShowsLoadingUntilTheFirstSnapshotArrives(t *testing.T) {
	m := newRig(t).newModel()
	wantScreen(t, "before Init", m, "Loading")
}

func TestView_IdleWhenNothingIsRunning(t *testing.T) {
	m := booted(newRig(t).newModel())
	wantScreen(t, "empty store", m, "Idle")
	if strings.Contains(screen(m), "Loading") {
		t.Error("still loading after the first snapshot")
	}
}

func TestView_FocusShowsACountdownThatShrinksOnEachTick(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	wantScreen(t, "just started", m, "Focus", "30:00")

	r.clock.Advance(5*time.Minute + 50*time.Second)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after a tick", m, "Focus", "24:10")
}

func TestView_CountdownRoundsUpToWholeSeconds(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.clock.Advance(30*time.Minute - 500*time.Millisecond)
	m := booted(r.newModel())
	wantScreen(t, "half a second left", m, "Focus", "00:01")
}

func TestView_BreakAndHandoffDue(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())

	r.clock.Advance(34 * time.Minute) // 4m into the 10m Break
	m = send(m, tui.TickMsg{})
	wantScreen(t, "in the Break", m, "Break", "06:00", "Hand-off due")

	if _, err := r.tracker.AddHandoffNote(ctx, "left off at the parser"); err != nil {
		t.Fatal(err)
	}
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after the note", m, "Break", "06:00")
	if strings.Contains(screen(m), "Hand-off due") {
		t.Errorf("hand-off still shown after it was resolved:\n%s", screen(m))
	}

	r.clock.Advance(10 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after the Break", m, "Idle")
}

func TestUpdate_QuitsOnQAndCtrlC(t *testing.T) {
	m := newRig(t).newModel()
	for name, key := range map[string]tea.KeyPressMsg{
		"q":      {Code: 'q', Text: "q"},
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	} {
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Errorf("%s: no command returned, want quit", name)
			continue
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: command did not produce a QuitMsg", name)
		}
	}

	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("an unrelated key quit the program")
		}
	}
}

func TestView_FitsAnyTerminalSizeWithoutPanicking(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.addTask(t, "a task with a rather long title to force truncation ##some-long-tag-name")
	r.clock.Advance(34 * time.Minute) // Break with a hand-off due: the busiest status
	m := booted(r.newModel())
	m = press(m, keyA) // and the add prompt open
	m = typeText(m, "typing a long long long long long long long long line")

	for _, size := range []tea.WindowSizeMsg{{Width: 0, Height: 0}, {Width: 1, Height: 1}, {Width: 10, Height: 3}, {Width: 80, Height: 24}, {Width: 300, Height: 100}} {
		m = send(m, size)
		lines := strings.Split(screen(m), "\n")
		if size.Height > 0 && len(lines) > size.Height {
			t.Errorf("%dx%d: %d lines, want at most %d", size.Width, size.Height, len(lines), size.Height)
		}
		for i, line := range lines {
			if w := ansi.StringWidth(line); size.Width > 0 && w > size.Width {
				t.Errorf("%dx%d: line %d is %d cells wide: %q", size.Width, size.Height, i, w, line)
			}
		}
	}
}

// failingStore fails the read the snapshot depends on.
type failingStore struct{ core.Store }

func (failingStore) LatestSession(context.Context) (core.FocusSession, error) {
	return core.FocusSession{}, errors.New("disk on fire")
}

func TestView_ShowsAnErrorAndKeepsTicking(t *testing.T) {
	r := newRigOn(t, failingStore{memory.New()})
	m := booted(r.newModel())
	wantScreen(t, "store failure", m, "disk on fire")
	m = send(m, tui.TickMsg{}) // must not panic or wedge
	wantScreen(t, "still failing", m, "disk on fire")
}
