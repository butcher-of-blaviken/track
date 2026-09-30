package core_test

import (
	"reflect"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// docs returns one Doc per title, with no Tags or notes.
func docs(titles ...string) []core.Doc {
	out := make([]core.Doc, len(titles))
	for i, title := range titles {
		out[i] = core.Doc{Title: title}
	}
	return out
}

// order is the input indexes of the results, best first.
func order(results []core.Result) []int {
	out := []int{}
	for _, r := range results {
		out = append(out, r.Doc)
	}
	return out
}

func wantOrder(t *testing.T, what string, got []core.Result, want ...int) {
	t.Helper()
	if want == nil {
		want = []int{}
	}
	if !reflect.DeepEqual(order(got), want) {
		t.Errorf("%s: result order = %v, want %v", what, order(got), want)
	}
}

func TestRank_MatchesATitleAndReportsRuneOffsets(t *testing.T) {
	got := core.Rank("prd", docs("write the PRD"))
	wantOrder(t, "prd", got, 0)
	want := []core.FieldMatch{{Field: core.FieldTitle, Runes: []int{10, 11, 12}}}
	if !reflect.DeepEqual(got[0].Matches, want) {
		t.Errorf("matches = %+v, want %+v", got[0].Matches, want)
	}
}

func TestRank_OffsetsAreRunesNotBytes(t *testing.T) {
	// "é" is two bytes and "🙂" four, so byte offsets would run past the rune offsets.
	for name, query := range map[string]string{"contiguous": "auth", "scattered": "ath"} {
		t.Run(name, func(t *testing.T) {
			got := core.Rank(query, docs("café 🙂 auth"))
			wantOrder(t, query, got, 0)
			runes := got[0].Matches[0].Runes
			title := []rune("café 🙂 auth")
			for i, r := range runes {
				if r < 0 || r >= len(title) || title[r] != []rune(query)[i] {
					t.Fatalf("offsets %v do not pick out %q in %q", runes, query, string(title))
				}
			}
		})
	}
}

func TestRank_EveryTokenMustMatch(t *testing.T) {
	d := docs("work on auth", "work", "auth only", "unrelated")
	wantOrder(t, "work auth", core.Rank("work auth", d), 0)
	wantOrder(t, "auth", core.Rank("auth", d), 2, 0)
}

func TestRank_TokensMayMatchDifferentFields(t *testing.T) {
	d := []core.Doc{
		{Title: "write the PRD", Tags: []string{"docs"}},
		{Title: "write the PRD"},
		{Title: "fix the build", Tags: []string{"docs"}},
	}
	got := core.Rank("write docs", d)
	wantOrder(t, "write docs", got, 0)
	fields := []core.Field{got[0].Matches[0].Field, got[0].Matches[1].Field}
	if !reflect.DeepEqual(fields, []core.Field{core.FieldTitle, core.FieldTag}) {
		t.Errorf("matched fields = %v, want title then tag", fields)
	}
}

func TestRank_WhitespaceInTheQueryIsJustSeparation(t *testing.T) {
	d := docs("work on auth", "work", "auth")
	want := order(core.Rank("work auth", d))
	for _, q := range []string{"  work   auth ", "work\tauth", "\nwork auth\n"} {
		if got := order(core.Rank(q, d)); !reflect.DeepEqual(got, want) {
			t.Errorf("Rank(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestRank_AnEmptyQueryListsEverythingInInputOrderWithoutMatches(t *testing.T) {
	d := docs("b", "a", "c")
	for _, q := range []string{"", "   ", "\t\n"} {
		got := core.Rank(q, d)
		wantOrder(t, q, got, 0, 1, 2)
		for _, r := range got {
			if len(r.Matches) != 0 {
				t.Errorf("Rank(%q) result %d has matches %+v", q, r.Doc, r.Matches)
			}
		}
	}
}

func TestRank_TitleBeatsTagBeatsNoteForTheSameText(t *testing.T) {
	d := []core.Doc{
		{Title: "other", Notes: []string{"parser"}},
		{Title: "other", Tags: []string{"parser"}},
		{Title: "parser"},
	}
	wantOrder(t, "parser", core.Rank("parser", d), 2, 1, 0)
}

func TestRank_FieldWeightsAddUpRatherThanInvertNegativeScores(t *testing.T) {
	// Scattered matches score below zero, and multiplying a negative score by a
	// bigger weight would rank the title last.
	scattered := "a very long title that mentions unrelated stuff about the u thing then the h"
	d := []core.Doc{
		{Title: "other", Notes: []string{scattered}},
		{Title: scattered},
	}
	wantOrder(t, "auth", core.Rank("auth", d), 1, 0)
}

func TestRank_AContiguousMatchBeatsAScatteredOne(t *testing.T) {
	d := docs("a bit unusual tea house", "about authentication")
	got := core.Rank("auth", d)
	wantOrder(t, "auth", got, 1, 0)

	// Even a scattered title match ranks below a contiguous match in a note.
	d = []core.Doc{
		{Title: "a bit unusual tea house"},
		{Title: "other", Notes: []string{"about authentication"}},
	}
	got = core.Rank("auth", d)
	wantOrder(t, "contiguous note over scattered title", got, 1, 0)

	// The highlight is the contiguous run, not the scattered alignment the
	// library prefers ("about authentication" as a-u-t-h across two words).
	got = core.Rank("auth", docs("about authentication"))
	if want := []int{6, 7, 8, 9}; !reflect.DeepEqual(got[0].Matches[0].Runes, want) {
		t.Errorf("highlight = %v, want %v", got[0].Matches[0].Runes, want)
	}
}

func TestRank_ShorterAndEarlierContiguousMatchesRankHigher(t *testing.T) {
	d := docs("authentication", "auth", "about auth")
	wantOrder(t, "auth", core.Rank("auth", d), 1, 0, 2)
}

func TestRank_MatchingIsCaseInsensitive(t *testing.T) {
	d := docs("Write The PRD", "write the prd")
	got := core.Rank("PRD", d)
	wantOrder(t, "PRD", got, 0, 1)
	wantOrder(t, "prd", core.Rank("prd", d), 0, 1)
}

func TestRank_ReportsTheWinningFieldAndEntry(t *testing.T) {
	d := []core.Doc{{
		Title: "other",
		Tags:  []string{"docs", "backend"},
		Notes: []string{"nothing here", "left off at the parser"},
	}}
	tests := []struct {
		query string
		want  core.FieldMatch
	}{
		{"backend", core.FieldMatch{Field: core.FieldTag, Entry: 1, Runes: []int{0, 1, 2, 3, 4, 5, 6}}},
		{"parser", core.FieldMatch{Field: core.FieldNote, Entry: 1, Runes: []int{16, 17, 18, 19, 20, 21}}},
	}
	for _, tc := range tests {
		got := core.Rank(tc.query, d)
		wantOrder(t, tc.query, got, 0)
		if !reflect.DeepEqual(got[0].Matches, []core.FieldMatch{tc.want}) {
			t.Errorf("%q matches = %+v, want %+v", tc.query, got[0].Matches, []core.FieldMatch{tc.want})
		}
	}
}

func TestRank_EqualScoresKeepInputOrder(t *testing.T) {
	d := docs("same", "same", "same")
	wantOrder(t, "same", core.Rank("same", d), 0, 1, 2)
}

func TestRank_ExcludesDocsThatDoNotMatch(t *testing.T) {
	wantOrder(t, "zzz", core.Rank("zzz", docs("write the PRD", "fix build")))
}
