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
}

// SearchTasks ranks tasks against query, best first, keeping the input order
// for equal matches and for an empty query, which matches every Task. It
// searches titles and Tags.
func SearchTasks(query string, tasks []Task) []TaskMatch {
	docs := make([]Doc, len(tasks))
	for i, task := range tasks {
		docs[i] = Doc{Title: task.Title, Tags: task.Tags}
	}
	results := Rank(query, docs)
	out := make([]TaskMatch, len(results))
	for i, r := range results {
		out[i] = TaskMatch{Task: tasks[r.Doc], Score: r.Score}
		for _, m := range r.Matches {
			switch m.Field {
			case FieldTitle:
				out[i].Runes = m.Runes
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
