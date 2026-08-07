// Package api owns the HTTP surface: the one command route, the one WebSocket,
// the sign-in routes, and serving the embedded frontend.
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
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/rollover"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/social"
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
	characters    *character.Characters
	health        *health.Health
	routines      *routine.Routines
	// playbooks is §3.14's other half: routines are rules for the body, and
	// these are rules for judgement. One system, one interface language — which
	// is why they sit beside each other here rather than one being a special
	// case of the other.
	playbooks *playbook.Playbooks
	// factors is progression (§3.5): the composite the player sees and the
	// three factors they never do. It is reachable from here because almost
	// every command moves it — Karma is a reading of a body, a balance and
	// three hidden totals, so it is restated after the commands that change any
	// of those rather than carried by any one of their responses.
	factors *factor.Factors
	// social is peer endorsement (§3.5): the one surface in V1 on which a
	// factor moves because another human being said something about this
	// player. It is reachable from here because both halves of it are — the
	// commands that answer an offer, and the world snapshot that is the only
	// thing which ever draws one.
	social *social.Endorsements
	// rollovers advances a player's world at their own local midnight (§3.3).
	// It is reachable from here because two request paths drive it — sign-in
	// and the world snapshot, both catch-ups — while the scheduled path runs
	// out of internal/jobs and never touches this router.
	rollovers *rollover.Rollovers
	google    *oauth.Google
	// clk is the one clock this process reads (§3.1). Handlers and modules take
	// it rather than calling time.Now, so a test can decide what day it is.
	clk simulation.Clock
	// realtime is the push side (§8). It is per-process rather than per-player
	// state: it holds the sockets this instance happens to have, and every
	// message they carry comes out of Postgres at the moment it is sent.
	realtime *realtime.Hub

	// handler is the wired router. Server serves through it rather than being
	// one, so that everything above stays reachable to this package's tests.
	handler http.Handler
}

// New wires the routes and returns the root handler. assets is the built
// frontend, already verified to contain an index.html; hub is the realtime fan-
// out, which the caller is expected to already be running.
func New(cfg config.Config, st *store.Store, schemaVersion int64, log *slog.Logger, assets http.Handler, hub *realtime.Hub) *Server {
	clk := clock.System{}
	// character.Quiet is §3.12's other half: a walk is the encounter surface,
	// and the thing that decides what happens on one is the event backlog (#21)
	// and its release policy (#22). Neither exists, so the walk is offered to
	// nobody — and replacing that is this argument, the same way #32 replaces
	// routine.Open below.
	characters := character.New(clk, character.Quiet{})
	accounts := account.New(st)
	bodies := health.New(clk, characters)
	// Every routine is configurable from the start, because the thing that is
	// meant to unlock them — a diagnosis the player arrived at (§3.6) — is #32.
	// Replacing routine.Open with the real gate is this line and nothing else,
	// which is why the gate is an interface at all.
	routines := routine.New(routine.Open{})
	// The playbook (§3.14). Its one argument is which environments a player
	// holds a rule set for — the districts today, populations once #29 lands —
	// and it is an interface for exactly that reason. What the player may
	// compose from is authored content this package never sees, and what
	// unlocks the rest of it is struggle-gating (§3.6) calling Unlock from
	// whichever module owns the experience.
	playbooks := playbook.New(districts{})
	// Progression (§3.5). The four arguments are the four things it is allowed
	// to know: the body it reads a biological age from, the balance it reads the
	// money factor from, where the player is, for §3.15's decay, and — for the
	// leaderboards — who else is playing and what they are called. None of them
	// is a module: each is the narrowest interface that answers one question, so
	// the package that composes Karma cannot reach a sample, a purchase, a home,
	// or an email address.
	factors := factor.New(bodies, purse{characters: characters},
		district{characters: characters}, roster{accounts: accounts})
	// Peer endorsement (§3.5). Two arguments, and both of them are one-way:
	// `crowd` is who may be named and what they are called, `grants` is where
	// the credit goes — and `grants` returns an error rather than the standing
	// internal/factor actually hands back, so the module that decides who gets
	// endorsed is structurally unable to learn anything about the person who
	// was. It is the one place a factor moves because of another human being,
	// and the narrow pair is what keeps that from becoming a peephole.
	endorsements := social.New(clk, crowd{accounts: accounts}, grants{factors: factors})

	s := &Server{
		cfg:           cfg,
		schemaVersion: schemaVersion,
		store:         st,
		log:           log,
		accounts:      accounts,
		characters:    characters,
		// The health module spends through the Wallet interface rather than
		// through the character module itself (§3.1): it is handed one here and
		// has no way to reach anything else the character owns.
		health:    bodies,
		routines:  routines,
		playbooks: playbooks,
		factors:   factors,
		social:    endorsements,
		clk:       clk,
		realtime:  hub,
		google: &oauth.Google{
			ClientID:     cfg.Google.ID,
			ClientSecret: cfg.Google.Secret,
			RedirectURL:  cfg.PublicURL + googleCallbackPath,
		},
	}

	// The day, composed (§3.3). It is built here rather than by the caller
	// because it needs the same four modules the handlers do, and there is no
	// version of this process in which the two should be different instances.
	//
	// `work` is §3.3's other three pieces, composed out of the modules that
	// exist: Karma decay is internal/factor's and is wired, while the event
	// backlog (#21) and environment ceilings (#29) are still stubs on it. The
	// composition lives in rollover.go rather than in internal/rollover, so a
	// ticket that fills one in replaces a method there and edits neither this
	// line nor that package.
	//
	// The queue is not passed. It cannot be: the queue's worker runs the
	// rollover, so cmd/server ties it on with Through once both exist. A server
	// whose queue was never tied still catches players up on the paths a
	// request drives; it simply schedules nothing, which is exactly right for a
	// process with no workers in it.
	s.rollovers = rollover.New(clk, st, log, accounts, characters, bodies, routines, work{factors: factors}, s)

	reg := command.NewRegistry()
	s.registerCommands(reg)
	s.commands = command.NewExecutor(reg, st, log)

	mux := http.NewServeMux()

	// One command route, and the auth check is on it rather than in it.
	mux.Handle("POST /api/commands/{name}", s.authenticated(http.HandlerFunc(s.handleCommand)))

	// The other half of the protocol (§8): commands go up over REST, and
	// everything the server decides on its own comes down here. Same middleware
	// — a socket is a request too, and it carries the same cookie.
	mux.Handle("GET "+socketPath, s.authenticated(http.HandlerFunc(s.handleSocket)))

	// What the client's world model is hydrated from before either of those
	// matters (§10.2). Behind the same guard, for the same reason: the answer
	// is per-player.
	mux.Handle("GET "+worldPath, s.authenticated(http.HandlerFunc(s.worldSnapshot)))

	// The leaderboards (§3.5). GETs rather than commands because they write
	// nothing, and separate from the world snapshot because they are the one
	// genuinely pull-shaped surface in this client (§10.2): a read-only
	// paginated list of other people, fetched when somebody looks at it. Behind
	// the same guard — a board says where *you* stand, so it is per-player even
	// though most of what it contains is not.
	mux.Handle("GET "+boardsPath, s.authenticated(http.HandlerFunc(s.boardIndex)))
	mux.Handle("GET "+boardPath, s.authenticated(http.HandlerFunc(s.boardPage)))

	// What the playbook did while the player was elsewhere (§3.14), and what a
	// week of it adds up to (§3.6). A GET for the same two reasons the boards
	// are: it writes nothing, and it is history — unbounded, so it is fetched a
	// page at a time rather than carried in the world snapshot.
	mux.Handle("GET "+logPath, s.authenticated(http.HandlerFunc(s.playbookLog)))

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

// Unwrap hands http.ResponseController the writer underneath.
//
// Without it this wrapper hides every optional interface the real writer
// implements — including the Hijacker the WebSocket upgrade needs, which fails
// as a 501 on a route that has nothing wrong with it. One line here rather than
// leaving the socket outside the access log.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
