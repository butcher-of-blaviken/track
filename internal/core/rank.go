package core

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sahilm/fuzzy"
)

// Doc is what one Task offers to search: its title, Tag names and note texts.
type Doc struct {
	Title string
	Tags  []string
	Notes []string
}

// Field is the part of a Doc a query matched.
type Field int

// The fields of a Doc, from most to least weighted.
const (
	FieldTitle Field = iota
	FieldTag
	FieldNote
)

// FieldMatch is one part of a Doc that matched: which field, which entry of
// Tags or Notes (0 for the title), and the matched rune offsets in that text,
// ascending, for highlighting.
type FieldMatch struct {
	Field Field
	Entry int
	Runes []int
}

// Result is a Doc that matched the query. Doc is its index in the input.
type Result struct {
	Doc     int
	Score   int
	Matches []FieldMatch
}

// The fuzzy scorer's scores are unnormalised and often negative (every
// unmatched character costs 1), so a field's weight is a fixed offset added to
// the score, never a multiplier: a multiplier would rank a title's negative
// score below a note's. The offsets order the fields for equally good matches;
// a much better match in a lower field can still win.
const (
	titleOffset = 200
	tagOffset   = 100
	noteOffset  = 0

	// contiguousTier lifts a contiguous substring match above any scattered
	// one, whatever the fields. The scorer's scores grow without bound for long
	// queries, so scattered scores are capped well below it.
	contiguousTier = 10000
	scatteredCap   = 5000

	startBonus     = 10 // the match begins the text
	wordStartBonus = 20 // the match begins a word
)

// token is one whitespace-separated part of a query.
type token struct {
	text string
	// tagOnly is set for a token written with a leading #, which matches Tags only.
	tagOnly bool
}

// parseQuery splits query into tokens, dropping a leading # (or ##) from each
// and any token that is nothing but #.
func parseQuery(query string) []token {
	var tokens []token
	for _, field := range strings.Fields(query) {
		text := strings.TrimLeft(field, "#")
		if text != "" {
			tokens = append(tokens, token{text: text, tagOnly: text != field})
		}
	}
	return tokens
}

// Rank returns the Docs that match query, best first, breaking ties by input
// order. The query is split on whitespace and every token must match some field
// of the Doc; tokens may match different fields. A token written with a leading
// # matches Tags only. An empty query returns every Doc in input order with no
// matches.
func Rank(query string, docs []Doc) []Result {
	tokens := parseQuery(query)
	results := make([]Result, 0, len(docs))
	for i, doc := range docs {
		if len(tokens) == 0 {
			results = append(results, Result{Doc: i})
			continue
		}
		if result, ok := rankDoc(tokens, doc); ok {
			result.Doc = i
			results = append(results, result)
		}
	}
	slices.SortStableFunc(results, func(a, b Result) int { return b.Score - a.Score })
	return results
}

// fieldText is one searchable text of a Doc.
type fieldText struct {
	field  Field
	entry  int
	offset int
	text   string
}

func rankDoc(tokens []token, doc Doc) (Result, bool) {
	texts := []fieldText{{FieldTitle, 0, titleOffset, doc.Title}}
	for i, tag := range doc.Tags {
		texts = append(texts, fieldText{FieldTag, i, tagOffset, tag})
	}
	for i, note := range doc.Notes {
		texts = append(texts, fieldText{FieldNote, i, noteOffset, note})
	}

	var result Result
	for _, tok := range tokens {
		best, bestRunes, found := 0, []int(nil), -1
		for i, ft := range texts {
			if tok.tagOnly && ft.field != FieldTag {
				continue
			}
			if score, runes, ok := scoreToken(tok.text, ft); ok && (found < 0 || score > best) {
				best, bestRunes, found = score, runes, i
			}
		}
		if found < 0 {
			return Result{}, false
		}
		result.Score += best
		result.addMatch(texts[found], bestRunes)
	}
	return result, true
}

// addMatch records runes as matched in ft, merging with earlier tokens that
// matched the same text.
func (r *Result) addMatch(ft fieldText, runes []int) {
	for i := range r.Matches {
		if m := &r.Matches[i]; m.Field == ft.field && m.Entry == ft.entry {
			m.Runes = mergeRunes(m.Runes, runes)
			return
		}
	}
	r.Matches = append(r.Matches, FieldMatch{Field: ft.field, Entry: ft.entry, Runes: runes})
}

func mergeRunes(a, b []int) []int {
	merged := append(slices.Clone(a), b...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// scoreToken scores one token against one text, and returns the matched rune
// offsets.
func scoreToken(token string, ft fieldText) (int, []int, bool) {
	text, pattern := []rune(ft.text), []rune(token)
	if start, ok := findContiguous(text, pattern); ok {
		score := contiguousTier + ft.offset - (len(text) - len(pattern))
		if start == 0 {
			score += startBonus
		}
		if start == 0 || !isWordRune(text[start-1]) {
			score += wordStartBonus
		}
		runes := make([]int, len(pattern))
		for i := range runes {
			runes[i] = start + i
		}
		return score, runes, true
	}

	matches := fuzzy.Find(token, []string{ft.text})
	if len(matches) == 0 {
		return 0, nil, false
	}
	m := matches[0]
	runes := make([]int, len(m.MatchedIndexes))
	for i, b := range m.MatchedIndexes { // byte offsets into ft.text
		runes[i] = utf8.RuneCountInString(ft.text[:b])
	}
	return min(m.Score, scatteredCap) + ft.offset, runes, true
}

// findContiguous returns where pattern occurs in text, ignoring case, taking
// the first occurrence that starts a word if there is one and otherwise the
// first.
func findContiguous(text, pattern []rune) (int, bool) {
	first := -1
	for i := 0; i+len(pattern) <= len(text); i++ {
		if !equalFoldRunes(text[i:i+len(pattern)], pattern) {
			continue
		}
		if i == 0 || !isWordRune(text[i-1]) {
			return i, true
		}
		if first < 0 {
			first = i
		}
	}
	return first, first >= 0
}

func equalFoldRunes(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] && unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
