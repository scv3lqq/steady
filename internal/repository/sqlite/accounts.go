package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/repository/sqlite/sqlcdb"
	"github.com/scv3lqq/steady/internal/usecase"
)

// CreateAccount creates an empty account without claiming any legacy data.
func (s *Store) CreateAccount(ctx context.Context, account domain.Account) error {
	if err := createAccount(ctx, s.q, account); err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func createAccount(ctx context.Context, q *sqlcdb.Queries, account domain.Account) error {
	n, err := q.CreateAccount(ctx, sqlcdb.CreateAccountParams{ID: account.ID, Username: account.Username, PasswordHash: account.PasswordHash})
	if err != nil {
		return fmt.Errorf("insert account: %w", err)
	}
	if n == 0 {
		return domain.ErrUsernameExists
	}
	return nil
}

// BootstrapOwner transfers the entire legacy dataset exactly once, atomically.
func (s *Store) BootstrapOwner(ctx context.Context, account domain.Account) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("bootstrap: begin: %w", err)
	}
	defer rollback(tx)
	q := s.q.WithTx(tx)
	username, err := q.BootstrapUsername(ctx)
	if err == nil {
		if username == account.Username {
			return false, nil
		}
		return false, fmt.Errorf("bootstrap: %w", domain.ErrBootstrapComplete)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("bootstrap: read ledger: %w", err)
	}
	if err := createAccount(ctx, q, account); err != nil {
		return false, fmt.Errorf("bootstrap: %w", err)
	}
	if err := q.AssignLegacyHabits(ctx, ownerID(account.ID)); err != nil {
		return false, fmt.Errorf("bootstrap: assign habits: %w", err)
	}
	if err := q.CopyLegacySettings(ctx, account.ID); err != nil {
		return false, fmt.Errorf("bootstrap: copy settings: %w", err)
	}
	if err := q.ClearLegacySettings(ctx); err != nil {
		return false, fmt.Errorf("bootstrap: clear settings: %w", err)
	}
	if err := q.CompleteBootstrap(ctx, account.Username); err != nil {
		return false, fmt.Errorf("bootstrap: complete ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("bootstrap: commit: %w", err)
	}
	return true, nil
}

// AccountByUsername returns the private credential record for an exact username.
func (s *Store) AccountByUsername(ctx context.Context, username string) (domain.Account, error) {
	account, err := s.q.AccountByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, fmt.Errorf("account by username: %w", domain.ErrAccountNotFound)
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("account by username: %w", err)
	}
	return accountRecord(account), nil
}

func accountRecord(account sqlcdb.User) domain.Account {
	return domain.Account{ID: account.ID, Username: account.Username, PasswordHash: account.PasswordHash}
}

// ResetPassword replaces the credential and revokes every existing session in one transaction.
func (s *Store) ResetPassword(ctx context.Context, username, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reset password: begin: %w", err)
	}
	defer rollback(tx)
	q := s.q.WithTx(tx)
	account, err := q.AccountByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reset password: %w", domain.ErrAccountNotFound)
	}
	if err != nil {
		return fmt.Errorf("reset password: find account: %w", err)
	}
	if _, err := q.ResetPassword(ctx, sqlcdb.ResetPasswordParams{Username: username, PasswordHash: passwordHash}); err != nil {
		return fmt.Errorf("reset password: replace hash: %w", err)
	}
	if err := q.RevokeUserSessions(ctx, account.ID); err != nil {
		return fmt.Errorf("reset password: revoke sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reset password: commit: %w", err)
	}
	return nil
}

// InsertSession checks the verified credential again while holding the write lock.
// A reset that committed after verification therefore cannot issue a stale session.
func (s *Store) InsertSession(ctx context.Context, tokenHash, userID, expectedHash string, expiry time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("insert session: begin: %w", err)
	}
	defer rollback(tx)
	q := s.q.WithTx(tx)
	if err := q.DeleteExpiredSessions(ctx, expiry.Add(-usecase.SessionLifetime).UnixNano()); err != nil {
		return fmt.Errorf("insert session: cleanup: %w", err)
	}
	n, err := q.InsertSession(ctx, sqlcdb.InsertSessionParams{TokenHash: tokenHash, UserID: userID, ExpectedPasswordHash: expectedHash, ExpiresAt: expiry.UnixNano()})
	if err != nil {
		return fmt.Errorf("insert session: write: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("insert session: %w", domain.ErrInvalidCredentials)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("insert session: commit: %w", err)
	}
	return nil
}

// SessionUser validates absolute expiry without extending it.
func (s *Store) SessionUser(ctx context.Context, tokenHash string, now time.Time) (domain.Account, error) {
	account, err := s.q.SessionUser(ctx, sqlcdb.SessionUserParams{TokenHash: tokenHash, Now: now.UnixNano()})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, fmt.Errorf("session user: %w", domain.ErrUnauthenticated)
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("session user: %w", err)
	}
	return accountRecord(account), nil
}

// RevokeSession idempotently removes one opaque session digest.
func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	if err := s.q.RevokeSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
