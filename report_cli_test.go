package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// midnight is the start of the local day n days from today.
func midnight(n int) time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d+n, 0, 0, 0, 0, time.Local)
}

// heading is how a report titles a day.
func heading(n int) string { return "## " + midnight(n).Format("Mon 2 Jan") }

// seedYesterday writes a day of work onto yesterday, as the app would have:
// 30 minutes on a task with a hand-off note, a note written on another task, an
// unfiled note and a task finished that afternoon.
func seedYesterday(t *testing.T, dir string) {
	t.Helper()
	y := midnight(-1)
	created := y.Add(-24 * time.Hour)
	err := stored(t, dir).Update(t.Context(), func(tx core.Tx) error {
		prd, err := tx.CreateTask(core.Task{Title: "write the PRD", State: core.StateActive, Tags: []string{"docs"}, CreatedAt: created})
		if err != nil {
			return err
		}
		other, err := tx.CreateTask(core.Task{Title: "call the bank", State: core.StateActive, CreatedAt: created})
		if err != nil {
			return err
		}
		done := y.Add(16 * time.Hour)
		if _, err = tx.CreateTask(core.Task{Title: "release it", State: core.StateDone, CreatedAt: created, DoneAt: &done}); err != nil {
			return err
		}
		session, err := tx.CreateSession(core.FocusSession{TaskID: prd, StartedAt: y.Add(10 * time.Hour), PlannedDuration: 30 * time.Minute, BreakDuration: 10 * time.Minute})
		if err != nil {
			return err
		}
		for _, n := range []core.Note{
			{TaskID: prd, SessionID: session, Text: "left off at section 2", CreatedAt: y.Add(10*time.Hour + 31*time.Minute)},
			{TaskID: other, Text: "they close at 5", CreatedAt: y.Add(11 * time.Hour)},
			{Text: "a stray idea", CreatedAt: y.Add(12 * time.Hour)},
		} {
			if _, err := tx.CreateNote(n); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReport_DefaultsToTodayAsMarkdown(t *testing.T) {
	dir := t.TempDir()
	track(t, dir, "note", "remember the milk")
	code, stdout, stderr := track(t, dir, "report")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{heading(0), "### Notes", "- remember the milk"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestReport_AnEmptyDayIsSaidSo(t *testing.T) {
	code, stdout, _ := track(t, t.TempDir(), "report", "--day", "yesterday")
	if code != 0 || stdout != heading(-1)+"\n\nNothing recorded.\n" {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestReport_DayTakesTodayYesterdayOrADate(t *testing.T) {
	dir := t.TempDir()
	seedYesterday(t, dir)
	track(t, dir, "note", "only today")

	_, byWord, _ := track(t, dir, "report", "--day", "yesterday")
	_, byDate, _ := track(t, dir, "report", "--day", midnight(-1).Format("2006-01-02"))
	_, upper, _ := track(t, dir, "report", "--day", "YESTERDAY")
	if byWord != byDate || byWord != upper {
		t.Errorf("the same day said three ways differs:\n%s\n---\n%s\n---\n%s", byWord, byDate, upper)
	}
	for _, want := range []string{
		heading(-1) + ": 30m focused",
		"### Finished\n- release it",
		"- **write the PRD** #docs · 30m (1 session)\n  - left off at section 2",
		"- **call the bank**\n  - they close at 5",
		"### Notes\n- a stray idea",
	} {
		if !strings.Contains(byWord, want) {
			t.Errorf("yesterday lacks %q:\n%s", want, byWord)
		}
	}
	if strings.Contains(byWord, "only today") {
		t.Errorf("yesterday shows today's note:\n%s", byWord)
	}
	_, today, _ := track(t, dir, "report", "--day", "today")
	if !strings.Contains(today, "only today") || strings.Contains(today, "write the PRD") {
		t.Errorf("today:\n%s", today)
	}
}

func TestReport_StandupIsYesterdayThenToday(t *testing.T) {
	dir := t.TempDir()
	seedYesterday(t, dir)
	track(t, dir, "note", "only today")
	code, stdout, _ := track(t, dir, "report", "--standup")
	yesterday, today := strings.Index(stdout, heading(-1)), strings.Index(stdout, heading(0))
	if code != 0 || yesterday < 0 || today <= yesterday {
		t.Fatalf("exit %d; want yesterday's heading and then today's:\n%s", code, stdout)
	}
	if !strings.Contains(stdout[yesterday:today], "write the PRD") || !strings.Contains(stdout[today:], "only today") {
		t.Errorf("each day under its heading:\n%s", stdout)
	}
}

func TestReport_JSONAndTheMarkdownAlias(t *testing.T) {
	dir := t.TempDir()
	seedYesterday(t, dir)
	_, stdout, _ := track(t, dir, "report", "--day", "yesterday", "--format", "json")
	var got struct {
		Version int `json:"version"`
		Days    []struct {
			Date           string `json:"date"`
			FocusedSeconds int    `json:"focused_seconds"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("%v:\n%s", err, stdout)
	}
	if got.Version != 1 || len(got.Days) != 1 || got.Days[0].Date != midnight(-1).Format("2006-01-02") || got.Days[0].FocusedSeconds != 1800 {
		t.Errorf("json = %+v", got)
	}
	_, std, _ := track(t, dir, "report", "-f", "json", "--standup")
	if !strings.Contains(std, midnight(0).Format("2006-01-02")) || strings.Count(std, `"date"`) != 2 {
		t.Errorf("standup json has two days:\n%s", std)
	}
	_, md, _ := track(t, dir, "report", "--day", "yesterday", "-f", "md")
	_, markdown, _ := track(t, dir, "report", "--day", "yesterday", "--format", "markdown")
	if md != markdown || !strings.HasPrefix(md, "## ") {
		t.Errorf("md alias differs:\n%s", md)
	}
}

func TestReport_OutputWritesAFileAndPrintsNothing(t *testing.T) {
	dir := t.TempDir()
	seedYesterday(t, dir)
	out := filepath.Join(t.TempDir(), "standup.md")
	code, stdout, stderr := track(t, dir, "report", "--day", "yesterday", "-o", out)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if data, err := os.ReadFile(out); err != nil || !strings.Contains(string(data), "write the PRD") {
		t.Errorf("file = %q, %v", data, err)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(out), "*.tmp*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
	// A failed write leaves an existing file as it was.
	if err := os.WriteFile(out, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := track(t, dir, "report", "-o", filepath.Join(out, "no", "such")); code != 1 {
		t.Errorf("a path under a file: exit %d, want 1", code)
	}
	if data, _ := os.ReadFile(out); string(data) != "precious" {
		t.Errorf("existing file changed: %q", data)
	}
}

func TestReport_UsageMistakesExitTwoAndPrintNothing(t *testing.T) {
	future := midnight(1).Format("2006-01-02")
	cases := map[string][]string{
		"an unknown word":         {"report", "--day", "tomorrow"},
		"a bad date":              {"report", "--day", "2026-13-40"},
		"a date with no zeros":    {"report", "--day", "2026-9-3"},
		"garbage":                 {"report", "--day", "last tuesday"},
		"a day that has not come": {"report", "--day", future},
		"standup and a day":       {"report", "--standup", "--day", "yesterday"},
		"standup and today":       {"report", "--standup", "--day", "today"},
		"a bad format":            {"report", "--format", "yaml"},
		"a stray argument":        {"report", "yesterday"},
		"an unknown flag":         {"report", "--nope"},
		"a missing day":           {"report", "--day"},
	}
	for name, args := range cases {
		if code, stdout, stderr := track(t, t.TempDir(), args...); code != 2 || stdout != "" || stderr == "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want 2, nothing, a message", name, code, stdout, stderr)
		}
	}
	if _, _, stderr := track(t, t.TempDir(), "report", "--day", "tomorrow"); !strings.Contains(stderr, `"tomorrow"`) || !strings.Contains(stderr, "yesterday") {
		t.Errorf("the message should name the word and the ones that work: %q", stderr)
	}
	if code, _, stderr := track(t, t.TempDir(), "report", "-h"); code != 0 || !strings.Contains(stderr, "Usage: track report") {
		t.Errorf("-h: exit %d, stderr %q", code, stderr)
	}
}

func TestReport_DoesNotNeedAWorkingConfigFileAndOnlyReads(t *testing.T) {
	dir := t.TempDir()
	seedYesterday(t, dir)
	cfg := writeConfigFile(t, t.TempDir(), `focus_duration = "soon"`)
	before, _ := stored(t, dir).Tasks(t.Context())
	var out, errOut strings.Builder
	code := realMain([]string{"--data-dir", dir, "--config", cfg, "report", "--day", "yesterday"}, env(nil), &out, &errOut)
	if code != 0 || !strings.Contains(out.String(), "write the PRD") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	after, _ := stored(t, dir).Tasks(t.Context())
	if len(before) != len(after) {
		t.Errorf("tasks %d before, %d after a report", len(before), len(after))
	}
}

func TestReport_IsInTheHelpAndTheUnknownCommandMessage(t *testing.T) {
	var errOut strings.Builder
	realMain([]string{"--help"}, env(nil), &strings.Builder{}, &errOut)
	if !strings.Contains(errOut.String(), "track report") {
		t.Errorf("usage lacks track report:\n%s", errOut.String())
	}
	if _, _, stderr := track(t, t.TempDir(), "frobnicate"); !strings.Contains(stderr, "report") {
		t.Errorf("the unknown-command message lacks report: %q", stderr)
	}
}
