package sqlite

import (
	"context"
	"errors"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
)

const (
	idA = "00000000-0000-4000-8000-00000000000a"
	idB = "00000000-0000-4000-8000-00000000000b"
	idC = "00000000-0000-4000-8000-00000000000c"
	idD = "00000000-0000-4000-8000-00000000000d"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "steady.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func mustState(t *testing.T, s *Store) domain.State {
	t.Helper()
	st, err := s.State(t.Context())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	return st
}

func mustAdd(t *testing.T, s *Store, id, name string) {
	t.Helper()
	if err := s.AddHabit(t.Context(), id, name); err != nil {
		t.Fatalf("AddHabit(%s, %q): %v", id, name, err)
	}
}

func mustDelete(t *testing.T, s *Store, id string, at time.Time) {
	t.Helper()
	if err := s.DeleteHabit(t.Context(), id, at); err != nil {
		t.Fatalf("DeleteHabit(%s): %v", id, err)
	}
}

func mustSetDay(t *testing.T, s *Store, id, day string, done bool) {
	t.Helper()
	if err := s.SetDay(t.Context(), id, day, done); err != nil {
		t.Fatalf("SetDay(%s, %s, %t): %v", id, day, done, err)
	}
}

func habitNames(st domain.State) []string {
	names := make([]string, 0, len(st.Habits))
	for _, h := range st.Habits {
		names = append(names, h.Name)
	}
	return names
}

func habitByID(t *testing.T, st domain.State, id string) domain.Habit {
	t.Helper()
	i := slices.IndexFunc(st.Habits, func(h domain.Habit) bool { return h.ID == id })
	if i < 0 {
		t.Fatalf("habit %s is not in the state %v", id, habitNames(st))
	}
	return st.Habits[i]
}

// assertNames stops the test on a mismatch: the scenarios below build on each other,
// so a wrong order or set would only make the later steps fail with noise.
func assertNames(t *testing.T, st domain.State, step string, want ...string) {
	t.Helper()
	if got := habitNames(st); !slices.Equal(got, want) {
		t.Fatalf("%s: names = %v, want %v", step, got, want)
	}
}

func TestOpenPragmas(t *testing.T) {
	// Every value is scanned as text: journal_mode answers with a word, the others with a number.
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "journal mode", query: "PRAGMA journal_mode", want: "wal"},
		{name: "foreign keys", query: "PRAGMA foreign_keys", want: "1"},
		{name: "busy timeout", query: "PRAGMA busy_timeout", want: "5000"},
	}
	s := newStore(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if err := s.db.QueryRowContext(t.Context(), tc.query).Scan(&got); err != nil {
				t.Fatalf("%s: %v", tc.query, err)
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestOpenCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "data", "steady.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open(%s) with missing parent directories: %v", path, err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

func TestAddHabitOrderAndConflict(t *testing.T) {
	s := newStore(t)
	mustAdd(t, s, idA, "Alpha")
	mustAdd(t, s, idB, "Beta")

	st := mustState(t, s)
	assertNames(t, st, "after adding A and B", "Alpha", "Beta")
	for _, h := range st.Habits {
		if h.Done == nil {
			t.Errorf("habit %q: Done is nil, want an empty slice", h.Name)
		}
		if len(h.Done) != 0 {
			t.Errorf("habit %q: Done = %v, want it empty", h.Name, h.Done)
		}
	}

	if err := s.AddHabit(t.Context(), idA, "Alpha"); !errors.Is(err, domain.ErrExists) {
		t.Errorf("AddHabit(A again) error = %v, want %v", err, domain.ErrExists)
	}

	if err := s.RenameHabit(t.Context(), idA, "Renamed"); err != nil {
		t.Fatalf("RenameHabit(A): %v", err)
	}
	assertNames(t, mustState(t, s), "after renaming A", "Renamed", "Beta")
}

func TestSoftDeleteRestoreKeepsPosition(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	s := newStore(t)
	mustAdd(t, s, idA, "Alpha")
	mustAdd(t, s, idB, "Beta")
	mustAdd(t, s, idC, "Gamma")
	mustSetDay(t, s, idB, "2026-10-31", true)

	mustDelete(t, s, idB, t0)
	assertNames(t, mustState(t, s), "after deleting B", "Alpha", "Gamma")

	mustAdd(t, s, idD, "Delta")
	assertNames(t, mustState(t, s), "after adding D", "Alpha", "Gamma", "Delta")

	if err := s.RestoreHabit(t.Context(), idB); err != nil {
		t.Fatalf("RestoreHabit(B): %v", err)
	}
	st := mustState(t, s)
	assertNames(t, st, "after restoring B", "Alpha", "Beta", "Gamma", "Delta")
	if got := habitByID(t, st, idB).Done; !slices.Equal(got, []string{"2026-10-31"}) {
		t.Errorf("restored B: Done = %v, want [2026-10-31]", got)
	}

	// Deleting the last habit and adding one with a smaller id: if the new position ignored the deleted row,
	// it would tie with the restored habit and the id order would put the new one first.
	fresh := newStore(t)
	mustAdd(t, fresh, idB, "Beta")
	mustAdd(t, fresh, idC, "Gamma")
	mustDelete(t, fresh, idC, t0)
	mustAdd(t, fresh, idA, "Alpha")
	if err := fresh.RestoreHabit(t.Context(), idC); err != nil {
		t.Fatalf("RestoreHabit(C) on a fresh store: %v", err)
	}
	assertNames(t, mustState(t, fresh), "after restoring the last habit", "Beta", "Gamma", "Alpha")
}

func TestNotFound(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	// Only B is ever added, so C is an id the store has never seen.
	tests := []struct {
		name     string
		deletedB bool
		call     func(ctx context.Context, s *Store) error
	}{
		{
			name: "rename unknown",
			call: func(ctx context.Context, s *Store) error { return s.RenameHabit(ctx, idC, "Renamed") },
		},
		{
			name: "delete unknown",
			call: func(ctx context.Context, s *Store) error { return s.DeleteHabit(ctx, idC, t0) },
		},
		{
			name:     "delete B twice",
			deletedB: true,
			call:     func(ctx context.Context, s *Store) error { return s.DeleteHabit(ctx, idB, t0) },
		},
		{
			name: "restore unknown",
			call: func(ctx context.Context, s *Store) error { return s.RestoreHabit(ctx, idC) },
		},
		{
			name: "set day unknown",
			call: func(ctx context.Context, s *Store) error { return s.SetDay(ctx, idC, "2026-10-31", true) },
		},
		{
			name:     "set day on soft-deleted B",
			deletedB: true,
			call:     func(ctx context.Context, s *Store) error { return s.SetDay(ctx, idB, "2026-10-31", true) },
		},
		{
			name:     "rename soft-deleted B",
			deletedB: true,
			call:     func(ctx context.Context, s *Store) error { return s.RenameHabit(ctx, idB, "Renamed") },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			mustAdd(t, s, idB, "Beta")
			if tc.deletedB {
				mustDelete(t, s, idB, t0)
			}
			if err := tc.call(t.Context(), s); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("error = %v, want %v", err, domain.ErrNotFound)
			}
		})
	}
}

func TestPurge(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	ctx := t.Context()
	s := newStore(t)
	mustAdd(t, s, idA, "Alpha")
	mustAdd(t, s, idB, "Beta")
	mustSetDay(t, s, idA, "2026-10-30", true)
	mustDelete(t, s, idA, t0)

	// The cutoff is exclusive: A was deleted exactly at t0.
	n, err := s.PurgeDeleted(ctx, t0)
	if err != nil {
		t.Fatalf("PurgeDeleted(t0): %v", err)
	}
	if n != 0 {
		t.Errorf("PurgeDeleted(t0) = %d, want 0", n)
	}
	n, err = s.PurgeDeleted(ctx, t0.Add(time.Second))
	if err != nil {
		t.Fatalf("PurgeDeleted(t0 + 1s): %v", err)
	}
	if n != 1 {
		t.Errorf("PurgeDeleted(t0 + 1s) = %d, want 1", n)
	}

	if err := s.RestoreHabit(ctx, idA); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("RestoreHabit(purged A) error = %v, want %v", err, domain.ErrNotFound)
	}
	var days int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_days WHERE habit_id = ?", idA).Scan(&days); err != nil {
		t.Fatalf("count days of A: %v", err)
	}
	if days != 0 {
		t.Errorf("days of purged A = %d, want 0", days)
	}
	assertNames(t, mustState(t, s), "after the purge", "Beta")
}

func TestSetDay(t *testing.T) {
	s := newStore(t)
	mustAdd(t, s, idA, "Alpha")
	assertDone := func(step string, want ...string) {
		t.Helper()
		if got := habitByID(t, mustState(t, s), idA).Done; !slices.Equal(got, want) {
			t.Fatalf("%s: Done = %v, want %v", step, got, want)
		}
	}

	mustSetDay(t, s, idA, "2026-10-31", true)
	mustSetDay(t, s, idA, "2026-10-31", true)
	assertDone("after marking 2026-10-31 twice", "2026-10-31")

	mustSetDay(t, s, idA, "2026-10-01", true)
	assertDone("after marking 2026-10-01", "2026-10-01", "2026-10-31")

	mustSetDay(t, s, idA, "2026-10-31", false)
	assertDone("after unmarking 2026-10-31", "2026-10-01")

	mustSetDay(t, s, idA, "2026-10-31", false)
}

func TestSettings(t *testing.T) {
	ctx := t.Context()
	s := newStore(t)
	assertSettings := func(step string, view domain.View, accent string) {
		t.Helper()
		if st := mustState(t, s); st.View != view || st.Accent != accent {
			t.Errorf("%s: view, accent = %q, %q, want %q, %q", step, st.View, st.Accent, view, accent)
		}
	}

	assertSettings("defaults", domain.ViewWeek, "#4FA872")

	month := domain.ViewMonth
	if err := s.UpdateSettings(ctx, domain.SettingsUpdate{View: &month}); err != nil {
		t.Fatalf("UpdateSettings(view): %v", err)
	}
	assertSettings("after the view update", domain.ViewMonth, "#4FA872")

	accent := "#5B8CEB"
	if err := s.UpdateSettings(ctx, domain.SettingsUpdate{Accent: &accent}); err != nil {
		t.Fatalf("UpdateSettings(accent): %v", err)
	}
	assertSettings("after the accent update", domain.ViewMonth, "#5B8CEB")

	// A view-only update must leave the stored accent alone.
	year := domain.ViewYear
	if err := s.UpdateSettings(ctx, domain.SettingsUpdate{View: &year}); err != nil {
		t.Fatalf("UpdateSettings(view again): %v", err)
	}
	assertSettings("after the second view update", domain.ViewYear, "#5B8CEB")

	if _, err := s.db.ExecContext(ctx, "UPDATE settings SET value = 'decade' WHERE key = 'view'"); err != nil {
		t.Fatalf("corrupt the stored view: %v", err)
	}
	if got := mustState(t, s).View; got != domain.ViewWeek {
		t.Errorf("view after storing 'decade' = %q, want %q", got, domain.ViewWeek)
	}
}

func TestSeedOnce(t *testing.T) {
	ctx := t.Context()
	s := newStore(t)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	rng := rand.New(rand.NewPCG(1, 2))
	seed := func(step string) bool {
		t.Helper()
		seeded, err := s.SeedOnce(ctx, now, rng)
		if err != nil {
			t.Fatalf("SeedOnce (%s): %v", step, err)
		}
		return seeded
	}

	if !seed("first call") {
		t.Fatal("first SeedOnce = false, want true")
	}
	st := mustState(t, s)
	// The names are spelled out: comparing with domain.SeedNames() would accept whatever it returns.
	assertNames(t, st, "after the first seed", "Drink 2 L of water", "Walk 30 minutes", "Read 10 pages")
	for _, h := range st.Habits {
		if err := domain.ValidateID(h.ID); err != nil {
			t.Errorf("habit %q: seeded id %q: %v", h.Name, h.ID, err)
		}
		if len(h.Done) == 0 {
			t.Errorf("habit %q: Done is empty, want a year of history", h.Name)
		}
		for _, day := range h.Done {
			if day < "2025-11-02" || day > "2026-11-01" {
				t.Errorf("habit %q: day %s is outside 2025-11-02..2026-11-01", h.Name, day)
			}
		}
	}

	if seed("second call") {
		t.Error("second SeedOnce = true, want false")
	}
	if got := len(mustState(t, s).Habits); got != 3 {
		t.Errorf("habits after the second seed = %d, want 3", got)
	}

	for _, h := range st.Habits {
		mustDelete(t, s, h.ID, now)
	}
	if seed("third call") {
		t.Error("third SeedOnce = true, want false")
	}
	if got := len(mustState(t, s).Habits); got != 0 {
		t.Errorf("habits after deleting all and seeding again = %d, want 0", got)
	}
}
