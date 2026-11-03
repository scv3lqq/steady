package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
)

// Accounts manages operator-created accounts and durable opaque sessions.
type Accounts struct {
	repo   AccountRepository
	hasher PasswordHasher
	now    func() time.Time
	dummy  string
}

// NewAccounts prepares constant-work verification for unknown usernames.
func NewAccounts(repo AccountRepository, hasher PasswordHasher, now func() time.Time) (*Accounts, error) {
	dummy, err := hasher.Hash([]byte("steady dummy password verification"))
	if err != nil {
		return nil, fmt.Errorf("prepare authentication: %w", err)
	}
	return &Accounts{repo: repo, hasher: hasher, now: now, dummy: dummy}, nil
}
func (a *Accounts) account(username string, password []byte) (domain.Account, error) {
	if err := domain.ValidateUsername(username); err != nil {
		return domain.Account{}, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return domain.Account{}, err
	}
	hash, err := a.hasher.Hash(password)
	if err != nil {
		return domain.Account{}, fmt.Errorf("hash password: %w", err)
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return domain.Account{}, fmt.Errorf("generate account id: %w", err)
	}
	return domain.Account{ID: hex.EncodeToString(id[:]), Username: username, PasswordHash: hash}, nil
}

// Create creates a private account without claiming legacy data.
func (a *Accounts) Create(ctx context.Context, username string, password []byte) error {
	account, err := a.account(username, password)
	if err != nil {
		return err
	}
	return a.repo.CreateAccount(ctx, account)
}

// Bootstrap atomically claims legacy data for the first explicit owner.
func (a *Accounts) Bootstrap(ctx context.Context, username string, password []byte) (bool, error) {
	account, err := a.account(username, password)
	if err != nil {
		return false, err
	}
	return a.repo.BootstrapOwner(ctx, account)
}

// ResetPassword changes the password and revokes all sessions for that user.
func (a *Accounts) ResetPassword(ctx context.Context, username string, password []byte) error {
	account, err := a.account(username, password)
	if err != nil {
		return err
	}
	return a.repo.ResetPassword(ctx, username, account.PasswordHash)
}

// Login verifies credentials and persists an absolute-expiry opaque session.
func (a *Accounts) Login(ctx context.Context, username string, password []byte) (string, time.Time, error) {
	if domain.ValidateUsername(username) != nil || domain.ValidatePassword(password) != nil {
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	account, err := a.repo.AccountByUsername(ctx, username)
	if errors.Is(err, domain.ErrAccountNotFound) {
		a.hasher.Verify(a.dummy, password)
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lookup account: %w", err)
	}
	if !a.hasher.Verify(account.PasswordHash, password) {
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("generate session: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random[:])
	expiry := a.now().Add(SessionLifetime)
	if err = a.repo.InsertSession(ctx, tokenDigest(token), account.ID, account.PasswordHash, expiry); err != nil {
		return "", time.Time{}, fmt.Errorf("insert session: %w", err)
	}
	return token, expiry, nil
}

// Current validates the raw token before querying its stored digest.
func (a *Accounts) Current(ctx context.Context, token string) (domain.Account, error) {
	if !validToken(token) {
		return domain.Account{}, domain.ErrUnauthenticated
	}
	return a.repo.SessionUser(ctx, tokenDigest(token), a.now())
}

// Logout revokes the current token and is idempotent for invalid tokens.
func (a *Accounts) Logout(ctx context.Context, token string) error {
	if !validToken(token) {
		return nil
	}
	return a.repo.RevokeSession(ctx, tokenDigest(token))
}
func validToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(decoded) == 32
}
func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
