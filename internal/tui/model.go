// Package tui is the Bubble Tea adapter over the core. It holds no domain
// logic: it asks the core for a Snapshot and the Active Tasks once a second,
// renders them, and turns key presses into core calls.
package tui

import (
	"context"
	"errors"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

const (
	// addPlaceholder's leading space is under the terminal's cursor, so no hint
	// text is hidden.
	addPlaceholder = " what are you working on? add ##tag to label it"
	tickEvery      = time.Second
	// defaultFocusDuration is used until the config and flags set one.
	defaultFocusDuration = 30 * time.Minute
)

// TickMsg asks the model to refresh from the core. The model schedules its own
// ticks; it is exported so tests can drive time by hand.
type TickMsg time.Time

// refreshMsg carries the result of reading the core back into Update.
type refreshMsg struct {
	snap  core.Snapshot
	tasks []core.Task
	// sessionTask is the Task of the latest session, running or ended.
	sessionTask *core.Task
	unfiled     int
	err         error
}

// sessionMsg carries the result of starting or stopping a session.
type sessionMsg struct{ err error }

// handoffMsg carries the result of saving or skipping a Hand-off note.
type handoffMsg struct {
	session core.SessionID
	err     error
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

// WithFocusDuration sets how long a started Focus session runs.
func WithFocusDuration(d time.Duration) Option {
	return func(m *Model) { m.focusDuration = d }
}

// Model is the Bubble Tea model for the main screen.
type Model struct {
	tracker *core.Tracker
	tick    func() tea.Cmd

	focusDuration time.Duration

	snap        core.Snapshot
	tasks       []core.Task
	sessionTask *core.Task
	unfiled     int
	loaded      bool
	err         error
	// notice is a one-line message about the last action, cleared on the next key.
	notice string

	width, height int

	// The cursor is tracked by Task ID so it stays on its Task when the list
	// changes underneath it; cursor is that Task's row, and top is the first
	// visible row.
	selected core.TaskID
	cursor   int
	top      int

	// mode is which prompt, if any, has the keyboard. The add-task and hand-off
	// prompts share input, and promptErr is the line under either.
	mode      mode
	input     textinput.Model
	promptErr string

	// handoffFor is the session the open hand-off prompt is for, and
	// handoffResolved the last one this program saved or skipped, so a refresh
	// that was already in flight cannot reopen the prompt for it.
	handoffFor, handoffResolved core.SessionID

	// breakStart is the Task the open Break confirmation would start a session on.
	breakStart core.TaskID

	keys keyMap
	help help.Model

	bell ringState
}

// mode is which prompt has the keyboard.
type mode int

const (
	modeList mode = iota
	modeAdd
	modeHandoff
	modeConfirmBreak
)

// bellID identifies one bell event: the end of a session, or of its Break.
type bellID struct {
	session core.SessionID
	kind    core.BellKind
}

// ringState is the acknowledgement state the core leaves to the UI, for the
// latest bell event only: how many rings this program has made, and whether the
// user has silenced it.
type ringState struct {
	id    bellID
	rung  int
	acked bool
}

// New returns a Model that reads its state from tracker.
func New(tracker *core.Tracker, opts ...Option) Model {
	input := textinput.New()
	input.Prompt = ""
	// The leading space is under the terminal's cursor, so no hint text is hidden.
	input.Placeholder = addPlaceholder
	// Typed text is plain, the hint is greyed out like a prompt (the same mid
	// grey Bubbles uses, readable on light and dark terminals), and the
	// terminal draws a steady cursor.
	grey := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styles := textinput.Styles{}
	styles.Focused.Placeholder, styles.Blurred.Placeholder = grey, grey
	styles.Cursor.Blink = false
	input.SetStyles(styles)
	input.SetVirtualCursor(false)

	// The footer is the same unobtrusive grey as the prompt's hint.
	footer := help.New()
	footer.Styles.ShortKey, footer.Styles.ShortDesc = grey, grey
	footer.Styles.ShortSeparator, footer.Styles.Ellipsis = grey, grey

	m := Model{tracker: tracker, tick: defaultTick, input: input, focusDuration: defaultFocusDuration, keys: newKeyMap(), help: footer}
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
		width = m.width - ansi.StringWidth(m.promptLabel()) - 1
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
		msg := refreshMsg{snap: snap}
		if msg.tasks, msg.err = tracker.Tasks(ctx, core.StateActive); msg.err != nil {
			return msg
		}
		if msg.unfiled, msg.err = tracker.UnfiledNoteCount(ctx); msg.err != nil {
			return msg
		}
		if snap.Session != nil {
			task, err := tracker.Task(ctx, snap.Session.TaskID)
			if err != nil {
				return refreshMsg{err: err}
			}
			msg.sessionTask = &task
		}
		return msg
	}
}

// start starts a session on a Task off the UI goroutine.
// override starts it even though a Break is running, and the core records that.
func (m Model) start(id core.TaskID, override bool) tea.Cmd {
	tracker, planned := m.tracker, m.focusDuration
	return func() tea.Msg {
		_, err := tracker.StartSession(context.Background(), id, planned, core.StartOptions{OverrideBreak: override})
		return sessionMsg{err: err}
	}
}

// stop ends the running session early off the UI goroutine.
func (m Model) stop() tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		_, err := tracker.StopSession(context.Background())
		return sessionMsg{err: err}
	}
}

// saveHandoff writes the hand-off note for a session off the UI goroutine.
func (m Model) saveHandoff(session core.SessionID, text string) tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		_, err := tracker.AddHandoffNote(context.Background(), text)
		return handoffMsg{session: session, err: err}
	}
}

// skipHandoff resolves a session's hand-off without a note off the UI goroutine.
func (m Model) skipHandoff(session core.SessionID) tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		return handoffMsg{session: session, err: tracker.SkipHandoff(context.Background())}
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
			m.snap, m.tasks, m.sessionTask, m.unfiled, m.loaded = msg.snap, msg.tasks, msg.sessionTask, msg.unfiled, true
			m.reselect()
			return m, tea.Batch(m.ring(), m.syncHandoff())
		}
		return m, nil
	case sessionMsg:
		if msg.err != nil {
			m.notice = m.describeSession(msg.err)
			return m, nil
		}
		return m, m.fetch()
	case handoffMsg:
		// Nothing pending means someone else resolved it: the prompt is done either way.
		if msg.err != nil && !errors.Is(msg.err, core.ErrNoHandoffPending) {
			m.promptErr = describeHandoff(msg.err)
			return m, nil
		}
		m.handoffResolved = msg.session
		m.closePrompt()
		return m, m.fetch()
	case addedMsg:
		if msg.err != nil {
			m.promptErr = describe(msg.err)
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
		m.acknowledgeBell() // any key silences a ringing bell; it is still handled below
		switch m.mode {
		case modeAdd:
			return m.updatePrompt(msg)
		case modeHandoff:
			return m.updateHandoff(msg)
		case modeConfirmBreak:
			return m.updateConfirmBreak(msg)
		}
		return m.updateList(msg)
	}
	if m.mode != modeList {
		// Anything else, such as a paste, belongs to the prompt.
		return m.forwardToPrompt(msg)
	}
	return m, nil
}

// syncHandoff opens the hand-off prompt when one is due and the keyboard is
// free, and closes it once the hand-off is resolved, e.g. by another process.
// It never takes the keyboard from the add-task prompt.
func (m *Model) syncHandoff() tea.Cmd {
	switch {
	case m.mode == modeHandoff && !m.snap.HandoffPending:
		m.closePrompt()
	case m.mode == modeList && m.snap.HandoffPending && m.snap.Session.ID != m.handoffResolved:
		m.mode, m.promptErr, m.handoffFor = modeHandoff, "", m.snap.Session.ID
		m.input.Reset()
		m.input.Placeholder = " where did you leave off?"
		m.sizeInput()
		return m.input.Focus()
	}
	return nil
}

// ring returns the command that rings the terminal bell when the core says
// more rings are due for the current event than have been made. It rings once
// per refresh however many are due, so reopening the app or a stalled tick does
// not fire a burst.
func (m *Model) ring() tea.Cmd {
	b := m.snap.Bell
	if b == nil {
		m.bell = ringState{}
		return nil
	}
	if id := (bellID{session: b.Session, kind: b.Kind}); m.bell.id != id {
		m.bell = ringState{id: id}
	}
	if m.bell.acked || b.Scheduled <= m.bell.rung {
		return nil
	}
	m.bell.rung = b.Scheduled
	return tea.Raw("\a")
}

func (m *Model) acknowledgeBell() {
	if m.snap.Bell != nil {
		m.bell.acked = true
	}
}

func (m Model) updateList(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch {
	case key.Matches(press, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Start):
		if len(m.tasks) == 0 {
			return m, nil
		}
		id := m.tasks[m.cursor].ID
		if m.snap.Phase == core.PhaseBreak {
			m.mode, m.breakStart = modeConfirmBreak, id
			return m, nil
		}
		return m, m.start(id, false)
	case key.Matches(press, m.keys.Stop):
		return m, m.stop()
	case key.Matches(press, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(press, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(press, m.keys.Add):
		m.mode, m.promptErr = modeAdd, ""
		m.input.Reset()
		m.input.Placeholder = addPlaceholder
		m.sizeInput()
		return m, m.input.Focus()
	}
	return m, nil
}

func (m Model) updatePrompt(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(press, m.keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Cancel):
		m.closePrompt()
		return m, nil
	case key.Matches(press, m.keys.Submit):
		return m, m.add(m.input.Value())
	}
	m.promptErr = ""
	return m.forwardToPrompt(press)
}

func (m Model) updateHandoff(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(press, m.keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Skip):
		return m, m.skipHandoff(m.handoffFor)
	case key.Matches(press, m.keys.Save):
		return m, m.saveHandoff(m.handoffFor, m.input.Value())
	}
	m.promptErr = ""
	return m.forwardToPrompt(press)
}

// updateConfirmBreak handles the Break-override confirmation. It is a yes/no
// question, not a text prompt, so every key other than the answers is ignored.
func (m Model) updateConfirmBreak(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(press, m.keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Confirm):
		m.mode = modeList
		// Always ask the core to override: if the Break ended meanwhile it
		// records nothing skipped.
		return m, m.start(m.breakStart, true)
	case key.Matches(press, m.keys.Decline):
		m.mode = modeList
	}
	return m, nil
}

func (m Model) forwardToPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) closePrompt() {
	m.mode, m.promptErr = modeList, ""
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

func describeHandoff(err error) string {
	if errors.Is(err, core.ErrEmptyNote) {
		return "Write a note, or press esc to skip"
	}
	return err.Error()
}

// describeSession words the reasons a start or stop can be refused.
func (m Model) describeSession(err error) string {
	switch {
	case errors.Is(err, core.ErrSessionRunning):
		return "A session is already running. Stop it first (x)."
	case errors.Is(err, core.ErrBreakActive):
		if m.snap.Phase == core.PhaseBreak {
			return "A break is in progress (" + clockText(m.snap.Remaining) + " left)."
		}
		return "A break is in progress."
	case errors.Is(err, core.ErrNoSessionRunning):
		return "Nothing to stop."
	case errors.Is(err, core.ErrTaskNotActive):
		return "That task is not active."
	}
	return err.Error()
}
