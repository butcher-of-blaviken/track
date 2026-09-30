package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// ReportVersion is the version of the report's JSON layout. Like the export's,
// keys are only ever added to a version.
const ReportVersion = 1

// worked is a Task that was worked on in a day: it had focused time, or a note
// written on it, or both.
type worked struct {
	Task     core.Task
	Focused  time.Duration
	Sessions int
	Notes    []core.Note
}

// workedOn merges a day's focused Tasks, largest first, with its notes: a Task
// with notes but no focused time follows them, by Task ID.
func workedOn(r core.Report) []worked {
	notes := map[core.TaskID][]core.Note{}
	for _, tn := range r.Notes {
		notes[tn.Task.ID] = tn.Notes
	}
	out := []worked{}
	for _, tt := range r.Tasks {
		out = append(out, worked{Task: tt.Task, Focused: tt.Focused, Sessions: tt.Sessions, Notes: notes[tt.Task.ID]})
		delete(notes, tt.Task.ID)
	}
	for _, tn := range r.Notes {
		if _, left := notes[tn.Task.ID]; left {
			out = append(out, worked{Task: tn.Task, Notes: tn.Notes})
		}
	}
	return out
}

// ReportMarkdown writes the days, in the order given, as an update to paste
// into a standup, a message or a pull request: per day what was finished, what
// was worked on with the notes written on it, and the notes that belong to no
// Task. A section with nothing in it is left out.
func ReportMarkdown(w io.Writer, days []core.Report) error {
	var b strings.Builder
	for i, r := range days {
		if i > 0 {
			b.WriteString("\n")
		}
		writeDay(&b, r)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeDay(b *strings.Builder, r core.Report) {
	heading := "## " + r.From.Format("Mon 2 Jan")
	if r.Focused > 0 {
		heading += ": " + spoken(r.Focused) + " focused"
	}
	b.WriteString(heading + "\n")
	work := workedOn(r)
	if len(r.Finished)+len(work)+len(r.Unfiled) == 0 {
		b.WriteString("\nNothing recorded.\n")
		return
	}
	if len(r.Finished) > 0 {
		b.WriteString("\n### Finished\n")
		for _, t := range r.Finished {
			b.WriteString("- " + titled(t, false) + "\n")
		}
	}
	if len(work) > 0 {
		b.WriteString("\n### Worked on\n")
		for _, w := range work {
			line := "- " + titled(w.Task, true)
			if w.Sessions > 0 {
				line += " · " + spoken(w.Focused) + " (" + fmt.Sprintf("%d %s", w.Sessions, plural(w.Sessions, "session")) + ")"
			}
			b.WriteString(line + "\n")
			for _, n := range w.Notes {
				b.WriteString("  - " + oneLine(n.Text) + "\n")
			}
		}
	}
	if len(r.Unfiled) > 0 {
		b.WriteString("\n### Notes\n")
		for _, n := range r.Unfiled {
			b.WriteString("- " + oneLine(n.Text) + "\n")
		}
	}
}

// titled is a Task's title, in bold if asked, followed by its Tags as #tags.
func titled(t core.Task, bold bool) string {
	title := oneLine(t.Title)
	if bold {
		title = "**" + title + "**"
	}
	for _, tag := range t.Tags {
		title += " #" + tag
	}
	return title
}

// spoken is a duration as "2h 10m", "25m" or, under a minute, "40s".
func spoken(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	m := int(d.Round(time.Minute) / time.Minute)
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", m)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

type jsonReport struct {
	Version int       `json:"version"`
	Days    []jsonDay `json:"days"`
}

type jsonDay struct {
	// Date is the local calendar day; From and To are its bounds in UTC.
	Date           string           `json:"date"`
	From           time.Time        `json:"from"`
	To             time.Time        `json:"to"`
	FocusedSeconds int64            `json:"focused_seconds"`
	Sessions       int              `json:"sessions"`
	Finished       []jsonFinished   `json:"finished"`
	Tasks          []jsonWorkedTask `json:"tasks"`
	UnfiledNotes   []jsonNote       `json:"unfiled_notes"`
}

type jsonFinished struct {
	ID     core.TaskID `json:"id"`
	Title  string      `json:"title"`
	Tags   []string    `json:"tags"`
	DoneAt time.Time   `json:"done_at"`
}

type jsonWorkedTask struct {
	ID             core.TaskID `json:"id"`
	Title          string      `json:"title"`
	Tags           []string    `json:"tags"`
	FocusedSeconds int64       `json:"focused_seconds"`
	Sessions       int         `json:"sessions"`
	Notes          []jsonNote  `json:"notes"`
}

// ReportJSON writes the days as JSON with the same conventions as JSON: snake_case
// keys, UTC RFC 3339 times, durations as whole seconds and empty arrays rather
// than null. A Task that was only written on has focused_seconds 0.
func ReportJSON(w io.Writer, days []core.Report) error {
	out := jsonReport{Version: ReportVersion, Days: make([]jsonDay, 0, len(days))}
	for _, r := range days {
		d := jsonDay{
			Date: r.From.Format("2006-01-02"), From: utc(r.From), To: utc(r.To),
			FocusedSeconds: int64(r.Focused / time.Second), Sessions: r.Sessions,
			Finished: make([]jsonFinished, 0, len(r.Finished)), Tasks: []jsonWorkedTask{}, UnfiledNotes: notes(r.Unfiled),
		}
		for _, t := range r.Finished {
			d.Finished = append(d.Finished, jsonFinished{ID: t.ID, Title: t.Title, Tags: append([]string{}, t.Tags...), DoneAt: utc(*t.DoneAt)})
		}
		for _, wk := range workedOn(r) {
			d.Tasks = append(d.Tasks, jsonWorkedTask{
				ID: wk.Task.ID, Title: wk.Task.Title, Tags: append([]string{}, wk.Task.Tags...),
				FocusedSeconds: int64(wk.Focused / time.Second), Sessions: wk.Sessions, Notes: notes(wk.Notes),
			})
		}
		out.Days = append(out.Days, d)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
