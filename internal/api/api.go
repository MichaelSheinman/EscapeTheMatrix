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
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/challenge"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/diagnosis"
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
	// diagnoses is §3.6: what the player has worked out about their own
	// failures, and the only thing in this game that unlocks either of the two
	// above. It is reachable from here because both of the surfaces it moves
	// are — the routines it gates and the vocabulary it grants — and because
	// the scene itself is a read composed at request time out of history four
	// other modules own.
	diagnoses *diagnosis.Diagnoses
	// factors is progression (§3.5): the composite the player sees and the
	// three factors they never do. It is reachable from here because almost
	// every command moves it — Karma is a reading of a body, a balance and
	// three hidden totals, so it is restated after the commands that change any
	// of those rather than carried by any one of their responses.
	factors *factor.Factors
	// events is §4's backlog: everything the world has ready to happen to this
	// player, and the one thing of it that is in front of them now. It is
	// reachable from here because both of the things that ever release one are
	// — a walk arriving somewhere, and a world snapshot being read — and
	// because the two commands that answer an offer are.
	events *event.Events
	// challenges is §5's framework: the authored catalogue, and one player's
	// run through one of it. It is reachable from here because the catalogue is
	// published from here at boot — a definition with no catalog row is a
	// definition no run can reference — and because the surface that plays one
	// arrives with the first challenge there is to play (#24).
	challenges *challenge.Challenges
	// careers is §3.14's ladder: the job the player holds, what it pays, and
	// what it costs their body. It is reachable from here because three
	// separate paths drive it — the two commands that take and leave one, the
	// challenge surface that hires somebody out of an interview, and the
	// rollover that pays for a day — and none of those belongs to a module that
	// could own the others.
	careers *career.Careers
	// situations is §3.14's other half of a job: the work a delegated role does
	// while the player is elsewhere. It is reachable from here because three
	// separate paths drive it — the rollover that produces a day's moment, the
	// world snapshot that shows the one nobody's rules could settle, and the
	// command that answers it — and because the playbook's log joins its writing
	// to internal/playbook's records.
	situations *situation.Situations
	// conflicts is §3.15's engine: the friction of living among a population,
	// generated daily from what stands between the player and each of them. It
	// is reachable from here because three separate paths drive it — the
	// rollover that produces a day's conflict, the world snapshot that shows the
	// one nobody's rules could settle along with how the fortnight has gone, and
	// the command that settles it — and because the playbook's log joins its
	// writing to internal/playbook's records.
	conflicts *conflict.Conflicts
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
	// §3.12's other half: a walk is the encounter surface, and what decides
	// whether anything comes of one is §4's backlog. This is the argument #12
	// left as character.Quiet for exactly this ticket to replace.
	//
	// It is filled in below rather than here, because the two modules need each
	// other at construction time and neither needs the other at *run* time: a
	// walk offers where it arrived to the backlog, and the backlog asks what
	// the walker is holding. A pointer bound now and completed once both exist
	// breaks that knot without either module learning about the other.
	walks := &encounters{}
	characters := character.New(clk, walks)
	accounts := account.New(st)
	bodies := health.New(clk, characters)
	// The playbook (§3.14). Its one argument is which environments a player
	// holds a rule set for — the populations of §3.15, one set apiece — and it
	// is an interface so that the module owning the rules never has to know
	// which catalogue defines them. What the player may compose from is
	// authored content this package never sees, and what unlocks the rest of it
	// is struggle-gating (§3.6), which is the module below.
	playbooks := playbook.New(populations{})
	// Struggle-gated unlocking (§3.6): the player naming the cause of a failure,
	// and the naming being what grants the tool. Four arguments, and each is a
	// thing the module is deliberately unable to reach for itself.
	//
	// Where the player lives decides whose kitchen table this happens at — the
	// same one-question interface internal/factor reads a population through.
	// The playbook is passed as itself for the one method that hands a word
	// over, which is all the interface admits: a package that could read the
	// vocabulary back would be one edit from telling the player how much of it
	// is left, and §3.14 says a locked term is absent rather than hidden.
	//
	// The last two are the ledgers a diagnosis is argued from, in the order the
	// evidence reads: what the body has been doing, then what the player's own
	// answers made of the week. Each turns its own rows into readings, because
	// what a rising resting heart rate *means* is internal/health's and could
	// not be anywhere else without becoming a second copy of it.
	diagnoses := diagnosis.New(clk, population{characters: characters}, playbooks,
		bodyLedger{bodies: bodies}, logLedger{playbooks: playbooks, clk: clk})
	// A cause that speaks from evidence nothing produces is a tool no player
	// could ever be handed, and it fails silently — the absence looks exactly
	// like somebody who has not struggled enough yet. So the catalogue is
	// checked against the observation keys the ledgers above can actually emit,
	// here, because which ledgers are bound is a fact about this line rather
	// than about content.
	if err := diagnosis.Check(observable()); err != nil {
		panic(err)
	}
	// And the line #9 left. "Every routine is configurable from the start,
	// because the thing that is meant to unlock them — a diagnosis the player
	// arrived at (§3.6) — is #32. Replacing routine.Open with the real gate is
	// this line and nothing else, which is why the gate is an interface at all."
	// It was one line.
	routines := routine.New(gate{diagnoses: diagnoses})
	// Progression (§3.5). The four arguments are the four things it is allowed
	// to know: the body it reads a biological age from, the balance it reads the
	// money factor from, where the player is, for §3.15's decay, and — for the
	// leaderboards — who else is playing and what they are called. None of them
	// is a module: each is the narrowest interface that answers one question, so
	// the package that composes Karma cannot reach a sample, a purchase, a home,
	// or an email address.
	factors := factor.New(bodies, purse{characters: characters},
		population{characters: characters}, roster{accounts: accounts})
	// Peer endorsement (§3.5). Two arguments, and both of them are one-way:
	// `crowd` is who may be named and what they are called, `grants` is where
	// the credit goes — and `grants` returns an error rather than the standing
	// internal/factor actually hands back, so the module that decides who gets
	// endorsed is structurally unable to learn anything about the person who
	// was. It is the one place a factor moves because of another human being,
	// and the narrow pair is what keeps that from becoming a peephole.
	endorsements := social.New(clk, crowd{accounts: accounts}, grants{factors: factors})
	// The event backlog (§4). Four arguments, and every one of them is
	// something the module is deliberately unable to reach for itself: what
	// the player is holding (the same purse internal/factor reads a balance
	// through), who is coming back to this — how long they were away and which
	// factors they are behind on — and, last, when to surface anything at all.
	//
	// event.Attentive is §4's release policy: caps that a long absence cannot
	// overrun, a family the player keeps walking away from slowed but never
	// silenced, the last release of the day spent on something that would serve
	// them, and an escalation exempt from all of it. Nothing it decides crosses
	// the wire — the offer is the recommendation, which is the whole of §4's
	// "the notification box is where the server makes recommendations".
	backlog := event.New(clk, purse{characters: characters},
		bearings{accounts: accounts, factors: factors}, event.Attentive{})
	// The challenge framework (§5). Two arguments, and both of them are seams
	// the module is deliberately unable to cross for itself: where a completed
	// run's authored deltas go, and how the event that opened it gets settled.
	//
	// The second is §4's line between the two systems — "an event *can* offer a
	// challenge… they are distinct systems, not one abstraction" — and it is
	// the backlog itself rather than an adapter, because Resolve is already the
	// one method internal/event exposes for exactly this.
	challenges := challenge.New(clk, awards{factors: factors}, backlog)
	// §3.14's ladder. Two arguments, and both are things the module is
	// deliberately unable to reach for itself: where a day's wage goes, and
	// whether a player reaches a rung's requirement.
	//
	// The first is not the character module. internal/character has two ways to
	// add money and only one of them is right for a wage — §3.4's income is
	// structural, so it must not spend the day's paid-action allowance — and
	// the adapter is what makes the wrong one unreachable from here.
	//
	// The second returns a boolean rather than a shortfall, because three of the
	// five factors a rung asks about are ones the player is never shown (§3.5).
	careers := career.New(clk, payroll{characters: characters}, capability{factors: factors})
	// §3.14's other half of the same section: the work a role does while nobody
	// is sitting in it. Four arguments, and every one of them is deliberately
	// not a module — the rung held, the playbook's four methods and no more,
	// where the player's rules are read from, and where an authored delta goes.
	//
	// The playbook is passed as itself because all four methods the seam names
	// are already exactly its own; what the interface does is keep a job from
	// reaching the other eleven, so a delegated role can be answered by the
	// player's rules and can never edit them.
	situations := situation.New(clk, holds{careers: careers}, playbooks,
		population{characters: characters}, awards{factors: factors})
	// §3.15's core progression, on the day-to-day side: living among a
	// population means resolving friction with them every day. Three arguments,
	// and none of them is a module — the playbook's four methods and no more,
	// which population the player lives among, and where an authored delta goes.
	//
	// Who those people *are* is not an argument, because it is not state:
	// internal/environment is authored content with no database behind it, the
	// same way internal/city is, and a seam in front of it would be an interface
	// standing between a package and a list of names.
	conflicts := conflict.New(clk, playbooks, population{characters: characters},
		awards{factors: factors})

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
		health:     bodies,
		routines:   routines,
		playbooks:  playbooks,
		diagnoses:  diagnoses,
		factors:    factors,
		social:     endorsements,
		events:     backlog,
		challenges: challenges,
		careers:    careers,
		situations: situations,
		conflicts:  conflicts,
		clk:        clk,
		realtime:   hub,
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
	// own them: Karma decay is internal/factor's, the event backlog is
	// internal/event's, and recomputing ceilings is deliberately nothing at all
	// now that a ceiling is composed on read (see the binding). The composition
	// lives beside each module's transport rather than in internal/rollover, so
	// a ticket that fills one in replaces a method there and edits neither this
	// line nor that package.
	//
	// The queue is not passed. It cannot be: the queue's worker runs the
	// rollover, so cmd/server ties it on with Through once both exist. A server
	// whose queue was never tied still catches players up on the paths a
	// request drives; it simply schedules nothing, which is exactly right for a
	// process with no workers in it.
	s.rollovers = rollover.New(clk, st, log, accounts, characters, bodies, routines,
		employed{careers: careers},
		work{factors: factors, events: backlog, playbooks: playbooks,
			situations: situations, conflicts: conflicts, clk: clk}, s)

	// The knot from the top of this function, tied. Everything a walk needs to
	// offer where it arrived to exists by now, including the server that
	// publishes what came of it — which is why this is two assignments here
	// rather than an interface internal/character would have had to grow.
	walks.events, walks.announce = backlog, s.offered

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

	// What the world has put in front of this player before, and what they did
	// about it (§4). A GET for the same reasons: it writes nothing, and it is
	// history rather than world. What it deliberately cannot answer is what is
	// still *queued* — a client that could read the backlog could walk towards
	// it, and §4's sandbox depends on not being a quest list.
	mux.Handle("GET "+eventLogPath, s.authenticated(http.HandlerFunc(s.eventLog)))

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
