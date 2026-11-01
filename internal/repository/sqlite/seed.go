package sqlite

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/repository/sqlite/sqlcdb"
)

// seedDays is how many days back, today included, the demo habits get history.
const seedDays = 365

// SeedOnce fills a new database with the demo habits and a year of random history and reports whether it did.
// The seeded marker outlives the habits, so deleting them all never brings the demo back.
func (s *Store) SeedOnce(ctx context.Context, now time.Time, rng *rand.Rand) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("seed: begin: %w", err)
	}
	defer rollback(tx)
	q := s.q.WithTx(tx)

	n, err := q.InsertSettingIfAbsent(ctx, sqlcdb.InsertSettingIfAbsentParams{Key: keySeeded, Value: "1"})
	if err != nil {
		return false, fmt.Errorf("seed: mark seeded: %w", err)
	}
	if n == 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("seed: commit: %w", err)
		}
		return false, nil
	}

	chances := []float64{0.8, 0.6, 0.45}
	for i, name := range domain.SeedNames() {
		id, err := newID()
		if err != nil {
			return false, fmt.Errorf("seed habit %q: %w", name, err)
		}
		if _, err := q.InsertHabit(ctx, sqlcdb.InsertHabitParams{ID: id, Name: name}); err != nil {
			return false, fmt.Errorf("seed habit %q: %w", name, err)
		}
		for d := range seedDays {
			if rng.Float64() < chances[i] {
				day := now.AddDate(0, 0, -d).Format(domain.DayLayout)
				if err := q.MarkDay(ctx, sqlcdb.MarkDayParams{HabitID: id, Day: day}); err != nil {
					return false, fmt.Errorf("seed habit %q day %s: %w", name, day, err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("seed: commit: %w", err)
	}
	return true, nil
}

// newID returns a random version 4 UUID in the lowercase form domain.ValidateID accepts.
func newID() (string, error) {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
