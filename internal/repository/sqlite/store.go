// Package sqlite is the SQLite adapter of the habit repository: embedded goose migrations,
// sqlc-generated queries and owner-scoped data access.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/repository/sqlite/sqlcdb"
)

// Keys of the settings table.
const (
	keyView   = "view"
	keyAccent = "accent"
)

// dsnQuery is applied to every connection: foreign_keys makes a purge cascade to habit_days,
// and _txlock=immediate takes the write lock at BEGIN, so a transaction never fails upgrading a read lock.
const dsnQuery = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate"

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store keeps habits, their done days and the settings in one SQLite file.
// It holds a single connection, so inside a transaction only the queries from WithTx may touch the database.
type Store struct {
	db *sql.DB
	q  *sqlcdb.Queries
}

// Open creates the database directory and file if needed, applies the migrations and returns the store.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", path+dsnQuery)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite has one writer; one connection also keeps the pragmas of the DSN in force for every statement.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db, q: sqlcdb.New(db)}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("sub migrations: %w", err)
	}
	// The provider is never closed: Close would close db, which the Store keeps using.
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, files)
	if err != nil {
		return fmt.Errorf("new provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close sqlite: %w", err)
	}
	return nil
}

// Ping checks that the database answers.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}

// State returns the active habits in position order with their done days. A missing or invalid stored view or
// accent falls back to the default.
func (s *Store) State(ctx context.Context, userID string) (domain.State, error) {
	rows, err := s.q.ListSettings(ctx, userID)
	if err != nil {
		return domain.State{}, fmt.Errorf("list settings: %w", err)
	}
	settings := make(map[string]string, len(rows))
	for _, row := range rows {
		settings[row.Key] = row.Value
	}
	habits, err := s.q.ListActiveHabits(ctx, ownerID(userID))
	if err != nil {
		return domain.State{}, fmt.Errorf("list habits: %w", err)
	}
	days, err := s.q.ListActiveDays(ctx, ownerID(userID))
	if err != nil {
		return domain.State{}, fmt.Errorf("list days: %w", err)
	}
	doneByHabit := make(map[string][]string, len(habits))
	for _, d := range days {
		doneByHabit[d.HabitID] = append(doneByHabit[d.HabitID], d.Day)
	}
	state := domain.State{
		Habits: make([]domain.Habit, 0, len(habits)),
		View:   viewOrDefault(settings[keyView]),
		Accent: accentOrDefault(settings[keyAccent]),
	}
	for _, h := range habits {
		done := doneByHabit[h.ID]
		if done == nil {
			done = []string{}
		}
		state.Habits = append(state.Habits, domain.Habit{ID: h.ID, Name: h.Name, Done: done})
	}
	return state, nil
}

func viewOrDefault(stored string) domain.View {
	if v, err := domain.ParseView(stored); err == nil {
		return v
	}
	return domain.DefaultView
}

func accentOrDefault(stored string) string {
	if domain.ValidateAccent(stored) == nil {
		return stored
	}
	return domain.DefaultAccent
}

// AddHabit appends a habit after the last one, deleted ones included. It returns domain.ErrExists when the id
// is taken, also by a soft-deleted habit.
func (s *Store) AddHabit(ctx context.Context, userID, id, name string) error {
	n, err := s.q.InsertHabit(ctx, sqlcdb.InsertHabitParams{UserID: ownerID(userID), ID: id, Name: name})
	if err != nil {
		return fmt.Errorf("add habit %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("add habit %s: %w", id, domain.ErrExists)
	}
	return nil
}

// RenameHabit renames an active habit. It returns domain.ErrNotFound when there is none.
func (s *Store) RenameHabit(ctx context.Context, userID, id, name string) error {
	n, err := s.q.RenameHabit(ctx, sqlcdb.RenameHabitParams{UserID: ownerID(userID), Name: name, ID: id})
	if err != nil {
		return fmt.Errorf("rename habit %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("rename habit %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// DeleteHabit soft-deletes an active habit at the given time, so RestoreHabit can bring it back at its
// position. It returns domain.ErrNotFound when there is no active habit.
func (s *Store) DeleteHabit(ctx context.Context, userID, id string, at time.Time) error {
	n, err := s.q.SoftDeleteHabit(ctx, sqlcdb.SoftDeleteHabitParams{UserID: ownerID(userID), DeletedAt: at.Unix(), ID: id})
	if err != nil {
		return fmt.Errorf("delete habit %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete habit %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// RestoreHabit undoes DeleteHabit; on an active habit it does nothing. It returns domain.ErrNotFound when the
// habit is gone.
func (s *Store) RestoreHabit(ctx context.Context, userID, id string) error {
	n, err := s.q.RestoreHabit(ctx, sqlcdb.RestoreHabitParams{ID: id, UserID: ownerID(userID)})
	if err != nil {
		return fmt.Errorf("restore habit %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("restore habit %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// PurgeDeleted hard-deletes the habits soft-deleted before the given time, with their days, and returns how
// many it removed.
func (s *Store) PurgeDeleted(ctx context.Context, before time.Time) (int64, error) {
	n, err := s.q.PurgeDeletedBefore(ctx, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("purge deleted habits: %w", err)
	}
	return n, nil
}

// SetDay marks or unmarks a day of an active habit; repeating either is not an error. It returns
// domain.ErrNotFound when there is no active habit.
func (s *Store) SetDay(ctx context.Context, userID, id, day string, done bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set day %s: begin: %w", id, err)
	}
	defer rollback(tx)
	q := s.q.WithTx(tx)
	n, err := q.CountActiveHabit(ctx, sqlcdb.CountActiveHabitParams{ID: id, UserID: ownerID(userID)})
	if err != nil {
		return fmt.Errorf("set day %s: count habit: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("set day %s: %w", id, domain.ErrNotFound)
	}
	if done {
		err = q.MarkDay(ctx, sqlcdb.MarkDayParams{UserID: ownerID(userID), HabitID: id, Day: day})
	} else {
		err = q.UnmarkDay(ctx, sqlcdb.UnmarkDayParams{UserID: ownerID(userID), HabitID: id, Day: day})
	}
	if err != nil {
		return fmt.Errorf("set day %s %s: %w", id, day, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set day %s: commit: %w", id, err)
	}
	return nil
}

// UpdateSettings stores the view and the accent that are set in u; both are written or neither.
func (s *Store) UpdateSettings(ctx context.Context, userID string, u domain.SettingsUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update settings: begin: %w", err)
	}
	defer rollback(tx)
	q := s.q.WithTx(tx)
	if u.View != nil {
		if err := q.UpsertSetting(ctx, sqlcdb.UpsertSettingParams{UserID: userID, Key: keyView, Value: string(*u.View)}); err != nil {
			return fmt.Errorf("update settings: view: %w", err)
		}
	}
	if u.Accent != nil {
		if err := q.UpsertSetting(ctx, sqlcdb.UpsertSettingParams{UserID: userID, Key: keyAccent, Value: *u.Accent}); err != nil {
			return fmt.Errorf("update settings: accent: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update settings: commit: %w", err)
	}
	return nil
}

// rollback undoes a transaction that was not committed; after Commit it has nothing to undo.
func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

// ownerID supplies an explicit non-NULL owner predicate, including for an empty caller ID.
func ownerID(userID string) sql.NullString {
	return sql.NullString{String: userID, Valid: true}
}
