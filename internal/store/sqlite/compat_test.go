package sqlite_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/store/sqlite"
)

// Track promises that a new version opens and upgrades any database an older
// version wrote (docs/adr/0003-backward-compatibility.md). testdata holds one
// database per released schema version, written by that version's code. Each
// must keep opening and reading back the same facts, whatever migrations are
// added later.
//
// Never regenerate or edit an existing fixture: that would make this test
// agree with whatever the new code does. To add one for a new schema, run
//
//	go test ./internal/store/sqlite -run TestWriteFixture -write-fixture N
//
// on the commit that adds migration N, then extend wantFixture for it.
var writeFixture = flag.Int("write-fixture", 0, "write testdata/schema_vN.db with the current code (see compat_test.go)")

var fixtureStart = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)

// TestWriteFixture is a generator, not a test: it does nothing without -write-fixture.
func TestWriteFixture(t *testing.T) {
	if *writeFixture == 0 {
		t.Skip("run with -write-fixture N to write a fixture")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "track.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	fake := clock.NewFake(fixtureStart)
	tr, err := core.NewTracker(store, fake, core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4})
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	prd, err := tr.AddTask(ctx, "write the PRD ##docs ##Q1")
	must(err)
	fake.Advance(time.Minute)
	old, err := tr.AddTask(ctx, "old thing")
	must(err)
	fake.Advance(time.Minute)
	dropped, err := tr.AddTask(ctx, "dropped")
	must(err)

	// A completed session with a hand-off note, then one stopped early with its
	// hand-off skipped.
	_, err = tr.StartSession(ctx, prd.ID, 30*time.Minute, core.StartOptions{})
	must(err)
	fake.Advance(30 * time.Minute)
	_, err = tr.AddHandoffNote(ctx, "left off at section 2")
	must(err)
	fake.Advance(11 * time.Minute) // the Break is over
	_, err = tr.StartSession(ctx, prd.ID, 30*time.Minute, core.StartOptions{})
	must(err)
	fake.Advance(12 * time.Minute)
	_, err = tr.StopSession(ctx)
	must(err)
	must(tr.SkipHandoff(ctx))

	_, err = tr.AddNote(ctx, prd.ID, "an ad hoc note")
	must(err)
	_, err = tr.AddUnfiledNote(ctx, "call the bank")
	must(err)
	_, err = tr.MarkDone(ctx, old.ID)
	must(err)
	_, err = tr.ArchiveTask(ctx, dropped.ID)
	must(err)
	must(store.Close())

	out := filepath.Join("testdata", "schema_v"+itoa(*writeFixture)+".db")
	data, err := os.ReadFile(path)
	must(err)
	must(os.WriteFile(out, data, 0o644))
}

func itoa(n int) string { return strconv.Itoa(n) }

// checkFixture is what every fixture holds, whatever its schema version.
func checkFixture(t *testing.T, store core.Store) {
	t.Helper()
	ctx := context.Background()
	fake := clock.NewFake(fixtureStart.Add(24 * time.Hour))
	tr, err := core.NewTracker(store, fake, core.BreakPolicy{Short: 10 * time.Minute, Long: 20 * time.Minute, LongEvery: 4})
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := tr.Tasks(ctx)
	if err != nil || len(tasks) != 3 {
		t.Fatalf("tasks = %+v, %v; want 3", tasks, err)
	}
	byTitle := map[string]core.Task{}
	for _, task := range tasks {
		byTitle[task.Title] = task
	}
	prd := byTitle["write the PRD"]
	if prd.State != core.StateActive || len(prd.Tags) != 2 || prd.Tags[0] != "docs" || prd.Tags[1] != "Q1" || !prd.CreatedAt.Equal(fixtureStart) {
		t.Errorf("PRD task = %+v", prd)
	}
	if byTitle["old thing"].State != core.StateDone || byTitle["dropped"].State != core.StateArchived {
		t.Errorf("states = %v, %v; want Done and Archived", byTitle["old thing"].State, byTitle["dropped"].State)
	}

	detail, err := tr.TaskDetail(ctx, prd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Sessions != 2 || detail.Focused != 42*time.Minute {
		t.Errorf("sessions %d, focused %v; want 2 and 42m", detail.Sessions, detail.Focused)
	}
	var texts []string
	for _, n := range detail.Notes {
		texts = append(texts, n.Text)
	}
	if len(texts) != 2 || texts[0] != "left off at section 2" || texts[1] != "an ad hoc note" {
		t.Errorf("notes = %q", texts)
	}
	if detail.Notes[0].SessionID == 0 || detail.Notes[1].SessionID != 0 {
		t.Errorf("only the hand-off note belongs to a session: %+v", detail.Notes)
	}

	sessions, err := store.Sessions(ctx)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	first, second := sessions[0], sessions[1]
	if first.PlannedDuration != 30*time.Minute || first.StoppedAt != nil || first.BreakDuration != 10*time.Minute || first.HandoffAt == nil {
		t.Errorf("completed session = %+v", first)
	}
	if second.StoppedAt == nil || second.StoppedAt.Sub(second.StartedAt) != 12*time.Minute || second.HandoffAt == nil {
		t.Errorf("stopped session = %+v", second)
	}

	unfiled, err := tr.UnfiledNotes(ctx)
	if err != nil || len(unfiled) != 1 || unfiled[0].Text != "call the bank" {
		t.Errorf("unfiled = %+v, %v", unfiled, err)
	}
}

// checkDoneAt is what schema 5 added: the time a Task was marked done. The "old
// thing" task was marked done 55 minutes into the fixture's clock; a database
// from before then has no date for it, and the Task is still Done.
func checkDoneAt(t *testing.T, store core.Store, version int) {
	t.Helper()
	tasks, err := store.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		switch {
		case task.Title == "old thing" && version >= 5:
			if task.DoneAt == nil || !task.DoneAt.Equal(fixtureStart.Add(55*time.Minute)) {
				t.Errorf("old thing DoneAt = %v, want %v", task.DoneAt, fixtureStart.Add(55*time.Minute))
			}
		case task.DoneAt != nil:
			t.Errorf("%q has DoneAt %v, want none (schema v%d)", task.Title, task.DoneAt, version)
		}
	}
}

func TestOldDatabases_StillOpenUpgradeAndReadBack(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "schema_v*.db"))
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "track.db")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			store := mustOpen(t, path)
			t.Cleanup(func() { _ = store.Close() })
			checkFixture(t, store)
			// Facts a later schema added: earlier fixtures have no such column
			// and must read back without them.
			version, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(fixture), "schema_v"), ".db"))
			checkDoneAt(t, store, version)
		})
	}
}
