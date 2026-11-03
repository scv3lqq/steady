package sqlite

import (
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/repository/sqlite/sqlcdb"
	"github.com/scv3lqq/steady/internal/usecase"
)

const otherOwner = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func openAccountStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func account(t *testing.T, s *Store, id, username string) domain.Account {
	t.Helper()
	a := domain.Account{ID: id, Username: username, PasswordHash: "original-hash-" + username}
	if err := s.CreateAccount(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func ownerState(t *testing.T, s *Store, owner string) domain.State {
	t.Helper()
	state, err := s.State(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func fixtureExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func scalar(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func legacyDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path+dsnQuery)
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, "INSERT INTO habits(id,name,position,deleted_at) VALUES (?,?,?,NULL),(?,?,?,?)", idA, "Legacy active", 9, idB, "Legacy deleted", 3, 1)
	fixtureExec(t, db, "INSERT INTO habit_days(habit_id,day) VALUES (?,?),(?,?)", idA, "2020-02-29", idB, "2001-01-01")
	fixtureExec(t, db, "INSERT INTO settings(key,value) VALUES ('view','year'),('accent','#E3C24F'),('seeded','1'),('future-key','opaque legacy value')")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertLegacyValues(t *testing.T, s *Store, owner string) {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(), "SELECT id,name,position,deleted_at,user_id,legacy_preserved FROM habits ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID, Name  string
		Position  int
		Deleted   sql.NullInt64
		Owner     sql.NullString
		Preserved int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Name, &r.Position, &r.Deleted, &r.Owner, &r.Preserved); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	wantOwner := sql.NullString{String: owner, Valid: owner != ""}
	want := []row{
		{idA, "Legacy active", 9, sql.NullInt64{}, wantOwner, 0},
		{idB, "Legacy deleted", 3, sql.NullInt64{Int64: 1, Valid: true}, wantOwner, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy rows = %#v, want %#v", got, want)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM habit_days WHERE (habit_id=? AND day='2020-02-29') OR (habit_id=? AND day='2001-01-01')", idA, idB); n != 2 {
		t.Fatalf("preserved marks = %d, want 2", n)
	}
}

func TestLegacyBootstrapPreservesEveryValueAcrossRestart(t *testing.T) {
	path := legacyDatabase(t)
	s := openAccountStore(t, path)
	account(t, s, otherOwner, "visitor")
	assertNames(t, ownerState(t, s, otherOwner), "before bootstrap")
	if n, err := s.PurgeDeleted(t.Context(), time.Now()); err != nil || n != 0 {
		t.Fatalf("pre-bootstrap purge = %d, %v", n, err)
	}
	assertLegacyValues(t, s, "")
	owner := domain.Account{ID: testOwner, Username: "owner", PasswordHash: "original-owner-hash"}
	created, err := s.BootstrapOwner(t.Context(), owner)
	if err != nil || !created {
		t.Fatalf("bootstrap = %t, %v", created, err)
	}
	assertLegacyValues(t, s, testOwner)
	state := ownerState(t, s, testOwner)
	assertNames(t, state, "bootstrapped", "Legacy active")
	if state.View != domain.ViewYear || state.Accent != "#E3C24F" {
		t.Fatalf("legacy settings = %#v", state)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM settings WHERE user_id=? AND ((key='future-key' AND value='opaque legacy value') OR (key='seeded' AND value='1'))", testOwner); n != 2 {
		t.Fatalf("unknown settings copied = %d, want 2", n)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM legacy_settings"); n != 0 {
		t.Fatalf("legacy staging rows = %d", n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAccountStore(t, path)
	if n, err := s.PurgeDeleted(t.Context(), time.Now()); err != nil || n != 0 {
		t.Fatalf("restart purge = %d, %v", n, err)
	}
	assertLegacyValues(t, s, testOwner)
	retry := domain.Account{ID: "cccccccccccccccccccccccccccccccc", Username: "owner", PasswordHash: "replacement-hash"}
	if changed, err := s.BootstrapOwner(t.Context(), retry); err != nil || changed {
		t.Fatalf("same bootstrap retry = %t, %v", changed, err)
	}
	stored, err := s.AccountByUsername(t.Context(), "owner")
	if err != nil || stored != owner {
		t.Fatalf("retry changed account: %#v, %v", stored, err)
	}
	retry.Username = "another"
	if _, err := s.BootstrapOwner(t.Context(), retry); !errors.Is(err, domain.ErrBootstrapComplete) {
		t.Fatalf("different retry = %v", err)
	}
	if err := s.RestoreHabit(t.Context(), testOwner, idB); err != nil {
		t.Fatal(err)
	}
	assertNames(t, ownerState(t, s, testOwner), "restored position", "Legacy deleted", "Legacy active")
	if err := s.DeleteHabit(t.Context(), testOwner, idB, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PurgeDeleted(t.Context(), time.Unix(3, 0)); err != nil || n != 1 {
		t.Fatalf("explicit re-delete purge = %d, %v", n, err)
	}
	assertNames(t, ownerState(t, s, otherOwner), "visitor never claimed legacy")
}

func TestBootstrapFailureRollsBackAndCanRetry(t *testing.T) {
	s := openAccountStore(t, legacyDatabase(t))
	fixtureExec(t, s.db, "CREATE TRIGGER fail_legacy_copy BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT,'injected fixture failure'); END")
	owner := domain.Account{ID: testOwner, Username: "owner", PasswordHash: "hash"}
	if changed, err := s.BootstrapOwner(t.Context(), owner); err == nil || changed {
		t.Fatalf("injected failure = %t, %v", changed, err)
	}
	if _, err := s.AccountByUsername(t.Context(), "owner"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("rolled back account = %v", err)
	}
	assertLegacyValues(t, s, "")
	if n := scalar(t, s, "SELECT COUNT(*) FROM legacy_settings"); n != 4 {
		t.Fatalf("staging after failure = %d, want 4", n)
	}
	fixtureExec(t, s.db, "DROP TRIGGER fail_legacy_copy")
	if changed, err := s.BootstrapOwner(t.Context(), owner); err != nil || !changed {
		t.Fatalf("retry after rollback = %t, %v", changed, err)
	}
	assertLegacyValues(t, s, testOwner)
}

func TestAccountsStartEmptyAndBootstrapRejectsExistingUsername(t *testing.T) {
	s := openAccountStore(t, legacyDatabase(t))
	a := account(t, s, testOwner, "owner")
	state := ownerState(t, s, testOwner)
	if state.Habits == nil || len(state.Habits) != 0 || state.View != domain.ViewWeek || state.Accent != "#4FA872" {
		t.Fatalf("new account state = %#v", state)
	}
	if err := s.CreateAccount(t.Context(), domain.Account{ID: otherOwner, Username: "owner", PasswordHash: "different"}); !errors.Is(err, domain.ErrUsernameExists) {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := s.BootstrapOwner(t.Context(), a); !errors.Is(err, domain.ErrUsernameExists) {
		t.Fatalf("existing bootstrap name = %v", err)
	}
	if _, err := s.AccountByUsername(t.Context(), "missing"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("missing lookup = %v", err)
	}
	assertLegacyValues(t, s, "")
}

func TestOwnershipForEveryMutationAndSettings(t *testing.T) {
	s := newStore(t)
	account(t, s, otherOwner, "other")
	ctx := t.Context()
	mustAdd(t, s, idA, "Private")
	mustSetDay(t, s, idA, "2026-10-31", true)
	cases := []struct {
		name   string
		mutate func() error
	}{
		{"rename", func() error { return s.RenameHabit(ctx, otherOwner, idA, "stolen") }},
		{"delete", func() error { return s.DeleteHabit(ctx, otherOwner, idA, time.Now()) }},
		{"restore", func() error { return s.RestoreHabit(ctx, otherOwner, idA) }},
		{"mark", func() error { return s.SetDay(ctx, otherOwner, idA, "2026-11-01", true) }},
		{"unmark", func() error { return s.SetDay(ctx, otherOwner, idA, "2026-10-31", false) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.mutate(); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("foreign mutation error = %v", err)
			}
		})
	}
	if err := s.AddHabit(ctx, otherOwner, idA, "duplicate global ID"); !errors.Is(err, domain.ErrExists) {
		t.Fatalf("global ID uniqueness = %v", err)
	}
	if err := s.AddHabit(ctx, otherOwner, idB, "Other"); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, s, idC, "Second private")
	if n := scalar(t, s, "SELECT position FROM habits WHERE id=?", idB); n != 0 {
		t.Fatalf("other initial position=%d, want 0", n)
	}
	if n := scalar(t, s, "SELECT position FROM habits WHERE id=?", idC); n != 1 {
		t.Fatalf("owner second position=%d, want 1", n)
	}
	view, accent := domain.ViewMonth, "#5B8CEB"
	if err := s.UpdateSettings(ctx, otherOwner, domain.SettingsUpdate{View: &view, Accent: &accent}); err != nil {
		t.Fatal(err)
	}
	own := ownerState(t, s, testOwner)
	assertNames(t, own, "owner state", "Private", "Second private")
	if !reflect.DeepEqual(own.Habits[0].Done, []string{"2026-10-31"}) || own.View != domain.ViewWeek || own.Accent != "#4FA872" {
		t.Fatalf("foreign mutations changed owner: %#v", own)
	}
	other := ownerState(t, s, otherOwner)
	assertNames(t, other, "other state", "Other")
	if len(other.Habits[0].Done) != 0 || other.View != domain.ViewMonth || other.Accent != "#5B8CEB" {
		t.Fatalf("other state = %#v", other)
	}
	mustDelete(t, s, idA, time.Now())
	if err := s.RestoreHabit(ctx, otherOwner, idA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign undo = %v", err)
	}
}

func TestDirectSQLCDayQueriesEnforceOwnerAndActiveHabit(t *testing.T) {
	s := newStore(t)
	account(t, s, otherOwner, "other")
	mustAdd(t, s, idA, "Private")
	mustSetDay(t, s, idA, "2026-10-31", true)
	for _, owner := range []string{otherOwner, ""} {
		if err := s.q.MarkDay(t.Context(), sqlcdb.MarkDayParams{UserID: sql.NullString{String: owner, Valid: true}, HabitID: idA, Day: "2026-11-01"}); err != nil {
			t.Fatal(err)
		}
		if err := s.q.UnmarkDay(t.Context(), sqlcdb.UnmarkDayParams{UserID: sql.NullString{String: owner, Valid: true}, HabitID: idA, Day: "2026-10-31"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM habit_days WHERE habit_id=? AND day='2026-11-01'", idA); n != 0 {
		t.Fatalf("foreign direct mark inserted %d rows", n)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM habit_days WHERE habit_id=? AND day='2026-10-31'", idA); n != 1 {
		t.Fatalf("foreign direct unmark left %d rows, want 1", n)
	}
	mustDelete(t, s, idA, time.Now())
	if err := s.q.MarkDay(t.Context(), sqlcdb.MarkDayParams{UserID: sql.NullString{String: testOwner, Valid: true}, HabitID: idA, Day: "2026-11-01"}); err != nil {
		t.Fatal(err)
	}
	if err := s.q.UnmarkDay(t.Context(), sqlcdb.UnmarkDayParams{UserID: sql.NullString{String: testOwner, Valid: true}, HabitID: idA, Day: "2026-10-31"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM habit_days WHERE habit_id=?", idA); n != 1 {
		t.Fatalf("inactive direct day changed marks: %d", n)
	}
}

func TestSessionsDurableExpiryResetIsolationAndStaleHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	s := openAccountStore(t, path)
	a := account(t, s, testOwner, "owner")
	b := account(t, s, otherOwner, "other")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(usecase.SessionLifetime)
	for _, session := range []struct {
		hash    string
		account domain.Account
	}{{"owner-session", a}, {"owner-session-two", a}, {"other-session", b}} {
		if err := s.InsertSession(t.Context(), session.hash, session.account.ID, session.account.PasswordHash, expiry); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAccountStore(t, path)
	got, err := s.SessionUser(t.Context(), "owner-session", expiry.Add(-time.Nanosecond))
	if err != nil || got.ID != testOwner {
		t.Fatalf("durable session before expiry = %#v, %v", got, err)
	}
	if _, err := s.SessionUser(t.Context(), "owner-session", expiry); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("exact expiry = %v", err)
	}
	if _, err := s.SessionUser(t.Context(), "missing", now); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("missing session = %v", err)
	}
	if err := s.ResetPassword(t.Context(), "owner", "new-hash"); err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{"owner-session", "owner-session-two"} {
		if _, err := s.SessionUser(t.Context(), hash, now); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("reset did not revoke %s: %v", hash, err)
		}
	}
	if got, err := s.SessionUser(t.Context(), "other-session", now); err != nil || got.ID != otherOwner {
		t.Fatalf("reset revoked another user: %#v, %v", got, err)
	}
	if err := s.InsertSession(t.Context(), "stale-session", a.ID, a.PasswordHash, expiry); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("stale verified password inserted: %v", err)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM sessions WHERE token_hash='stale-session'"); n != 0 {
		t.Fatalf("stale session persisted: %d", n)
	}
	updated, err := s.AccountByUsername(t.Context(), "owner")
	if err != nil || updated.PasswordHash != "new-hash" {
		t.Fatalf("reset hash: %#v, %v", updated, err)
	}
	if err := s.InsertSession(t.Context(), "new-session", a.ID, "new-hash", expiry); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeSession(t.Context(), "new-session"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeSession(t.Context(), "new-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(t.Context(), "new-session", now); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked session = %v", err)
	}
	if err := s.ResetPassword(t.Context(), "missing", "hash"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("missing reset = %v", err)
	}
	if err := s.InsertSession(t.Context(), "later-session", b.ID, b.PasswordHash, expiry.Add(usecase.SessionLifetime)); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT COUNT(*) FROM sessions WHERE token_hash='other-session'"); n != 0 {
		t.Fatalf("expired session cleanup left %d", n)
	}
}

func TestPasswordResetFailureRollsBackHashAndSessions(t *testing.T) {
	s := newStore(t)
	a, err := s.AccountByUsername(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	if err := s.InsertSession(t.Context(), "retained-session", a.ID, a.PasswordHash, now.Add(usecase.SessionLifetime)); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, s.db, "CREATE TRIGGER fail_session_revoke BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'injected revoke failure'); END")
	if err := s.ResetPassword(t.Context(), "owner", "replacement-hash"); err == nil {
		t.Fatal("reset succeeded despite injected revoke failure")
	}
	stored, err := s.AccountByUsername(t.Context(), "owner")
	if err != nil || stored.PasswordHash != "hash" {
		t.Fatalf("reset failure changed hash: %#v, %v", stored, err)
	}
	if current, err := s.SessionUser(t.Context(), "retained-session", now); err != nil || current.ID != testOwner {
		t.Fatalf("reset failure revoked session: %#v, %v", current, err)
	}
}

func TestMigrationDowngradeRejectsWithoutChangingData(t *testing.T) {
	s := newStore(t)
	mustAdd(t, s, idA, "Retain through rejected downgrade")
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, s.db, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Down(t.Context()); err == nil {
		t.Fatal("account migration downgrade must reject in-place ownership loss")
	}
	assertNames(t, ownerState(t, s, testOwner), "after rejected downgrade", "Retain through rejected downgrade")
	if _, err := s.AccountByUsername(t.Context(), "owner"); err != nil {
		t.Fatalf("downgrade lost account: %v", err)
	}
}
