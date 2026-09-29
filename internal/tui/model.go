// Package tui is the Bubble Tea adapter over the core. It holds no domain
// logic: it asks the core for a Snapshot once a second and renders it.
package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const tickEvery = time.Second

// TickMsg asks the model to refresh its snapshot. The model schedules its own
// ticks; it is exported so tests can drive time by hand.
type TickMsg time.Time

// snapshotMsg carries the result of a Snapshot read back into Update.
type snapshotMsg struct {
	snap core.Snapshot
	err  error
}

// Option customises a Model.
type Option func(*Model)

// WithTick replaces how the next tick is scheduled. Tests pass a function that
// returns nil, so no real timer runs and TickMsg is sent by hand.
func WithTick(tick func() tea.Cmd) Option {
	return func(m *Model) { m.tick = tick }
}

// Model is the Bubble Tea model for the main screen.
type Model struct {
	tracker       *core.Tracker
	tick          func() tea.Cmd
	snap          core.Snapshot
	loaded        bool
	err           error
	width, height int
}

// New returns a Model that reads its state from tracker.
func New(tracker *core.Tracker, opts ...Option) Model {
	m := Model{tracker: tracker, tick: defaultTick}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func defaultTick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return TickMsg(t) })
}

// fetch reads a Snapshot off the UI goroutine's critical path.
func (m Model) fetch() tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		snap, err := tracker.Snapshot(context.Background())
		return snapshotMsg{snap: snap, err: err}
	}
}

// Init implements tea.Model: read the first snapshot and start ticking.
func (m Model) Init() tea.Cmd { return tea.Batch(m.fetch(), m.tick()) }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		return m, tea.Batch(m.fetch(), m.tick())
	case snapshotMsg:
		// On error keep the last good snapshot and show the error; the next
		// tick tries again.
		m.err = msg.err
		if msg.err == nil {
			m.snap, m.loaded = msg.snap, true
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}
