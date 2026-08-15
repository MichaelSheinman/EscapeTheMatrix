# Escape the Matrix — one binary, one datastore.
#
# The build order is the point of this file. `web/dist` is a prerequisite of the
# binary, so make physically cannot produce a server without first building the
# frontend it embeds (technical.md §2). Reaching for `go build` by hand skips
# that — which is why web.Assets() also refuses to start such a binary rather
# than letting it serve a blank page.

GO  ?= go
NPM ?= npm

BIN  := bin/server
DIST := web/dist/index.html

# Loaded so `make run` needs no exported environment. Absent in production,
# where the platform supplies the variables.
ifneq (,$(wildcard .env))
include .env
export
endif

# ui/ is a source of the build: a token change must rebuild dist.
WEB_SRC := web/package.json web/index.html web/vite.config.ts web/tsconfig.json \
           $(shell find web/src ui -type f -not -path '*/node_modules/*' 2>/dev/null)
GO_SRC  := go.mod $(shell find . -name '*.go' -not -path './web/node_modules/*' 2>/dev/null) \
           $(wildcard migrations/*.sql)

.DEFAULT_GOAL := build


# ─── build ────────────────────────────────────────────────────────────────────

## build: the deployable — frontend, then the binary that embeds it
.PHONY: build
build: $(BIN)

$(BIN): $(DIST) $(GO_SRC)
	@mkdir -p bin
	$(GO) build -o $(BIN) ./cmd/server
	@echo "built $(BIN)"

$(DIST): $(WEB_SRC) web/node_modules
	$(NPM) --prefix web run build
	@# Vite empties outDir, which takes the committed placeholder with it.
	@# Restoring it keeps a fresh clone compilable without a frontend build.
	@touch web/dist/.gitkeep

web/node_modules: web/package.json
	$(NPM) --prefix web install --no-audit --no-fund
	@touch web/node_modules

## frontend: build web/dist only
.PHONY: frontend
frontend: $(DIST)

## generate: rewrite the client's message types from the Go definitions (§8)
.PHONY: generate
generate:
	$(GO) run ./cmd/tsgen


# ─── run ──────────────────────────────────────────────────────────────────────

## run: build and serve on $ETM_ADDR (needs `make db` first)
.PHONY: run
run: build
	./$(BIN)

## dev: Vite with hot reload on :5173, proxying /api to a separately run server
.PHONY: dev
dev: web/node_modules
	$(NPM) --prefix web run dev

## serve: run the Go server from source, without rebuilding the frontend
.PHONY: serve
serve: $(DIST)
	$(GO) run ./cmd/server

## watch: serve, restarting on any source change — Go, content, migrations, or
## the frontend (`make dev` is still the fast loop for client-only work)
#
# It watches `make serve` rather than the server package, and the indirection is
# the whole point. web/dist is embedded at *compile* time (web/embed.go), so a
# rebuilt binary picks up whatever dist is on disk and a stale dist is invisible
# — which is exactly what happened on 2026-08-08: wgo dutifully restarted on
# every Go change while serving an eleven-minute-old client, because nothing in
# the loop rebuilt the frontend and .tsx was not even watched.
#
# `serve` depends on $(DIST), so make's own mtime check does the deciding: a
# client edit rebuilds dist and then recompiles, a Go edit skips straight to the
# compile. Nobody has to remember which kind of change they just made.
#
# Excluding dist is load-bearing rather than tidiness. Without it the frontend
# build writes files this watcher is watching, and it rebuilds forever.
.PHONY: watch
watch: $(DIST)
	$(GO) run github.com/bokwoon95/wgo@v0.6.4 \
	  -file '\.go$$' -file '\.yaml$$' -file '\.sql$$' \
	  -file '\.tsx?$$' -file '\.css$$' -file '\.html$$' \
	  -xdir 'node_modules' -xdir 'dist' -xdir 'bin' -xdir '\.git' \
	  $(MAKE) serve


# ─── database ─────────────────────────────────────────────────────────────────

## db: start local Postgres and wait until it accepts connections
.PHONY: db
db:
	@docker info >/dev/null 2>&1 || { \
		echo "Docker is not running. Open Docker Desktop, then re-run 'make db'."; \
		exit 1; \
	}
	docker compose up -d
	@printf 'waiting for postgres'
	@until docker compose exec -T postgres pg_isready -U etm -d etm >/dev/null 2>&1; do \
		printf '.'; sleep 1; \
	done
	@echo ' ready'

## db-stop: stop Postgres, keeping its data
.PHONY: db-stop
db-stop:
	docker compose stop

## db-reset: destroy the database and its volume, then start fresh
.PHONY: db-reset
db-reset:
	docker compose down -v
	@$(MAKE) --no-print-directory db


# ─── checks ───────────────────────────────────────────────────────────────────

# There is no test target. This project is checked by opening the browser and
# playing it — unit tests are not written here, and a unit test written to chase
# a bug is deleted with the bug.

## check: format, vet, seams, typecheck
.PHONY: check
check: fmt vet seams typecheck

.PHONY: fmt
fmt:
	$(GO) fmt ./...

# seams: the rules that keep a guarantee a guarantee rather than a convention.
# Today that is one rule — package api may not import encoding/json, because
# wire.Encode is the only thing allowed to turn a view into a response body.
# Fast, offline, and no dependencies, so unlike `doctor` it belongs in `check`.
.PHONY: seams
seams:
	$(GO) run ./cmd/wirecheck

.PHONY: vet
vet: $(DIST)
	$(GO) vet ./...

.PHONY: typecheck
typecheck: web/node_modules
	$(NPM) --prefix web run typecheck

.PHONY: tidy
tidy:
	$(GO) mod tidy

## doctor: scan the React client — correctness, perf, a11y, architecture
#
# Deliberately not part of `check`. The scan reaches the network (supply-chain
# and score APIs) and takes far longer than a typecheck, so it does not belong
# in the edit loop — run it when a feature is finished, not on every save.
#
# `make doctor SCOPE=changed` reports only the issues the working tree
# introduced rather than the whole existing backlog.
SCOPE ?= full
.PHONY: doctor
doctor: web/node_modules
	$(NPM) --prefix web run doctor -- --verbose --scope $(SCOPE)


# ─── housekeeping ─────────────────────────────────────────────────────────────

## clean: remove build products
.PHONY: clean
clean:
	rm -rf bin web/dist
	@mkdir -p web/dist && touch web/dist/.gitkeep

## help: list targets
.PHONY: help
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /' | sort
