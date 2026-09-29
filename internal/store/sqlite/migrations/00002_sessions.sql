-- +goose Up
CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER NOT NULL REFERENCES tasks (id),
    started_at INTEGER NOT NULL, -- Unix nanoseconds, UTC
    planned_ns INTEGER NOT NULL, -- planned duration in nanoseconds; never rewritten
    stopped_at INTEGER           -- Unix nanoseconds; NULL unless stopped early
);
CREATE INDEX sessions_task_id ON sessions (task_id);

-- +goose Down
DROP INDEX sessions_task_id;
DROP TABLE sessions;
