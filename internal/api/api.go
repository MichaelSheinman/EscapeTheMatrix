// Package api owns the HTTP surface: the command endpoints, and serving the
// embedded frontend.
//
// The shape follows technical.md §8 — REST for commands, one ordinary
// http.Handler each. The generic command registry, idempotency keys, and status
// codes as protocol arrive with the command layer; this package currently holds
// the single command the walking skeleton needs, registered at the address that
// layer will generalise.
package api

import (
	"log/slog"
	"net/http"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// Server holds what the handlers need. It carries no per-player state: §3.2
// requires servers to be stateless so rolling deploys drop nothing.
type Server struct {
	cfg config.Config
	// schemaVersion is fixed at boot: migrations run before the server starts
	// and never again in the life of the process.
	schemaVersion int64
	store         *store.Store
	log           *slog.Logger
}

// New wires the routes and returns the root handler. assets is the built
// frontend, already verified to contain an index.html.
func New(cfg config.Config, st *store.Store, schemaVersion int64, log *slog.Logger, assets http.Handler) http.Handler {
	s := &Server{cfg: cfg, schemaVersion: schemaVersion, store: st, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/commands/probe", s.handleProbe)
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
