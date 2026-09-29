package core_test

import (
	"reflect"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func TestParseTaskText_ExtractsTagAndStripsItFromTitle(t *testing.T) {
	title, tags, err := core.ParseTaskText("finishing up auth for service x ##PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "finishing up auth for service x"; title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
	if want := []string{"PROJ-123"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

func TestParseTaskText_TagsAreCaseInsensitiveAndKeepFirstUseCasing(t *testing.T) {
	_, tags, err := core.ParseTaskText("write docs ##Auth ##proj-1 ##auth ##PROJ-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"Auth", "proj-1"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

func TestParseTaskText_InvalidTagMarkersStayInTitle(t *testing.T) {
	const text = "compare a ## b ##!x ###y ##ok!"
	title, tags, err := core.ParseTaskText(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != text {
		t.Errorf("title = %q, want %q", title, text)
	}
	if len(tags) != 0 {
		t.Errorf("tags = %v, want none", tags)
	}
}

func TestParseTaskText_AllowsLettersDigitsDashUnderscoreSlash(t *testing.T) {
	_, tags, err := core.ParseTaskText("x ##team/back_end-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"team/back_end-2"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

func TestParseTaskText_TagInMiddleIsStrippedAndSpacesCollapsed(t *testing.T) {
	title, tags, err := core.ParseTaskText("  fix   ##bug the   parser  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "fix the parser"; title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
	if want := []string{"bug"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
}

func TestParseTaskText_EmptyTitleIsAnError(t *testing.T) {
	for _, text := range []string{"", "   ", "##only ##tags"} {
		if _, _, err := core.ParseTaskText(text); err == nil {
			t.Errorf("ParseTaskText(%q) returned no error, want one", text)
		}
	}
}
