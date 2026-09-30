// Package export renders a core.Export as JSON or Markdown, so the data can be
// taken out of the database.
package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// Version is the JSON layout's version. Bump it when a key changes meaning.
const Version = 1

type jsonExport struct {
	Version    int        `json:"version"`
	ExportedAt time.Time  `json:"exported_at"`
	Tasks      []jsonTask `json:"tasks"`
	Unfiled    []jsonNote `json:"unfiled_notes"`
}

type jsonTask struct {
	ID             core.TaskID   `json:"id"`
	Title          string        `json:"title"`
	State          string        `json:"state"`
	Tags           []string      `json:"tags"`
	CreatedAt      time.Time     `json:"created_at"`
	FocusedSeconds int64         `json:"focused_seconds"`
	Sessions       []jsonSession `json:"sessions"`
	Notes          []jsonNote    `json:"notes"`
}

type jsonSession struct {
	ID                  core.SessionID `json:"id"`
	StartedAt           time.Time      `json:"started_at"`
	Outcome             string         `json:"outcome"`
	PlannedSeconds      int64          `json:"planned_seconds"`
	StoppedAt           *time.Time     `json:"stopped_at"`
	BreakSeconds        int64          `json:"break_seconds"`
	LongBreak           bool           `json:"long_break"`
	SkippedBreakSeconds int64          `json:"skipped_break_seconds"`
	HandoffAt           *time.Time     `json:"handoff_at"`
}

type jsonNote struct {
	ID        core.NoteID     `json:"id"`
	SessionID *core.SessionID `json:"session_id"`
	Text      string          `json:"text"`
	CreatedAt time.Time       `json:"created_at"`
}

func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func notes(in []core.Note) []jsonNote {
	out := make([]jsonNote, 0, len(in))
	for _, n := range in {
		jn := jsonNote{ID: n.ID, Text: n.Text, CreatedAt: utc(n.CreatedAt)}
		if n.SessionID != 0 {
			id := n.SessionID
			jn.SessionID = &id
		}
		out = append(out, jn)
	}
	return out
}

// JSON writes e as indented JSON: snake_case keys, UTC RFC 3339 times,
// durations as whole seconds, and empty arrays rather than null.
func JSON(w io.Writer, e core.Export) error {
	out := jsonExport{Version: Version, ExportedAt: utc(e.ExportedAt), Tasks: make([]jsonTask, 0, len(e.Tasks)), Unfiled: notes(e.Unfiled)}
	for _, et := range e.Tasks {
		t := jsonTask{
			ID: et.Task.ID, Title: et.Task.Title, State: strings.ToLower(et.Task.State.String()),
			Tags: append([]string{}, et.Task.Tags...), CreatedAt: utc(et.Task.CreatedAt),
			FocusedSeconds: int64(et.Focused / time.Second),
			Sessions:       make([]jsonSession, 0, len(et.Sessions)), Notes: notes(et.Notes),
		}
		for _, s := range et.Sessions {
			t.Sessions = append(t.Sessions, jsonSession{
				ID: s.ID, StartedAt: utc(s.StartedAt), Outcome: outcome(s.Outcome(e.ExportedAt)), PlannedSeconds: int64(s.PlannedDuration / time.Second),
				StoppedAt: utcPtr(s.StoppedAt), BreakSeconds: int64(s.BreakDuration / time.Second), LongBreak: s.LongBreak,
				SkippedBreakSeconds: int64(s.SkippedBreak / time.Second), HandoffAt: utcPtr(s.HandoffAt),
			})
		}
		out.Tasks = append(out.Tasks, t)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

const stamp = "2006-01-02 15:04"

// Markdown writes e as a document: a section per Task, grouped Active, Done
// then Archived, and an Inbox of unfiled notes. Note text is quoted so that
// it cannot change the document's structure.
func Markdown(w io.Writer, e core.Export) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# track export\n\nExported %s UTC.\n", e.ExportedAt.UTC().Format(stamp))
	if len(e.Tasks) == 0 {
		b.WriteString("\nNo tasks.\n")
	}
	for _, state := range []core.State{core.StateActive, core.StateDone, core.StateArchived} {
		for _, et := range e.Tasks {
			if et.Task.State != state {
				continue
			}
			writeTask(&b, e.ExportedAt, et)
		}
	}
	b.WriteString("\n## Inbox\n")
	if len(e.Unfiled) == 0 {
		b.WriteString("\nNo unfiled notes.\n")
	}
	for _, n := range e.Unfiled {
		writeNote(&b, n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeTask(b *strings.Builder, exportedAt time.Time, et core.ExportTask) {
	t := et.Task
	fmt.Fprintf(b, "\n## %s\n\n", oneLine(t.Title))
	fmt.Fprintf(b, "- State: %s\n- Created: %s UTC\n", t.State, t.CreatedAt.UTC().Format(stamp))
	if len(t.Tags) > 0 {
		chips := make([]string, len(t.Tags))
		for i, tag := range t.Tags {
			chips[i] = "#" + tag
		}
		fmt.Fprintf(b, "- Tags: %s\n", strings.Join(chips, " "))
	}
	fmt.Fprintf(b, "- Focused: %s over %d %s\n", minutes(et.Focused), len(et.Sessions), plural(len(et.Sessions), "session"))
	for _, s := range et.Sessions {
		end := outcome(s.Outcome(exportedAt))
		if s.StoppedAt != nil {
			end = "stopped at " + s.StoppedAt.UTC().Format("15:04")
		}
		fmt.Fprintf(b, "  - %s UTC, %s planned, %s\n", s.StartedAt.UTC().Format(stamp), minutes(s.PlannedDuration), end)
	}
	if len(et.Notes) > 0 {
		b.WriteString("\nNotes:\n")
	}
	for _, n := range et.Notes {
		writeNote(b, n)
	}
}

func writeNote(b *strings.Builder, n core.Note) {
	kind := "Note"
	if n.SessionID != 0 {
		kind = "Hand-off note"
	}
	fmt.Fprintf(b, "\n%s, %s UTC:\n\n", kind, n.CreatedAt.UTC().Format(stamp))
	for _, line := range strings.Split(n.Text, "\n") {
		b.WriteString(strings.TrimRight("> "+line, " ") + "\n")
	}
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func outcome(o core.Outcome) string {
	switch o {
	case core.OutcomeRunning:
		return "running"
	case core.OutcomeCompleted:
		return "completed"
	}
	return "stopped_early"
}

// minutes formats d as whole minutes, like "42m" or "1h05m".
func minutes(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", m/60, m%60)
}
