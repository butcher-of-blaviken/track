package tui

import "charm.land/bubbles/v2/key"

// keyMap is every key the TUI reacts to, defined once: the same bindings are
// matched in Update and shown in the footer, so the two cannot drift apart.
type keyMap struct {
	// Main list.
	Up, Down, Start, Stop, Add, Quit key.Binding
	// Move stands for Up and Down together in the footer; it is never matched.
	Move key.Binding

	// Add-task prompt.
	Submit, Cancel, ForceQuit key.Binding

	// Hand-off note prompt. It shares ForceQuit with the add prompt.
	Save, Skip key.Binding

	// Break-override confirmation. It shares ForceQuit too.
	Confirm, Decline key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:    key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
		Down:  key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
		Move:  key.NewBinding(key.WithKeys("j", "k", "up", "down"), key.WithHelp("j/k", "move")),
		Start: key.NewBinding(key.WithKeys("s", "enter"), key.WithHelp("enter", "start")),
		Stop:  key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Add:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		Submit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add")),
		Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),

		Save: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
		Skip: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "skip")),

		Confirm: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "start")),
		Decline: key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "cancel")),
	}
}

// listHelp is what the footer shows on the main list.
func (k keyMap) listHelp() []key.Binding {
	return []key.Binding{k.Start, k.Stop, k.Add, k.Move, k.Quit}
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
