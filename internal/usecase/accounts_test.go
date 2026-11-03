package usecase_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/repository/sqlite"
	"github.com/scv3lqq/steady/internal/usecase"
)

type accountTestHasher struct {
	verifies     int
	beforeVerify func()
}

func (h *accountTestHasher) Hash(p []byte) (string, error) { return "test:" + string(p), nil }
func (h *accountTestHasher) Verify(hash string, p []byte) bool {
	h.verifies++
	if h.beforeVerify != nil {
		h.beforeVerify()
	}
	return hash == "test:"+string(p)
}
func accountTestNow() time.Time { return time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC) }
func accountTestStore(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	s, e := sqlite.Open(t.Context(), path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func accountTestService(t *testing.T, s *sqlite.Store, h usecase.PasswordHasher, now func() time.Time) *usecase.Accounts {
	t.Helper()
	a, e := usecase.NewAccounts(s, h, now)
	if e != nil {
		t.Fatal(e)
	}
	return a
}

func TestAccountSessionLifecycleAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := accountTestStore(t, path)
	now := accountTestNow()
	h := &accountTestHasher{}
	a := accountTestService(t, s, h, func() time.Time { return now })
	if e := a.Create(t.Context(), "alice", []byte("alice password")); e != nil {
		t.Fatal(e)
	}
	if e := a.Create(t.Context(), "bob", []byte("bobby password")); e != nil {
		t.Fatal(e)
	}
	if e := a.Create(t.Context(), "alice", []byte("another password")); !errors.Is(e, domain.ErrUsernameExists) {
		t.Fatalf("duplicate: %v", e)
	}
	for _, u := range []string{"alice", "unknown"} {
		before := h.verifies
		_, _, e := a.Login(t.Context(), u, []byte("wrong password"))
		if !errors.Is(e, domain.ErrInvalidCredentials) || h.verifies != before+1 {
			t.Fatalf("generic login and dummy work: %v", e)
		}
	}
	token, expiry, e := a.Login(t.Context(), "alice", []byte("alice password"))
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := base64.RawURLEncoding.DecodeString(token)
	if e != nil || len(decoded) != 32 || expiry != now.Add(30*24*time.Hour) {
		t.Fatal("token entropy or expiry")
	}
	u, e := a.Current(t.Context(), token)
	if e != nil || u.Username != "alice" {
		t.Fatalf("current: %v", e)
	}
	digest := sha256.Sum256([]byte(token))
	if _, e = s.SessionUser(t.Context(), hex.EncodeToString(digest[:]), now); e != nil {
		t.Fatal("session digest not persisted")
	}
	if _, e = s.SessionUser(t.Context(), token, now); !errors.Is(e, domain.ErrUnauthenticated) {
		t.Fatal("raw token persisted")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = accountTestStore(t, path)
	a = accountTestService(t, s, h, func() time.Time { return now })
	if _, e = a.Current(t.Context(), token); e != nil {
		t.Fatal("session not durable")
	}
	for _, bad := range []string{"", "short", strings.Repeat("a", 42), strings.Repeat("!", 43), token + "="} {
		if _, e = a.Current(t.Context(), bad); !errors.Is(e, domain.ErrUnauthenticated) {
			t.Fatalf("malformed token: %v", e)
		}
	}
	now = expiry.Add(-time.Nanosecond)
	if _, e = a.Current(t.Context(), token); e != nil {
		t.Fatal("expired too early")
	}
	now = expiry
	if _, e = a.Current(t.Context(), token); !errors.Is(e, domain.ErrUnauthenticated) {
		t.Fatal("expiry is not absolute")
	}
	now = accountTestNow()
	token, _, e = a.Login(t.Context(), "alice", []byte("alice password"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Logout(t.Context(), token); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Current(t.Context(), token); !errors.Is(e, domain.ErrUnauthenticated) {
		t.Fatal("logout did not revoke")
	}
	if e = a.Logout(t.Context(), token); e != nil {
		t.Fatal("logout must be idempotent")
	}
}

func TestResetRevokesOnlyTargetAndRejectsInFlightOldPassword(t *testing.T) {
	s := accountTestStore(t, filepath.Join(t.TempDir(), "accounts.db"))
	h := &accountTestHasher{}
	a := accountTestService(t, s, h, accountTestNow)
	for _, u := range []string{"alice", "bobby"} {
		if e := a.Create(t.Context(), u, []byte("old password value")); e != nil {
			t.Fatal(e)
		}
	}
	alice, _, e := a.Login(t.Context(), "alice", []byte("old password value"))
	if e != nil {
		t.Fatal(e)
	}
	bob, _, e := a.Login(t.Context(), "bobby", []byte("old password value"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.ResetPassword(t.Context(), "alice", []byte("new password value")); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Current(t.Context(), alice); !errors.Is(e, domain.ErrUnauthenticated) {
		t.Fatal("reset left session")
	}
	if _, e = a.Current(t.Context(), bob); e != nil {
		t.Fatal("reset revoked another account")
	}
	h.beforeVerify = func() {
		h.beforeVerify = nil
		if e := a.ResetPassword(t.Context(), "bobby", []byte("next password value")); e != nil {
			t.Fatal(e)
		}
	}
	if _, _, e = a.Login(t.Context(), "bobby", []byte("old password value")); !errors.Is(e, domain.ErrInvalidCredentials) {
		t.Fatalf("stale hash session insertion: %v", e)
	}
}

func TestAccountCommandsValidateBeforeHashing(t *testing.T) {
	s := accountTestStore(t, filepath.Join(t.TempDir(), "accounts.db"))
	a := accountTestService(t, s, &accountTestHasher{}, accountTestNow)
	for _, call := range []func(context.Context, string, []byte) error{a.Create, a.ResetPassword} {
		if e := call(t.Context(), " Alice", []byte("valid password")); !errors.Is(e, domain.ErrInvalidUsername) {
			t.Fatalf("username: %v", e)
		}
		if e := call(t.Context(), "alice", []byte("short")); !errors.Is(e, domain.ErrInvalidPassword) {
			t.Fatalf("password: %v", e)
		}
	}
	done, e := a.Bootstrap(t.Context(), "alice", []byte("first password"))
	if e != nil || !done {
		t.Fatalf("bootstrap: %v", e)
	}
	done, e = a.Bootstrap(t.Context(), "alice", []byte("second password"))
	if e != nil || done {
		t.Fatalf("bootstrap retry: %v", e)
	}
	if _, _, e = a.Login(t.Context(), "alice", []byte("first password")); e != nil {
		t.Fatal("bootstrap retry reset password")
	}
}

func TestExpiredSessionsAreDeletedOnInsertion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := accountTestStore(t, path)
	now := accountTestNow()
	a := accountTestService(t, s, &accountTestHasher{}, func() time.Time { return now })
	if e := a.Create(t.Context(), "alice", []byte("alice password")); e != nil {
		t.Fatal(e)
	}
	token, _, e := a.Login(t.Context(), "alice", []byte("alice password"))
	if e != nil {
		t.Fatal(e)
	}
	now = now.Add(usecase.SessionLifetime)
	if _, _, e = a.Login(t.Context(), "alice", []byte("alice password")); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	digest := sha256.Sum256([]byte(token))
	var count int
	if e = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions WHERE token_hash = ?", hex.EncodeToString(digest[:])).Scan(&count); e != nil || count != 0 {
		t.Fatal("expired session retained")
	}
	if e = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions WHERE token_hash = ?", token).Scan(&count); e != nil || count != 0 {
		t.Fatal("raw token persisted")
	}
}

type sessionQueryCounter struct {
	usecase.AccountRepository
	queries int
}

func (r *sessionQueryCounter) SessionUser(ctx context.Context, hash string, now time.Time) (domain.Account, error) {
	r.queries++
	return r.AccountRepository.SessionUser(ctx, hash, now)
}
func TestMalformedSessionNeverQueriesRepository(t *testing.T) {
	s := accountTestStore(t, filepath.Join(t.TempDir(), "accounts.db"))
	r := &sessionQueryCounter{AccountRepository: s}
	a, e := usecase.NewAccounts(r, &accountTestHasher{}, accountTestNow)
	if e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{"", "short", strings.Repeat("a", 42), strings.Repeat("!", 43), strings.Repeat("a", 43) + "="} {
		if _, e = a.Current(t.Context(), token); !errors.Is(e, domain.ErrUnauthenticated) {
			t.Fatal("malformed token accepted")
		}
	}
	if r.queries != 0 {
		t.Fatal("malformed session reached repository")
	}
}
