package usecase

import (
	"context"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
)

// SessionLifetime is the absolute session validity interval, without renewal.
const SessionLifetime = 30 * 24 * time.Hour

// AccountRepository persists identities, legacy ownership and opaque session digests.
type AccountRepository interface {
	CreateAccount(context.Context, domain.Account) error
	BootstrapOwner(context.Context, domain.Account) (bool, error)
	AccountByUsername(context.Context, string) (domain.Account, error)
	ResetPassword(context.Context, string, string) error
	InsertSession(context.Context, string, string, string, time.Time) error
	SessionUser(context.Context, string, time.Time) (domain.Account, error)
	RevokeSession(context.Context, string) error
}

// PasswordHasher encodes passwords and verifies supported password records.
type PasswordHasher interface {
	Hash([]byte) (string, error)
	Verify(string, []byte) bool
}
