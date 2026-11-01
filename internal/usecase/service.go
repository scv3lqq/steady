// Package usecase holds the application services of Steady. It validates input with the domain package,
// reaches storage through the Repository port and wraps every error with the name of the failed operation.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/domain"
)

// PurgeAfter is how long a soft-deleted habit stays restorable before a purge removes it for good.
const PurgeAfter = 24 * time.Hour

// Repository is the storage port the service needs. An implementation reports a missing habit as
// domain.ErrNotFound and a taken id as domain.ErrExists, wrapped or not.
type Repository interface {
	State(ctx context.Context) (domain.State, error)
	AddHabit(ctx context.Context, id, name string) error               // domain.ErrExists when the id is taken (also by a soft-deleted row)
	RenameHabit(ctx context.Context, id, name string) error            // domain.ErrNotFound when no active row
	DeleteHabit(ctx context.Context, id string, at time.Time) error    // soft delete; domain.ErrNotFound when no active row
	RestoreHabit(ctx context.Context, id string) error                 // domain.ErrNotFound when the row is gone (idempotent on an active row)
	PurgeDeleted(ctx context.Context, before time.Time) (int64, error) // hard-deletes rows with deleted_at < before.Unix()
	SetDay(ctx context.Context, id, day string, done bool) error       // domain.ErrNotFound when no active row
	UpdateSettings(ctx context.Context, u domain.SettingsUpdate) error
	Ping(ctx context.Context) error
}

// Service runs the use cases of Steady on top of a Repository.
type Service struct {
	repo Repository
	now  func() time.Time
	log  zerolog.Logger
}

// New returns a Service that reads the current time from now and logs to log.
func New(repo Repository, now func() time.Time, log zerolog.Logger) *Service {
	return &Service{repo: repo, now: now, log: log}
}

// State returns the active habits with their done days, plus the stored settings.
func (s *Service) State(ctx context.Context) (domain.State, error) {
	st, err := s.repo.State(ctx)
	if err != nil {
		return domain.State{}, fmt.Errorf("state: %w", err)
	}
	return st, nil
}

// AddHabit stores a new habit and returns it with an empty, non-nil Done. A blank name becomes domain.DefaultName.
func (s *Service) AddHabit(ctx context.Context, id, name string) (domain.Habit, error) {
	if err := domain.ValidateID(id); err != nil {
		return domain.Habit{}, fmt.Errorf("add habit: %w", err)
	}
	norm, err := domain.NormalizeName(name)
	if err != nil {
		return domain.Habit{}, fmt.Errorf("add habit: %w", err)
	}
	if err = s.repo.AddHabit(ctx, id, norm); err != nil {
		return domain.Habit{}, fmt.Errorf("add habit: %w", err)
	}
	return domain.Habit{ID: id, Name: norm, Done: []string{}}, nil
}

// RenameHabit renames a habit and returns its id and new name; Done is left nil.
// A nil name is domain.ErrNameRequired, a blank one becomes domain.DefaultName.
func (s *Service) RenameHabit(ctx context.Context, id string, name *string) (domain.Habit, error) {
	if err := domain.ValidateID(id); err != nil {
		return domain.Habit{}, fmt.Errorf("rename habit: %w", err)
	}
	if name == nil {
		return domain.Habit{}, fmt.Errorf("rename habit: %w", domain.ErrNameRequired)
	}
	norm, err := domain.NormalizeName(*name)
	if err != nil {
		return domain.Habit{}, fmt.Errorf("rename habit: %w", err)
	}
	if err = s.repo.RenameHabit(ctx, id, norm); err != nil {
		return domain.Habit{}, fmt.Errorf("rename habit: %w", err)
	}
	return domain.Habit{ID: id, Name: norm}, nil
}

// DeleteHabit soft-deletes a habit, then purges habits deleted more than PurgeAfter ago.
// The delete is already stored when the purge runs, so a purge failure is logged and not returned.
func (s *Service) DeleteHabit(ctx context.Context, id string) error {
	if err := domain.ValidateID(id); err != nil {
		return fmt.Errorf("delete habit: %w", err)
	}
	if err := s.repo.DeleteHabit(ctx, id, s.now()); err != nil {
		return fmt.Errorf("delete habit: %w", err)
	}
	if _, err := s.Purge(ctx); err != nil {
		s.log.Warn().Err(err).Msg("purge deleted habits")
	}
	return nil
}

// RestoreHabit brings a soft-deleted habit back at its original position.
func (s *Service) RestoreHabit(ctx context.Context, id string) error {
	if err := domain.ValidateID(id); err != nil {
		return fmt.Errorf("restore habit: %w", err)
	}
	if err := s.repo.RestoreHabit(ctx, id); err != nil {
		return fmt.Errorf("restore habit: %w", err)
	}
	return nil
}

// SetDay marks or unmarks one day of a habit. Repeating a call changes nothing.
func (s *Service) SetDay(ctx context.Context, id, day string, done bool) error {
	if err := domain.ValidateID(id); err != nil {
		return fmt.Errorf("set day: %w", err)
	}
	if err := domain.ValidateDay(day); err != nil {
		return fmt.Errorf("set day: %w", err)
	}
	if err := s.repo.SetDay(ctx, id, day, done); err != nil {
		return fmt.Errorf("set day: %w", err)
	}
	return nil
}

// UpdateSettings stores the view and the accent that are not nil. Both nil is domain.ErrEmptySettings.
func (s *Service) UpdateSettings(ctx context.Context, view, accent *string) error {
	u, err := domain.NewSettingsUpdate(view, accent)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	if err = s.repo.UpdateSettings(ctx, u); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// Purge hard-deletes habits that were soft-deleted before now minus PurgeAfter and returns how many it removed.
func (s *Service) Purge(ctx context.Context) (int64, error) {
	n, err := s.repo.PurgeDeleted(ctx, s.now().Add(-PurgeAfter))
	if err != nil {
		return 0, fmt.Errorf("purge: %w", err)
	}
	return n, nil
}

// Ping reports whether the storage answers.
func (s *Service) Ping(ctx context.Context) error {
	if err := s.repo.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}
