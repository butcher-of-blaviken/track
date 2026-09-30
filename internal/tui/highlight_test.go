package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
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
