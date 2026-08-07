// Command server is the whole of Escape the Matrix: it serves the API, the
// WebSocket, the frontend, and — from later tickets — the background workers.
// One binary, one datastore (technical.md §1).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/api"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/web"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet when configuration is what failed, so
		// the last-resort path is stderr.
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	// Checked before anything expensive: a binary built without the frontend
	// build must refuse to start, not serve a blank page.
	assets, err := web.Assets()
	if err != nil {
		return err
	}

	// Signals cancel the whole boot sequence, so Ctrl-C during a slow migration
	// stops rather than hanging.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	schemaVersion, err := store.Migrate(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	// The push side (§8). It holds one Postgres LISTEN for the whole process,
	// so it starts before anything can connect to a socket — and it stops on
	// the same context, which is what closes those sockets: a hijacked
	// connection is invisible to http.Server.Shutdown, so nothing else would.
	hub := realtime.NewHub(st, log)
	go hub.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.New(cfg, st, schemaVersion, log, api.Assets(assets), hub),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "env", string(cfg.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	// In-flight requests are each one transaction (§3.2), so a short grace
	// period is enough to let them commit rather than roll back.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// newLogger returns the structured logger for the environment: JSON in
// production so the log pipeline can index fields, human-readable in
// development so a person can read it (§13).
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	opts.Level = slog.LevelDebug
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
