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
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/jobs"
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

	// The queue's own schema, next to the game's rather than inside it (§3.4).
	// River moves on its own release schedule, so it keeps its own migration
	// ledger and `migrations/` stays the game's history alone.
	if err := jobs.Migrate(ctx, st.Pool(), log); err != nil {
		return err
	}

	// The push side (§8). It holds one Postgres LISTEN for the whole process,
	// so it starts before anything can connect to a socket — and it stops on
	// the same context, which is what closes those sockets: a hijacked
	// connection is invisible to http.Server.Shutdown, so nothing else would.
	hub := realtime.NewHub(st, log)
	go hub.Run(ctx)

	app := api.New(cfg, st, schemaVersion, log, api.Assets(assets), hub)

	// The background workers, in this binary (§3.4). The knot is tied here and
	// only here: the queue's worker runs the rollover, and the rollover needs
	// the queue to schedule its successor, so the queue is built second and
	// handed back. Both happen before the listener opens, which keeps
	// everything the request path touches read-only while the process serves.
	//
	// This is also the whole of §3.3's scheduling. There is no ticker and no
	// loop over players: each rollover arms the next one for the player it ran
	// for, and only while that player is still playing.
	// The second worker is §3.5's leaderboard rebuild, which needs nothing tied
	// back on: it is a global sort on a timer, so the queue drives it and it
	// drives nothing.
	queue, err := jobs.New(st, log, app.Rollover(), app)
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

	// In-flight requests are each one transaction (§3.2), so a short grace
	// period is enough to let them commit rather than roll back.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// The workers stop first, and a rollover that does not finish in time is
	// not lost: it is still a row, and the next process to start picks it up.
	// That durability is the entire argument for a queue over a goroutine
	// (§3.4), and this is where it is relied on.
	if err := queue.Stop(shutdownCtx); err != nil {
		log.Error("stop job queue", "error", err)
	}
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
