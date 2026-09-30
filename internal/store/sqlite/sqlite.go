// Package sqlite is the SQLite-backed core.Store (see ADR-0001). It uses a
// pure-Go driver, so the binary needs no CGO.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	msqlite "modernc.org/sqlite" // registers the "sqlite" driver
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/butcher-of-blaviken/track/internal/core"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrSchemaTooNew is returned by Open when the database was migrated by a newer
// version of Track than this binary understands.
var ErrSchemaTooNew = errors.New("database was created by a newer version of track; upgrade track")

// Store is a core.Store backed by a SQLite file.
type Store struct {
	db *sql.DB
}

var _ core.Store = (*Store)(nil)

// Open opens (creating if needed) the database at path and migrates it to the
// latest schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	q := url.Values{}
	// busy_timeout comes first so that later pragmas, notably the switch to WAL,
	// wait for a competing process instead of failing immediately.
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)", "foreign_keys(1)"} {
		q.Add("_pragma", p)
	}
	// Take the write lock when a transaction starts, so two processes racing
	// to write queue on busy_timeout instead of failing on lock upgrade.
	q.Set("_txlock", "immediate")
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}).String()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection per process avoids in-process lock contention; separate
	// processes are coordinated by WAL and busy_timeout.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrateWithRetry(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrateWithRetry migrates the schema, retrying while another process holds a
// lock. SQLite reports SQLITE_BUSY without waiting for busy_timeout in some
// cases, e.g. when two processes create a fresh database and switch it to WAL
// at the same moment, so the timeout alone does not cover first use.
func (s *Store) migrateWithRetry(ctx context.Context) error {
	const window = 5 * time.Second
	deadline := time.Now().Add(window)
	for delay := 10 * time.Millisecond; ; delay = min(delay*2, 200*time.Millisecond) {
		err := s.migrate(ctx)
		if err == nil || !(isBusy(err) || isLostRace(err)) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// isLostRace reports that a migration failed because another process ran it
// first: goose read the applied versions before that process committed, so it
// tried to create what now exists. Trying again sees the new version and has
// nothing left to do.
func isLostRace(err error) bool {
	var e *msqlite.Error
	return errors.As(err, &e) && e.Code()&0xff == sqlite3.SQLITE_ERROR && strings.Contains(e.Error(), "already exists")
}

func isBusy(err error) bool {
	var e *msqlite.Error
	return errors.As(err, &e) && e.Code()&0xff == sqlite3.SQLITE_BUSY
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	sub, err := migrationsSub()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, s.db, sub)
	if err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		// goose reads the applied versions before taking the write lock, so when
		// two processes open a database that needs migrating, the second one
		// tries to re-apply what the first just committed and fails. That is
		// fine if the schema is now up to date.
		if current, target, verr := p.GetVersions(ctx); verr != nil || current < target {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	// goose ignores applied migrations it has no file for, so a database from
	// a newer build would open silently; refuse it instead.
	current, target, err := p.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > target {
		return ErrSchemaTooNew
	}
	return nil
}

func toTime(nanos int64) time.Time { return time.Unix(0, nanos).UTC() }

const sessionColumns = `id, task_id, started_at, planned_ns, stopped_at, break_ns, long_break, skipped_break_ns, handoff_at`

type scanner interface{ Scan(dest ...any) error }

func scanSession(r scanner) (core.FocusSession, error) {
	var (
		sess    core.FocusSession
		started int64
		planned int64
		stopped sql.NullInt64
		brk     int64
		skipped int64
		handoff sql.NullInt64
	)
	if err := r.Scan(&sess.ID, &sess.TaskID, &started, &planned, &stopped, &brk, &sess.LongBreak, &skipped, &handoff); err != nil {
		return core.FocusSession{}, err
	}
	sess.StartedAt = toTime(started)
	sess.PlannedDuration = time.Duration(planned)
	sess.BreakDuration = time.Duration(brk)
	sess.SkippedBreak = time.Duration(skipped)
	if stopped.Valid {
		t := toTime(stopped.Int64)
		sess.StoppedAt = &t
	}
	if handoff.Valid {
		t := toTime(handoff.Int64)
		sess.HandoffAt = &t
	}
	return sess, nil
}

// Sessions implements core.Store.
func (s *Store) Sessions(ctx context.Context) ([]core.FocusSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM sessions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []core.FocusSession{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func scanNote(r scanner) (core.Note, error) {
	var (
		n       core.Note
		task    sql.NullInt64
		session sql.NullInt64
		created int64
	)
	if err := r.Scan(&n.ID, &task, &session, &n.Text, &created); err != nil {
		return core.Note{}, err
	}
	n.TaskID, n.SessionID = core.TaskID(task.Int64), core.SessionID(session.Int64)
	n.CreatedAt = toTime(created)
	return n, nil
}

const noteColumns = `id, task_id, session_id, text, created_at`

// LatestSession implements core.Store.
func (s *Store) LatestSession(ctx context.Context) (core.FocusSession, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions ORDER BY id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return core.FocusSession{}, core.ErrNotFound
	}
	return sess, err
}

// UnfiledNoteCount implements core.Store.
func (s *Store) UnfiledNoteCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE task_id IS NULL`).Scan(&n)
	return n, err
}

// Notes implements core.Store.
func (s *Store) Notes(ctx context.Context) ([]core.Note, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+noteColumns+` FROM notes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []core.Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Tasks implements core.Store.
func (s *Store) Tasks(ctx context.Context) ([]core.Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, state, created_at FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	tasks := []core.Task{}
	byID := map[core.TaskID]int{}
	for rows.Next() {
		var t core.Task
		var nanos int64
		if err := rows.Scan(&t.ID, &t.Title, &t.State, &nanos); err != nil {
			_ = rows.Close()
			return nil, err
		}
		t.CreatedAt = toTime(nanos)
		t.Tags = []string{}
		byID[t.ID] = len(tasks)
		tasks = append(tasks, t)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagRows, err := s.db.QueryContext(ctx, `
		SELECT task_tags.task_id, tags.name FROM task_tags JOIN tags ON tags.id = task_tags.tag_id
		ORDER BY task_tags.task_id, task_tags.position`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tagRows.Close() }()
	for tagRows.Next() {
		var id core.TaskID
		var name string
		if err := tagRows.Scan(&id, &name); err != nil {
			return nil, err
		}
		i := byID[id]
		tasks[i].Tags = append(tasks[i].Tags, name)
	}
	return tasks, tagRows.Err()
}

// Task implements core.Store.
func (s *Store) Task(ctx context.Context, id core.TaskID) (core.Task, error) {
	return getTask(ctx, s.db, id)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func getTask(ctx context.Context, q queryer, id core.TaskID) (core.Task, error) {
	var t core.Task
	var nanos int64
	err := q.QueryRowContext(ctx, `SELECT id, title, state, created_at FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.Title, &t.State, &nanos)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Task{}, core.ErrNotFound
	}
	if err != nil {
		return core.Task{}, err
	}
	t.CreatedAt = toTime(nanos)
	if t.Tags, err = getTags(ctx, q, id); err != nil {
		return core.Task{}, err
	}
	return t, nil
}

// getTags returns a Task's Tag names in the order they were saved.
func getTags(ctx context.Context, q queryer, id core.TaskID) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT tags.name FROM task_tags JOIN tags ON tags.id = task_tags.tag_id
		WHERE task_tags.task_id = ? ORDER BY task_tags.position`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tags := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// Update implements core.Store.
func (s *Store) Update(ctx context.Context, fn func(core.Tx) error) (err error) {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = sqlTx.Rollback()
		}
	}()
	if err = fn(&tx{ctx: ctx, tx: sqlTx}); err != nil {
		return err
	}
	return sqlTx.Commit()
}

type tx struct {
	ctx context.Context
	tx  *sql.Tx
}

func (x *tx) Task(id core.TaskID) (core.Task, error) { return getTask(x.ctx, x.tx, id) }

func (x *tx) CreateTask(t core.Task) (core.TaskID, error) {
	res, err := x.tx.ExecContext(x.ctx,
		`INSERT INTO tasks (title, state, created_at) VALUES (?, ?, ?)`,
		t.Title, int(t.State), t.CreatedAt.UnixNano())
	if err != nil {
		return 0, err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	id := core.TaskID(n)
	return id, x.setTags(id, t.Tags)
}

func (x *tx) SaveTask(t core.Task) error {
	res, err := x.tx.ExecContext(x.ctx, `UPDATE tasks SET title = ?, state = ? WHERE id = ?`,
		t.Title, int(t.State), t.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return core.ErrNotFound
	}
	return x.setTags(t.ID, t.Tags)
}

// setTags replaces a Task's Tags. A Tag that already exists, in any casing,
// keeps its first-use name. Repeats within the list are ignored.
func (x *tx) setTags(id core.TaskID, names []string) error {
	if _, err := x.tx.ExecContext(x.ctx, `DELETE FROM task_tags WHERE task_id = ?`, id); err != nil {
		return err
	}
	for pos, name := range names {
		key := strings.ToLower(name)
		if _, err := x.tx.ExecContext(x.ctx,
			`INSERT INTO tags (key, name) VALUES (?, ?) ON CONFLICT (key) DO NOTHING`, key, name); err != nil {
			return err
		}
		if _, err := x.tx.ExecContext(x.ctx,
			`INSERT OR IGNORE INTO task_tags (task_id, tag_id, position)
			 SELECT ?, id, ? FROM tags WHERE key = ?`, id, pos, key); err != nil {
			return err
		}
	}
	return nil
}

func (x *tx) CreateSession(sess core.FocusSession) (core.SessionID, error) {
	var exists int
	err := x.tx.QueryRowContext(x.ctx, `SELECT 1 FROM tasks WHERE id = ?`, sess.TaskID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	res, err := x.tx.ExecContext(x.ctx,
		`INSERT INTO sessions (task_id, started_at, planned_ns, stopped_at, break_ns, long_break, skipped_break_ns, handoff_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.TaskID, sess.StartedAt.UnixNano(), int64(sess.PlannedDuration), optNanos(sess.StoppedAt),
		int64(sess.BreakDuration), sess.LongBreak, int64(sess.SkippedBreak), optNanos(sess.HandoffAt))
	if err != nil {
		return 0, err
	}
	n, err := res.LastInsertId()
	return core.SessionID(n), err
}

func (x *tx) SaveSession(sess core.FocusSession) error {
	res, err := x.tx.ExecContext(x.ctx, `UPDATE sessions SET stopped_at = ?, handoff_at = ? WHERE id = ?`,
		optNanos(sess.StoppedAt), optNanos(sess.HandoffAt), sess.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (x *tx) LatestSession() (core.FocusSession, error) {
	sess, err := scanSession(x.tx.QueryRowContext(x.ctx,
		`SELECT `+sessionColumns+` FROM sessions ORDER BY id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return core.FocusSession{}, core.ErrNotFound
	}
	return sess, err
}

func optNanos(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixNano(), Valid: true}
}

func (x *tx) CompletedSessionCount(now time.Time) (int, error) {
	var n int
	err := x.tx.QueryRowContext(x.ctx,
		`SELECT COUNT(*) FROM sessions WHERE stopped_at IS NULL AND started_at + planned_ns <= ?`,
		now.UnixNano()).Scan(&n)
	return n, err
}

// nullID stores 0 as NULL, for the optional references on a Note.
func nullID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func (x *tx) exists(table string, id int64) (bool, error) {
	var one int
	err := x.tx.QueryRowContext(x.ctx, `SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (x *tx) CreateNote(n core.Note) (core.NoteID, error) {
	for table, id := range map[string]int64{"tasks": int64(n.TaskID), "sessions": int64(n.SessionID)} {
		if id == 0 {
			continue
		}
		ok, err := x.exists(table, id)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, core.ErrNotFound
		}
	}
	res, err := x.tx.ExecContext(x.ctx,
		`INSERT INTO notes (task_id, session_id, text, created_at) VALUES (?, ?, ?, ?)`,
		nullID(int64(n.TaskID)), nullID(int64(n.SessionID)), n.Text, n.CreatedAt.UnixNano())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return core.NoteID(id), err
}

func (x *tx) Note(id core.NoteID) (core.Note, error) {
	n, err := scanNote(x.tx.QueryRowContext(x.ctx, `SELECT `+noteColumns+` FROM notes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return core.Note{}, core.ErrNotFound
	}
	return n, err
}

func (x *tx) SaveNote(n core.Note) error {
	if ok, err := x.exists("tasks", int64(n.TaskID)); err != nil || !ok {
		if err != nil {
			return err
		}
		return core.ErrNotFound
	}
	res, err := x.tx.ExecContext(x.ctx, `UPDATE notes SET task_id = ? WHERE id = ?`, n.TaskID, n.ID)
	if err != nil {
		return err
	}
	if rows, err := res.RowsAffected(); err != nil {
		return err
	} else if rows == 0 {
		return core.ErrNotFound
	}
	return nil
}
