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
cp .env.example .env   # once
make db                # start Postgres and wait for it
make run               # build the frontend, build the binary, serve it
```

Then open <http://localhost:8080> and press **Probe the database**.

The number that comes back is a row count from Postgres. That is the whole of the walking
skeleton: a click becomes a REST command, which becomes one transaction, which becomes a value on
screen that could not have been produced client-side.

Press **Send again, same key** and the count does not move. That is the command layer: the same
command delivered twice applies once, and the second delivery is answered from the record of the
first.

### Frontend hot reload

`make run` serves the built assets from inside the binary, so a CSS change needs a rebuild. While
working on the client, run the two halves separately:

```bash
make serve   # Go server on :8080
make dev     # Vite on :5173, proxying /api to :8080
```

Use <http://localhost:5173>. There is no proxy in a built binary — the same origin serves both.

---

## Make targets

```
make          build the deployable
make db       start local Postgres (db-stop, db-reset)
make run      build and serve
make dev      Vite with hot reload
make check    fmt, vet, typecheck, test
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
internal/
  api/           the HTTP surface, and serving the embedded frontend
  command/       the command layer: registry, idempotency, status codes
  config/        environment → typed struct, once, at boot
  store/         pgx pool, migrations, transaction helper
    storetest/   a real Postgres for a test, created and dropped per test
migrations/      forward-only SQL, embedded and applied at startup
content/         challenge definitions and dialogue (§5.2)
ui/              @etm/ui — the design system, consumed by web/ and preview/
web/             the client: React over Vite, built into web/dist
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
curl -X POST localhost:8080/api/commands/probe -H "Idempotency-Key: $K" -d '{"label":"demo"}'
curl -X POST localhost:8080/api/commands/probe -H "Idempotency-Key: $K" -d '{"label":"demo"}'
# same answer, one row written
```

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

---

## Migrations

Plain SQL in `migrations/`, embedded into the binary, applied at startup, **forward-only**.

A merged migration is never edited — the schema of a deployed database can only move in one
direction, so a mistake is corrected by a new migration. Schema follows the binary automatically
on deploy.

---

## Tests

```bash
make db      # once — the integration tests want a real Postgres
make check   # fmt, vet, typecheck, test
```

Commands and domain modules are tested against a real database rather than a mock (§14): the
behaviours worth proving — one transaction per command, a rollback taking its idempotency claim
with it, a duplicate blocking on a claim rather than racing it — are exactly the ones a mock would
be inventing. `internal/store/storetest` gives each test its own database and drops it afterwards.

Without a reachable Postgres those tests **skip**, so `go test ./...` still passes on a machine
where Docker is not running. Set `ETM_TEST_REQUIRE_POSTGRES=1` to turn the skip into a failure —
CI should, because a suite that quietly skips the test proving a command cannot double-apply is
worse than no suite.
