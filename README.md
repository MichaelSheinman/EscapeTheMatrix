# Escape the Matrix

One Go binary serves the API and the frontend it embeds, over one Postgres database.

Design lives in `Documents/HighLevelOverview.md` (product) and `Documents/technical.md`
(architecture). Section references below (§) point at the latter.

---

## Prerequisites

| | Why |
| --- | --- |
| **Go 1.26+** | the backend; version pinned in `go.mod` |
| **Node 22.12+** | the frontend build (Vite 8's floor) |
| **Docker Desktop** | local Postgres, via `compose.yaml` |

```bash
brew install go
brew install node
brew install --cask docker-desktop   # then launch it once to accept the terms
```

---

## Running it

```bash
cp .env.example .env   # once — then fill in the Google OAuth client, below
make db                # start Postgres and wait for it
make run               # build the frontend, build the binary, serve it
```

Then open <http://localhost:8080>, sign in with Google, and press **Probe the database**.

The number that comes back is a row count from Postgres. That is the whole of the walking
skeleton: a click becomes a REST command, which becomes one transaction, which becomes a value on
screen that could not have been produced client-side.

Press **Send again, same key** and the count does not move. That is the command layer: the same
command delivered twice applies once, and the second delivery is answered from the record of the
first.

Open the page in a second tab and probe there. The first tab is told over the socket, unasked —
and the tab that probed is not, because it already has the command's response. Take the network
down in between and the message is still delivered when the socket returns.

### A Google OAuth client

Sign-in is required — there is no anonymous play (§9) — so the server will not start without one.
At <https://console.cloud.google.com/apis/credentials>, choose **Create credentials → OAuth client
ID → Web application**, and register exactly this redirect URI:

```
http://localhost:8080/api/auth/google/callback
```

Put the client ID and secret in `.env`. Nothing else is needed: the only scopes asked for are
`openid email profile`, and the access token is spent on one read and discarded.

### Frontend hot reload

`make run` serves the built assets from inside the binary, so a CSS change needs a rebuild. While
working on the client, run the two halves separately:

```bash
make serve   # Go server on :8080
make dev     # Vite on :5173, proxying /api to :8080
```

Use <http://localhost:5173>. There is no proxy in a built binary — the same origin serves both.

Sign-in finishes on :8080, because that is the redirect URI Google has registered. That is not a
problem to work around: cookies are scoped to a host and ignore the port, so a session opened on
:8080 is already in force on :5173.

---

## Make targets

```
make          build the deployable
make db       start local Postgres (db-stop, db-reset)
make run      build and serve
make dev      Vite with hot reload
make generate rewrite the client's socket message types from the Go ones
make check    fmt, vet, typecheck, Go tests, client tests
make help     list everything
```

---

## Why the Makefile exists

`web/dist` is a **prerequisite of the binary**, so make cannot produce a server without first
building the frontend it embeds (§2). Running `go build` by hand skips that — which is why the
binary also refuses to start when its embedded assets are missing, rather than serving a blank
page.

`web/dist` is a build product and is not committed. The one exception is `web/dist/.gitkeep`:
`//go:embed` needs the directory to exist at compile time, so without it a fresh clone could not
run `go test ./...` before its first frontend build.

---

## Layout

```
cmd/server/      the binary
cmd/tsgen/       writes the client's socket types from the Go definitions
internal/
  account/       accounts and server-side sessions; owns those two tables
  api/           the HTTP surface, the auth middleware, and the embedded frontend
  character/     money, location, home — owns the character table
  clock/         the only time.Now() outside a test
  command/       the command layer: registry, idempotency, status codes
  config/        environment → typed struct, once, at boot
  health/        the biomarker panel, instrumentation tiers, the time series
  jobs/          the durable Postgres-backed queue, and the workers in this binary
  oauth/         a provider's sign-in, reduced to "who is this"
  realtime/      the WebSocket, the per-player outbox, and Postgres fan-out
  rollover/      the daily rollover: whose day it is, which days are owed, what happens
  routine/       the rules the player sets for their body, and the unlock gate
  simulation/    the rules: pure, no I/O, no clock — everything else is plumbing
  store/         pgx pool, migrations, transaction helper
    storetest/   a real Postgres for a test, created and dropped per test
migrations/      forward-only SQL, embedded and applied at startup
content/         challenge definitions and dialogue (§5.2)
ui/              @etm/ui — the design system, consumed by web/ and preview/
web/             the client: React over Vite, built into web/dist
  src/world/     the world model both trees read
  src/render/    the renderer, and the only files that import Pixi
preview/         design-system playground; never embedded
```

`ui/` depends on neither consumer, and that arrow only points one way (§2).

---

## Commands

Every player action is a command. There is **one** route — `POST /api/commands/{name}` — because
idempotency, the single transaction, and status codes as protocol are things no handler should be
able to opt out of by being registered somewhere else (§8).

A command is one function. It receives a transaction the executor opened and will commit, and
returns the value the client sees:

```go
func (s *Server) probe(ctx context.Context, tx pgx.Tx, actor command.Actor, in probeInput) (probeResult, error)

command.Register(reg, "probe", s.probe)   // at boot; the registry is read-only afterwards
```

The input and output types are inferred from the handler, so decoding, the 422 for a payload the
server does not recognise, and encoding the result are free for every command.

**Status codes are the protocol.** A handler chooses one only through an error constructor, and
anything else it returns is a 500 whose text the client never sees.

| | | |
| --- | --- | --- |
| `200` | applied | with `Idempotent-Replay: true` when this delivery was a duplicate |
| `400` | the envelope is wrong | no `Idempotency-Key`, body too large |
| `401` | nobody is signed in | refused by the middleware, before any handler |
| `404` | `command.NotFound` | no such command, no such id |
| `409` | `command.Conflict` | the world is in the way — retrying later may work |
| `422` | `command.Invalid` | the request is wrong — retrying never works |

**Idempotency.** Every command carries an `Idempotency-Key`; one per *intent*, reused for every
delivery of that intent. The key is claimed in the same transaction as the handler, so a command
that failed leaves no record and may be retried, while one that committed is answered from that
record forever after — byte for byte, from any instance. A key reused for a *different* payload is
a client bug and is refused 422.

```bash
K=$(uuidgen)
C="etm_session=<paste from the browser's cookie jar>"
curl -X POST localhost:8080/api/commands/probe -H "Idempotency-Key: $K" -H "Cookie: $C" -d '{"label":"demo"}'
curl -X POST localhost:8080/api/commands/probe -H "Idempotency-Key: $K" -H "Cookie: $C" -d '{"label":"demo"}'
# same answer, one row written
```

Without the cookie both are `401`. That is not a per-command check — see below.

---

## Accounts and sessions

**An account is the price of entry.** No anonymous play, no password storage, and no
anonymous→registered merge path (§9). Leaderboards and peer endorsement need a stable identity, so
identity comes first rather than being retrofitted.

| | |
| --- | --- |
| Sign-in | Google OAuth — authorization code, PKCE, `state` in a short-lived cookie |
| Identity | `(provider, subject)`, never the email address — an email can be reassigned |
| Sessions | server-side rows in Postgres; the cookie carries an opaque token and nothing else |
| Cookie | `httpOnly`, `Secure`, `SameSite=Lax`, 30 days, matching the row's expiry exactly |
| Storage | only the token's sha256 — a dump of the table cannot be replayed as sign-ins |

```
GET  /api/auth/google/start      leave for Google
GET  /api/auth/google/callback   come back, and be given a session
GET  /api/auth/session           who am I — 200 with a player, or 200 with none
POST /api/auth/signout           revoke the session server-side
```

Those four are routes rather than commands because each one is a cookie operation, and a command
handler never sees an `http.ResponseWriter`. Everything a *player* does is still a command.

**Auth is one middleware over the command route** (§8), not a check inside each handler. With one
door the guard can only be on it: a command cannot be written that forgets to check, and no handler
can run for someone who is not signed in. The signed-in account becomes the command's `Actor`,
which is also what scopes idempotency records — those hold a response body, so replaying one
player's answer to another's key would be a data leak by construction.

**`SameSite=Lax` is what makes commands CSRF-safe** without a token of their own: the browser
withholds the cookie from cross-site POSTs, and every command is a POST. It still rides the
top-level navigation that brings a player back from Google, which is the case that needs it.

Signing out sets `revoked_at` on the row. The cookie is cleared too, but that is the cosmetic half
— the token is dead on every instance the moment the transaction commits.

---

## The socket

**Commands go up over REST; everything the server decides on its own comes down one WebSocket**
(§8) — event releases, results of things that happened while the player was elsewhere, leaderboard
movement, chat. There is no SSE and no polling: a second long-lived connection would mean a second
reconnect story and a second auth path to deliver messages this one already carries.

```
GET /api/socket?session=<tab>[&since=<seq>]
```

Behind the same auth middleware as the command route — a socket is a request, and it carries the
same cookie. Every message on it is a typed envelope:

```json
{ "type": "probe.recorded", "seq": 7, "payload": { "probes": 12, "label": "demo", "notedAt": "…" } }
```

**The connection dropping is the normal case.** A sleeping laptop must not silently miss an event
release and come back to a world that skipped, so:

| | |
| --- | --- |
| Outbox | every message is a row in `realtime_outbox`, written in the publishing command's own transaction |
| `seq` | per player, gapless, drawn from `realtime_stream` — a client resumes by naming one |
| Resume | `?since=N` replays everything after N; omitting it means "from now on", for a page that just loaded its state |
| Gap | when `N` is older than the outbox still holds, the welcome says so and the client rehydrates over REST |
| Retention | one hour. The outbox covers a drop, not a history |
| Fan-out | `LISTEN`/`NOTIFY` on one channel, so a command handled by one instance reaches a socket held by another — no second datastore |

`NOTIFY` carries no payload; it is a doorbell that tells a connection to read the outbox. Live
delivery and resume are therefore **the same query**, which is what makes "a message that was
written is a message that gets delivered" structural rather than careful.

**Messages are keyed by session — a tab, not a player.** The client picks a session id per tab and
sends it on both the handshake and every command (`Etm-Session`). A message records the tab that
caused it, and that tab's socket skips it: responses tell you about your own actions, the socket
tells you about the world's (§8, rule 3). Open the app twice and probe in one tab to watch the
other one hear about it.

**The client's types are generated, not hand-maintained** (§8, rule 1). `web/src/realtime/messages.gen.ts`
comes from the Go structs registered with `realtime.Define`; `make generate` rewrites it and a test
fails if the committed file is stale. Two hand-maintained lists of one protocol drift, and the
drift is found in production.

```go
// in the module that owns the table (§3.1)
var probeRecordedMessage = realtime.Define[probeRecorded]("probe.recorded")

// inside a command handler, on the transaction it was given
probeRecordedMessage.Publish(ctx, tx, realtime.To{Account: actor.ID, Origin: actor.Session}, payload)
```

Publishing inside the command's transaction is the point: the message exists exactly when the thing
it describes committed. A command that rolled back announced nothing, and there is no announcement
to take back.

---

## Modules

**A module owns its tables and is the only thing that touches them** (§3.1). Cross-module access
goes through a Go interface, never a `SELECT` into someone else's tables. `internal/character` is
where that rule is set: it owns `character`, and a module that needs to know what a player can
afford holds `character.Wallet` — two methods, and no way to read a district or a timezone that are
none of its business.

**Modules never open a transaction.** Every method takes the caller's, because §3.2 puts a player
command inside exactly one transaction and a module that reaches into the pool has escaped it. A
money change that committed while the command causing it rolled back is then unreachable rather
than handled.

**Everything derived is computed on read** (§3.3). The character stores how many paid actions have
been taken and which local day that count belongs to; how many are *left* is read from the two
against the player's own midnight. A stored remainder would be wrong every night.

```
POST /api/commands/character.move   walk to a district        → 422 for a place that is not there
POST /api/commands/character.work   take a paid action        → 409 once the day's three are spent
```

---

## Health, and what you can see of it

**The body is simulated in full for everyone. Instrumentation changes the player's resolution on
their own body, never their health** (§3.16). A player without a wearable still suffers the real
HRV collapse — they simply cannot see the cause, and that gap *is* the diagnosis problem (§3.6).

The separation is structural rather than careful:

| | |
| --- | --- |
| `internal/simulation` | computes the whole panel, and does not know instrumentation exists |
| `internal/health` | stores it, and applies visibility **on read, on the way out** |
| `health_sample` | every day recorded in full, including the metrics nobody can read yet |

There is no code path in which a tier changes a number, and
`TestAnUninstrumentedPlayerSuffersTheSameEffects` proves it: two accounts, one with a watch, the
same fourteen days of living badly, and the two stored bodies compared field for field.

**Buying an instrument reveals a history, not just a number.** The samples were being written all
along, so the day the watch is bought the trend is already there — which is what makes it worth
money rather than worth waiting for, and what "your recovery has been collapsing for three weeks"
is later argued from.

**Biological age is composed on read and never stored** (§3.3, §3.5), visible at every tier. A
metric with no instrument behind it is still sent — named, explained, and carrying the tier that
would read it — with no value and no history. What is withheld is the reading, not the idea.

```
POST /api/commands/health.live  {"sleep":"short","training":"hard","diet":"poor"}   409 if today is lived
POST /api/commands/health.buy   {"tier":"wearable"}                                 409 if you cannot afford it
```

`health.live` is the hand-driven day, kept for saying how a day actually went. Routines (below)
produce those three choices automatically, and the daily rollover (further below) applies them at
the player's own midnight without anybody pressing anything.

---

## Routines, which are rules for the body

**A routine is already a rule, and the playbook is the same concept pointed at judgement** (§3.14).
So they are one system with one interface language: a routine is a standing *question* about the
body and a closed list of *sentences* answering it — the form the playbook composes in — and
nothing on the screen is a dial or a field.

| | |
| --- | --- |
| `internal/simulation/routine.go` | the mechanics: what a routine does, and the day it produces for a date |
| `internal/routine` | the content: which routines exist, what they ask, what may be answered |
| `routine.choices` | one `jsonb` document per player, read and written whole |

**The week has a shape.** "Three hard sessions a week, easy in between" is a training plan and
"train hard" is not, so the training half of a routine is seven entries indexed by weekday and a
rest day is part of the plan rather than a day it failed.

**An intention is not an outcome, and organization is the difference.** §6.1 named the organization
routine as a habit worth teaching and left it undefined; it is defined here as *adherence*. What
happens is what was intended, held to whatever the arrangement can hold it to — so organization
matters in proportion to ambition, and a player intending five hours of sleep loses nothing by being
disorganised while one intending eight and a half loses most of the difference. Both ends of it
cost: chaos is a stressor and so is a week with no slack in it, which is what stops "maximise
organization" from being the right answer (§3.14). Slippage is spread evenly across the days rather
than rolled — nothing in the core is random — so the same rules over the same dates are the same
life, forever.

**A routine is legible before it has happened.** `simulation.Project` runs the rules forward from the
current body and reports the fortnight they lead to. It is the *same* function the days are lived
with, which is what `TestAProjectionIsExactlyTheDaysItPredicts` exists to keep true: a forecast the
screen shows and a fortnight the game later applies cannot be two models. It is composed on read and
never stored (§3.3), and it is instrumented exactly like the panel — a projection is a reading of a
body too, so a player with no watch is not told what their HRV will be either.

**Configured as a unit.** There is no command that changes one routine. A set of rules for the body
is one decision — deciding to sleep more is deciding what that costs the evening — so all four are
stated together, validated together, and written as one document.

```
POST /api/commands/routine.set   {"choices":{"sleep":"protected","workout":"split",…}}   422 outside the vocabulary
POST /api/commands/routine.live  {}                                                      409 if today is lived
```

**The unlock gate is stubbed open.** §3.6 wants struggle before mastery: a routine should not be
configurable until the player has experienced the problem it solves. What that experience is, is a
diagnosis — `#32` — so `routine.Gate` is a one-method interface with a single implementation that
opens everything, bound in `internal/api`. Replacing it is one line there, and the refusal it will
produce is already a tested path (`ErrLocked` → 409).

---

## The daily rollover

**One game day is one real calendar day, so the world advances once per player, at that player's own
local midnight** (§3.3). There is no global tick: nothing in the design has sub-day granularity, so
a loop that woke for everybody would be pure cost.

| | |
| --- | --- |
| `internal/rollover` | the day, composed — which days are owed, in what order, and what happens on each |
| `internal/jobs` | the durable queue (River) and the workers that drain it, in this same binary |
| `rollover` | one row per player: the last local day the world was advanced through |

**The schedule is a chain, not a list.** Each rollover arms exactly one successor — for the player
it just ran for, at their next local midnight — and only if that player has been seen inside
`rollover.Dormant`. A player who stops playing stops being scheduled within a day and holds nothing
in the queue at all, which is what "dormant accounts cost nothing" means concretely. Coming back is
what puts them on the chain again, because `Wake` catches them up *and* re-arms them, and it runs
on the world snapshot every page load fetches.

**Catch-up is bounded.** A player back after a season is caught up from `rollover.Window` days, and
the rest are counted as skipped and stamped over — those days happened to nobody, and writing thirty
thousand samples for them would be inventing evidence for a diagnosis (§3.6) to later argue from.

**The rollover stops strictly short of today.** A day still happening cannot be rolled up, so
`routine.live` lives the day that is in progress and the rollover lives the ones that have ended.
Neither can take the other's, and the two ledgers — `rollover.last_day` and `health_panel.last_day`
— are allowed to disagree by a day for exactly that reason. Running it twice is running it once: the
ledger row is taken `FOR UPDATE` and the walk starts after whichever ledger is further along.

**Jobs are durable, retried, and readable.** They survive the process that made them, come back
after a restart, retry with backoff, and are inspectable with one query rather than a dashboard:

```sql
SELECT state, scheduled_at, attempt, args
  FROM river_job
 WHERE kind = 'rollover' AND state IN ('available','scheduled','retryable','running')
 ORDER BY scheduled_at;
```

**Enqueueing is transactional.** `Queue.Schedule` inserts inside the caller's transaction, so a
rollover that failed halfway leaves no job behind and a ledger that moved forward without arming its
successor is unreachable rather than a state to recover from.

**River keeps its own schema.** It migrates separately from `migrations/`, at boot, because it moves
on its own release schedule — copying its SQL into the game's migration history would pin one to the
other. `internal/store/storetest` runs the same migration, so a test database is the production
database in every respect.

**Three quarters of §3.3's work is a named stub.** Replenishing the event backlog (`#21`), Karma
decay (`#15`) and environment ceilings (`#29`) are the `rollover.World` interface, bound to
`rollover.Unbuilt` in `internal/api`. Each of those tickets replaces one argument; none of them edits
`internal/rollover`. Biomarker roll-up is the quarter that is real today.

```
rollover.ran   ← socket only; there is no command, because a player cannot ask for their day to end
```

---

## The simulation core

**`internal/simulation` is the rules, and nothing else.** State in, new state plus a list of
effects out. No database, no network, no `time.Now` (§3.1) — everything else in the backend is
plumbing around it.

```go
body, effects := simulation.Advance(clock, body, simulation.Day{
    Sleep: 8 * time.Hour, Training: simulation.Hard, Diet: simulation.Clean,
})
age := simulation.BiologicalAge(body)          // computed on read, never stored (§3.3)
ahead := simulation.Project(body, rules, on, 14)  // the same arithmetic, run forward
```

**Time is injected.** The package declares a one-method `Clock` and reads it nowhere else; the real
one lives in `internal/clock`, which is the only `time.Now()` outside a test. One game day is one
real calendar day and the boundary is the player's local midnight (§3.3), so "what happens at the
rollover in Auckland" has to be a test rather than a wait.

**Effects are values, not commands.** A rule that notices something says what happened — a sample
was taken, recovery crossed below poor — and the module that called it decides whether that is a
socket message, a row in the time series, or an event release (§4). The core never learns that any
of those exist.

**Two structural tests keep it honest.** One parses the package's own source and fails if it
imports anything but `math` and `time` — an allowlist, because a denylist is what a new import
slips past. The other fails if any file calls `time.Now`. Both are in `purity_test.go`.

**Nothing is random**, so a given input can only produce one output, asserted directly by
`TestTheSameInputProducesTheSameOutput`: a hundred days run twice must produce the identical body
and the identical effects. That is what makes a rollover reproducible and a regression in tuning a
failing assertion rather than a feeling.

The tuning loop §17 asks for is this package's test suite: change a constant, run `go test
./internal/simulation/`, read the table. It takes about a second and needs no database.

---

## The client

**One world model, two readers, and neither of them owns it** (§10.1, §10.2).

```
web/src/
  world/state.ts    the model, and the pure reducers that change it
  world/store.ts    the one instance, and the only writers
  world/react.ts    useWorld — useSyncExternalStore over the same store
  world/actions.ts  the intents: boot, probe, sign out — the one path to the server
  render/renderer.ts  the Pixi lifecycle and the frame loop
  render/scene.ts     what is drawn; the thin interface Pixi stays behind
```

**The store is plain TypeScript, never React state** (`zustand/vanilla`). React subscribes through
`useSyncExternalStore`; the renderer reads `world.getState()` **every frame** and subscribes to
nothing. If the world lived in React, the renderer could only see it at React's update cadence —
which is the coupling `@pixi/react` was rejected to avoid, arriving by another route.

**The canvas never unmounts.** `index.html` holds `<canvas id="world">` and `<div id="root">` as
siblings; the renderer is created once at boot and destroyed at teardown, never by a component
mounting. UI screens are overlays above the world, not routes that replace it — press <kbd>Esc</kbd>
and the same scene is still running behind where the panel was. Unmounting would rebuild the WebGL
context and the texture cache on every navigation.

**Both trees dispatch through the same command layer.** A click on the world and the button in the
panel call the same `probe()` in `world/actions.ts`; there is no second path to the server and no
second place an answer is interpreted.

```
REST  GET /api/world   hydrates the model at boot, and again after a socket gap
      POST /api/commands/…  changes it; the response is applied by the sender (§8, rule 3)
WS    typed messages   patch it for everything the sender did not do
```

**Patches are typed domain messages, not generic diffs.** One reducer per message type, in a table
the compiler checks for completeness — a message added on the server regenerates `messages.gen.ts`
and the table stops compiling until it has a reducer, rather than the message being silently
ignored.

**The client computes no game rules.** Every number on screen was stated by the server; the rest is
pure presentation. That is what makes the deferred WASM prediction plan (§1) an addition rather than
a reconciliation.

**WebGL2 is a hard floor** (§10.4). Pixi v8 has no Canvas2D fallback, so a device without it does
not degrade — the store carries `renderer: 'unsupported'` and the UI says so in a sentence. The
placeholder scene uses no full-screen filters: §10.4 ships the Low tier first, and a look designed
around a lighting pass cannot have it taken away later.

The scene itself is a placeholder — a beacon, the server's row count, and one mark per probe this
client has been told about, warm for the ones this tab made and cold for the ones the socket
brought. The painted location with a walkable polygon over it is `#11`.

---

## Configuration

Environment variables, parsed once into a typed struct at boot. The process refuses to start if
any are missing or invalid, and reports **every** problem at once rather than one per restart
(§13). See `.env.example`.

| | |
| --- | --- |
| `ETM_ENV` | `development` \| `production` — decides the log format |
| `ETM_ADDR` | bind address, default `:8080` |
| `ETM_DATABASE_URL` | required |
| `ETM_PUBLIC_URL` | the origin browsers reach this server on, default `http://localhost:8080` |
| `ETM_GOOGLE_CLIENT_ID` | required |
| `ETM_GOOGLE_CLIENT_SECRET` | required |

`ETM_PUBLIC_URL` is configuration rather than something read off the request, because the OAuth
redirect URI is built from it and a `Host` header is attacker-controlled. Its scheme also decides
whether the session cookie carries `Secure` — plain http in production is refused at boot, so in
production it is always set, and locally the cookie works over http where a `Secure` one would be
silently dropped.

---

## Migrations

Plain SQL in `migrations/`, embedded into the binary, applied at startup, **forward-only**.

A merged migration is never edited — the schema of a deployed database can only move in one
direction, so a mistake is corrected by a new migration. Schema follows the binary automatically
on deploy.

**The job queue migrates itself, separately.** River owns its own tables and its own ledger and
moves on its own release schedule, so `internal/jobs.Migrate` runs immediately after the goose run
rather than inside it. `migrations/` stays the game's history alone.

---

## Tests

```bash
make db      # once — the integration tests want a real Postgres
make check   # fmt, vet, typecheck, Go tests, client tests
```

Commands and domain modules are tested against a real database rather than a mock (§14): the
behaviours worth proving — one transaction per command, a rollback taking its idempotency claim
with it, a duplicate blocking on a claim rather than racing it — are exactly the ones a mock would
be inventing. `internal/store/storetest` gives each test its own database and drops it afterwards.

The socket is tested the same way and over a real WebSocket: `internal/api/socket_test.go` kills a
connection mid-flight, publishes while nothing is listening, reconnects naming the last sequence
number it processed, and asserts every missed message arrives once and in order. Another test
stands up two servers over one database to prove the fan-out crosses instances.

Without a reachable Postgres those tests **skip**, so `go test ./...` still passes on a machine
where Docker is not running. Set `ETM_TEST_REQUIRE_POSTGRES=1` to turn the skip into a failure —
CI should, because a suite that quietly skips the test proving a command cannot double-apply is
worse than no suite.

On the client, what is unit-tested is the store's reducers (`web/src/world/state.test.ts`, Vitest).
They are pure by construction — no fetch, no DOM, no renderer — so the assertions are about what a
statement from the server does to the model and what it deliberately leaves alone. The rest of the
client is covered by the type system rather than by tests: an unhandled socket message and a
selector with the wrong shape are both compile errors.
