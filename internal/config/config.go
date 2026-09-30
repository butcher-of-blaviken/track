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
}

// Defaults are the settings when nothing overrides them.
func Defaults() Settings {
	return Settings{
		FocusDuration:     30 * time.Minute,
		BreakDuration:     10 * time.Minute,
		LongBreakDuration: 20 * time.Minute,
		LongBreakInterval: 4,
		BellInterval:      30 * time.Second,
		BellRepeats:       10,
	}
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
	}
	return fmt.Errorf("unknown key %q", key)
}

// duration reads a positive duration written as a string such as "30m". A bare
// number would be nanoseconds, which is never what the file means.
func duration(key string, value any, into *time.Duration) error {
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf(`%s: must be a string such as "30m"`, key)
	}
	d, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf(`%s: %q is not a duration (try "30m")`, key, text)
	}
	if d <= 0 {
		return fmt.Errorf("%s: must be positive, got %s", key, text)
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
