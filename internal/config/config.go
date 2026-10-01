// Package config reads Track's optional TOML config file and holds the settings
// it and the command-line flags produce.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Settings is everything the config file and the flags can set.
type Settings struct {
	FocusDuration     time.Duration
	BreakDuration     time.Duration
	LongBreakDuration time.Duration
	// LongBreakInterval is how many completed sessions earn a long Break.
	LongBreakInterval int
	BellInterval      time.Duration
	// BellRepeats is how many times the bell repeats after its first ring.
	BellRepeats int
	// Layout is how the main screen is laid out: LayoutAuto, LayoutSplit or
	// LayoutSingle.
	Layout string
	// Theme is the colours: ThemeDefault, or the name of a built-in palette.
	Theme string
}

// The layouts. Split and auto both show the panels side by side when the
// terminal has room; single always shows one pane at a time.
const (
	LayoutAuto   = "auto"
	LayoutSplit  = "split"
	LayoutSingle = "single"
)

// ThemeDefault takes the colours of the terminal's own theme. The other themes
// are fixed palettes; Themes lists them all.
const ThemeDefault = "default"

// Themes are the values of the theme key. A test in the main package checks
// that this is ThemeDefault and the palettes the UI has.
var Themes = []string{ThemeDefault, "gruvbox-dark", "one-dark", "one-light", "solarized-dark", "solarized-light"}

// Defaults are the settings when nothing overrides them.
func Defaults() Settings {
	return Settings{
		FocusDuration:     30 * time.Minute,
		BreakDuration:     10 * time.Minute,
		LongBreakDuration: 20 * time.Minute,
		LongBreakInterval: 4,
		BellInterval:      30 * time.Second,
		BellRepeats:       10,
		Layout:            LayoutAuto,
		Theme:             ThemeDefault,
	}
}

// Overrides are the settings the command-line flags set. A nil field was not
// given and leaves the setting alone.
type Overrides struct {
	FocusDuration     *time.Duration
	BreakDuration     *time.Duration
	LongBreakDuration *time.Duration
	LongBreakInterval *int
}

// Apply returns the settings with the overrides that are set replacing theirs:
// flags over the config file over the defaults.
func (s Settings) Apply(o Overrides) Settings {
	if o.FocusDuration != nil {
		s.FocusDuration = *o.FocusDuration
	}
	if o.BreakDuration != nil {
		s.BreakDuration = *o.BreakDuration
	}
	if o.LongBreakDuration != nil {
		s.LongBreakDuration = *o.LongBreakDuration
	}
	if o.LongBreakInterval != nil {
		s.LongBreakInterval = *o.LongBreakInterval
	}
	return s
}

// Env is the slice of the process environment that Resolve depends on,
// injectable for tests.
type Env struct {
	Getenv func(string) string
	GOOS   string
	Home   string
}

// Resolve returns the config file to read and whether it must exist: override
// if set (a file named on purpose must exist), otherwise the platform default
// for the current process, which is optional.
func Resolve(override string) (path string, required bool) {
	home, _ := os.UserHomeDir()
	return ResolveFrom(override, Env{Getenv: os.Getenv, GOOS: runtime.GOOS, Home: home})
}

// ResolveFrom is Resolve with an explicit environment. Precedence: override,
// then an absolute $XDG_CONFIG_HOME, then the platform default under the home
// directory. With none of those there is no file to read, which is not an
// error: the config is optional.
func ResolveFrom(override string, e Env) (path string, required bool) {
	if override != "" {
		return override, true
	}
	if xdg := e.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "track", "config.toml"), false
	}
	switch {
	case e.Home == "":
		return "", false
	case e.GOOS == "darwin":
		return filepath.Join(e.Home, "Library", "Application Support", "track", "config.toml"), false
	}
	return filepath.Join(e.Home, ".config", "track", "config.toml"), false
}

// Load reads the config file over the defaults: keys the file leaves out keep
// their default. An empty path, or a missing file that is not required, gives the
// defaults. Anything wrong with the file is an error naming the file and, where
// there is one, the key.
func Load(path string, required bool) (Settings, error) {
	s := Defaults()
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !required {
		return s, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("config %s: %w", path, err)
	}

	var raw map[string]any
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return Settings{}, fmt.Errorf("config %s: line %d: %s", path, pe.Position.Line, pe.Message)
		}
		return Settings{}, fmt.Errorf("config %s: %w", path, err)
	}
	for _, k := range md.Keys() {
		if len(k) != 1 {
			continue // inside a table, which its own key already rejected
		}
		if err := s.set(k[0], raw[k[0]]); err != nil {
			return Settings{}, fmt.Errorf("config %s: %w", path, err)
		}
	}
	return s, nil
}

// set applies one config key.
func (s *Settings) set(key string, value any) error {
	switch key {
	case "focus_duration":
		return duration(key, value, &s.FocusDuration)
	case "break_duration":
		return duration(key, value, &s.BreakDuration)
	case "long_break_duration":
		return duration(key, value, &s.LongBreakDuration)
	case "bell_interval":
		return duration(key, value, &s.BellInterval)
	case "long_break_interval":
		return integer(key, value, 1, "at least 1", &s.LongBreakInterval)
	case "bell_repeats":
		return integer(key, value, 0, "0 or more", &s.BellRepeats)
	case "layout":
		text, ok := value.(string)
		if !ok || (text != LayoutAuto && text != LayoutSplit && text != LayoutSingle) {
			return fmt.Errorf(`layout: must be "auto", "split" or "single", got %v`, value)
		}
		s.Layout = text
		return nil
	case "theme":
		text, ok := value.(string)
		if !ok || !slices.Contains(Themes, text) {
			return fmt.Errorf("theme: must be one of %s, got %v", quoted(Themes), value)
		}
		s.Theme = text
		return nil
	}
	return fmt.Errorf("unknown key %q", key)
}

// quoted lists names as "a", "b" and "c".
func quoted(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = strconv.Quote(n)
	}
	return strings.Join(q, ", ")
}

// ParseDuration reads a positive duration written as a string such as "30m".
// A bare number would be nanoseconds, which is never what anyone means.
func ParseDuration(text string) (time.Duration, error) {
	d, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf(`%q is not a duration (try "30m")`, text)
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %s", text)
	}
	return d, nil
}

// ParseInterval reads a whole number of at least 1.
func ParseInterval(text string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, fmt.Errorf("%q is not an integer", text)
	}
	if n < 1 {
		return 0, fmt.Errorf("must be at least 1, got %d", n)
	}
	return n, nil
}

// duration reads a duration config value, which must be a string.
func duration(key string, value any, into *time.Duration) error {
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf(`%s: must be a string such as "30m"`, key)
	}
	d, err := ParseDuration(text)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*into = d
	return nil
}

// integer reads a whole number of at least minimum.
func integer(key string, value any, minimum int, want string, into *int) error {
	n, ok := value.(int64)
	if !ok {
		return fmt.Errorf("%s: must be an integer", key)
	}
	if n < int64(minimum) {
		return fmt.Errorf("%s: must be %s, got %d", key, want, n)
	}
	*into = int(n)
	return nil
}
