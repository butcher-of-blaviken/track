package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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

func TestExport_DefaultsToJSONOnStdout(t *testing.T) {
	dir := t.TempDir()
	track(t, dir, "add", "write the PRD ##docs")
	track(t, dir, "note", "call the bank")

	code, stdout, stderr := track(t, dir, "export")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var got struct {
		Version int `json:"version"`
		Tasks   []struct {
			Title string   `json:"title"`
			Tags  []string `json:"tags"`
		} `json:"tasks"`
		Unfiled []struct {
			Text string `json:"text"`
		} `json:"unfiled_notes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if got.Version != 1 || len(got.Tasks) != 1 || got.Tasks[0].Title != "write the PRD" || got.Tasks[0].Tags[0] != "docs" || got.Unfiled[0].Text != "call the bank" {
		t.Errorf("export = %+v", got)
	}
}

func TestExport_MarkdownFormatAndItsAlias(t *testing.T) {
	dir := t.TempDir()
	track(t, dir, "add", "write the PRD")
	for _, format := range []string{"markdown", "md"} {
		code, stdout, _ := track(t, dir, "export", "--format", format)
		if code != 0 || !strings.Contains(stdout, "## write the PRD") || !strings.HasPrefix(stdout, "# track export") {
			t.Errorf("--format %s: exit %d, stdout %q", format, code, stdout)
		}
	}
}

func TestExport_OutputWritesAFileAndPrintsNothing(t *testing.T) {
	dir := t.TempDir()
	track(t, dir, "add", "write the PRD")
	out := filepath.Join(t.TempDir(), "backup.json")
	code, stdout, stderr := track(t, dir, "export", "-o", out)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(data), `"write the PRD"`) {
		t.Errorf("file = %q, %v", data, err)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(out), "*.tmp*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestExport_AFailedWriteLeavesAnExistingFileAlone(t *testing.T) {
	dir := t.TempDir()
	track(t, dir, "add", "x")
	out := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(out, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory cannot be replaced by a file, so the rename fails.
	bad := filepath.Join(t.TempDir(), "sub")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := track(t, dir, "export", "-o", bad); code != 1 || stderr == "" {
		t.Errorf("exit %d, stderr %q; want 1 and a message", code, stderr)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(bad), "*.tmp*")); len(left) != 0 {
		t.Errorf("a failed export left temp files: %v", left)
	}
	if code, _, _ := track(t, dir, "export", "-o", filepath.Join(out, "no", "such")); code != 1 {
		t.Errorf("a path under a file: exit %d, want 1", code)
	}
	if data, _ := os.ReadFile(out); string(data) != "precious" {
		t.Errorf("existing file changed: %q", data)
	}
}

func TestExport_UsageMistakesExitTwo(t *testing.T) {
	cases := map[string][]string{
		"bad format":     {"export", "--format", "yaml"},
		"stray argument": {"export", "extra"},
		"unknown flag":   {"export", "--nope"},
	}
	for name, args := range cases {
		if code, stdout, stderr := track(t, t.TempDir(), args...); code != 2 || stdout != "" || stderr == "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want 2, nothing, a message", name, code, stdout, stderr)
		}
	}
	if code, _, stderr := track(t, t.TempDir(), "export", "-h"); code != 0 || !strings.Contains(stderr, "Usage: track export") {
		t.Errorf("-h: exit %d, stderr %q", code, stderr)
	}
}

func TestExport_AnEmptyDatabaseExportsEmptyLists(t *testing.T) {
	code, stdout, _ := track(t, t.TempDir(), "export")
	if code != 0 || !strings.Contains(stdout, `"tasks": []`) {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestDocs_PrintsTheREADMEThisBinaryCarries(t *testing.T) {
	want, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := track(t, t.TempDir(), "docs")
	if code != 0 || stderr != "" || stdout != string(want) {
		t.Errorf("exit %d, stderr %q, %d bytes printed, want the %d bytes of README.md", code, stderr, len(stdout), len(want))
	}
}

func TestDocs_TakesNoArguments(t *testing.T) {
	if code, stdout, stderr := track(t, t.TempDir(), "docs", "now"); code != 2 || stdout != "" || stderr == "" {
		t.Errorf("exit %d, stdout %d bytes, stderr %q", code, len(stdout), stderr)
	}
}
