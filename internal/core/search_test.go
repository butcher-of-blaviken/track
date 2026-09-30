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
	got := core.SearchTasks("prd", tasks)
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
	got := core.SearchTasks("  ", tasks)
	if len(got) != 3 || got[0].Task.ID != 1 || got[1].Task.ID != 2 || got[2].Task.ID != 3 {
		t.Errorf("got %+v, want all three Tasks in input order", got)
	}
	if len(got[0].Runes) != 0 {
		t.Errorf("matched runes = %v, want none", got[0].Runes)
	}
}

func TestSearchTasks_ReportsTheMatchedTitleRunes(t *testing.T) {
	got := core.SearchTasks("prd", tasksTitled("write the PRD"))
	if want := []int{10, 11, 12}; len(got) != 1 || !reflect.DeepEqual(got[0].Runes, want) {
		t.Errorf("got %+v, want runes %v", got, want)
	}
}

func TestSearchTasks_NothingMatchesReturnsNoTasks(t *testing.T) {
	if got := core.SearchTasks("zzz", tasksTitled("write the PRD")); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestSearchTasks_MatchesTagNames(t *testing.T) {
	tasks := []core.Task{
		{ID: 1, Title: "unrelated", Tags: []string{"docs"}},
		{ID: 2, Title: "write docs"},
		{ID: 3, Title: "nothing"},
	}
	got := core.SearchTasks("docs", tasks)
	if len(got) != 2 || got[0].Task.ID != 2 || got[1].Task.ID != 1 {
		t.Fatalf("got %+v, want the title match (2) then the Tag match (1)", got)
	}
}

func TestSearchTasks_ReportsTheMatchedTagsAndTheirRunes(t *testing.T) {
	tasks := []core.Task{{ID: 1, Title: "x", Tags: []string{"backend", "Docs"}}}
	got := core.SearchTasks("doc", tasks)
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
	got := core.SearchTasks("#docs", tasks)
	if len(got) != 1 || got[0].Task.ID != 2 {
		t.Errorf("got %+v, want only the tagged Task", got)
	}
}
