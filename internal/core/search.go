package core

// TaskMatch is a Task that matched a search. Runes are the matched offsets in
// its title, ascending, for highlighting.
type TaskMatch struct {
	Task  Task
	Score int
	Runes []int
	// TagRunes are the matched offsets in the Tags that matched, by index into
	// Task.Tags.
	TagRunes map[int][]int
	// Note is the note that won a token of the query, or nil if none did. Notes
	// are not shown when the title or a Tag matched better.
	Note *NoteMatch
}

// NoteMatch is the note a search matched. Runes are the matched offsets in its text.
type NoteMatch struct {
	Note  Note
	Runes []int
}

// SearchTasks ranks tasks against query, best first, keeping the input order
// for equal matches and for an empty query, which matches every Task. It
// searches titles, Tags and notes. notes is each Task's log, oldest first, as
// NotesByTask returns it, and may be nil.
func SearchTasks(query string, tasks []Task, notes map[TaskID][]Note) []TaskMatch {
	docs := make([]Doc, len(tasks))
	for i, task := range tasks {
		docs[i] = Doc{Title: task.Title, Tags: task.Tags}
		// Newest first, so the newest of equally good notes is the one reported.
		log := notes[task.ID]
		for j := len(log) - 1; j >= 0; j-- {
			docs[i].Notes = append(docs[i].Notes, log[j].Text)
		}
	}
	results := Rank(query, docs)
	out := make([]TaskMatch, len(results))
	for i, r := range results {
		out[i] = TaskMatch{Task: tasks[r.Doc], Score: r.Score}
		for _, m := range r.Matches {
			switch m.Field {
			case FieldTitle:
				out[i].Runes = m.Runes
			case FieldNote:
				if out[i].Note == nil {
					log := notes[tasks[r.Doc].ID]
					out[i].Note = &NoteMatch{Note: log[len(log)-1-m.Entry], Runes: m.Runes}
				}
			case FieldTag:
				if out[i].TagRunes == nil {
					out[i].TagRunes = map[int][]int{}
				}
				out[i].TagRunes[m.Entry] = m.Runes
			}
		}
	}
	return out
}
