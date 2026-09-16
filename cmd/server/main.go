// Command server is the whole of Escape the Matrix: it serves the API, the
// WebSocket, the frontend, and the background workers.
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
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/jobs"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/web"
)

func main() {
	if err := run(); err != nil {
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

	assets, err := web.Assets()
	if err != nil {
		return err
	}

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

	if err := jobs.Migrate(ctx, st.Pool(), log); err != nil {
		return err
	}

	hub := realtime.NewHub(st, log)
	go hub.Run(ctx)

	app := api.New(cfg, st, schemaVersion, log, api.Assets(assets), hub)

	if err := app.PublishContent(ctx); err != nil {
		return err
	}
	go app.WatchContent(ctx)

	queue, err := jobs.New(st, log, app.Rollover(), app, app)
	if err != nil {
		return err
	}
	app.Rollover().Through(queue)
	if err := queue.Start(ctx); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app,
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := queue.Stop(shutdownCtx); err != nil {
		log.Error("stop job queue", "error", err)
	}
	return srv.Shutdown(shutdownCtx)
}

// newLogger returns the structured logger for the environment.
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	opts.Level = slog.LevelDebug
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
