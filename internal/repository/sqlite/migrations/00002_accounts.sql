-- +goose Up
CREATE TABLE users (
    id TEXT PRIMARY KEY NOT NULL,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL
) STRICT;

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL
) STRICT;
CREATE INDEX sessions_user_id ON sessions(user_id);
CREATE INDEX sessions_expires_at ON sessions(expires_at);

CREATE TABLE bootstrap_ledger (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    username TEXT NOT NULL
) STRICT;

ALTER TABLE habits ADD COLUMN user_id TEXT REFERENCES users(id);
ALTER TABLE habits ADD COLUMN legacy_preserved INTEGER NOT NULL DEFAULT 0
    CHECK (legacy_preserved IN (0, 1));
-- Preserve deleted legacy rows until their owner explicitly restores and deletes them again.
UPDATE habits SET legacy_preserved = 1 WHERE deleted_at IS NOT NULL;
CREATE INDEX habits_user_position ON habits(user_id, position);

ALTER TABLE settings RENAME TO legacy_settings;
CREATE TABLE settings (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (user_id, key)
) STRICT, WITHOUT ROWID;

-- +goose Down
-- Account migrations cannot safely reverse ownership. Restore a protected backup
-- together with its matching binary instead of attempting an in-place downgrade.
SELECT * FROM downgrade_requires_backup_and_matching_binary;
