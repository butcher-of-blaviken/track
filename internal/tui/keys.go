package tui

import "charm.land/bubbles/v2/key"

// keyMap is every key the TUI reacts to, defined once: the same bindings are
// matched in Update and shown in the footer, so the two cannot drift apart.
type keyMap struct {
	// Main list.
	Up, Down, Start, Stop, Add, Quit key.Binding
	// Find opens the picker as a filter, Pick opens it to choose a Task to start.
	Find, Pick key.Binding
	// Done, Archive and Reopen change the state of the Task under the cursor.
	Done, Archive, Reopen key.Binding
	// Inbox opens the Unfiled notes, Note captures one.
	Inbox, Note key.Binding

	// Inbox view. It shares Up, Down and Quit with the list; Back also closes it
	// with Inbox.
	File, NewTask, Back key.Binding

	// Task detail view.
	Detail, DetailBack, Scroll key.Binding

	// Report view. Its Back is the detail's, plus r.
	Report, ReportBack, Period key.Binding

	// Help toggles the full help in the list, inbox, detail and report.
	Help key.Binding
	// All toggles between Active Tasks and every state, in the list and the picker.
	All key.Binding
	// Move stands for Up and Down together in the footer; it is never matched.
	Move key.Binding

	// Add-task prompt.
	Submit, Cancel, ForceQuit key.Binding

	// Hand-off note prompt. It shares ForceQuit with the add prompt.
	Save, Skip key.Binding

	// Break-override confirmation. It shares ForceQuit too.
	Confirm, Decline key.Binding

	// Resume is the launch resume prompt's yes; its no is Decline.
	Resume key.Binding

	// Picker. PickMove stands for PickUp and PickDown together in the footer.
	PickUp, PickDown, PickMove key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
		Move:       key.NewBinding(key.WithKeys("j", "k", "up", "down"), key.WithHelp("j/k", "move")),
		Start:      key.NewBinding(key.WithKeys("s", "enter"), key.WithHelp("enter", "start")),
		Stop:       key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Add:        key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Report:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "report")),
		ReportBack: key.NewBinding(key.WithKeys("esc", "r"), key.WithHelp("esc", "back")),
		Period:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "period")),
		Detail:     key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "open")),
		DetailBack: key.NewBinding(key.WithKeys("esc", "h"), key.WithHelp("esc", "back")),
		Scroll:     key.NewBinding(key.WithKeys("j", "k", "up", "down"), key.WithHelp("j/k", "scroll")),
		Inbox:      key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "inbox")),
		Note:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "note")),
		File:       key.NewBinding(key.WithKeys("f", "enter"), key.WithHelp("enter", "file")),
		NewTask:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "new task")),
		Back:       key.NewBinding(key.WithKeys("esc", "i"), key.WithHelp("esc", "back")),
		Done:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "done")),
		Archive:    key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "archive")),
		Reopen:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "reopen")),
		Find:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "find")),
		All:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "scope")),
		Pick:       key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "pick")),

		Submit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add")),
		Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),

		Save: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
		Skip: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "skip")),

		Confirm: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "start")),
		Decline: key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "cancel")),
		Resume:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "resume")),

		PickUp:   key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("up", "up")),
		PickDown: key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("down", "down")),
		PickMove: key.NewBinding(key.WithKeys("up", "down", "ctrl+p", "ctrl+n"), key.WithHelp("↑/↓", "move")),
	}
}

// listHelp is what the footer shows on the main list: the essentials only. The
// rest is in the full help, listFull.
func (k keyMap) listHelp() []key.Binding {
	return []key.Binding{k.Start, k.Stop, k.Add, k.Move, k.Help, k.Quit}
}

// listFull is the list's full help, one column per group.
func (k keyMap) listFull() [][]key.Binding {
	return [][]key.Binding{
		{k.Start, k.Stop, k.Add, k.Note, k.Done, k.Archive, k.Reopen},
		{k.Detail, k.Report, k.Inbox, k.Find, k.Pick, k.All},
		{k.Move, k.Help, k.Quit},
	}
}

// promptHelp is what the footer shows while the add prompt is open.
func (k keyMap) promptHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel, k.ForceQuit}
}

// handoffHelp is what the footer shows while the hand-off prompt is open.
func (k keyMap) handoffHelp() []key.Binding {
	return []key.Binding{k.Save, k.Skip, k.ForceQuit}
}

// confirmBreakHelp is what the footer shows while the Break confirmation is open.
func (k keyMap) confirmBreakHelp() []key.Binding {
	return []key.Binding{k.Confirm, k.Decline, k.ForceQuit}
}

// resumeHelp is what the footer shows while the resume prompt is open.
func (k keyMap) resumeHelp() []key.Binding {
	return []key.Binding{k.Resume, k.Decline, k.ForceQuit}
}

// pickerHelp is what the footer shows while the picker is open; accept is what
// Enter does.
func (k keyMap) pickerHelp(accept string) []key.Binding {
	enter := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", accept))
	return []key.Binding{enter, k.PickMove, k.All, k.Cancel, k.ForceQuit}
}

// inboxHelp is what the footer shows in the inbox, and inboxFull its full help.
func (k keyMap) inboxHelp() []key.Binding {
	return []key.Binding{k.File, k.NewTask, k.Move, k.Back, k.Help, k.Quit}
}

func (k keyMap) inboxFull() [][]key.Binding {
	return [][]key.Binding{{k.File, k.NewTask}, {k.Move, k.Back}, {k.Help, k.Quit}}
}

// noteHelp is what the footer shows while the note prompt is open.
func (k keyMap) noteHelp() []key.Binding {
	save := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save"))
	return []key.Binding{save, k.Cancel, k.ForceQuit}
}

// detailHelp is what the footer shows in the Task detail view, and detailFull
// its full help.
func (k keyMap) detailHelp() []key.Binding {
	return []key.Binding{k.Start, k.Note, k.Scroll, k.DetailBack, k.Help, k.Quit}
}

func (k keyMap) detailFull() [][]key.Binding {
	return [][]key.Binding{{k.Start, k.Note}, {k.Scroll, k.DetailBack}, {k.Help, k.Quit}}
}

// reportHelp is what the footer shows in the report, and reportFull its full help.
func (k keyMap) reportHelp() []key.Binding {
	return []key.Binding{k.Period, k.Scroll, k.ReportBack, k.Help, k.Quit}
}

func (k keyMap) reportFull() [][]key.Binding {
	return [][]key.Binding{{k.Period}, {k.Scroll, k.ReportBack}, {k.Help, k.Quit}}
}
