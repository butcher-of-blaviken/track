package export_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/export"
)

// A zone that is not UTC, so the local day is what is named and the JSON
// bounds are converted.
var zone = time.FixedZone("EEST", 3*60*60)

func day(d, h, m int) time.Time { return time.Date(2026, 9, d, h, m, 0, 0, zone) }

func task(id core.TaskID, title string, tags ...string) core.Task {
	return core.Task{ID: id, Title: title, State: core.StateActive, Tags: tags, CreatedAt: day(1, 9, 0)}
}

// busyDay has every kind of entry: two finished tasks, tasks with focused time
// and notes, a task with only notes, and unfiled notes.
func busyDay() core.Report {
	prd, review, noted := task(1, "write the PRD", "docs", "Q1"), task(2, "review PR 91", "work"), task(3, "call   the\tbank")
	done := day(30, 16, 40)
	released := task(4, "release v1.0.0", "track")
	released.State, released.DoneAt = core.StateDone, &done
	earlier := day(30, 10, 5)
	small := task(5, "fix typo")
	small.State, small.DoneAt = core.StateDone, &earlier
	return core.Report{
		From: day(30, 0, 0), To: day(31, 0, 0), Focused: 2*time.Hour + 10*time.Minute, Sessions: 5,
		Tasks: []core.TaskTime{
			{Task: prd, Focused: 85 * time.Minute, Sessions: 3},
			{Task: review, Focused: 45 * time.Minute, Sessions: 2},
		},
		Notes: []core.TaskNotes{
			{Task: prd, Notes: []core.Note{
				{ID: 1, TaskID: 1, SessionID: 1, Text: "left off at section 2; the retry numbers are still missing", CreatedAt: day(30, 10, 0)},
				{ID: 2, TaskID: 1, Text: "stopped early for a meeting,\n  pick up at the risks list", CreatedAt: day(30, 14, 0)},
			}},
			{Task: noted, Notes: []core.Note{{ID: 3, TaskID: 3, Text: "they close at 5", CreatedAt: day(30, 11, 0)}}},
		},
		Unfiled:  []core.Note{{ID: 4, Text: "ask whether the export needs a version field", CreatedAt: day(30, 12, 0)}},
		Finished: []core.Task{small, released},
	}
}

func quietDay() core.Report {
	return core.Report{From: day(29, 0, 0), To: day(30, 0, 0)}
}

// onlyNotes is a day with no focused time at all.
func onlyNotes() core.Report {
	return core.Report{From: day(28, 0, 0), To: day(29, 0, 0), Unfiled: []core.Note{{ID: 9, Text: "a stray thought", CreatedAt: day(28, 8, 0)}}}
}

func TestReportMarkdown_MatchesTheGoldenFiles(t *testing.T) {
	for name, days := range map[string][]core.Report{
		"report_busy.md":    {busyDay()},
		"report_empty.md":   {quietDay()},
		"report_standup.md": {quietDay(), busyDay()},
		"report_notes.md":   {onlyNotes()},
	} {
		golden(t, name, render(t, func(b *bytes.Buffer) error { return export.ReportMarkdown(b, days) }))
	}
}

func TestReportJSON_MatchesTheGoldenFiles(t *testing.T) {
	for name, days := range map[string][]core.Report{
		"report_busy.json":    {busyDay()},
		"report_standup.json": {quietDay(), busyDay()},
	} {
		golden(t, name, render(t, func(b *bytes.Buffer) error { return export.ReportJSON(b, days) }))
	}
}

func TestReportMarkdown_NoDaysWritesNothing(t *testing.T) {
	if out := render(t, func(b *bytes.Buffer) error { return export.ReportMarkdown(b, nil) }); len(out) != 0 {
		t.Errorf("no days wrote %q", out)
	}
}

func TestReportMarkdown_TheFocusedTimeIsWordedLikeTheApp(t *testing.T) {
	for d, want := range map[time.Duration]string{
		40 * time.Second: "40s focused", 25 * time.Minute: "25m focused", time.Hour: "1h focused",
		2*time.Hour + 10*time.Minute: "2h 10m focused", 89*time.Minute + 40*time.Second: "1h 30m focused",
	} {
		r := core.Report{From: day(30, 0, 0), To: day(31, 0, 0), Focused: d}
		out := render(t, func(b *bytes.Buffer) error { return export.ReportMarkdown(b, []core.Report{r}) })
		if !strings.Contains(string(out), ": "+want) {
			t.Errorf("%v: %q lacks %q", d, out, want)
		}
	}
}

func TestReportMarkdown_ANoteCannotBreakOutOfItsBullet(t *testing.T) {
	r := busyDay()
	r.Unfiled = []core.Note{{ID: 7, Text: "line one\n## Not a heading\n- not a bullet", CreatedAt: day(30, 9, 0)}}
	out := string(render(t, func(b *bytes.Buffer) error { return export.ReportMarkdown(b, []core.Report{r}) }))
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "## Not") || strings.HasPrefix(line, "- not") {
			t.Errorf("a note started a line of its own: %q", line)
		}
	}
}

func TestReportJSON_DecodesBackToTheSameFacts(t *testing.T) {
	out := render(t, func(b *bytes.Buffer) error { return export.ReportJSON(b, []core.Report{quietDay(), busyDay()}) })
	var got struct {
		Version int `json:"version"`
		Days    []struct {
			Date           string    `json:"date"`
			From           time.Time `json:"from"`
			FocusedSeconds int       `json:"focused_seconds"`
			Finished       []struct {
				Title  string    `json:"title"`
				DoneAt time.Time `json:"done_at"`
			} `json:"finished"`
			Tasks []struct {
				ID             int    `json:"id"`
				Title          string `json:"title"`
				FocusedSeconds int    `json:"focused_seconds"`
				Sessions       int    `json:"sessions"`
				Notes          []struct {
					SessionID *int   `json:"session_id"`
					Text      string `json:"text"`
				} `json:"notes"`
			} `json:"tasks"`
			UnfiledNotes []struct{ Text string } `json:"unfiled_notes"`
		} `json:"days"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Days) != 2 || got.Days[0].Date != "2026-09-29" || got.Days[1].Date != "2026-09-30" {
		t.Fatalf("header: %+v", got)
	}
	if !got.Days[1].From.Equal(day(30, 0, 0)) || got.Days[1].From.Location() != time.UTC {
		t.Errorf("from = %v, want midnight local expressed in UTC", got.Days[1].From)
	}
	busy := got.Days[1]
	if busy.FocusedSeconds != 7800 || len(busy.Finished) != 2 || busy.Finished[0].Title != "fix typo" || !busy.Finished[1].DoneAt.Equal(day(30, 16, 40)) {
		t.Errorf("busy day: %+v", busy)
	}
	if len(busy.Tasks) != 3 || busy.Tasks[0].Title != "write the PRD" || busy.Tasks[0].FocusedSeconds != 5100 || busy.Tasks[0].Sessions != 3 {
		t.Fatalf("tasks: %+v", busy.Tasks)
	}
	if n := busy.Tasks[0].Notes; len(n) != 2 || n[0].SessionID == nil || n[1].SessionID != nil {
		t.Errorf("only the hand-off note has a session: %+v", n)
	}
	if last := busy.Tasks[2]; last.FocusedSeconds != 0 || last.Sessions != 0 || len(last.Notes) != 1 {
		t.Errorf("a task that was only written on: %+v", last)
	}
	if len(busy.UnfiledNotes) != 1 {
		t.Errorf("unfiled: %+v", busy.UnfiledNotes)
	}
	// A quiet day has empty lists, never null (a note's session_id is null when it has none).
	for _, list := range []string{"finished", "tasks", "notes", "tags", "unfiled_notes"} {
		if strings.Contains(string(out), `"`+list+`": null`) {
			t.Errorf("%s is null, want an empty list:\n%s", list, out)
		}
	}
}

// Scripts read the report, so version 1 keeps every key it had
// (docs/adr/0003-backward-compatibility.md): keys may be added, never removed.
func TestReportJSON_Version1KeepsEveryKeyItHad(t *testing.T) {
	frozen, err := os.ReadFile(filepath.Join("testdata", "report_json_v1_paths.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	out := render(t, func(b *bytes.Buffer) error { return export.ReportJSON(b, []core.Report{busyDay()}) })
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	flatten(doc, "", have)
	for _, line := range strings.Split(string(frozen), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !have[line] {
			t.Errorf("the version 1 report lost the key %s", line)
		}
	}
}
