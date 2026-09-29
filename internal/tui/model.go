// Package tui is the Bubble Tea adapter over the core. It holds no domain
// logic: it asks the core for a Snapshot and the Active Tasks once a second,
// renders them, and turns key presses into core calls.
package tui

import (
	"context"
	"errors"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const tickEvery = time.Second

// TickMsg asks the model to refresh from the core. The model schedules its own
// ticks; it is exported so tests can drive time by hand.
type TickMsg time.Time

// refreshMsg carries the result of reading the core back into Update.
type refreshMsg struct {
	snap  core.Snapshot
	tasks []core.Task
	err   error
}

// addedMsg carries the result of creating a Task.
type addedMsg struct {
	task core.Task
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
	tracker *core.Tracker
	tick    func() tea.Cmd

	snap   core.Snapshot
	tasks  []core.Task
	loaded bool
	err    error

	width, height int

	// The cursor is tracked by Task ID so it stays on its Task when the list
	// changes underneath it; cursor is that Task's row, and top is the first
	// visible row.
	selected core.TaskID
	cursor   int
	top      int

	adding bool
	input  textinput.Model
	addErr string
}

// New returns a Model that reads its state from tracker.
func New(tracker *core.Tracker, opts ...Option) Model {
	input := textinput.New()
	input.Prompt = ""
	// The leading space is under the terminal's cursor, so no hint text is hidden.
	input.Placeholder = " what are you working on? add ##tag to label it"
	// Plain and steady: draw no styles, and let the terminal show the cursor.
	styles := textinput.Styles{}
	styles.Cursor.Blink = false
	input.SetStyles(styles)
	input.SetVirtualCursor(false)

	m := Model{tracker: tracker, tick: defaultTick, input: input}
	for _, opt := range opts {
		opt(&m)
	}
	m.sizeInput()
	return m
}

// sizeInput gives the prompt the width left after its label. Without a width
// the input draws only the first character of its placeholder and does not
// scroll long text. One column is kept free for the cursor at the end of the text.
func (m *Model) sizeInput() {
	width := len([]rune(m.input.Placeholder)) + 1 // terminal size not known yet
	if m.width > 0 {
		width = m.width - ansi.StringWidth(promptLabel) - 1
	}
	m.input.SetWidth(max(width, 1))
}

func defaultTick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return TickMsg(t) })
}

// fetch reads the snapshot and the Active Tasks off the UI goroutine.
func (m Model) fetch() tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		ctx := context.Background()
		snap, err := tracker.Snapshot(ctx)
		if err != nil {
			return refreshMsg{err: err}
		}
		tasks, err := tracker.Tasks(ctx, core.StateActive)
		return refreshMsg{snap: snap, tasks: tasks, err: err}
	}
}

// add creates a Task from text off the UI goroutine.
func (m Model) add(text string) tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		task, err := tracker.AddTask(context.Background(), text)
		return addedMsg{task: task, err: err}
	}
}

// Init implements tea.Model: read the first state and start ticking.
func (m Model) Init() tea.Cmd { return tea.Batch(m.fetch(), m.tick()) }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		return m, tea.Batch(m.fetch(), m.tick())
	case refreshMsg:
		// On error keep the last good state and show the error; the next tick
		// tries again.
		m.err = msg.err
		if msg.err == nil {
			m.snap, m.tasks, m.loaded = msg.snap, msg.tasks, true
			m.reselect()
		}
		return m, nil
	case addedMsg:
		if msg.err != nil {
			m.addErr = describe(msg.err)
			return m, nil
		}
		m.closePrompt()
		m.selected = msg.task.ID // the refresh puts the cursor on it
		return m, m.fetch()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sizeInput()
		m.scrollToCursor()
		return m, nil
	case tea.KeyPressMsg:
		if m.adding {
			return m.updatePrompt(msg)
		}
		return m.updateList(msg)
	}
	if m.adding {
		// Anything else, such as a paste, belongs to the prompt.
		return m.forwardToPrompt(msg)
	}
	return m, nil
}

func (m Model) updateList(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "a":
		m.adding, m.addErr = true, ""
		m.input.Reset()
		return m, m.input.Focus()
	}
	return m, nil
}

func (m Model) updatePrompt(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.closePrompt()
		return m, nil
	case "enter":
		return m, m.add(m.input.Value())
	}
	m.addErr = ""
	return m.forwardToPrompt(key)
}

func (m Model) forwardToPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) closePrompt() {
	m.adding, m.addErr = false, ""
	m.input.Reset()
	m.input.Blur()
}

func (m *Model) moveCursor(delta int) {
	if len(m.tasks) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.tasks)-1)
	m.selected = m.tasks[m.cursor].ID
	m.scrollToCursor()
}

// reselect puts the cursor back on the selected Task after the list changed,
// or clamps it to the list if that Task is gone.
func (m *Model) reselect() {
	for i, task := range m.tasks {
		if task.ID == m.selected {
			m.cursor = i
			m.scrollToCursor()
			return
		}
	}
	m.cursor = min(max(m.cursor, 0), max(len(m.tasks)-1, 0))
	m.selected = 0
	if len(m.tasks) > 0 {
		m.selected = m.tasks[m.cursor].ID
	}
	m.scrollToCursor()
}

func (m *Model) scrollToCursor() {
	m.top = clampTop(m.top, m.cursor, m.listRows(), len(m.tasks))
}

// clampTop returns the first visible row so that the cursor row is inside a
// window of rows rows over n items, moving the window as little as possible.
func clampTop(top, cursor, rows, n int) int {
	if cursor < top {
		top = cursor
	}
	if cursor >= top+rows {
		top = cursor - rows + 1
	}
	return max(min(top, n-rows), 0)
}

func describe(err error) string {
	if errors.Is(err, core.ErrEmptyTitle) {
		return "A task needs a title (tags alone don't count)"
	}
	return err.Error()
}
