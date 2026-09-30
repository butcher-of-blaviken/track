package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestHighlight_StylesOnlyTheMatchedRunes(t *testing.T) {
	got := highlight("café prd", []int{3, 5, 6})
	if plain := ansi.Strip(got); plain != "café prd" {
		t.Fatalf("text changed: %q", plain)
	}
	want := "caf" + matchStyle.Render("é") + " " + matchStyle.Render("pr") + "d"
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
	got := matchText(tm)
	if plain := ansi.Strip(got); plain != "x (done)  #backend #docs" {
		t.Fatalf("text = %q", plain)
	}
	if want := "#" + matchStyle.Render("do") + "cs"; !strings.Contains(got, want) {
		t.Errorf("%q lacks the highlighted chip %q", got, want)
	}
}

func TestNoteLine_HighlightsTheMatchAndShowsTheAge(t *testing.T) {
	note := core.Note{Text: "left off at the parser", CreatedAt: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)}
	tm := core.TaskMatch{Task: core.Task{Title: "x"}, Note: &core.NoteMatch{Note: note, Runes: []int{16, 17, 18}}}
	got := noteLine(tm, note.CreatedAt.Add(2*time.Hour))
	if plain := ansi.Strip(got); plain != "      ↳ left off at the parser (2h 0m ago)" {
		t.Fatalf("line = %q", plain)
	}
	if want := "left off at the " + matchStyle.Render("par") + "ser"; !strings.Contains(got, want) {
		t.Errorf("%q lacks the highlighted match %q", got, want)
	}
}
