package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

// get serves GET path through the handler of a, as a browser on the loopback address would send it.
func get(t *testing.T, a *App, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Host = loopbackHost
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	return rec
}

// assertSeededHabits stops the test unless GET /api/state lists the three demo habits in the order of
// domain.SeedNames. step says which start of the app is checked.
func assertSeededHabits(t *testing.T, a *App, step string) {
	t.Helper()
	rec := get(t, a, "/api/state")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: GET /api/state status = %d, want %d (body %q)", step, rec.Code, http.StatusOK, rec.Body.String())
	}
	var state struct {
		Habits []struct {
			Name string `json:"name"`
		} `json:"habits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("%s: decode GET /api/state: %v", step, err)
	}
	if len(state.Habits) != 3 {
		t.Fatalf("%s: GET /api/state lists %d habits, want 3", step, len(state.Habits))
	}
	got := make([]string, 0, len(state.Habits))
	for _, h := range state.Habits {
		got = append(got, h.Name)
	}
	if want := domain.SeedNames(); !slices.Equal(got, want) {
		t.Errorf("%s: habit names = %q, want %q", step, got, want)
	}
}

func TestNewServesSeededApp(t *testing.T) {
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
			rec := get(t, first, tc.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", tc.path, rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("GET %s body does not contain %q", tc.path, tc.wantBody)
			}
		})
	}
	assertSeededHabits(t, first, "first start")

	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// The seeded marker lives in the database file, so a start on the same file must not add a second set.
	assertSeededHabits(t, startApp(t, cfg), "restart")
}
