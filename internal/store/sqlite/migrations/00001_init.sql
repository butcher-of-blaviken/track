-- +goose Up
CREATE TABLE tasks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT    NOT NULL,
    state      INTEGER NOT NULL,
    created_at INTEGER NOT NULL -- Unix nanoseconds, UTC
);

-- A Tag is unique case-insensitively. key is the lowercased name (folded in Go,
-- so Unicode folds like the in-memory store); name keeps first-use casing.
CREATE TABLE tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    key  TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

CREATE TABLE task_tags (
    task_id  INTEGER NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    tag_id   INTEGER NOT NULL REFERENCES tags (id),
    position INTEGER NOT NULL, -- keeps the order Tags were given in
    PRIMARY KEY (task_id, tag_id)
);

-- +goose Down
DROP TABLE task_tags;
DROP TABLE tags;
DROP TABLE tasks;
