package datadir_test

import (
	"testing"

	"github.com/butcher-of-blaviken/track/internal/datadir"
)

func TestResolveFrom(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		name     string
		override string
		e        datadir.Env
		want     string
	}{
		{"override wins over everything", "/custom/dir",
			datadir.Env{Getenv: env(map[string]string{"XDG_DATA_HOME": "/xdg"}), GOOS: "linux", Home: "/home/u"},
			"/custom/dir"},
		{"XDG_DATA_HOME is honored on linux", "",
			datadir.Env{Getenv: env(map[string]string{"XDG_DATA_HOME": "/xdg"}), GOOS: "linux", Home: "/home/u"},
			"/xdg/track"},
		{"XDG_DATA_HOME is honored on darwin too", "",
			datadir.Env{Getenv: env(map[string]string{"XDG_DATA_HOME": "/xdg"}), GOOS: "darwin", Home: "/Users/u"},
			"/xdg/track"},
		{"relative XDG_DATA_HOME is ignored", "",
			datadir.Env{Getenv: env(map[string]string{"XDG_DATA_HOME": "relative/path"}), GOOS: "linux", Home: "/home/u"},
			"/home/u/.local/share/track"},
		{"linux default", "",
			datadir.Env{Getenv: env(nil), GOOS: "linux", Home: "/home/u"},
			"/home/u/.local/share/track"},
		{"darwin default", "",
			datadir.Env{Getenv: env(nil), GOOS: "darwin", Home: "/Users/u"},
			"/Users/u/Library/Application Support/track"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := datadir.ResolveFrom(tt.override, tt.e)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveFrom = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveFrom_NoHomeAndNoOverrideIsAnError(t *testing.T) {
	e := datadir.Env{Getenv: func(string) string { return "" }, GOOS: "linux", Home: ""}
	if _, err := datadir.ResolveFrom("", e); err == nil {
		t.Error("ResolveFrom with no override, XDG or home returned no error")
	}
}
