-- +goose Up
ALTER TABLE sessions ADD COLUMN handoff_at INTEGER; -- Unix ns; NULL while the hand-off is pending

CREATE TABLE notes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER REFERENCES tasks (id),    -- NULL for an Unfiled note
    session_id INTEGER REFERENCES sessions (id), -- set only for a Hand-off note
    text       TEXT    NOT NULL,
    created_at INTEGER NOT NULL                  -- Unix nanoseconds, UTC; kept when filed
);
CREATE INDEX notes_task_id ON notes (task_id);

-- +goose Down
DROP INDEX notes_task_id;
DROP TABLE notes;
ALTER TABLE sessions DROP COLUMN handoff_at;
