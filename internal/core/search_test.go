package core_test

import (
	"reflect"
	"testing"

	"github.com/butcher-of-blaviken/track/internal/core"
)

func tasksTitled(titles ...string) []core.Task {
	out := make([]core.Task, len(titles))
	for i, title := range titles {
		out[i] = core.Task{ID: core.TaskID(i + 1), Title: title}
	}
	return out
}

func TestSearchTasks_ReturnsMatchingTasksBestFirst(t *testing.T) {
	tasks := tasksTitled("fix the build", "write the PRD", "prd review")
	got := core.SearchTasks("prd", tasks, nil)
	ids := []core.TaskID{}
	for _, m := range got {
		ids = append(ids, m.Task.ID)
	}
	if want := []core.TaskID{3, 2}; !reflect.DeepEqual(ids, want) {
		t.Errorf("task order = %v, want %v", ids, want)
	}
}

func TestSearchTasks_AnEmptyQueryReturnsEveryTaskInOrder(t *testing.T) {
	tasks := tasksTitled("b", "a", "c")
	got := core.SearchTasks("  ", tasks, nil)
	if len(got) != 3 || got[0].Task.ID != 1 || got[1].Task.ID != 2 || got[2].Task.ID != 3 {
		t.Errorf("got %+v, want all three Tasks in input order", got)
	}
	if len(got[0].Runes) != 0 {
		t.Errorf("matched runes = %v, want none", got[0].Runes)
	}
}

func TestSearchTasks_ReportsTheMatchedTitleRunes(t *testing.T) {
	got := core.SearchTasks("prd", tasksTitled("write the PRD"), nil)
	if want := []int{10, 11, 12}; len(got) != 1 || !reflect.DeepEqual(got[0].Runes, want) {
		t.Errorf("got %+v, want runes %v", got, want)
	}
}

func TestSearchTasks_NothingMatchesReturnsNoTasks(t *testing.T) {
	if got := core.SearchTasks("zzz", tasksTitled("write the PRD"), nil); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestSearchTasks_MatchesTagNames(t *testing.T) {
	tasks := []core.Task{
		{ID: 1, Title: "unrelated", Tags: []string{"docs"}},
		{ID: 2, Title: "write docs"},
		{ID: 3, Title: "nothing"},
	}
	got := core.SearchTasks("docs", tasks, nil)
	if len(got) != 2 || got[0].Task.ID != 2 || got[1].Task.ID != 1 {
		t.Fatalf("got %+v, want the title match (2) then the Tag match (1)", got)
	}
}

func TestSearchTasks_ReportsTheMatchedTagsAndTheirRunes(t *testing.T) {
	tasks := []core.Task{{ID: 1, Title: "x", Tags: []string{"backend", "Docs"}}}
	got := core.SearchTasks("doc", tasks, nil)
	if len(got) != 1 {
		t.Fatalf("got %+v, want one match", got)
	}
	want := map[int][]int{1: {0, 1, 2}}
	if !reflect.DeepEqual(got[0].TagRunes, want) {
		t.Errorf("TagRunes = %v, want %v (by index into Task.Tags)", got[0].TagRunes, want)
	}
	if len(got[0].Runes) != 0 {
		t.Errorf("title Runes = %v, want none", got[0].Runes)
	}
}

func TestSearchTasks_HashTokensFilterByTag(t *testing.T) {
	tasks := []core.Task{
		{ID: 1, Title: "docs"},
		{ID: 2, Title: "other", Tags: []string{"docs"}},
	}
	got := core.SearchTasks("#docs", tasks, nil)
	if len(got) != 1 || got[0].Task.ID != 2 {
		t.Errorf("got %+v, want only the tagged Task", got)
	}
}

func notesFor(id core.TaskID, texts ...string) map[core.TaskID][]core.Note {
	notes := make([]core.Note, len(texts))
	for i, text := range texts {
		notes[i] = core.Note{ID: core.NoteID(int(id)*100 + i + 1), TaskID: id, Text: text}
	}
	return map[core.TaskID][]core.Note{id: notes}
}

func TestSearchTasks_MatchesNotesAndReportsTheMatchingOne(t *testing.T) {
	tasks := tasksTitled("write the PRD", "fix the build")
	got := core.SearchTasks("parser", tasks, notesFor(1, "left off at the parser"))
	if len(got) != 1 || got[0].Task.ID != 1 {
		t.Fatalf("got %+v, want only Task 1", got)
	}
	n := got[0].Note
	if n == nil || n.Note.Text != "left off at the parser" || !reflect.DeepEqual(n.Runes, []int{16, 17, 18, 19, 20, 21}) {
		t.Errorf("Note = %+v, want the parser note with its runes", n)
	}
}

func TestSearchTasks_NoNoteWhenTheTitleOrATagWonTheMatch(t *testing.T) {
	tasks := []core.Task{{ID: 1, Title: "parser work", Tags: []string{"backend"}}}
	notes := notesFor(1, "the parser again")
	if got := core.SearchTasks("parser", tasks, notes); len(got) != 1 || got[0].Note != nil {
		t.Errorf("title win: got %+v, want no Note", got)
	}
	if got := core.SearchTasks("backend", tasks, notes); len(got) != 1 || got[0].Note != nil {
		t.Errorf("tag win: got %+v, want no Note", got)
	}
}

func TestSearchTasks_PicksTheBestNoteAndTheNewestOnATie(t *testing.T) {
	tasks := tasksTitled("task")
	got := core.SearchTasks("parser", tasks, notesFor(1, "parser", "the parser again", "parser"))
	if got[0].Note == nil || got[0].Note.Note.ID != 103 {
		t.Errorf("Note = %+v, want the newest of the two exact matches (103)", got[0].Note)
	}
	got = core.SearchTasks("parser", tasks, notesFor(1, "parser stuff and a lot more words", "parser"))
	if got[0].Note.Note.ID != 102 {
		t.Errorf("Note = %+v, want the better (shorter) match (102)", got[0].Note)
	}
}

func TestSearchTasks_NotesOfAnotherTaskAreNotUsed(t *testing.T) {
	tasks := tasksTitled("one", "two")
	got := core.SearchTasks("parser", tasks, notesFor(2, "parser"))
	if len(got) != 1 || got[0].Task.ID != 2 {
		t.Errorf("got %+v, want only Task 2", got)
	}
}

func TestSearchTasks_NilNotesBehaveAsBefore(t *testing.T) {
	if got := core.SearchTasks("prd", tasksTitled("write the PRD"), nil); len(got) != 1 || got[0].Note != nil {
		t.Errorf("got %+v", got)
	}
}
