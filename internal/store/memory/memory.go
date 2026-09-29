// Package memory is an in-memory core.Store for tests and headless runs.
package memory

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// state is everything the store holds. Update works on a copy and swaps it in
// only if the transaction succeeds, which makes writes atomic.
type state struct {
	tasks  map[core.TaskID]core.Task
	nextID core.TaskID
	// tags maps a lowercased Tag name to its first-use casing.
	tags map[string]string
	// sessions are kept in ID order, which is also creation order.
	sessions      []core.FocusSession
	nextSessionID core.SessionID
	// notes are kept in ID order.
	notes      []core.Note
	nextNoteID core.NoteID
}

func (st state) clone() state {
	return state{
		tasks: maps.Clone(st.tasks), nextID: st.nextID, tags: maps.Clone(st.tags),
		sessions: slices.Clone(st.sessions), nextSessionID: st.nextSessionID,
		notes: slices.Clone(st.notes), nextNoteID: st.nextNoteID,
	}
}

// Store is an in-memory core.Store.
type Store struct {
	mu sync.Mutex
	st state
}

var _ core.Store = (*Store)(nil)

// New returns an empty Store.
func New() *Store {
	return &Store{st: state{tasks: map[core.TaskID]core.Task{}, nextID: 1, tags: map[string]string{}, nextSessionID: 1, nextNoteID: 1}}
}

func cloneTask(t core.Task) core.Task {
	t.Tags = slices.Clone(t.Tags)
	return t
}

func cloneSession(s core.FocusSession) core.FocusSession {
	if s.StoppedAt != nil {
		stopped := *s.StoppedAt
		s.StoppedAt = &stopped
	}
	if s.HandoffAt != nil {
		handoff := *s.HandoffAt
		s.HandoffAt = &handoff
	}
	return s
}

// Notes implements core.Store.
func (s *Store) Notes(context.Context) ([]core.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.st.notes), nil
}

// Sessions implements core.Store.
func (s *Store) Sessions(context.Context) ([]core.FocusSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]core.FocusSession, len(s.st.sessions))
	for i, session := range s.st.sessions {
		out[i] = cloneSession(session)
	}
	return out, nil
}

// Tasks implements core.Store.
func (s *Store) Tasks(context.Context) ([]core.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := make([]core.Task, 0, len(s.st.tasks))
	for _, t := range s.st.tasks {
		tasks = append(tasks, cloneTask(t))
	}
	slices.SortFunc(tasks, func(a, b core.Task) int { return cmp.Compare(a.ID, b.ID) })
	return tasks, nil
}

// Task implements core.Store.
func (s *Store) Task(_ context.Context, id core.TaskID) (core.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.st.tasks[id]
	if !ok {
		return core.Task{}, core.ErrNotFound
	}
	return cloneTask(t), nil
}

// Update implements core.Store.
func (s *Store) Update(_ context.Context, fn func(core.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	work := s.st.clone()
	if err := fn(&tx{st: &work}); err != nil {
		return err
	}
	s.st = work
	return nil
}

type tx struct{ st *state }

// canonical rewrites t's Tags to their first-use casing, registering new Tags.
func (x *tx) canonical(t core.Task) core.Task {
	tags := make([]string, len(t.Tags))
	for i, name := range t.Tags {
		key := strings.ToLower(name)
		if _, ok := x.st.tags[key]; !ok {
			x.st.tags[key] = name
		}
		tags[i] = x.st.tags[key]
	}
	t.Tags = tags
	return t
}

func (x *tx) Task(id core.TaskID) (core.Task, error) {
	t, ok := x.st.tasks[id]
	if !ok {
		return core.Task{}, core.ErrNotFound
	}
	return cloneTask(t), nil
}

func (x *tx) CreateTask(t core.Task) (core.TaskID, error) {
	t.ID = x.st.nextID
	x.st.nextID++
	x.st.tasks[t.ID] = x.canonical(t)
	return t.ID, nil
}

func (x *tx) SaveTask(t core.Task) error {
	old, ok := x.st.tasks[t.ID]
	if !ok {
		return core.ErrNotFound
	}
	t.CreatedAt = old.CreatedAt
	x.st.tasks[t.ID] = x.canonical(t)
	return nil
}

func (x *tx) CreateSession(sess core.FocusSession) (core.SessionID, error) {
	if _, ok := x.st.tasks[sess.TaskID]; !ok {
		return 0, core.ErrNotFound
	}
	sess.ID = x.st.nextSessionID
	x.st.nextSessionID++
	x.st.sessions = append(x.st.sessions, cloneSession(sess))
	return sess.ID, nil
}

func (x *tx) SaveSession(sess core.FocusSession) error {
	i := slices.IndexFunc(x.st.sessions, func(c core.FocusSession) bool { return c.ID == sess.ID })
	if i < 0 {
		return core.ErrNotFound
	}
	updated := x.st.sessions[i] // cloned slice header, so replacing the element is transaction-local
	saved := cloneSession(sess)
	updated.StoppedAt, updated.HandoffAt = saved.StoppedAt, saved.HandoffAt
	x.st.sessions[i] = updated
	return nil
}

func (x *tx) LatestSession() (core.FocusSession, error) {
	if len(x.st.sessions) == 0 {
		return core.FocusSession{}, core.ErrNotFound
	}
	return cloneSession(x.st.sessions[len(x.st.sessions)-1]), nil
}

func (x *tx) CompletedSessionCount(now time.Time) (int, error) {
	n := 0
	for _, sess := range x.st.sessions {
		if sess.Outcome(now) == core.OutcomeCompleted {
			n++
		}
	}
	return n, nil
}

func (x *tx) CreateNote(n core.Note) (core.NoteID, error) {
	if n.TaskID != 0 {
		if _, ok := x.st.tasks[n.TaskID]; !ok {
			return 0, core.ErrNotFound
		}
	}
	if n.SessionID != 0 && !slices.ContainsFunc(x.st.sessions, func(c core.FocusSession) bool { return c.ID == n.SessionID }) {
		return 0, core.ErrNotFound
	}
	n.ID = x.st.nextNoteID
	x.st.nextNoteID++
	x.st.notes = append(x.st.notes, n)
	return n.ID, nil
}

func (x *tx) Note(id core.NoteID) (core.Note, error) {
	i := slices.IndexFunc(x.st.notes, func(n core.Note) bool { return n.ID == id })
	if i < 0 {
		return core.Note{}, core.ErrNotFound
	}
	return x.st.notes[i], nil
}

func (x *tx) SaveNote(n core.Note) error {
	i := slices.IndexFunc(x.st.notes, func(c core.Note) bool { return c.ID == n.ID })
	if i < 0 {
		return core.ErrNotFound
	}
	if _, ok := x.st.tasks[n.TaskID]; !ok {
		return core.ErrNotFound
	}
	x.st.notes[i].TaskID = n.TaskID
	return nil
}
