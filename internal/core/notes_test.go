package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// endSession starts a 30m session and lets it complete, returning it.
func (r *rig) endedSession(t *testing.T, task core.TaskID) core.FocusSession {
	t.Helper()
	s, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(31 * time.Minute)
	return s
}

func (r *rig) notes(t *testing.T) []core.Note {
	t.Helper()
	got, err := r.store.Notes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAddHandoffNote_LinksToTheSessionAndResolvesTheHandoff(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	session := r.endedSession(t, task)

	note, err := r.tracker.AddHandoffNote(ctx, "  left off\n  at   the parser \t")
	if err != nil {
		t.Fatalf("AddHandoffNote: %v", err)
	}
	if note.ID == 0 || note.TaskID != task || note.SessionID != session.ID {
		t.Errorf("note links = %+v, want task %d and session %d", note, task, session.ID)
	}
	if note.Text != "left off at the parser" {
		t.Errorf("Text = %q, want whitespace collapsed to one line", note.Text)
	}
	if !note.CreatedAt.Equal(r.clock.Now()) {
		t.Errorf("CreatedAt = %v, want %v", note.CreatedAt, r.clock.Now())
	}
	if got := r.sessions(t)[0]; got.HandoffAt == nil || !got.HandoffAt.Equal(r.clock.Now()) {
		t.Errorf("session HandoffAt = %v, want %v", got.HandoffAt, r.clock.Now())
	}
	if n := len(r.notes(t)); n != 1 {
		t.Errorf("%d notes stored, want 1", n)
	}
}

func TestAddHandoffNote_WorksAfterAnEarlyStop(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(10 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.AddHandoffNote(ctx, "meeting came up"); err != nil {
		t.Errorf("AddHandoffNote after an early stop: %v", err)
	}
}

func TestAddHandoffNote_NothingPending(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)

	if _, err := r.tracker.AddHandoffNote(ctx, "x"); !errors.Is(err, core.ErrNoHandoffPending) {
		t.Errorf("with no sessions: error = %v, want ErrNoHandoffPending", err)
	}

	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(5 * time.Minute)
	if _, err := r.tracker.AddHandoffNote(ctx, "x"); !errors.Is(err, core.ErrNoHandoffPending) {
		t.Errorf("while the session runs: error = %v, want ErrNoHandoffPending", err)
	}

	r.clock.Advance(30 * time.Minute)
	if _, err := r.tracker.AddHandoffNote(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.AddHandoffNote(ctx, "second"); !errors.Is(err, core.ErrNoHandoffPending) {
		t.Errorf("when already resolved: error = %v, want ErrNoHandoffPending", err)
	}
	if n := len(r.notes(t)); n != 1 {
		t.Errorf("%d notes stored, want only the first", n)
	}
}

func TestAddHandoffNote_EmptyTextIsRejectedAndLeavesTheHandoffPending(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	r.endedSession(t, task)

	for _, text := range []string{"", "   ", "\n\t "} {
		if _, err := r.tracker.AddHandoffNote(ctx, text); !errors.Is(err, core.ErrEmptyNote) {
			t.Errorf("AddHandoffNote(%q) error = %v, want ErrEmptyNote", text, err)
		}
	}
	if _, err := r.tracker.AddHandoffNote(ctx, "now a real one"); err != nil {
		t.Errorf("hand-off should still be pending after rejected notes: %v", err)
	}
}

func TestSkipHandoff_ResolvesWithoutANote(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)

	if err := r.tracker.SkipHandoff(ctx); !errors.Is(err, core.ErrNoHandoffPending) {
		t.Errorf("with nothing pending: error = %v, want ErrNoHandoffPending", err)
	}

	r.endedSession(t, task)
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatalf("SkipHandoff: %v", err)
	}
	if got := r.sessions(t)[0]; got.HandoffAt == nil {
		t.Error("HandoffAt is nil after skipping, want it set")
	}
	if n := len(r.notes(t)); n != 0 {
		t.Errorf("%d notes stored after skipping, want none", n)
	}
	if _, err := r.tracker.AddHandoffNote(ctx, "too late"); !errors.Is(err, core.ErrNoHandoffPending) {
		t.Errorf("AddHandoffNote after skipping: error = %v, want ErrNoHandoffPending", err)
	}
	if err := r.tracker.SkipHandoff(ctx); !errors.Is(err, core.ErrNoHandoffPending) {
		t.Errorf("second SkipHandoff: error = %v, want ErrNoHandoffPending", err)
	}
}

func TestAddNote_AdHocOnAnyTaskWithNoSessionLink(t *testing.T) {
	r := newRig(t)
	for _, state := range []core.State{core.StateActive, core.StateDone, core.StateArchived} {
		task := r.addTask(t, state)
		note, err := r.tracker.AddNote(ctx, task, "remember the edge case")
		if err != nil {
			t.Fatalf("AddNote on a %v Task: %v", state, err)
		}
		if note.TaskID != task || note.SessionID != 0 || note.Text != "remember the edge case" {
			t.Errorf("note = %+v, want an ad-hoc note on task %d", note, task)
		}
	}
	if _, err := r.tracker.AddNote(ctx, 9999, "x"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("AddNote on an unknown Task: error = %v, want ErrNotFound", err)
	}
	task := r.addTask(t, core.StateActive)
	if _, err := r.tracker.AddNote(ctx, task, "  "); !errors.Is(err, core.ErrEmptyNote) {
		t.Errorf("AddNote with empty text: error = %v, want ErrEmptyNote", err)
	}
}

func TestAddUnfiledNote_HasNoTask(t *testing.T) {
	r := newRig(t)
	note, err := r.tracker.AddUnfiledNote(ctx, " check the\nretry logic ")
	if err != nil {
		t.Fatal(err)
	}
	if note.ID == 0 || note.TaskID != 0 || note.SessionID != 0 || note.Text != "check the retry logic" {
		t.Errorf("note = %+v, want an unfiled one-line note", note)
	}
	if _, err := r.tracker.AddUnfiledNote(ctx, ""); !errors.Is(err, core.ErrEmptyNote) {
		t.Errorf("empty unfiled note: error = %v, want ErrEmptyNote", err)
	}
}

func TestFileNote_AttachesAndKeepsTheOriginalTimestamp(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	written := r.clock.Now()
	note, err := r.tracker.AddUnfiledNote(ctx, "jotted before I knew the task")
	if err != nil {
		t.Fatal(err)
	}

	r.clock.Advance(3 * time.Hour)
	filed, err := r.tracker.FileNote(ctx, note.ID, task)
	if err != nil {
		t.Fatalf("FileNote: %v", err)
	}
	if filed.TaskID != task || !filed.CreatedAt.Equal(written) || filed.Text != note.Text {
		t.Errorf("filed note = %+v, want task %d, CreatedAt %v, same text", filed, task, written)
	}

	if _, err := r.tracker.FileNote(ctx, note.ID, task); !errors.Is(err, core.ErrNoteAlreadyFiled) {
		t.Errorf("filing twice: error = %v, want ErrNoteAlreadyFiled", err)
	}
	other, _ := r.tracker.AddUnfiledNote(ctx, "another")
	if _, err := r.tracker.FileNote(ctx, other.ID, 9999); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("filing onto an unknown Task: error = %v, want ErrNotFound", err)
	}
	if _, err := r.tracker.FileNote(ctx, 9999, task); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("filing an unknown note: error = %v, want ErrNotFound", err)
	}
	if got, _ := r.tracker.UnfiledNotes(ctx); len(got) != 1 || got[0].ID != other.ID {
		t.Errorf("UnfiledNotes = %+v, want only the note that failed to file", got)
	}
}

func TestTaskNotes_AreOrderedByWhenTheyWereWritten(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, core.StateActive)
	texts := func(notes []core.Note) []string {
		var out []string
		for _, n := range notes {
			out = append(out, n.Text)
		}
		return out
	}

	mustAdd := func(text string) core.Note {
		n, err := r.tracker.AddNote(ctx, task, text)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	mustAdd("first")
	r.clock.Advance(time.Hour)
	unfiled, _ := r.tracker.AddUnfiledNote(ctx, "second, written unfiled")
	r.clock.Advance(time.Hour)
	mustAdd("third")

	if _, err := r.tracker.FileNote(ctx, unfiled.ID, task); err != nil { // filed last, but written second
		t.Fatal(err)
	}
	got, err := r.tracker.TaskNotes(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "second, written unfiled", "third"}
	if !slices.Equal(texts(got), want) {
		t.Errorf("TaskNotes = %v, want %v", texts(got), want)
	}
	if left, _ := r.tracker.UnfiledNotes(ctx); len(left) != 0 {
		t.Errorf("UnfiledNotes = %+v after filing, want none", left)
	}
	if _, err := r.tracker.TaskNotes(ctx, 9999); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("TaskNotes of an unknown Task: error = %v, want ErrNotFound", err)
	}
}

func TestNotesByTask_GroupsFiledNotesOldestFirstAndSkipsUnfiled(t *testing.T) {
	r := newRig(t)
	a := r.addTask(t, core.StateActive)
	b := r.addTask(t, core.StateActive)
	first, _ := r.tracker.AddNote(ctx, a, "first on a")
	r.clock.Advance(time.Minute)
	other, _ := r.tracker.AddNote(ctx, b, "only on b")
	r.clock.Advance(time.Minute)
	second, _ := r.tracker.AddNote(ctx, a, "second on a")
	if _, err := r.tracker.AddUnfiledNote(ctx, "not filed"); err != nil {
		t.Fatal(err)
	}

	got, err := r.tracker.NotesByTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0]) != 0 {
		t.Fatalf("got %+v, want notes for exactly two Tasks and none unfiled", got)
	}
	if want := []core.NoteID{first.ID, second.ID}; !slices.Equal(ids(got[a]), want) {
		t.Errorf("a's notes = %v, want %v", ids(got[a]), want)
	}
	if want := []core.NoteID{other.ID}; !slices.Equal(ids(got[b]), want) {
		t.Errorf("b's notes = %v, want %v", ids(got[b]), want)
	}
}

func ids(notes []core.Note) []core.NoteID {
	out := []core.NoteID{}
	for _, n := range notes {
		out = append(out, n.ID)
	}
	return out
}
