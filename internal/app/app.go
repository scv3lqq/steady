// Package app wires the SQLite store, the use case service and the HTTP handler into one application.
package app

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/config"
	"github.com/scv3lqq/steady/internal/handler/httpapi"
	"github.com/scv3lqq/steady/internal/repository/sqlite"
	"github.com/scv3lqq/steady/internal/usecase"
	"github.com/scv3lqq/steady/web"
)

// The wiring is the one place that knows both sides of the port, so the compile-time check that the adapter
// satisfies it lives here.
var _ usecase.Repository = (*sqlite.Store)(nil)

// App is the wired application: the HTTP handler and the store behind it.
type App struct {
	handler http.Handler
	store   *sqlite.Store
}

// New opens the database, seeds the demo habits on a first run, purges the deleted habits that are past their undo
// window and builds the HTTP handler. The caller must Close the App.
func New(ctx context.Context, cfg config.Config, log zerolog.Logger) (*App, error) {
	store, err := sqlite.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	handler, err := wire(ctx, store, cfg, log)
	if err != nil {
		// The wiring error is the one worth reporting; a failed close would only hide it.
		_ = store.Close()
		return nil, err
	}
	return &App{handler: handler, store: store}, nil
}

// wire prepares the data in store and puts the HTTP handler on top of it.
func wire(ctx context.Context, store *sqlite.Store, cfg config.Config, log zerolog.Logger) (http.Handler, error) {
	// The demo history is drawn from a generator seeded by the OS, so no two installs share one.
	var seed [32]byte
	if _, err := crand.Read(seed[:]); err != nil {
		return nil, fmt.Errorf("read random seed: %w", err)
	}
	rng := rand.New(rand.NewChaCha8(seed)) //nolint:gosec // demo data only, nothing secret is drawn from it
	seeded, err := store.SeedOnce(ctx, time.Now(), rng)
	if err != nil {
		return nil, fmt.Errorf("seed demo habits: %w", err)
	}
	if seeded {
		log.Info().Msg("seeded demo habits")
	}

	svc := usecase.New(store, time.Now, log)
	purged, err := svc.Purge(ctx)
	if err != nil {
		return nil, fmt.Errorf("purge deleted habits: %w", err)
	}
	if purged > 0 {
		log.Info().Int64("count", purged).Msg("purged deleted habits")
	}

	handler, err := httpapi.New(svc, web.FS, httpapi.Options{AllowedHosts: cfg.AllowedHosts()}, log)
	if err != nil {
		return nil, fmt.Errorf("build handler: %w", err)
	}
	return handler, nil
}

// Handler returns the HTTP handler of the application.
func (a *App) Handler() http.Handler {
	return a.handler
}

// Close closes the database. Stop serving first: a request in flight still needs it.
func (a *App) Close() error {
	if err := a.store.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	return nil
}
