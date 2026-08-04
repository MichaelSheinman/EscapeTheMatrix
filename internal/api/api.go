// Package api owns the HTTP surface: the one command route, the sign-in routes,
// and serving the embedded frontend.
//
// The shape follows technical.md §8 — commands over REST, addressed by name.
// There is exactly one command route, `POST /api/commands/{name}`, because
// idempotency, the single transaction, and status codes as protocol are things
// no handler should be able to opt out of by being registered somewhere else.
// internal/command owns those; this package is the transport in front of them.
//
// The same argument is why auth is one middleware over that route rather than a
// check inside each handler (§8): with one door, the guard can only be on it.
package api

import (
	"log/slog"
	"net/http"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
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
	accounts      *account.Accounts
	google        *oauth.Google

	// handler is the wired router. Server serves through it rather than being
	// one, so that everything above stays reachable to this package's tests.
	handler http.Handler
}

// New wires the routes and returns the root handler. assets is the built
// frontend, already verified to contain an index.html.
func New(cfg config.Config, st *store.Store, schemaVersion int64, log *slog.Logger, assets http.Handler) *Server {
	s := &Server{
		cfg:           cfg,
		schemaVersion: schemaVersion,
		store:         st,
		log:           log,
		accounts:      account.New(st),
		google: &oauth.Google{
			ClientID:     cfg.Google.ID,
			ClientSecret: cfg.Google.Secret,
			RedirectURL:  cfg.PublicURL + googleCallbackPath,
		},
	}

	reg := command.NewRegistry()
	s.registerCommands(reg)
	s.commands = command.NewExecutor(reg, st, log)

	mux := http.NewServeMux()

	// One command route, and the auth check is on it rather than in it.
	mux.Handle("POST /api/commands/{name}", s.authenticated(http.HandlerFunc(s.handleCommand)))

	// The sign-in routes are the only ones outside that guard, because they are
	// how a request comes to have a session at all.
	mux.HandleFunc("GET "+googleStartPath, s.startGoogleSignIn)
	mux.HandleFunc("GET "+googleCallbackPath, s.completeGoogleSignIn)
	mux.HandleFunc("GET "+sessionPath, s.currentSession)
	mux.HandleFunc("POST "+signOutPath, s.signOut)

	mux.Handle("/", assets)

	s.handler = requestLogger(log, mux)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

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
