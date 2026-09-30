package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestHighlight_StylesOnlyTheMatchedRunes(t *testing.T) {
	th := newTheme()
	got := th.highlight("café prd", []int{3, 5, 6}, lipgloss.NewStyle(), th.match)
	if plain := ansi.Strip(got); plain != "café prd" {
		t.Fatalf("text changed: %q", plain)
	}
	want := "caf" + th.match.Render("é") + " " + th.match.Render("pr") + "d"
	if got != want {
		t.Errorf("highlight = %q, want %q", got, want)
	}
	if got == "café prd" {
		t.Error("nothing was styled")
	}
}

func TestMatchText_HighlightsMatchedTagsAndMarksInactiveTasks(t *testing.T) {
	tm := core.TaskMatch{
		Task:     core.Task{Title: "x", State: core.StateDone, Tags: []string{"backend", "docs"}},
		TagRunes: map[int][]int{1: {0, 1}},
	}
	th := newTheme()
	got := th.taskText(tm, false)
	if plain := ansi.Strip(got); plain != "x (done)  #backend #docs" {
		t.Fatalf("text = %q", plain)
	}
	if want := th.match.Render("do"); !strings.Contains(got, want) {
		t.Errorf("%q lacks the highlighted chip %q", got, want)
	}
}

func TestNoteLine_HighlightsTheMatchAndShowsTheAge(t *testing.T) {
	note := core.Note{Text: "left off at the parser", CreatedAt: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)}
	tm := core.TaskMatch{Task: core.Task{Title: "x"}, Note: &core.NoteMatch{Note: note, Runes: []int{16, 17, 18}}}
	th := newTheme()
	got := th.noteLine(tm, note.CreatedAt.Add(2*time.Hour))
	if plain := ansi.Strip(got); plain != "      ↳ left off at the parser (2h 0m ago)" {
		t.Fatalf("line = %q", plain)
	}
	if want := th.match.Render("par"); !strings.Contains(got, want) {
		t.Errorf("%q lacks the highlighted match %q", got, want)
	}
}

func TestTheme_TagColourIgnoresCaseAndUsesEveryColour(t *testing.T) {
	th := newTheme()
	used := map[string]bool{}
	names := []string{"docs", "work", "track", "Q1", "backend"}
	for i := 0; i < 40; i++ {
		names = append(names, fmt.Sprintf("tag%d", i))
	}
	for _, name := range names {
		got := th.tag(name).Render("#x")
		if upper := th.tag(strings.ToUpper(name)).Render("#x"); got != upper {
			t.Errorf("%q and %q have different colours", name, strings.ToUpper(name))
		}
		if lower := th.tag(strings.ToLower(name)).Render("#x"); got != lower {
			t.Errorf("%q and %q have different colours", name, strings.ToLower(name))
		}
		used[got] = true
	}
	if len(used) != len(th.tags) {
		t.Errorf("%d of %d tag colours used across %d names", len(used), len(th.tags), len(names))
	}
}
