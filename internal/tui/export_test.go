package tui

import "charm.land/bubbles/v2/key"

// FullHelp is the full help of each view that has one, for tests outside the
// package that check the docs list every key.
func FullHelp() map[string][][]key.Binding {
	k := newKeyMap()
	return map[string][][]key.Binding{
		"The list":        k.listFull(),
		"The inbox":       k.inboxFull(),
		"A task's detail": k.detailFull(),
		"The report":      k.reportFull(),
	}
}
