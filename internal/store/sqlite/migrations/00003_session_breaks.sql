-- +goose Up
ALTER TABLE sessions ADD COLUMN break_ns INTEGER NOT NULL DEFAULT 0;         -- Break earned on completion
ALTER TABLE sessions ADD COLUMN long_break INTEGER NOT NULL DEFAULT 0;       -- 1 if that Break is long
ALTER TABLE sessions ADD COLUMN skipped_break_ns INTEGER NOT NULL DEFAULT 0; -- Break time skipped by an override

-- +goose Down
ALTER TABLE sessions DROP COLUMN skipped_break_ns;
ALTER TABLE sessions DROP COLUMN long_break;
ALTER TABLE sessions DROP COLUMN break_ns;
