-- +goose Up
-- Nullable on purpose: the previous binary inserts habits without this column and must keep
-- working on a migrated database after a rollback. NULL means the creation time is unknown.
ALTER TABLE habits ADD COLUMN created_at INTEGER;

-- +goose Down
-- Dropping created_at would discard recorded creation times. Restore a protected backup
-- together with its matching binary instead of attempting an in-place downgrade.
SELECT * FROM downgrade_requires_backup_and_matching_binary;
