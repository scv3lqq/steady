-- name: ListActiveHabits :many
SELECT id, name FROM habits
WHERE user_id = sqlc.arg(user_id) AND deleted_at IS NULL ORDER BY position, id;

-- name: ListActiveDays :many
SELECT d.habit_id, d.day FROM habit_days AS d JOIN habits AS h ON h.id = d.habit_id
WHERE h.user_id = sqlc.arg(user_id) AND h.deleted_at IS NULL ORDER BY d.habit_id, d.day;

-- name: InsertHabit :execrows
INSERT INTO habits (id, name, user_id, position)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(user_id),
    (SELECT COALESCE(MAX(position), -1) + 1 FROM habits WHERE user_id = sqlc.arg(user_id)))
ON CONFLICT (id) DO NOTHING;

-- name: RenameHabit :execrows
UPDATE habits SET name = sqlc.arg(name)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL;

-- name: SoftDeleteHabit :execrows
UPDATE habits SET deleted_at = CAST(sqlc.arg(deleted_at) AS INTEGER)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL;

-- name: RestoreHabit :execrows
UPDATE habits SET deleted_at = NULL, legacy_preserved = 0
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: PurgeDeletedBefore :execrows
DELETE FROM habits WHERE deleted_at IS NOT NULL AND deleted_at < CAST(sqlc.arg(cutoff) AS INTEGER)
AND user_id IS NOT NULL AND legacy_preserved = 0;

-- name: CountActiveHabit :one
SELECT COUNT(*) FROM habits
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL;

-- name: MarkDay :exec
INSERT INTO habit_days (habit_id, day)
SELECT id, sqlc.arg(day) FROM habits
WHERE id = sqlc.arg(habit_id) AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL
ON CONFLICT (habit_id, day) DO NOTHING;

-- name: UnmarkDay :exec
DELETE FROM habit_days WHERE habit_id = sqlc.arg(habit_id) AND day = sqlc.arg(day)
AND EXISTS (SELECT 1 FROM habits WHERE id = sqlc.arg(habit_id)
AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL);

-- name: ListSettings :many
SELECT key, value FROM settings WHERE user_id = sqlc.arg(user_id);

-- name: UpsertSetting :exec
INSERT INTO settings (user_id, key, value) VALUES (sqlc.arg(user_id), sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value;

-- name: CreateAccount :execrows
INSERT INTO users(id, username, password_hash)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(password_hash))
ON CONFLICT (username) DO NOTHING;

-- name: AccountByUsername :one
SELECT id, username, password_hash FROM users WHERE username = sqlc.arg(username);

-- name: BootstrapUsername :one
SELECT username FROM bootstrap_ledger WHERE singleton = 1;

-- name: AssignLegacyHabits :exec
UPDATE habits SET user_id = sqlc.arg(user_id) WHERE user_id IS NULL;

-- name: CopyLegacySettings :exec
INSERT INTO settings(user_id, key, value)
SELECT sqlc.arg(user_id), key, value FROM legacy_settings;

-- name: ClearLegacySettings :exec
DELETE FROM legacy_settings;

-- name: CompleteBootstrap :exec
INSERT INTO bootstrap_ledger(singleton, username) VALUES(1, sqlc.arg(username));

-- name: ResetPassword :execrows
UPDATE users SET password_hash = sqlc.arg(password_hash) WHERE username = sqlc.arg(username);

-- name: RevokeUserSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id);

-- name: InsertSession :execrows
INSERT INTO sessions(token_hash, user_id, expires_at)
SELECT sqlc.arg(token_hash), id, CAST(sqlc.arg(expires_at) AS INTEGER) FROM users
WHERE id = sqlc.arg(user_id) AND password_hash = sqlc.arg(expected_password_hash);

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= CAST(sqlc.arg(now) AS INTEGER);

-- name: SessionUser :one
SELECT u.id, u.username, u.password_hash FROM sessions AS s JOIN users AS u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg(token_hash) AND s.expires_at > CAST(sqlc.arg(now) AS INTEGER);

-- name: RevokeSession :exec
DELETE FROM sessions WHERE token_hash = sqlc.arg(token_hash);
