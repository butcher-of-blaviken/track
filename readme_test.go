package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/config"
)

func readme(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Every flag and subcommand in --help is in the README.
func TestReadme_DocumentsEveryFlagAndSubcommand(t *testing.T) {
	var usageText bytes.Buffer
	usage(&usageText)
	doc := readme(t)
	for _, flag := range regexp.MustCompile(`(?m)^  (--[a-z-]+)`).FindAllStringSubmatch(usageText.String(), -1) {
		if !strings.Contains(doc, "`"+flag[1]+"`") {
			t.Errorf("README does not document %s", flag[1])
		}
	}
	for _, cmd := range regexp.MustCompile(`(?m)^  track (\w+)`).FindAllStringSubmatch(usageText.String(), -1) {
		if !strings.Contains(doc, "track "+cmd[1]) {
			t.Errorf("README does not show `track %s`", cmd[1])
		}
	}
}

// The example config is the defaults, and it loads.
func TestReadme_ConfigExampleIsTheDefaultsAndLoads(t *testing.T) {
	_, block, ok := strings.Cut(readme(t), "```toml\n")
	if !ok {
		t.Fatal("README has no toml example")
	}
	block, _, _ = strings.Cut(block, "```")
	path := writeConfigFile(t, t.TempDir(), block)
	got, err := loadSettings(path, env(nil))
	if err != nil {
		t.Fatalf("the README's example config does not load: %v", err)
	}
	if want := config.Defaults(); got != want {
		t.Errorf("the README's example is not the defaults:\n got  %+v\n want %+v", got, want)
	}
}

func TestReadme_ShowsQuotedTagExamplesAndTheFishCaveat(t *testing.T) {
	doc := readme(t)
	for _, want := range []string{`track add "write the design doc ##docs"`, "fish", "starts a comment"} {
		if !strings.Contains(doc, want) {
			t.Errorf("README lacks %q", want)
		}
	}
}
