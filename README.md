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
  api/           HTTP handlers, and serving the embedded frontend
  config/        environment → typed struct, once, at boot
  store/         pgx pool, migrations, transaction helper
migrations/      forward-only SQL, embedded and applied at startup
content/         challenge definitions and dialogue (§5.2)
ui/              @etm/ui — the design system, consumed by web/ and preview/
web/             the client: React over Vite, built into web/dist
preview/         design-system playground; never embedded
```

`ui/` depends on neither consumer, and that arrow only points one way (§2).

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
