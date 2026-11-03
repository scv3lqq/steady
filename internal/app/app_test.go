package app

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/config"
	"github.com/scv3lqq/steady/internal/domain"
)

// loopbackHost is the listen address of the test config and the Host a browser on this machine sends to it.
const loopbackHost = "127.0.0.1:8770"

// startApp opens an App on cfg and closes it when the test ends. A second Close is a no-op, so a test may close
// the App itself before that.
func startApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	a, err := New(t.Context(), cfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return a
}

// get serves GET path through the handler of a, as a browser that reached the app under host would send it.
func get(t *testing.T, a *App, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	return rec
}

// assertEmptyAccount verifies that startup creates no demo habits and private routes require a session.
func assertEmptyAccount(t *testing.T, a *App, step string) {
	t.Helper()
	if rec := get(t, a, loopbackHost, "/api/state"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("%s: unauthenticated state status %d", step, rec.Code)
	}
	owner, err := a.store.AccountByUsername(t.Context(), "alice")
	if errors.Is(err, domain.ErrAccountNotFound) {
		owner = domain.Account{ID: strings.Repeat("a", 32), Username: "alice", PasswordHash: "test:hash"}
		err = a.store.CreateAccount(t.Context(), owner)
	}
	if err != nil {
		t.Fatal(err)
	}
	state, err := a.store.State(t.Context(), owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Habits) != 0 || string(state.View) != "week" || state.Accent != "#4FA872" {
		t.Fatal("startup seeded demo data or changed account defaults")
	}
}

func TestNewServesPublicAssetsWithoutSeeding(t *testing.T) {
	cfg := config.Config{
		Addr:     loopbackHost,
		DBPath:   filepath.Join(t.TempDir(), "steady.db"),
		LogLevel: zerolog.Disabled,
	}
	first := startApp(t, cfg)

	// The files come from the real embedded web.FS, so a script left out of the embed answers 404 here.
	pages := []struct {
		name     string
		path     string
		wantBody string // a substring of the body; empty accepts any body
	}{
		{name: "healthz", path: "/healthz"},
		{name: "index", path: "/", wantBody: `<script type="module" src="/dist/main.js"></script>`},
		{name: "main script", path: "/dist/main.js"},
		{name: "render script", path: "/dist/render.js"},
		{name: "store script", path: "/dist/store.js"},
		{name: "dates script", path: "/dist/dates.js"},
	}
	for _, tc := range pages {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, first, loopbackHost, tc.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", tc.path, rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("GET %s body does not contain %q", tc.path, tc.wantBody)
			}
		})
	}
	assertEmptyAccount(t, first, "first start")

	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertEmptyAccount(t, startApp(t, cfg), "restart")
}

// The wiring is where the config meets the Host allowlist, so a dropped option shows up here and not in the
// handler tests.
func TestHostAllowlistFromConfig(t *testing.T) {
	cfg := config.Config{
		Addr:       loopbackHost,
		DBPath:     filepath.Join(t.TempDir(), "steady.db"),
		LogLevel:   zerolog.Disabled,
		ExtraHosts: []string{"steady.test"},
	}
	a := startApp(t, cfg)

	tests := []struct {
		name string
		host string
		want int
	}{
		{name: "host from the config", host: "steady.test", want: http.StatusOK},
		{name: "host not in the config", host: "other.test", want: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := get(t, a, tc.host, "/healthz"); rec.Code != tc.want {
				t.Errorf("GET /healthz with Host %s: status = %d, want %d", tc.host, rec.Code, tc.want)
			}
		})
	}
}

func TestStartupPreservesLegacyWithoutClaimingOrPurging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	provider, e := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("../repository/sqlite/migrations"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = provider.UpTo(t.Context(), 1); e != nil {
		t.Fatal(e)
	}
	fixture := `
 INSERT INTO habits(id,name,position,deleted_at) VALUES('00000000-0000-4000-8000-000000000001','Legacy private',7,1);
 INSERT INTO habit_days(habit_id,day) VALUES('00000000-0000-4000-8000-000000000001','2025-01-01');
 INSERT INTO settings(key,value) VALUES('view','year'),('unknown','preserve me');`
	if _, e = db.ExecContext(t.Context(), fixture); e != nil {
		t.Fatal(e)
	}
	_ = db.Close()
	cfg := config.Config{Addr: loopbackHost, DBPath: path}
	a := startApp(t, cfg)
	if e = a.Close(); e != nil {
		t.Fatal(e)
	}
	a = startApp(t, cfg)
	db, e = sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	var name, day, value string
	var position, deleted int
	if e = db.QueryRowContext(t.Context(), `SELECT name,position,deleted_at FROM habits WHERE user_id IS NULL`).Scan(&name, &position, &deleted); e != nil || name != "Legacy private" || position != 7 || deleted != 1 {
		t.Fatal("startup changed or purged legacy habit")
	}
	if e = db.QueryRowContext(t.Context(), `SELECT day FROM habit_days`).Scan(&day); e != nil || day != "2025-01-01" {
		t.Fatal("startup changed legacy days")
	}
	if e = db.QueryRowContext(t.Context(), `SELECT value FROM legacy_settings WHERE key='unknown'`).Scan(&value); e != nil || value != "preserve me" {
		t.Fatal("startup changed legacy settings")
	}
	var count int
	if e = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM users`).Scan(&count); e != nil || count != 0 {
		t.Fatal("startup created owner")
	}
	assertEmptyAccount(t, a, "legacy startup")
}
