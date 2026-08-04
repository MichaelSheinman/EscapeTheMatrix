// Package api owns the HTTP surface: the one command route, and serving the
// embedded frontend.
//
// The shape follows technical.md §8 — commands over REST, addressed by name.
// There is exactly one command route, `POST /api/commands/{name}`, because
// idempotency, the single transaction, and status codes as protocol are things
// no handler should be able to opt out of by being registered somewhere else.
// internal/command owns those; this package is the transport in front of them.
package api

import (
	"log/slog"
	"net/http"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// Server holds what the handlers need. It carries no per-player state: §3.2
// requires servers to be stateless so rolling deploys drop nothing.
//
// Everything in here is fixed at boot and read-only while the process serves,
// so two instances of this binary in front of one database are interchangeable
// — which is the property the idempotency record depends on, since a retry may
// land on a different server than the delivery it duplicates.
type Server struct {
	cfg config.Config
	// schemaVersion is fixed at boot: migrations run before the server starts
	// and never again in the life of the process.
	schemaVersion int64
	store         *store.Store
	log           *slog.Logger
	commands      *command.Executor
}

// New wires the routes and returns the root handler. assets is the built
// frontend, already verified to contain an index.html.
func New(cfg config.Config, st *store.Store, schemaVersion int64, log *slog.Logger, assets http.Handler) http.Handler {
	s := &Server{cfg: cfg, schemaVersion: schemaVersion, store: st, log: log}

	reg := command.NewRegistry()
	s.registerCommands(reg)
	s.commands = command.NewExecutor(reg, st, log)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/commands/{name}", s.handleCommand)
	mux.Handle("/", assets)

	return requestLogger(log, mux)
}

// requestLogger records one line per request. Structured throughout, so the
// production JSON handler (§13) emits fields rather than a formatted sentence.
func requestLogger(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
		)
	})
}

// statusRecorder captures the status code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
