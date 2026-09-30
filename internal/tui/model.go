// Package tui is the Bubble Tea adapter over the core. It holds no domain
// logic: it asks the core for a Snapshot and the Active Tasks once a second,
// renders them, and turns key presses into core calls.
package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	// all is whether tasks holds every state or only the Active Tasks.
	all bool
	// sessionTask is the Task of the latest session, running or ended.
	sessionTask *core.Task
	unfiled     int
	// lastNote is the newest note of sessionTask, or nil. It is read only until
	// the resume prompt has been decided.
	lastNote *core.Note
	err      error
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
	// thenStart says the Task was created from the picker to start a session on.
	thenStart bool
	err       error
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

	// resume is where the launch resume prompt is, and resumeNote the last
	// note it shows. The Task it offers is sessionTask.
	resume     resumeState
	resumeNote *core.Note

	// filter is the applied list filter, empty for none. While the picker is
	// open its own text is the query. pick is the open picker's purpose, and
	// pickFrom the Task the cursor was on when it opened, for cancelling.
	filter string
	// showAll includes Done and Archived Tasks in the list and the picker.
	showAll  bool
	pick     pickPurpose
	pickFrom core.TaskID

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
	modeResume
	modePicker
)

// pickPurpose is what the open picker is for.
type pickPurpose int

const (
	// pickFilter narrows the list and keeps it narrowed on Enter.
	pickFilter pickPurpose = iota
	// pickStart starts a session on the chosen Task, or on a new one.
	pickStart
)

// pickPlaceholders are the picker's hints, by purpose.
var pickPlaceholders = map[pickPurpose]string{
	pickFilter: " type to narrow the list",
	pickStart:  " pick a task to start, or type a new one",
}

// resumeState tracks the once-per-launch offer to resume the last Task.
type resumeState int

const (
	// resumeUndecided is before the first snapshot has arrived.
	resumeUndecided resumeState = iota
	// resumeArmed means the app launched Idle after a session, so the offer is
	// made as soon as the keyboard is free and no hand-off is due.
	resumeArmed
	// resumeOver means the offer was made or can no longer be made this run.
	resumeOver
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
	tracker, wantNote, all := m.tracker, m.resume != resumeOver, m.showAll
	return func() tea.Msg {
		ctx := context.Background()
		snap, err := tracker.Snapshot(ctx)
		if err != nil {
			return refreshMsg{err: err}
		}
		msg := refreshMsg{snap: snap, all: all}
		states := []core.State{core.StateActive}
		if all {
			states = nil
		}
		if msg.tasks, msg.err = tracker.Tasks(ctx, states...); msg.err != nil {
			return msg
		}
		// Newest first within each state, Active before Done before Archived.
		slices.SortStableFunc(msg.tasks, func(a, b core.Task) int { return int(a.State) - int(b.State) })
		if msg.unfiled, msg.err = tracker.UnfiledNoteCount(ctx); msg.err != nil {
			return msg
		}
		if snap.Session != nil {
			task, err := tracker.Task(ctx, snap.Session.TaskID)
			if err != nil {
				return refreshMsg{err: err}
			}
			msg.sessionTask = &task
			if wantNote {
				notes, err := tracker.TaskNotes(ctx, task.ID)
				if err != nil {
					return refreshMsg{err: err}
				}
				if len(notes) > 0 {
					msg.lastNote = &notes[len(notes)-1]
				}
			}
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
func (m Model) add(text string, thenStart bool) tea.Cmd {
	tracker := m.tracker
	return func() tea.Msg {
		task, err := tracker.AddTask(context.Background(), text)
		return addedMsg{task: task, thenStart: thenStart, err: err}
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
			m.snap, m.sessionTask, m.unfiled, m.loaded = msg.snap, msg.sessionTask, msg.unfiled, true
			if msg.all == m.showAll { // one fetched before a toggle carries the old scope
				m.tasks = msg.tasks
			}
			m.reselect()
			cmd := tea.Batch(m.ring(), m.syncHandoff())
			m.syncResume(msg.lastNote)
			return m, cmd
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
		if msg.thenStart {
			next, cmd := m.startTask(msg.task.ID)
			return next, tea.Batch(cmd, m.fetch())
		}
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
		case modeResume:
			return m.updateResume(msg)
		case modePicker:
			return m.updatePicker(msg)
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
		rows := m.rows()
		if len(rows) == 0 {
			return m, nil
		}
		return m.startTask(rows[min(m.cursor, len(rows)-1)].Task.ID)
	case key.Matches(press, m.keys.All):
		m.showAll = !m.showAll
		return m, m.fetch()
	case key.Matches(press, m.keys.Find):
		return m.openPicker(pickFilter)
	case key.Matches(press, m.keys.Pick):
		return m.openPicker(pickStart)
	case key.Matches(press, m.keys.Cancel) && m.filter != "":
		m.filter = ""
		m.reselect()
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

// startTask starts a session on a Task, asking first if a Break is running.
func (m Model) startTask(id core.TaskID) (tea.Model, tea.Cmd) {
	for _, task := range m.tasks {
		if task.ID == id && task.State != core.StateActive {
			m.notice = m.describeSession(core.ErrTaskNotActive)
			return m, nil
		}
	}
	if m.snap.Phase == core.PhaseBreak {
		m.mode, m.breakStart = modeConfirmBreak, id
		return m, nil
	}
	return m, m.start(id, false)
}

// rows is what the list shows, best match first: every Active Task when there
// is no query, and otherwise the Tasks matching the open picker's text or, when
// it is closed, the applied filter.
func (m Model) rows() []core.TaskMatch {
	query := m.filter
	if m.mode == modePicker {
		query = m.input.Value()
	}
	return core.SearchTasks(query, m.tasks)
}

func (m Model) openPicker(purpose pickPurpose) (tea.Model, tea.Cmd) {
	m.mode, m.pick, m.pickFrom, m.promptErr = modePicker, purpose, m.selected, ""
	m.input.Reset()
	m.input.Placeholder = pickPlaceholders[purpose]
	m.sizeInput()
	m.homePicker()
	return m, m.input.Focus()
}

// homePicker puts the highlight on the best match, as the query just changed.
func (m *Model) homePicker() {
	m.cursor, m.top, m.selected = 0, 0, 0
	if rows := m.rows(); len(rows) > 0 {
		m.selected = rows[0].Task.ID
	}
}

func (m Model) updatePicker(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := m.rows()
	switch {
	case key.Matches(press, m.keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Cancel):
		m.selected = m.pickFrom
		m.closePrompt()
		m.reselect()
		return m, nil
	case key.Matches(press, m.keys.All):
		m.showAll = !m.showAll
		return m, m.fetch()
	case key.Matches(press, m.keys.PickDown):
		m.moveCursor(1)
		return m, nil
	case key.Matches(press, m.keys.PickUp):
		m.moveCursor(-1)
		return m, nil
	case key.Matches(press, m.keys.Submit):
		query := strings.TrimSpace(m.input.Value())
		switch {
		case len(rows) > 0 && m.pick == pickFilter:
			m.filter = query
			m.selected = rows[min(m.cursor, len(rows)-1)].Task.ID
			m.closePrompt()
			m.reselect()
			return m, nil
		case len(rows) > 0:
			id := rows[min(m.cursor, len(rows)-1)].Task.ID
			m.closePrompt()
			m.selected = id
			m.reselect()
			return m.startTask(id)
		case m.pick == pickStart && query != "":
			return m, m.add(query, true)
		}
		return m, nil
	}
	m.promptErr = ""
	return m.forwardToPrompt(press)
}

func (m Model) updatePrompt(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(press, m.keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Cancel):
		m.closePrompt()
		return m, nil
	case key.Matches(press, m.keys.Submit):
		return m, m.add(m.input.Value(), false)
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

// syncResume decides, on the first snapshot, whether to offer resuming the
// last Task: only when the app launched Idle after a session. It then opens the
// prompt once the keyboard is free and no hand-off is due, so the note saved
// there is the one shown. Anything that makes the offer moot ends it for the run.
func (m *Model) syncResume(lastNote *core.Note) {
	if m.resume == resumeUndecided {
		m.resume = resumeOver
		if m.snap.Phase == core.PhaseIdle && m.snap.Session != nil {
			m.resume = resumeArmed
		}
	}
	if m.resume != resumeArmed || m.snap.HandoffPending {
		return
	}
	if m.snap.Phase != core.PhaseIdle || m.sessionTask == nil || m.sessionTask.State != core.StateActive {
		m.resume = resumeOver
		return
	}
	if m.mode == modeList {
		m.mode, m.resume, m.resumeNote = modeResume, resumeOver, lastNote
	}
}

// updateResume handles the launch resume prompt, a yes/no question like the
// Break confirmation.
func (m Model) updateResume(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(press, m.keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(press, m.keys.Resume):
		m.mode = modeList
		return m, m.start(m.sessionTask.ID, false)
	case key.Matches(press, m.keys.Decline):
		m.mode = modeList
	}
	return m, nil
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
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.mode == modePicker && m.input.Value() != before {
		m.homePicker()
	}
	return m, cmd
}

func (m *Model) closePrompt() {
	m.mode, m.promptErr = modeList, ""
	m.input.Reset()
	m.input.Blur()
}

func (m *Model) moveCursor(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(rows)-1)
	m.selected = rows[m.cursor].Task.ID
	m.scrollToCursor()
}

// reselect puts the cursor back on the selected Task after the list changed,
// or clamps it to the list if that Task is gone.
func (m *Model) reselect() {
	rows := m.rows()
	for i, row := range rows {
		if row.Task.ID == m.selected {
			m.cursor = i
			m.scrollToCursor()
			return
		}
	}
	m.cursor = min(max(m.cursor, 0), max(len(rows)-1, 0))
	m.selected = 0
	if len(rows) > 0 {
		m.selected = rows[m.cursor].Task.ID
	}
	m.scrollToCursor()
}

func (m *Model) scrollToCursor() {
	m.top = clampTop(m.top, m.cursor, m.listRows(), len(m.rows()))
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
