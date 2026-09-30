package tui_test

import (
	"os"
	"strings"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

// section is the README text under a "### name" heading, up to the next heading.
func section(t *testing.T, readme, name string) string {
	t.Helper()
	_, rest, ok := strings.Cut(readme, "### "+name+"\n")
	if !ok {
		t.Fatalf("README has no section %q", name)
	}
	body, _, _ := strings.Cut(rest, "\n#")
	return body
}

// The README documents every key the app's own help shows, so the two cannot
// drift apart.
func TestReadme_ListsEveryKeyTheAppHelpShows(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	for view, groups := range tui.FullHelp() {
		body := section(t, readme, view)
		for _, group := range groups {
			for _, b := range group {
				h := b.Help()
				cell := "`" + h.Key + "`"
				if h.Key != "/" { // j/k is two keys; / is one
					cell = "`" + strings.ReplaceAll(h.Key, "/", "`/`") + "`"
				}
				found := false
				for _, line := range strings.Split(body, "\n") {
					if strings.HasPrefix(line, "| "+cell+" |") {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: README has no row for %s (%s)", view, cell, h.Desc)
				}
			}
		}
	}
}
