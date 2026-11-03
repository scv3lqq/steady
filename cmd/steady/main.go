// Command steady serves the Steady habit tracker over HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/app"
	"github.com/scv3lqq/steady/internal/config"
)

// shutdownTimeout is how long the requests in flight get to finish after a stop signal.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "steady:", err)
		os.Exit(1)
	}
}

// run serves until SIGINT or SIGTERM and returns after a graceful stop. It is separate from main so that its defers
// run before os.Exit.
func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(os.Args) > 1 {
		if os.Args[1] != "account" {
			return fmt.Errorf("unknown command")
		}
		return runAccount(context.Background(), os.Args[2:], cfg, terminalPasswordReader{input: os.Stdin, output: os.Stderr}, os.Stdout)
	}
	log := zerolog.New(os.Stderr).Level(cfg.LogLevel).With().Timestamp().Logger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("start app: %w", err)
	}
	serveErr := serve(ctx, a.Handler(), cfg.Addr, log)
	// The database is closed only after serve has drained the requests that use it.
	return errors.Join(serveErr, a.Close())
}

// serve listens on addr and serves handler until ctx is cancelled, then shuts the server down gracefully.
func serve(ctx context.Context, handler http.Handler, addr string, log zerolog.Logger) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Info().Str("addr", ln.Addr().String()).Msg("listening")
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	log.Info().Msg("shutting down")
	// ctx is already cancelled by the signal, so the drain needs a context of its own.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
