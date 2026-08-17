// Package api owns the HTTP surface: the one command route, the one WebSocket,
// the sign-in routes, and serving the embedded frontend.
//
// Commands are addressed by name over a single route, POST
// /api/commands/{name}, so idempotency, the single transaction and status codes
// are enforced in internal/command rather than per handler (§8). Auth is one
// middleware over that route for the same reason.
package api

import (
	"log/slog"
	"net/http"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/challenge"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/rollover"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/situation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/social"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// Server holds what the handlers need. Everything in here is fixed at boot and
// read-only while the process serves, so two instances in front of one database
// are interchangeable (§3.2) — the property idempotency depends on, since a
// retry may land on a different server than the delivery it duplicates.
type Server struct {
	cfg           config.Config
	schemaVersion int64
	store         *store.Store
	log           *slog.Logger
	commands      *command.Executor
	accounts      *account.Accounts
	characters    *character.Characters
	health        *health.Health
	routines      *routine.Routines
	// playbooks is rules for judgement, as routines are rules for the body
	// (§3.14). One system, one interface language.
	playbooks *playbook.Playbooks
	// factors is progression (§3.5). Karma is a reading of a body, a balance
	// and three hidden totals, so it is restated after any command that moves
	// one of those rather than carried by a single response.
	factors *factor.Factors
	// events is §4's backlog. Both things that release one are here — a walk
	// arriving somewhere, and a world snapshot being read — as are the two
	// commands that answer an offer.
	events *event.Events
	// challenges is §5's framework. The catalogue is published from here at
	// boot, and the surface that plays a run is served from here.
	challenges *challenge.Challenges
	// careers is §3.14's ladder. Three separate paths drive it: the commands
	// that take and leave a job, an interview that hires, and the rollover that
	// pays for a day.
	careers *career.Careers
	// situations is the work a delegated role does while the player is
	// elsewhere (§3.14): produced by the rollover, shown by the world snapshot,
	// answered by a command.
	situations *situation.Situations
	// conflicts is §3.15's engine: friction with a population, generated daily
	// from what stands between the player and each of them. Produced by the
	// rollover, shown by the world snapshot, settled by a command.
	conflicts *conflict.Conflicts
	// social is peer endorsement (§3.5) — the one surface on which a factor
	// moves because another player said something.
	social *social.Endorsements
	// rollovers advances a player's world at their own local midnight (§3.3).
	// Two request paths drive it, sign-in and the world snapshot; the scheduled
	// path runs out of internal/jobs.
	rollovers *rollover.Rollovers
	google    *oauth.Google
	// clk is the one clock this process reads (§3.1), so a test can decide what
	// day it is.
	clk simulation.Clock
	// realtime holds the sockets this instance happens to have. Every message
	// they carry comes out of Postgres at the moment it is sent (§8).
	realtime *realtime.Hub

	// handler is the wired router. Server serves through it rather than being
	// one, so the fields above stay reachable to this package's tests.
	handler http.Handler
}

// New wires the routes and returns the root handler. assets is the built
// frontend, already verified to contain an index.html; hub is the realtime fan-
// out, which the caller is expected to already be running.
func New(cfg config.Config, st *store.Store, schemaVersion int64, log *slog.Logger, assets http.Handler, hub *realtime.Hub) *Server {
	clk := clock.System{}
	// A walk offers where it arrived to the backlog, and the backlog asks what
	// the walker is holding. Each needs the other at construction time and
	// neither at run time, so the pointer is bound now and filled in below.
	walks := &encounters{}
	characters := character.New(clk, walks)
	accounts := account.New(st)
	bodies := health.New(clk, characters)
	// The playbook (§3.14). Its one argument is which environments a player
	// holds a rule set for, as an interface, so the module owning the rules
	// never learns which catalogue defines them.
	playbooks := playbook.New(populations{})
	// The four routines are the early game's main loop (§3.4) and are
	// configurable by everybody from the first day, so there is no gate here.
	routines := routine.New()
	// Progression (§3.5). Each argument is the narrowest interface answering
	// one question — biological age, balance, where the player is (for §3.15's
	// decay), and who else is playing — so the package composing Karma cannot
	// reach a sample, a purchase, a home or an email address.
	factors := factor.New(bodies, purse{characters: characters},
		population{characters: characters}, roster{accounts: accounts})
	// Peer endorsement (§3.5). Both arguments are one-way: crowd is who may be
	// named, grants is where the credit goes. grants returns an error rather
	// than the standing internal/factor hands back, so the module deciding who
	// gets endorsed cannot learn anything about them.
	endorsements := social.New(clk, crowd{accounts: accounts}, grants{factors: factors})
	// The event backlog (§4): what the player is holding, who is coming back to
	// this and how far behind they are, and when to surface anything at all.
	//
	// event.Attentive is §4's release policy — caps a long absence cannot
	// overrun, a repeatedly ignored family slowed but never silenced, the day's
	// last release spent on something useful, and an escalation exempt from all
	// of it. Nothing it decides crosses the wire; the offer is the
	// recommendation.
	backlog := event.New(clk, purse{characters: characters},
		bearings{accounts: accounts, factors: factors}, event.Attentive{})
	// The challenge framework (§5): where a run's authored deltas go, where
	// money it handed the player goes, and how the event that opened it gets
	// settled. The first two are separate because a hidden factor's credit is
	// internal/factor's and a balance is internal/character's, and both are
	// write-only, so authored content cannot branch on what it already did.
	challenges := challenge.New(clk, awards{factors: factors}, pocket{characters: characters}, backlog)
	// §3.14's ladder: where a day's wage goes, whether a player reaches a
	// rung's requirement, and where a day of the wrong job goes.
	//
	// payroll is an adapter rather than the character module because §3.4's
	// income is structural and must not spend the day's paid-action allowance.
	// capability returns a boolean rather than a shortfall, since three of the
	// five factors a rung asks about are never shown (§3.5). tolls records and
	// cannot read, so what a role quietly costs stays for the player to work
	// out.
	careers := career.New(
		clk,
		payroll{characters: characters},
		capability{factors: factors},
		tolls{factors: factors},
	)
	// The work a role does while nobody is sitting in it (§3.14). The playbook
	// is passed as itself, narrowed to four methods: a delegated role can be
	// answered by the player's rules and can never edit them.
	situations := situation.New(clk, holds{careers: careers}, playbooks,
		population{characters: characters}, awards{factors: factors})
	// §3.15's core progression: resolving friction with a population every day.
	// Who those people are is not an argument, because it is not state —
	// internal/environment is authored content with no database behind it.
	conflicts := conflict.New(clk, playbooks, population{characters: characters},
		awards{factors: factors}, presence{accounts: accounts})

	s := &Server{
		cfg:           cfg,
		schemaVersion: schemaVersion,
		store:         st,
		log:           log,
		accounts:      accounts,
		characters:    characters,
		health:        bodies,
		routines:      routines,
		playbooks:     playbooks,
		factors:       factors,
		social:        endorsements,
		events:        backlog,
		challenges:    challenges,
		careers:       careers,
		situations:    situations,
		conflicts:     conflicts,
		clk:           clk,
		realtime:      hub,
		google: &oauth.Google{
			ClientID:     cfg.Google.ID,
			ClientSecret: cfg.Google.Secret,
			RedirectURL:  cfg.PublicURL + googleCallbackPath,
		},
	}

	// The day, composed (§3.3). work gathers the pieces each module owns —
	// Karma decay, the event backlog, the playbook's overnight writing — so a
	// ticket that fills one in edits that module rather than internal/rollover.
	//
	// The queue is not passed: its worker runs the rollover, so cmd/server ties
	// it on with Through once both exist. A server with no queue still catches
	// players up on request-driven paths and simply schedules nothing.
	s.rollovers = rollover.New(clk, st, log, accounts, characters, bodies, routines,
		employed{careers: careers},
		work{factors: factors, events: backlog, playbooks: playbooks,
			situations: situations, conflicts: conflicts, clk: clk}, s)

	// The pair from the top of this function, completed now that both exist.
	walks.events, walks.announce = backlog, s.offered

	reg := command.NewRegistry()
	s.registerCommands(reg)
	s.commands = command.NewExecutor(reg, st, log)

	mux := http.NewServeMux()

	// One command route, and the auth check is on it rather than in it.
	mux.Handle("POST /api/commands/{name}", s.authenticated(http.HandlerFunc(s.handleCommand)))

	// Everything the server decides on its own comes down here (§8). Same
	// middleware — a socket is a request too, and carries the same cookie.
	mux.Handle("GET "+socketPath, s.authenticated(http.HandlerFunc(s.handleSocket)))

	// What the client's world model is hydrated from (§10.2).
	mux.Handle("GET "+worldPath, s.authenticated(http.HandlerFunc(s.worldSnapshot)))

	// The reads below are GETs rather than commands because they write nothing
	// and are unbounded history, fetched a page at a time rather than carried
	// in the world snapshot (§10.2). All are per-player, so all are guarded.

	// The leaderboards (§3.5).
	mux.Handle("GET "+boardsPath, s.authenticated(http.HandlerFunc(s.boardIndex)))
	mux.Handle("GET "+boardPath, s.authenticated(http.HandlerFunc(s.boardPage)))

	// What the playbook did while the player was elsewhere (§3.14), and what a
	// week of it adds up to (§3.6).
	mux.Handle("GET "+logPath, s.authenticated(http.HandlerFunc(s.playbookLog)))

	// What the world has put in front of this player before, and what they did
	// about it (§4). It deliberately cannot answer what is still queued: a
	// client that could read the backlog could walk towards it, and §4's
	// sandbox depends on not being a quest list.
	mux.Handle("GET "+eventLogPath, s.authenticated(http.HandlerFunc(s.eventLog)))

	// Everybody this player has ever had anything with, and what (§3.15).
	mux.Handle("GET "+phonePath, s.authenticated(http.HandlerFunc(s.phone)))

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

// requestLogger records one structured line per request.
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

// Unwrap hands http.ResponseController the writer underneath. Without it this
// wrapper hides the Hijacker the WebSocket upgrade needs, and the upgrade fails
// as a 501.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
