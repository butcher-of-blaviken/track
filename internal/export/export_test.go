package export_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/export"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func at(h, m int) time.Time      { return time.Date(2026, 3, 2, h, m, 0, 0, time.UTC) }
func ptr(t time.Time) *time.Time { return &t }

// sample has every kind of thing: tasks in each state, a completed session, a
// stopped one, a hand-off note, a filed note and an unfiled one.
func sample() core.Export {
	return core.Export{
		ExportedAt: at(17, 0),
		Tasks: []core.ExportTask{
			{
				Task:    core.Task{ID: 1, Title: "write the PRD", State: core.StateActive, Tags: []string{"docs", "Q1"}, CreatedAt: at(9, 0)},
				Focused: 42 * time.Minute,
				Sessions: []core.FocusSession{
					{ID: 1, TaskID: 1, StartedAt: at(9, 5), PlannedDuration: 30 * time.Minute, BreakDuration: 5 * time.Minute, HandoffAt: ptr(at(9, 40))},
					{ID: 2, TaskID: 1, StartedAt: at(10, 0), PlannedDuration: 30 * time.Minute, StoppedAt: ptr(at(10, 12)), BreakDuration: 15 * time.Minute, LongBreak: true, SkippedBreak: 2 * time.Minute},
				},
				Notes: []core.Note{
					{ID: 1, TaskID: 1, SessionID: 1, Text: "left off at section 2", CreatedAt: at(9, 40)},
					{ID: 2, TaskID: 1, Text: "# not a heading\n---\n`code`", CreatedAt: at(11, 0)},
				},
			},
			{Task: core.Task{ID: 2, Title: "old thing", State: core.StateDone, CreatedAt: at(8, 0)}, Sessions: []core.FocusSession{}, Notes: []core.Note{}},
			{Task: core.Task{ID: 3, Title: "dropped", State: core.StateArchived, CreatedAt: at(8, 30)}, Sessions: []core.FocusSession{}, Notes: []core.Note{}},
		},
		Unfiled: []core.Note{{ID: 3, Text: "call the bank", CreatedAt: at(12, 0)}},
	}
}

func empty() core.Export {
	return core.Export{ExportedAt: at(17, 0), Tasks: []core.ExportTask{}, Unfiled: []core.Note{}}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs (run with -update to accept):\n%s", name, got)
	}
}

func render(t *testing.T, f func(*bytes.Buffer) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := f(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestJSON_MatchesTheGoldenFile(t *testing.T) {
	golden(t, "sample.json", render(t, func(b *bytes.Buffer) error { return export.JSON(b, sample()) }))
}

func TestJSON_DecodesBackToTheSameFacts(t *testing.T) {
	out := render(t, func(b *bytes.Buffer) error { return export.JSON(b, sample()) })
	var got struct {
		Version    int       `json:"version"`
		ExportedAt time.Time `json:"exported_at"`
		Tasks      []struct {
			ID             int      `json:"id"`
			Title          string   `json:"title"`
			State          string   `json:"state"`
			Tags           []string `json:"tags"`
			FocusedSeconds int      `json:"focused_seconds"`
			Sessions       []struct {
				PlannedSeconds int        `json:"planned_seconds"`
				StoppedAt      *time.Time `json:"stopped_at"`
				HandoffAt      *time.Time `json:"handoff_at"`
				LongBreak      bool       `json:"long_break"`
			} `json:"sessions"`
			Notes []struct {
				SessionID *int   `json:"session_id"`
				Text      string `json:"text"`
			} `json:"notes"`
		} `json:"tasks"`
		Unfiled []struct{ Text string } `json:"unfiled_notes"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || !got.ExportedAt.Equal(at(17, 0)) || len(got.Tasks) != 3 {
		t.Fatalf("header: %+v", got)
	}
	first := got.Tasks[0]
	if first.Title != "write the PRD" || first.State != "active" || strings.Join(first.Tags, ",") != "docs,Q1" || first.FocusedSeconds != 42*60 {
		t.Errorf("first task: %+v", first)
	}
	if len(first.Sessions) != 2 || first.Sessions[0].StoppedAt != nil || first.Sessions[0].HandoffAt == nil || first.Sessions[1].StoppedAt == nil || !first.Sessions[1].LongBreak || first.Sessions[1].PlannedSeconds != 1800 {
		t.Errorf("sessions: %+v", first.Sessions)
	}
	if first.Notes[0].SessionID == nil || *first.Notes[0].SessionID != 1 || first.Notes[1].SessionID != nil || first.Notes[1].Text != "# not a heading\n---\n`code`" {
		t.Errorf("notes: %+v", first.Notes)
	}
	if len(got.Unfiled) != 1 || got.Unfiled[0].Text != "call the bank" {
		t.Errorf("unfiled: %+v", got.Unfiled)
	}
}

func TestJSON_AnEmptyExportHasEmptyArraysNotNull(t *testing.T) {
	out := string(render(t, func(b *bytes.Buffer) error { return export.JSON(b, empty()) }))
	for _, want := range []string{`"tasks": []`, `"unfiled_notes": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

func TestMarkdown_MatchesTheGoldenFile(t *testing.T) {
	golden(t, "sample.md", render(t, func(b *bytes.Buffer) error { return export.Markdown(b, sample()) }))
}

func TestMarkdown_NoteTextCannotStartAHeadingOrRule(t *testing.T) {
	out := string(render(t, func(b *bytes.Buffer) error { return export.Markdown(b, sample()) }))
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# not a heading") || line == "---" || line == "`code`" {
			t.Errorf("note text leaked out of its quote: %q", line)
		}
	}
	if !strings.Contains(out, "> # not a heading") {
		t.Errorf("quoted note missing:\n%s", out)
	}
}

func TestMarkdown_AnEmptyExportSaysSo(t *testing.T) {
	out := string(render(t, func(b *bytes.Buffer) error { return export.Markdown(b, empty()) }))
	if !strings.Contains(out, "No tasks.") {
		t.Errorf("got:\n%s", out)
	}
}
