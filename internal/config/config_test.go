package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/config"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	want := config.Settings{
		FocusDuration: 30 * time.Minute, BreakDuration: 10 * time.Minute, LongBreakDuration: 20 * time.Minute,
		LongBreakInterval: 4, BellInterval: 30 * time.Second, BellRepeats: 10,
	}
	if got := config.Defaults(); got != want {
		t.Errorf("Defaults() = %+v, want %+v", got, want)
	}
}

func TestLoad_AMissingOrEmptyOptionalFileGivesTheDefaults(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.toml")
	for name, path := range map[string]string{
		"no path":       "",
		"missing":       missing,
		"empty":         writeConfig(t, ""),
		"only comments": writeConfig(t, "# nothing set\n"),
	} {
		got, err := config.Load(path, false)
		if err != nil || got != config.Defaults() {
			t.Errorf("%s: Load = %+v, %v; want the defaults", name, got, err)
		}
	}
}

func TestLoad_AMissingRequiredFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.toml")
	if _, err := config.Load(path, true); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want one naming %s", err, path)
	}
}

func TestLoad_ReadsEveryKey(t *testing.T) {
	path := writeConfig(t, `
focus_duration = "45m"
break_duration = "7m30s"
long_break_duration = "25m"
long_break_interval = 3
bell_interval = "10s"
bell_repeats = 2
`)
	got, err := config.Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Settings{
		FocusDuration: 45 * time.Minute, BreakDuration: 7*time.Minute + 30*time.Second, LongBreakDuration: 25 * time.Minute,
		LongBreakInterval: 3, BellInterval: 10 * time.Second, BellRepeats: 2,
	}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoad_AFileOverridesOnlyTheKeysItSets(t *testing.T) {
	got, err := config.Load(writeConfig(t, `focus_duration = "50m"`), false)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Defaults()
	want.FocusDuration = 50 * time.Minute
	if got != want {
		t.Errorf("Load = %+v, want only the focus duration changed: %+v", got, want)
	}
}

func TestLoad_BellRepeatsMayBeZero(t *testing.T) {
	got, err := config.Load(writeConfig(t, `bell_repeats = 0`), false)
	if err != nil || got.BellRepeats != 0 {
		t.Errorf("Load = %+v, %v; want no repeats", got, err)
	}
}

func TestLoad_BadConfigNamesTheFileAndTheKey(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want []string // every one must appear in the message
	}{
		"unknown key":               {"focus_durration = \"30m\"\n", []string{"focus_durration", "unknown"}},
		"not a duration":            {"focus_duration = \"3x\"\n", []string{"focus_duration", `"3x"`, "duration"}},
		"integer for a duration":    {"focus_duration = 30\n", []string{"focus_duration", "string", `"30m"`}},
		"zero duration":             {"break_duration = \"0s\"\n", []string{"break_duration", "positive"}},
		"negative duration":         {"long_break_duration = \"-5m\"\n", []string{"long_break_duration", "positive"}},
		"string for an integer":     {"long_break_interval = \"4\"\n", []string{"long_break_interval", "integer"}},
		"interval below one":        {"long_break_interval = 0\n", []string{"long_break_interval", "at least 1"}},
		"negative repeats":          {"bell_repeats = -1\n", []string{"bell_repeats", "0 or more"}},
		"zero bell interval":        {"bell_interval = \"0s\"\n", []string{"bell_interval", "positive"}},
		"syntax error reports line": {"focus_duration = \"30m\"\nthis is not toml\n", []string{"line 2"}},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, tc.text)
			_, err := config.Load(path, false)
			if err == nil {
				t.Fatal("Load returned no error")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %s", err, path)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestResolveFrom(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		name         string
		override     string
		e            config.Env
		want         string
		wantRequired bool
	}{
		{"override wins and is required", "/my/config.toml",
			config.Env{Getenv: env(map[string]string{"XDG_CONFIG_HOME": "/xdg"}), GOOS: "linux", Home: "/home/u"},
			"/my/config.toml", true},
		{"XDG_CONFIG_HOME is honored on linux", "",
			config.Env{Getenv: env(map[string]string{"XDG_CONFIG_HOME": "/xdg"}), GOOS: "linux", Home: "/home/u"},
			"/xdg/track/config.toml", false},
		{"XDG_CONFIG_HOME is honored on darwin too", "",
			config.Env{Getenv: env(map[string]string{"XDG_CONFIG_HOME": "/xdg"}), GOOS: "darwin", Home: "/Users/u"},
			"/xdg/track/config.toml", false},
		{"relative XDG_CONFIG_HOME is ignored", "",
			config.Env{Getenv: env(map[string]string{"XDG_CONFIG_HOME": "relative"}), GOOS: "linux", Home: "/home/u"},
			"/home/u/.config/track/config.toml", false},
		{"linux default", "",
			config.Env{Getenv: env(nil), GOOS: "linux", Home: "/home/u"},
			"/home/u/.config/track/config.toml", false},
		{"darwin default", "",
			config.Env{Getenv: env(nil), GOOS: "darwin", Home: "/Users/u"},
			"/Users/u/Library/Application Support/track/config.toml", false},
		{"no home and no XDG means no file, not an error", "",
			config.Env{Getenv: env(nil), GOOS: "linux", Home: ""},
			"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, required := config.ResolveFrom(tt.override, tt.e)
			if got != tt.want || required != tt.wantRequired {
				t.Errorf("ResolveFrom = %q, %v; want %q, %v", got, required, tt.want, tt.wantRequired)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	if d, err := config.ParseDuration("20m"); err != nil || d != 20*time.Minute {
		t.Errorf("ParseDuration(20m) = %v, %v", d, err)
	}
	for text, want := range map[string]string{"3x": "not a duration", "": "not a duration", "0s": "positive", "-5m": "positive"} {
		if _, err := config.ParseDuration(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseDuration(%q) error = %v, want one saying %q", text, err, want)
		}
	}
}

func TestParseInterval(t *testing.T) {
	if n, err := config.ParseInterval("3"); err != nil || n != 3 {
		t.Errorf("ParseInterval(3) = %d, %v", n, err)
	}
	for text, want := range map[string]string{"0": "at least 1", "-2": "at least 1", "x": "integer", "1.5": "integer", "": "integer"} {
		if _, err := config.ParseInterval(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseInterval(%q) error = %v, want one saying %q", text, err, want)
		}
	}
}

func TestApply_OverridesOnlyTheFieldsThatAreSet(t *testing.T) {
	base := config.Defaults()
	base.FocusDuration = 45 * time.Minute // as if from a config file

	focus, interval := 20*time.Minute, 2
	got := base.Apply(config.Overrides{FocusDuration: &focus, LongBreakInterval: &interval})
	want := base
	want.FocusDuration, want.LongBreakInterval = 20*time.Minute, 2
	if got != want {
		t.Errorf("Apply = %+v, want %+v", got, want)
	}

	if got := base.Apply(config.Overrides{}); got != base {
		t.Errorf("Apply with nothing set = %+v, want the settings unchanged: %+v", got, base)
	}

	breakD, long := 3*time.Minute, 15*time.Minute
	got = base.Apply(config.Overrides{BreakDuration: &breakD, LongBreakDuration: &long})
	if got.BreakDuration != 3*time.Minute || got.LongBreakDuration != 15*time.Minute || got.FocusDuration != 45*time.Minute {
		t.Errorf("Apply = %+v, want the two Break durations replaced and the focus duration kept", got)
	}
}

// A config file written for v1.0.0 keeps loading, with the same meaning, in
// every later version (docs/adr/0003-backward-compatibility.md). Keys may be
// added; none may be removed, renamed or change meaning.
func TestV1ConfigStillLoads(t *testing.T) {
	got, err := config.Load(filepath.Join("testdata", "config_v1.toml"), true)
	if err != nil {
		t.Fatalf("a v1 config no longer loads: %v", err)
	}
	want := config.Settings{
		FocusDuration:     45 * time.Minute,
		BreakDuration:     7 * time.Minute,
		LongBreakDuration: 25 * time.Minute,
		LongBreakInterval: 3,
		BellInterval:      15 * time.Second,
		BellRepeats:       2,
	}
	if got != want {
		t.Errorf("a v1 config now means something else:\n got  %+v\n want %+v", got, want)
	}
}
