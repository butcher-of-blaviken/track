package core

import (
	"errors"
	"strings"
	"unicode"
)

// ErrEmptyTitle is returned when the text has no title once Tags are stripped.
var ErrEmptyTitle = errors.New("task title is empty")

// ParseTaskText splits free text into a title and its Tags. A Tag is written
// as ##name anywhere in the text and is stripped from the title.
func ParseTaskText(text string) (title string, tags []string, err error) {
	var words []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(text) {
		if name, ok := strings.CutPrefix(w, "##"); ok && validTagName(name) {
			if key := strings.ToLower(name); !seen[key] {
				seen[key] = true
				tags = append(tags, name)
			}
			continue
		}
		words = append(words, w)
	}
	if len(words) == 0 {
		return "", nil, ErrEmptyTitle
	}
	return strings.Join(words, " "), tags, nil
}

// validTagName reports whether name is non-empty and made only of letters,
// digits, '-', '_' and '/'.
func validTagName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '/' {
			return false
		}
	}
	return true
}
