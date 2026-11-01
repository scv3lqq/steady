-- +goose Up
CREATE TABLE habits (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    deleted_at INTEGER
) STRICT;

CREATE TABLE habit_days (
    habit_id TEXT NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    day TEXT NOT NULL,
    PRIMARY KEY (habit_id, day)
) STRICT, WITHOUT ROWID;

CREATE TABLE settings (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
) STRICT;

-- +goose Down
DROP TABLE settings;
DROP TABLE habit_days;
DROP TABLE habits;
