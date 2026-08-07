// Package store owns the pgx pool, the migration run at startup, and the
// transaction helper every command is expected to go through.
//
// It is the only package that knows a connection string exists. Domain modules
// (§3.1) take a *Store and speak SQL through it; they never open their own
// connections.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/MichaelSheinman/EscapeTheMatrix/migrations"
)

// gooseGlobals guards the migration runner's package-level state. See Migrate.
var gooseGlobals sync.Mutex

// Store is a handle on Postgres. Postgres is truth (§3.2); there is no second
// datastore and no cache in front of this.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects, verifies the connection, and returns a Store. The caller
// closes it.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure pool: %w", err)
	}
	// Fail at boot rather than on the first request. A server that starts
	// happily without a database is a server that reports green while every
	// command 500s.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases every pooled connection.
func (s *Store) Close() { s.pool.Close() }

// Pool exposes the underlying pool for reads that do not need a transaction.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// InTx runs fn inside a single transaction, committing when it returns nil and
// rolling back on error or panic.
//
// §3.2: every player command is handled inside one Postgres transaction. This
// is the function that promise is kept by — a command that reaches into the
// pool directly has escaped it.
func (s *Store) InTx(ctx context.Context, fn func(pgx.Tx) error) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			// Rollback failure after a real error is noise; the original
			// error is the one worth surfacing.
			_ = tx.Rollback(ctx)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Migrate applies every embedded migration that has not run yet, forward only,
// and returns the schema version now in force.
//
// Migrations run once, at boot, so that returned version is good for the life
// of the process — callers that want to report it should hold it rather than
// re-reading goose's bookkeeping table.
//
// It takes the connection string rather than the pool because goose speaks
// database/sql. That shim is confined to this function and lives for the few
// milliseconds it takes to run; the application never touches database/sql.
func Migrate(ctx context.Context, databaseURL string, log *slog.Logger) (int64, error) {
	// goose keeps its filesystem, dialect, and logger in package-level
	// variables, so two concurrent migrations would write them underneath each
	// other. A process migrates once, at boot, so serialising costs nothing —
	// except in tests, where every database is created in parallel and this is
	// the difference between `go test -race` being usable and being noise.
	gooseGlobals.Lock()
	defer gooseGlobals.Unlock()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return 0, fmt.Errorf("set dialect: %w", err)
	}

	pgxCfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return 0, fmt.Errorf("parse database url: %w", err)
	}
	db := stdlib.OpenDB(*pgxCfg)
	defer db.Close()

	before, err := currentVersion(ctx, db)
	if err != nil {
		return 0, err
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	after, err := currentVersion(ctx, db)
	if err != nil {
		return 0, err
	}

	if after == before {
		log.Info("schema up to date", "version", after)
	} else {
		log.Info("schema migrated", "from", before, "to", after)
	}
	return after, nil
}

// currentVersion reports the applied schema version, treating a not-yet-created
// goose table as version 0 rather than an error.
func currentVersion(ctx context.Context, db *sql.DB) (int64, error) {
	v, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "42P01" { // undefined_table
			return 0, nil
		}
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
}
