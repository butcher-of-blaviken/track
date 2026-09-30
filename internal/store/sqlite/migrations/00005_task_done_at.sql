-- +goose Up
ALTER TABLE tasks ADD COLUMN done_at INTEGER; -- Unix ns, UTC; NULL unless the Task is Done and when is known

-- +goose Down
ALTER TABLE tasks DROP COLUMN done_at;
