// Package datadir decides where Track keeps its data.
package datadir

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Env is the slice of the process environment that Resolve depends on,
// injectable for tests.
type Env struct {
	Getenv func(string) string
	GOOS   string
	Home   string
}

// Resolve returns the data directory: override if set, otherwise the platform
// default for the current process.
func Resolve(override string) (string, error) {
	home, _ := os.UserHomeDir()
	return ResolveFrom(override, Env{Getenv: os.Getenv, GOOS: runtime.GOOS, Home: home})
}

// ResolveFrom is Resolve with an explicit environment. Precedence: override,
// then an absolute $XDG_DATA_HOME, then the platform default under the home
// directory.
func ResolveFrom(override string, e Env) (string, error) {
	if override != "" {
		return override, nil
	}
	if xdg := e.Getenv("XDG_DATA_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "track"), nil
	}
	if e.Home == "" {
		return "", errors.New("cannot determine the data directory: no home directory; set --data-dir")
	}
	if e.GOOS == "darwin" {
		return filepath.Join(e.Home, "Library", "Application Support", "track"), nil
	}
	return filepath.Join(e.Home, ".local", "share", "track"), nil
}
