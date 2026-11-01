-- name: ListActiveHabits :many
SELECT id, name FROM habits WHERE deleted_at IS NULL ORDER BY position, id;

-- name: ListActiveDays :many
SELECT d.habit_id, d.day FROM habit_days AS d JOIN habits AS h ON h.id = d.habit_id
WHERE h.deleted_at IS NULL ORDER BY d.habit_id, d.day;

-- name: InsertHabit :execrows
INSERT INTO habits (id, name, position)
VALUES (sqlc.arg(id), sqlc.arg(name), (SELECT COALESCE(MAX(position), -1) + 1 FROM habits))
ON CONFLICT (id) DO NOTHING;

-- name: RenameHabit :execrows
UPDATE habits SET name = sqlc.arg(name) WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteHabit :execrows
UPDATE habits SET deleted_at = CAST(sqlc.arg(deleted_at) AS INTEGER) WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: RestoreHabit :execrows
UPDATE habits SET deleted_at = NULL WHERE id = sqlc.arg(id);

-- name: PurgeDeletedBefore :execrows
DELETE FROM habits WHERE deleted_at IS NOT NULL AND deleted_at < CAST(sqlc.arg(cutoff) AS INTEGER);

-- name: CountActiveHabit :one
SELECT COUNT(*) FROM habits WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: MarkDay :exec
INSERT INTO habit_days (habit_id, day) VALUES (sqlc.arg(habit_id), sqlc.arg(day))
ON CONFLICT (habit_id, day) DO NOTHING;

-- name: UnmarkDay :exec
DELETE FROM habit_days WHERE habit_id = sqlc.arg(habit_id) AND day = sqlc.arg(day);

-- name: ListSettings :many
SELECT key, value FROM settings;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO UPDATE SET value = excluded.value;

-- name: InsertSettingIfAbsent :execrows
INSERT INTO settings (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO NOTHING;
