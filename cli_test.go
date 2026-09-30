package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/sqlite"
)

// track runs the command line against a fresh data directory, as the real
// process would, and returns the exit code and what it printed.
func track(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	full := append([]string{"--data-dir", dir}, args...)
	code = realMain(full, env(nil), &out, &errOut)
	return code, out.String(), errOut.String()
}

// stored opens the database the command line wrote to.
func stored(t *testing.T, dir string) core.Store {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(dir, "track.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestAdd_CreatesATaskAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := track(t, dir, "add", "write the PRD ##docs")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if want := "Added task 1: write the PRD  #docs\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	tasks, err := stored(t, dir).Tasks(context.Background())
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v, %v", tasks, err)
	}
	if got := tasks[0]; got.Title != "write the PRD" || got.State != core.StateActive || len(got.Tags) != 1 || got.Tags[0] != "docs" {
		t.Errorf("task = %+v, want an Active \"write the PRD\" tagged docs", got)
	}
}

func TestAdd_JoinsSeveralArgumentsWithSpaces(t *testing.T) {
	dir := t.TempDir()
	if code, stdout, _ := track(t, dir, "add", "fix", "the", "build"); code != 0 || stdout != "Added task 1: fix the build\n" {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestAdd_IDsGrowAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	track(t, dir, "add", "one")
	if _, stdout, _ := track(t, dir, "add", "two"); stdout != "Added task 2: two\n" {
		t.Errorf("second add printed %q", stdout)
	}
}

func TestAdd_WithNoTextIsAUsageError(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := track(t, dir, "add")
	if code != 2 || stdout != "" {
		t.Errorf("exit %d, stdout %q; want exit 2 and nothing on stdout", code, stdout)
	}
	if !strings.Contains(stderr, "track add") || !strings.Contains(stderr, "text") {
		t.Errorf("stderr = %q, want usage for track add", stderr)
	}
	if tasks, _ := stored(t, dir).Tasks(context.Background()); len(tasks) != 0 {
		t.Errorf("tasks = %+v, want none", tasks)
	}
}

func TestAdd_ATagsOnlyTextIsRefusedAndCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := track(t, dir, "add", "##docs")
	if code != 1 || !strings.HasPrefix(stderr, "track: ") || !strings.Contains(stderr, "title") {
		t.Errorf("exit %d, stderr %q; want exit 1 and a title message", code, stderr)
	}
	if tasks, _ := stored(t, dir).Tasks(context.Background()); len(tasks) != 0 {
		t.Errorf("tasks = %+v, want none", tasks)
	}
}

func TestNote_CreatesAnUnfiledNoteWithNormalisedWhitespace(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := track(t, dir, "note", "check   the\tretry logic")
	if code != 0 || stderr != "" || stdout != "Saved note 1 to the inbox\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	notes, err := stored(t, dir).Notes(context.Background())
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes = %+v, %v", notes, err)
	}
	if n := notes[0]; n.TaskID != 0 || n.Text != "check the retry logic" {
		t.Errorf("note = %+v, want an Unfiled \"check the retry logic\"", n)
	}
}

func TestNote_AnEmptyNoteIsRefusedAndAMissingOneIsAUsageError(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := track(t, dir, "note", "   "); code != 1 || !strings.HasPrefix(stderr, "track: ") {
		t.Errorf("blank note: exit %d, stderr %q; want exit 1", code, stderr)
	}
	if code, _, _ := track(t, dir, "note"); code != 2 {
		t.Errorf("no note text: exit %d, want 2", code)
	}
	if notes, _ := stored(t, dir).Notes(context.Background()); len(notes) != 0 {
		t.Errorf("notes = %+v, want none", notes)
	}
}

func TestSubcommands_DoubleDashLetsTheTextStartWithADash(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := track(t, dir, "note", "--", "- follow up"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	notes, _ := stored(t, dir).Notes(context.Background())
	if len(notes) != 1 || notes[0].Text != "- follow up" {
		t.Errorf("notes = %+v, want \"- follow up\"", notes)
	}
}

func TestSubcommands_HelpPrintsUsageAndSucceeds(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"add", "note"} {
		var errOut bytes.Buffer
		code := realMain([]string{"--data-dir", dir, name, "-h"}, env(nil), &bytes.Buffer{}, &errOut)
		if code != 0 || !strings.Contains(errOut.String(), "track "+name) {
			t.Errorf("%s -h: exit %d, usage %q", name, code, errOut.String())
		}
	}
}

func TestRealMain_AnUnknownSubcommandOrFlagIsAUsageError(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := track(t, dir, "frobnicate")
	if code != 2 || !strings.Contains(stderr, `"frobnicate"`) || !strings.Contains(stderr, "add") || !strings.Contains(stderr, "note") {
		t.Errorf("unknown subcommand: exit %d, stderr %q; want exit 2 naming it and listing the real ones", code, stderr)
	}
	if code, _, _ := track(t, dir, "add", "--nope", "x"); code != 2 {
		t.Errorf("unknown flag after the subcommand: exit %d, want 2", code)
	}
	if code := realMain([]string{"--nope"}, env(nil), &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Errorf("unknown global flag: exit %d, want 2", code)
	}
	if code := realMain([]string{"--focus-duration", "soon", "add", "x"}, env(nil), &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Errorf("bad flag value: exit %d, want 2", code)
	}
}

func TestRealMain_HelpListsTheSubcommands(t *testing.T) {
	var errOut bytes.Buffer
	if code := realMain([]string{"--help"}, env(nil), &bytes.Buffer{}, &errOut); code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	for _, want := range []string{"track add", "track note"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("usage lacks %q:\n%s", want, errOut.String())
		}
	}
}

func TestSubcommands_DoNotNeedAWorkingConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfg := writeConfigFile(t, t.TempDir(), `focus_duration = "soon"`)
	var out, errOut bytes.Buffer
	code := realMain([]string{"--data-dir", dir, "--config", cfg, "add", "still works"}, env(nil), &out, &errOut)
	if code != 0 || out.String() != "Added task 1: still works\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}
