// Package memory is an in-memory core.Store for tests and headless runs.
package memory

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/butcher-of-blaviken/track/internal/core"
)

// state is everything the store holds. Update works on a copy and swaps it in
// only if the transaction succeeds, which makes writes atomic.
type state struct {
	tasks  map[core.TaskID]core.Task
	nextID core.TaskID
	// tags maps a lowercased Tag name to its first-use casing.
	tags map[string]string
}

func (st state) clone() state {
	return state{tasks: maps.Clone(st.tasks), nextID: st.nextID, tags: maps.Clone(st.tags)}
}

// Store is an in-memory core.Store.
type Store struct {
	mu sync.Mutex
	st state
}

var _ core.Store = (*Store)(nil)

// New returns an empty Store.
func New() *Store {
	return &Store{st: state{tasks: map[core.TaskID]core.Task{}, nextID: 1, tags: map[string]string{}}}
}

func cloneTask(t core.Task) core.Task {
	t.Tags = slices.Clone(t.Tags)
	return t
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
