package usecase

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/domain"
)

// testID is a valid habit id. The expected calls below spell it out again instead of reusing the constant.
const testID = "0f8fad5b-d9cb-469f-a165-70867728950e"

// fakeRepo is a hand-written Repository. Each call appends one line, Method(arg, ...), to calls, so a test
// compares the whole conversation with a literal slice: the order of the calls and the absence of a call
// are checked the same way as an argument.
type fakeRepo struct {
	calls []string

	deleteErr error // returned by DeleteHabit
	purgeErr  error // returned by PurgeDeleted
}

func (r *fakeRepo) State(_ context.Context) (domain.State, error) {
	r.calls = append(r.calls, "State()")
	return domain.State{}, nil
}

func (r *fakeRepo) AddHabit(_ context.Context, id, name string) error {
	r.calls = append(r.calls, fmt.Sprintf("AddHabit(%q, %q)", id, name))
	return nil
}

func (r *fakeRepo) RenameHabit(_ context.Context, id, name string) error {
	r.calls = append(r.calls, fmt.Sprintf("RenameHabit(%q, %q)", id, name))
	return nil
}

func (r *fakeRepo) DeleteHabit(_ context.Context, id string, at time.Time) error {
	r.calls = append(r.calls, fmt.Sprintf("DeleteHabit(%q, %s)", id, stamp(at)))
	return r.deleteErr
}

func (r *fakeRepo) RestoreHabit(_ context.Context, id string) error {
	r.calls = append(r.calls, fmt.Sprintf("RestoreHabit(%q)", id))
	return nil
}

func (r *fakeRepo) PurgeDeleted(_ context.Context, before time.Time) (int64, error) {
	r.calls = append(r.calls, fmt.Sprintf("PurgeDeleted(%s)", stamp(before)))
	return 0, r.purgeErr
}

func (r *fakeRepo) SetDay(_ context.Context, id, day string, done bool) error {
	r.calls = append(r.calls, fmt.Sprintf("SetDay(%q, %q, %t)", id, day, done))
	return nil
}

func (r *fakeRepo) UpdateSettings(_ context.Context, u domain.SettingsUpdate) error {
	r.calls = append(r.calls, fmt.Sprintf("UpdateSettings(view=%s, accent=%s)", show(u.View), show(u.Accent)))
	return nil
}

func (r *fakeRepo) Ping(_ context.Context) error {
	r.calls = append(r.calls, "Ping()")
	return nil
}

// stamp prints an instant in UTC with full precision, so that expected times are literal text
// and a drift of a single nanosecond shows.
func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// show prints a pointer by value, so a failure names the text instead of an address
// and a nil pointer stays distinct from a pointer to an empty string.
func show[T ~string](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%q", string(*p))
}

// newService returns a Service over repo whose clock stands still at 2026-11-01T12:00:00Z.
func newService(repo Repository) *Service {
	now := func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) }
	return New(repo, now, zerolog.Nop())
}

func checkCalls(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("repository calls = %q, want %q", got, want)
	}
}

func TestAddHabit(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		habitName string
		want      domain.Habit
		wantErr   error
		wantCalls []string
	}{
		{
			name:      "bad id never reaches the repository",
			id:        "x",
			habitName: "Read",
			wantErr:   domain.ErrInvalidID,
		},
		{
			name:      "name is trimmed before it is stored",
			id:        testID,
			habitName: "  Read ",
			want:      domain.Habit{ID: testID, Name: "Read", Done: []string{}},
			wantCalls: []string{`AddHabit("0f8fad5b-d9cb-469f-a165-70867728950e", "Read")`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}

			got, err := newService(repo).AddHabit(t.Context(), tc.id, tc.habitName)

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("AddHabit(%q, %q) error = %v, want %v", tc.id, tc.habitName, err, tc.wantErr)
			}
			// DeepEqual tells a nil Done from an empty one, and the client gets null for the first and [] for the second.
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("AddHabit(%q, %q) = %#v, want %#v", tc.id, tc.habitName, got, tc.want)
			}
			checkCalls(t, repo.calls, tc.wantCalls)
		})
	}
}

func TestRenameHabit(t *testing.T) {
	tests := []struct {
		name      string
		newName   *string
		wantErr   error
		wantCalls []string
	}{
		{
			name:    "missing name is an error, not Untitled",
			wantErr: domain.ErrNameRequired,
		},
		{
			name:      "empty name becomes Untitled",
			newName:   new(""),
			wantCalls: []string{`RenameHabit("0f8fad5b-d9cb-469f-a165-70867728950e", "Untitled")`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}

			_, err := newService(repo).RenameHabit(t.Context(), testID, tc.newName)

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("RenameHabit(%q, %s) error = %v, want %v", testID, show(tc.newName), err, tc.wantErr)
			}
			checkCalls(t, repo.calls, tc.wantCalls)
		})
	}
}

func TestDeleteHabit(t *testing.T) {
	const (
		deleted = `DeleteHabit("0f8fad5b-d9cb-469f-a165-70867728950e", 2026-11-01T12:00:00Z)`
		purged  = `PurgeDeleted(2026-10-31T12:00:00Z)`
	)
	tests := []struct {
		name      string
		deleteErr error
		purgeErr  error
		wantErr   error
		wantCalls []string
	}{
		{
			name:      "deletes at now, then purges what is older than a day",
			wantCalls: []string{deleted, purged},
		},
		{
			name:      "purge failure is not returned",
			purgeErr:  errors.New("database is locked"),
			wantCalls: []string{deleted, purged},
		},
		{
			name:      "unknown habit skips the purge",
			deleteErr: domain.ErrNotFound,
			wantErr:   domain.ErrNotFound,
			wantCalls: []string{deleted},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{deleteErr: tc.deleteErr, purgeErr: tc.purgeErr}

			err := newService(repo).DeleteHabit(t.Context(), testID)

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("DeleteHabit(%q) error = %v, want %v", testID, err, tc.wantErr)
			}
			checkCalls(t, repo.calls, tc.wantCalls)
		})
	}
}

func TestSetDay(t *testing.T) {
	tests := []struct {
		name      string
		day       string
		wantErr   error
		wantCalls []string
	}{
		{
			name:    "day without zero padding never reaches the repository",
			day:     "2026-1-1",
			wantErr: domain.ErrInvalidDay,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}

			err := newService(repo).SetDay(t.Context(), testID, tc.day, true)

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("SetDay(%q, %q, true) error = %v, want %v", testID, tc.day, err, tc.wantErr)
			}
			checkCalls(t, repo.calls, tc.wantCalls)
		})
	}
}

func TestUpdateSettings(t *testing.T) {
	tests := []struct {
		name      string
		view      *string
		accent    *string
		wantErr   error
		wantCalls []string
	}{
		{
			name:    "nothing to update",
			wantErr: domain.ErrEmptySettings,
		},
		{
			name:      "view only leaves the accent alone",
			view:      new("year"),
			wantCalls: []string{`UpdateSettings(view="year", accent=nil)`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}

			err := newService(repo).UpdateSettings(t.Context(), tc.view, tc.accent)

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("UpdateSettings(%s, %s) error = %v, want %v", show(tc.view), show(tc.accent), err, tc.wantErr)
			}
			checkCalls(t, repo.calls, tc.wantCalls)
		})
	}
}

func TestPurge(t *testing.T) {
	repo := &fakeRepo{}

	if _, err := newService(repo).Purge(t.Context()); err != nil {
		t.Fatalf("Purge() error = %v, want nil", err)
	}

	checkCalls(t, repo.calls, []string{`PurgeDeleted(2026-10-31T12:00:00Z)`})
}
