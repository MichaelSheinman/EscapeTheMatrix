# Escape the Matrix — Technical Architecture

Implementation detail extracted from `HighLevelOverview.md`. That document owns product and
design decisions; this one owns how they get built. Section references (§) point back to it.

**Status:** architecture decided — see the DECIDED markers throughout.
**Last session:** 2026-07-31

> **§16 lists amendments this document forces on `HighLevelOverview.md`.** Several architecture
> decisions here contradict what that document currently says. They are listed rather than
> silently applied.

---

## 1. Stack — DECIDED

**Backend is Go.** The browser only executes JS and WASM, so a client layer is unavoidable — but
game logic does not have to live there.

| Layer | Choice | Notes |
| --- | --- | --- |
| Backend | **Go** | Owns simulation, game logic, state, persistence |
| HTTP / routing | `net/http` stdlib | Go 1.22+ router is sufficient; minimal deps |
| Realtime | **WebSocket only** (`coder/websocket`) | SSE dropped — see §8 |
| Persistence | **Postgres** via `pgx` | The only datastore. No Redis, no second store |
| Background jobs | **`riverqueue/river`** (Postgres-backed) | Durable, retryable, same binary |
| Migrations | plain SQL + `goose`/`tern`, embedded | Forward-only, applied at startup |
| AI | **none at runtime** | Claude is an offline authoring tool only (§7) |
| World renderer | **PixiJS v8** | WebGL2. No Canvas2D fallback in v8 — WebGL2 is the hard floor |
| Level authoring | **Tiled** | Image layer + object layers → JSON → Pixi (§10.3) |
| UI / dialogue / panels | **React (DOM)** | A life sim is mostly UI; DOM is the right tool |
| Client state | **`zustand/vanilla`** | Framework-independent store, read by React *and* Pixi (§10.2) |
| Client language | TypeScript | Rendering, input, presentation only — no game logic |
| Asset serving | Go `embed.FS` | One binary serves API + WebSocket + built frontend |
| Hosting | **Fly.io** | Long-lived processes required. Railway/Render equivalent |

### Why this split

Go stays authoritative over all rules and the simulation. The client renders what the server sends
and forwards input. TypeScript exists as a rendering layer, keeping game logic in one language.

**Deliberately rejected: Go → WASM via Ebitengine** (whole game in Go). Ebitengine is mature and
compiles to WASM, but renders everything into a canvas — no DOM or CSS UI. For a UI-heavy life sim
that is the wrong trade. Large binaries, and TinyGo can't shrink them well due to reflection use.

**Deliberately rejected: Vercel / any serverless platform.** Two decided requirements rule it out:
persistent WebSockets (§8) and in-process durable background workers (§4). On Vercel both become
third-party vendors — a realtime service plus an external scheduler — to replace what one Go binary
does natively. Cloud Run fails for the same reason: scale-to-zero terminates the socket server and
the rollover scheduler. **The hosting requirement is "runs a long-lived process," nothing more.**

**Planned evolution: shared rules package.** `internal/simulation` (§3.1) is pure and I/O-free, so
it compiles natively *and* to WASM for client-side prediction. Rules written once, in Go. The shape
exists from day one; the WASM target is enabled when latency demands it.

---

## 2. Repository layout — DECIDED

**One monorepo, one Go module, three npm packages** — the client, the design system it consumes,
and the playground that exercises the design system without a backend.

```
escape-the-matrix/
  go.mod                    # Go module at repo root
  Makefile                  # enforces build order
  cmd/server/main.go        # the single binary
  internal/                 # Go packages (§3.1)
  content/                  # challenge definitions, dialogue, rule vocabulary (§5.2)
  migrations/               # forward-only SQL
  ui/                       # @etm/ui — design system: tokens, components, gallery
  preview/                  # playground: the design system driven by static fixtures
  web/                      # frontend, own package.json, Vite
    src/
    dist/                   # built assets, embedded into the Go binary
```

`go build` embeds `web/dist` via `embed.FS` and produces one binary serving API, WebSocket, and
frontend. **Build order is `npm run build` then `go build`** — hence the Makefile, so nobody
produces a binary that serves a blank page.

Content and migrations are versioned alongside the code that reads them, which is what §5.2's
repo-as-truth decision requires.

### Why `ui/` is its own package — DECIDED

**`ui/` is consumed by both `preview/` and `web/`, and depends on neither.** It owns the art bible
as code (§4's locked palette encoded as tokens), the component set, and a static gallery page that
renders every component in every state.

This supersedes the earlier "one npm package" decision. The reason is that the design system needs
to be exercised *without* a running backend. `preview/` drives the components from static fixtures,
so a styling change can be made and reviewed in isolation; `web/` wires the same components to the
store (§10.2). One component set, two consumers, and no path by which the playground and the real
client can drift apart — which is exactly what happens when a prototype is styled separately and
later "ported".

The dependency runs one way and must stay that way: `ui/` imports nothing from `web/` or `preview/`,
and holds no game logic, no store access, and no fetching. It renders what it is handed.
`preview/` is a development artifact and is never embedded into the binary — only `web/dist` is.

---

## 3. Backend architecture

### 3.1 Module boundaries and the simulation core — DECIDED

**Package by feature, not by layer.** Each module owns its own tables and is the only thing that
touches them. Cross-module access goes through a Go interface, never a `SELECT` into someone else's
tables.

```
internal/
  simulation/   # PURE. No I/O, no DB, no time.Now. The rules.
  character/    # identity, money, location, home
  health/       # biomarker panel, instrumentation tiers (§3.16)
  playbook/     # structured rules, matcher, rule-fire records (§3.14)
  challenge/    # catalog, runs, mechanic type registry (§5)
  event/        # backlog, release policy, chaining (§4)
  environment/  # populations, ceilings, decay (§3.15)
  factor/       # Karma composition, hidden factors, leaderboards (§3.5)
  social/       # peer endorsement, leaderboard identity
  chat/         # global channel + DMs — outside the simulation (§11)
  content/      # loads and validates repo content files at boot
  api/          # HTTP handlers, WebSocket, session middleware
  jobs/         # River workers: rollover, chat analysis
  store/        # pgx pool, migrations, transaction helpers
```

Package names match the design doc's vocabulary — `playbook`, `environment`, `challenge`, `factor` —
so §3.14 and `internal/playbook` are obviously the same thing.

**This is not DDD.** It takes bounded contexts and shared vocabulary; it skips aggregates,
repositories-as-a-pattern, entity/value-object ceremony, per-module layering, and CQRS. The
questions are "does this package own these tables" and "are the rules pure," not "is Character an
aggregate root."

#### The pure core — the most important structural rule

**`internal/simulation` is a functional core: state in, new state plus a list of effects out.**
Biomarker progression, biological-age composition, Karma weighting and decay, playbook condition
matching, factor deltas from challenge options — all pure functions. **No database access, no
network, no `time.Now`.**

Three things this buys, all already committed to:

1. **The whole simulation is testable without Postgres.** Table-driven tests running in
   milliseconds. §17 lists biomarker formulas, Karma weights, and decay curves as things to tune
   during implementation — that tuning loop must be a fast unit test, not a playthrough.
2. **It is exactly the package the WASM plan needs.** Pure from the start means it's a build
   target. Entangled with `pgx` means a rewrite.
3. **Determinism by construction.** No clock and no I/O inside means one input can only produce one
   output — which is what makes daily rollover reproducible and debuggable.

**Corollary: time is injected everywhere.** A `Clock` interface, never `time.Now()`. Not purism —
per-player timezone rollover (§3.3) is untestable otherwise, and "what happens at the day boundary
in Auckland" is a test that gets written repeatedly.

### 3.2 Persistence model — DECIDED

**Postgres is truth. Mutable current state, written through on every meaningful action.**

- Every player command is handled inside **one Postgres transaction**. Servers hold no per-player
  session state, so they are stateless and horizontally scalable, and rolling deploys drop nothing.
- **In-memory state exists only for in-flight interaction steps** — a challenge scene in progress, a
  dialogue exchange — and is persisted at **semantic boundaries** (end of scene, end of turn), not
  on a timer.

**Write-behind (periodic flush of an in-memory player model) was considered and rejected.** Three
reasons: the load doesn't justify it (a small transaction is ~1ms; an engaged player acts every
10–20 seconds); it costs statelessness, forcing sticky sessions and breaking rolling deploys; and it
directly attacks the mandatory save-and-resume guarantee (§5.4) by losing exactly the in-progress
challenge that was promised to resume.

Checkpointing at semantic boundaries — rather than on a timer — means resume always lands somewhere
coherent.

#### No universal ledger

**There is no write-on-every-change audit table.** History lives per-domain, and the surfaces are
enumerated deliberately rather than accreted:

| History surface | Why it exists |
| --- | --- |
| Biomarker time series | §3.16 trends; what an instrument reveals when it is bought |
| Event records — fired, outcome | §4; what the player reviews |
| Challenge outcome records | §5; completion, factor deltas |
| Chat transcripts | §11; scoring input, moderation |
| Playbook rule-fire records | §3.14 "rule 4 fired" |

Everything else is current-state only. **This list is the contract** — a player has to be able to
look back at what actually happened to them, and "add a history table when a feature needs it" is
how you end up with five incompatible timelines that can't be rendered as one.

> **Naming:** "journal" is reserved. §3.14's rule journal is renamed **Playbook** throughout
> (see §16). Nothing in the persistence layer uses the word.

#### Relational shape

Normalised tables for anything queried across players or appended to. `JSONB` is permitted only
where all three hold: no cross-player query, read and written as a unit, and schema genuinely fluid
during tuning. Candidates: routine configuration, in-flight challenge run state, biomarker panel
snapshots.

**No ORM.** `pgx` with hand-written SQL.

### 3.3 Time and the daily rollover — DECIDED

One game day = one real calendar day (§3.1), so the world advances on a **per-player scheduled
rollover at that player's local day boundary**, executed by the job queue.

Rollover work:

- replenish the event backlog (§4)
- apply Karma decay for the current environment (§3.15)
- roll up health / biomarker progression (§3.16)
- recompute environment ceilings and factor caps

**Gated on recent activity.** A player who hasn't played in N days doesn't get rollover work
scheduled, so dormant accounts cost nothing. Their next login triggers a catch-up rollover.

**Everything derived is computed on read, never stored as a stale number** — biological age, current
Karma, current factor values. Only the inputs are persisted.

**Rejected: a continuous global tick.** Nothing in the design has sub-day granularity; a tick loop
would be pure cost.

### 3.4 Background jobs — DECIDED

**`riverqueue/river` — a durable Postgres-backed queue, workers running in the same binary.**

- Durable across restarts, automatic retries, inspectable with `SELECT` rather than a dashboard.
- Zero added infrastructure — keeps the one-binary, one-datastore property.
- If worker load ever needs isolating, it's the same binary run with a flag, not an architectural
  change.

Job types at V1: per-player daily rollover, moderation actions, leaderboard refresh.

**Rejected:** in-process `time.Ticker` (lost on restart, no retry, no visibility — and rollover
matters); Redis or any external queue (adds a datastore avoided everywhere else).

---

## 4. The event system — DECIDED

**The world maintains a backlog.** Events are generated ahead of demand and sit in a queue, so there
is always something to do. **Release is a server decision, not a clock event** — based on activity,
time away, and what would serve the player next. **Events chain: responding to one immediately
queues its successor.** A long absence does not dump the backlog; there is a cap on what gets
surfaced, and the notification box is where the server makes recommendations.

### Three delivery surfaces, not one

| Surface | Direction | What lives there |
| --- | --- | --- |
| **Event queue** | pushed — server decides release | job situations from the playbook, character responses, requests for help, dating, recommendations |
| **World / sandbox** | pulled — player walks and interacts | encounters found by traversal (§3.12), standing entry points |
| **Challenge catalog** | discrete quests that begin and end | jury duty, conflict resolution, missions (§5) |

**They are distinct systems, not one abstraction.** An event *can* offer a challenge; the world *can*
expose a challenge entry point; some challenges are simply always available. **Availability and
offer are separate concepts** — the sandbox feel depends on not everything arriving as a push.

### Event lifecycle

`generated → eligible → offered → (declined | accepted) → resolved`

- **Declining does not consume** (§3.9). Walking past returns the event to the backlog.
- **Declining is signal.** A player routing around every interpersonal encounter is telling the
  release policy something worth acting on.
- **Placement rules** are authored per event — the contexts it may legitimately appear in — so
  nothing is ever placed absurdly. Placement conditions and world trigger zones are the same objects
  (§10.3).
- **Revalidation at release.** A backlogged event is re-checked against current state before being
  surfaced, and silently dropped or requeued if its preconditions no longer hold.

### Generation

**Daily conflicts and job situations are authored pathways per environment, selected deterministically
from relationship state** — not LLM-generated (§7). This is a real content cost and it replaces
§3.15's claim of an endless procedural tier (see §16).

Escalation to the player follows §5.2 of the previous draft and stands: **escalate on three
conditions, not one** — no rule matches, two rules conflict, or a rule fired and the outcome was
poor. Escalating only on "no match" lets one catch-all rule silence the game forever.

---

## 5. Challenges

### 5.1 Shape — DECIDED

**~6 minutes. Once ever, per player.** `challenge_run` carries a unique constraint on
(player, challenge). **~24 authored instances at V1: 6–8 families × ~4 variants each.** No runtime
generation, no variant swapping, no parameterised shells.

**Player input is multiple choice**, not free text. Each option carries authored hidden factor
deltas. **Reactions are authored per outcome bucket** — the rubric-equivalent is a scored bucket
(dismissed you / heard you / moved by you) with written reaction text per branch.

The hidden-factor hook (§3.5) is unaffected: deltas being authored doesn't make them visible.
Mystery comes from the weighting being hidden, not from the scorer being nondeterministic.

**Save and resume is mandatory** — even at 6 minutes, players abandon partway. In-flight run state
is checkpointed at scene boundaries (§3.2).

### 5.2 Content model — DECIDED

**Mechanic types are code. Challenges are data.**

- A **mechanic type** is implemented once: a Go handler plus a matching client scene module, with a
  typed config schema. V1 spans 3–4 types (`dialogue_branch`, `evidence_weigh`, `negotiation`, …).
- A **challenge** is `mechanic type + config document + content files`.
- **New content in an existing type is data only. A new mechanic is code.**

**Discipline: extract the type on the second instance, not the first.** At a 6–8 challenge V1 the
type-to-instance ratio is roughly 2:1, so the config machinery does not pay for itself in V1 — the
payoff is V2+. Write the first challenge of a type as if it were code, then extract. This matches
§3.10's own sequencing note: build the framework and one challenge to playable before starting the
rest.

**Repo files are the source of truth. Not a CMS.**

```
content/challenges/resolve_conflict/a/
  challenge.yaml     # mechanic type, placement rules, factor rewards
  dialogue.yaml      # nodes, options, authored reactions per bucket
```

On boot the server loads, validates against the mechanic's config schema, and **upserts into a
`challenge` catalog table** so foreign keys work. The database stores *runs*, never definitions.
**Dev mode hot-reloads on file change**; production validates at startup so a malformed challenge
fails in CI, not at 2am.

Content is versioned, diffable, and reviewable, and it travels with the code that interprets it.
Every content change is a deploy — near-zero cost with a single binary and one author.

### 5.3 Identity — DECIDED

**Three columns, not one string:** `family` (`resolve_conflict`), `variant` (`a`), `mechanic_type`
(`dialogue_branch`). The display code is derived.

Flat codes like `challenge_resolve_conflict_variant_a` bury structure in a string — "has this player
done a resolve-conflict-shaped thing?" becomes an unindexable `LIKE` prefix match. With columns it's
a real query, and the release policy can avoid repeating a *shape* rather than just an instance.

**Not a Postgres `enum` type.** Adding a value requires `ALTER TYPE`, can't share a transaction with
other DDL, and values can never be removed — and challenge codes will be added constantly. Use
`text` with a foreign key to the catalog table: same integrity, no migration pain. Go gets a
generated constant set for compile-time safety.

---

## 6. The Playbook (formerly "rule journal", §3.14) — DECIDED

**Rules are structured, not free text.** A rule is `when <condition> then <action>`, both selected
from an **authored vocabulary** the player unlocks. A deterministic matcher executes them. No LLM in the decision path.

**The player selects an initial set of rules they resonate with during onboarding** (§12).

Why structured beats free text here — and it's a design argument, not just a cost one:

- **Consistency is free.** §5.1.3 of the previous draft named inconsistent adjudication as the main
  risk; it stops existing.
- **Attribution is exact.** The log genuinely knows which rule fired. Free text requires the model to
  self-report, and it will sometimes blend or misattribute.
- **Failure is diagnosable.** §3.15 requires transfer failure to be diagnosable, not random. A
  structured rule makes failure legible — *this condition matched, this action fired, here's why it
  was wrong for these people.* Free text leaves the player unable to distinguish "my rule was wrong"
  from "the model misread my rule," and that distinction is the entire lesson.
- **Rule conflicts are statically detectable** — "these two both match and disagree" is good UI, and
  impossible with prose.
- Zero marginal cost, instant, fully testable.

**The condition/action vocabulary is a major design artifact.** It *is* the space of thinkable
strategies. Too small and the playbook reads as a settings screen rather than authorship. Unlocking
vocabulary terms is a natural fit for struggle-gating (§3.6), and §3.14's requirement that
over-general rules backfire becomes something authored deliberately rather than hoped for.

**Rule sets are environment-scoped** (§3.15) — the playbook is a set of context-tagged rule sets, not
one list, and the interface must make that structure obvious.

---

## 7. The AI surface — DECIDED

**There is no LLM in the game loop.** The simulation, playbook adjudication, event and conflict
generation, challenge selection, challenge scoring, **factor scoring**, and character dialogue are
all deterministic Go.

**V1 has no runtime LLM at all.** `anthropic-sdk-go` is not a runtime dependency.

**Hidden factors are scored from committed choices, not from text.** Every challenge option and every
escalated daily conflict carries authored factor deltas (§5.1). Emotional stability and Kindness are
aggregations over *which interpretations the player commits to* across many scenarios — behaviour
under stakes rather than tone in a message box. That is a sum, not a model call.

**Everything the previous draft's §5 described is gone:** no rule adjudication, no conflict
generation, no scoring of prose, no challenge selection, no live AI interview (deferred to V2), no
character chat generation, **no chat analysis**. With it goes the API key in the request path,
per-player inference cost, seconds of latency, consistency guardrails, the transcript store, the
replay harness, and debug views for nondeterministic selection.

**The consequence worth naming: the whole game is deterministic.** Same inputs, same outputs, every
time — the single largest testability and cost win in these decisions, and why §14 is as short as it
is.

**The seam is preserved.** Scoring is an interface with a deterministic implementation. Challenge
input is modelled so a free-text node *type* can be added without reshaping the schema. The event
pipeline doesn't assume synchronous resolution. When V2 wants real conversation or free-text
judgement, that's implementing an interface, not rebuilding the engine.

**Claude remains valuable as an authoring tool** — generating and varying the 24 challenges' content
offline. That's a dev-time dependency with none of the runtime costs.

### Privacy

**Nothing the player writes is analysed.** Scoring reads only what they chose, so §6.1's concern —
that the game must never appear to be gathering data about the person — dissolves rather than
needing mitigation. Chat messages, where chat is enabled (§11), are retained for moderation and
reporting only, and feed no game system.

---

## 8. Transport and API — DECIDED

**REST for commands. WebSocket for everything server-initiated. No SSE.**

### Commands over REST

`POST /api/commands/{name}`, one ordinary `http.Handler` each, testable with `httptest`.

What this buys that a raw socket would force you to rebuild:

- **Idempotency.** Commands carry an `Idempotency-Key`; the server records it and returns the
  original result on retry. Mobile networks and page reloads *will* duplicate commands, and "the
  player paid twice" or "the challenge advanced two nodes" are real bugs.
- **Auth as one middleware**, not a per-message check.
- **Status codes as protocol** — 409 for invalid state, 422 for validation, and so on.
- **Debuggability.** Every command is visible in the network panel and reproducible with `curl`.
- **Backpressure and timeouts** handled by the HTTP stack.

### WebSocket for push

Carries event releases, results of things the player didn't just do, leaderboard changes, chat.

**SSE was dropped** from the earlier draft. A WebSocket is required regardless (server-decided event
release can't be expressed by polling), so SSE would be a *second* long-lived connection with a
second reconnect story, second auth path, and second set of proxy/idle-timeout tuning — to deliver
messages the socket can carry as one more typed message kind.

### Protocol rules

1. **Typed envelope and registry.** Every server→client message is `{type, seq, payload}` with a Go
   struct and a matching TypeScript type. **Generate the TS types from the Go structs** — two
   hand-maintained lists drift, and you find out in production.
2. **Reconnect with resume.** The socket *will* drop — sleeping laptops, flaky wifi, deploys. The
   client tracks the last `seq` processed and sends it on reconnect; the server replays anything
   newer from a short-lived per-player outbox. **Without this a reconnecting player silently misses
   an event release and the world appears to skip.** This is the most commonly forgotten piece of
   this design.
3. **Responses tell you about your actions; the socket tells you about the world's.** The acting
   client applies the command's HTTP response directly and does not wait for a socket echo. This
   avoids double-apply without sequence gymnastics.
4. **Messages are keyed by session ID**, so the same player open in two tabs works — the originating
   session ignores its own echo. Cheap now, a protocol change later.
5. **Cross-instance fan-out via Postgres `LISTEN`/`NOTIFY`.** A global chat message from a player on
   instance 1 must reach instance 2. Postgres pub/sub handles this volume and keeps the dependency
   list at one datastore.

---

## 9. Accounts and sessions — DECIDED

**Account required up front. No anonymous play.** Onboarding is a real 2–3 minute flow, and it is in
V1 — this un-defers §3.13.

Forced by the social layer: leaderboards and peer endorsement require stable identity, and if the
chat flag is on (§11), **a public channel with strangers requires accountability** — a ban that costs
nothing to evade isn't moderation.

| Decision | Choice |
| --- | --- |
| Sign-in | **OAuth: Google + Discord** primary. Email/password addable later |
| Sessions | **Server-side, in Postgres**. `httpOnly` `Secure` `SameSite=Lax` cookies |
| Auth SaaS | **No.** Breaks the one-binary, one-datastore property; hard to remove later |

No password storage, and no anonymous→registered merge path to build.

**Onboarding includes** (per §3.13's constraints and §6): character selection from a curated portrait
roster, a starting situation, and the initial playbook rule selection. Every answer must visibly
change the starting state — not a form.

---

## 10. Frontend architecture

### 10.1 The React / Pixi boundary — DECIDED

**Two independent trees, one explicit bridge.**

- React owns a DOM tree. **Pixi owns a canvas and its own `requestAnimationFrame` loop.** They are
  siblings in the page, not nested.
- Both read the same plain TypeScript store — React via `useSyncExternalStore`, Pixi directly each
  frame.
- Canvas input dispatches through the same command layer React uses.
- **Pixi's lifecycle is owned outside React entirely.** Created at app boot, destroyed at teardown,
  never touched by a component's mount/unmount. React renders an empty positioned container and
  never looks inside it.

**The canvas never unmounts.** UI screens are overlays above it, not routes that replace it.
Unmounting would rebuild the WebGL context and texture cache on every navigation — visibly slow and a
source of GPU memory churn. It also matches the art bible's "the UI is part of the world, not a
dashboard bolted onto it": the world can stay visible and dimmed behind a panel.

**Rejected: `@pixi/react`.** Cadence mismatch — React's reconciler runs on state change, a renderer
wants a fixed rAF loop, and per-frame animation either bypasses React or pays diffing cost 60× a
second. The abstraction also leaks precisely where §3's art plan lives (custom filters, particles,
tweens, parallax), and it couples the renderer's version to a binding library's release cadence.

**Discipline rule: the store is plain TypeScript, never React state.** If world state lives in React,
Pixi can only read it at React's cadence, which reintroduces the rejected coupling by another route.

### 10.2 Client state — DECIDED

**One normalised client world-model in a vanilla store** (`zustand/vanilla`). REST hydrates it at
boot and on navigation; WebSocket messages apply typed patches; React subscribes via
`useSyncExternalStore`; Pixi reads it directly in the rAF loop.

1. **Patches are typed domain messages, not generic diffs.** `{type: "factors_updated", payload}`
   with one reducer per message type. Generic JSON-Patch against a client model is untyped,
   unreviewable, and produces production-only bugs. It also makes every socket message a deliberate
   API decision.
2. **The client never computes game rules.** It mirrors and renders. Any number the player sees is
   either sent by the server or derived by pure presentation code. This is what makes the deferred
   WASM prediction plan (§1) an *addition* rather than a reconciliation — there's no client-side
   rules code to conflict with.

**Rejected as the primary: TanStack Query.** Its model is pull — poll, refetch on focus, treat cache
as possibly-stale. Ours is push: the server states exactly what changed. A socket message saying
"money is now $4,200" would become "mark stale → refetch → receive the number you were already told."
It also lives inside React, so Pixi can't read it.

**Where a query library still fits:** read-only paginated lists — leaderboards, chat backscroll,
challenge history. Genuinely pull-shaped, never read by the renderer. Plain `fetch` plus the store
may cover it at V1 scale.

### 10.3 World representation — DECIDED

**Painted backgrounds with an invisible logic layer.** Not a tilemap.

Each location is a hand-corrected painted image — exactly what §3's art pipeline produces. Over it
sits a non-rendered data layer:

| Layer element | Purpose |
| --- | --- |
| Walkable polygon | Navmesh / waypoint graph for pathfinding |
| Depth-sort anchors | Sprites occlude correctly under the fixed 3/4 camera |
| Portals | Transitions between locations (home grid → interior → district → city map) |
| **Named trigger zones** | Where encounters fire — **the same objects as event placement rules (§4)** |

This is the classic adventure-game construction (the LucasArts lineage, Disco Elysium, Pillars of
Eternity), and it's the only option consistent with the locked art bible. Tiling a *painterly* city
produces visible repetition, contradicting "specificity over density" and "every scene needs at least
one detail implying a person lives there." The numbers agree: §3.12's "6–10 hand-built enterable
interiors per district" is a count of paintings, not a tileset.

**Encounter placement falls out for free.** §3.9's placement rules ("on foot downtown, at home late,
at work") *are* named trigger zones — a challenge's placement condition and a physical spot in the
world become the same object.

**Tiled is the authoring tool.** Image layer plus object layers gives a real visual editor for
polygons, portals, anchors, and trigger zones, exporting JSON that Pixi loads directly. **This avoids
building a level editor, which is otherwise a genuine trap in this design.**

### 10.4 Performance and the effects tier — DECIDED

**WebGL2 is the hard floor.** Pixi v8 removed v7's Canvas2D fallback — a device without WebGL2
doesn't degrade, it fails. WebGL2 is supported by ~97%+ of browsers in use, integrated graphics
handle 2D sprite rendering comfortably, and Chrome software-rasterises through ANGLE where no usable
driver exists. This is not a discrete-GPU requirement.

**The expensive part is the art plan, not the sprites.** §11's highest-ROI list — parallax, rim
lighting, particles, bloom, vignette — is full-screen filter work, and filters are fill-rate bound. A
thousand sprites is fine; three stacked full-screen shaders at 1440p on weak integrated graphics is
not.

**So effects are a switchable quality tier, and this must be true from the first scene:**

| Tier | Contents |
| --- | --- |
| **Low** | No full-screen filters, no particles, lighting baked into the source art |
| **High** | Full filter stack, real-time lighting, particles, post-processing |

Retrofitting "works without the lighting pass" into a look designed around the lighting pass means
redoing the look. **Ship Low first** — a game that looks right without the juice pass looks better
with it; the reverse is never true.

**Pixi types stay confined behind a thin scene interface** (`drawScene`, `moveCamera`, `playEffect`).
Good hygiene regardless, and it keeps a renderer swap reachable without a rewrite.

**Asset pipeline is the main frontend performance constraint.** Painted backgrounds at above-target
resolution are heavy. Required: compressed texture formats or high-quality WebP, per-location lazy
loading with adjacent-location preload, and a texture budget so the GPU never holds the whole city.

---

## 11. The social layer — DECIDED

**A persistent global chat box, behind a feature flag. No DMs in V1.** It rides the existing
WebSocket. No presence system.

**Not a core feature.** It ships dark, can be switched off if moderation load outgrows the value, and
nothing in the game depends on it.

**Chat sits outside the simulation.** It is a parallel social layer, not part of the world. Chat
messages create no relationship state, generate no events, and never touch the game model. **There is
no wire back into the game at all** — hidden factors are scored from committed choices (§7), not
from anything typed here.

Two consequences:

- **§3.7's deferral of *in-game* human interaction to V2 stands unchanged.** This is out-of-game
  chat, which is a different thing.
- **Chat can be built, deployed, and reasoned about entirely independently of the game** — which is
  what makes feature-flagging it honest rather than cosmetic.

**Moderation is rule-based.** Rate limiting, word filters, mute/ban, and report-and-review. No model
in the loop. Model-assisted moderation is an operational upgrade if volume ever demands it, not an
architectural change — but it is also a reason to keep the flag off until someone can watch the
channel.

---

## 12. Art production pipeline

The bottleneck isn't generating art — it's generating art that looks like it came from one game.

1. **Art bible first, locked, one page.** Camera angle, palette, lighting rules, line weight, detail
   level, character proportions. Every asset checked against it. (The bible itself is in §4.)
2. **Backgrounds:** generate above target resolution, hand-correct perspective. Reuse heavily — a
   life sim revisits the same locations constantly, and §3.12's single-city decision makes every
   interior high-traffic.
3. **Characters, two tiers:** high-fidelity *portrait* for dialogue (what the player stares at);
   simple *world sprite* that only needs to read clearly at small size. Onboarding (§9) needs a
   curated portrait roster, so this work arrives early.
4. **Style consistency:** LoRA trained on our own approved outputs, or locked seed + style reference.
   Revisit at ~20 approved assets.
5. **Motion sells more than resolution.** Dedicated juice pass — parallax depth, rim lighting,
   particles, breathing/blink idles, bloom, vignette, easing on every transition. Highest-ROI item in
   the art plan — and the contents of the High tier (§10.4).

**Note:** a life sim is UI-heavy — health tabs, schedule editors, playbook, job screens,
leaderboards. That UI needs as much art direction as the world does, and it is where most of the
player's time is actually spent.

---

## 13. Operations

| Concern | Decision |
| --- | --- |
| **Hosting** | Fly.io. Requirement is long-lived processes — not serverless, not scale-to-zero |
| **Migrations** | Plain SQL in `migrations/`, embedded in the binary, applied at startup. **Forward-only, never edited after merge** |
| **Config** | Environment variables, parsed once into a typed struct at boot. Fail fast on missing values |
| **Secrets** | Fly secrets. Never in the repo |
| **Logging** | `log/slog`, structured, JSON in production |
| **Deploy** | `npm run build && go build` → one binary → `fly deploy`. Schema follows automatically |

**Recommended, not yet decided:** error tracking (Sentry), and metrics for rollover job duration,
command latency, and WebSocket connection count. Worth having before the first real players, not
before the first playable build.

---

## 14. Testing strategy

Falls out of the architecture rather than being bolted on:

| Layer | Approach |
| --- | --- |
| **`internal/simulation`** | Table-driven unit tests. No database, milliseconds. **This is where tuning happens** (§17) |
| **Domain modules** | Integration tests against a real Postgres (Docker), one transaction per test |
| **Commands** | `httptest` against the handler, asserting state and response |
| **Rollover** | Injected `Clock`, asserting day-boundary behaviour across timezones |
| **Content** | Validation runs in CI — a malformed challenge fails the build, never production |

**There is nothing nondeterministic to test around.** With no runtime LLM (§7), a given input can
only produce one output — so a full playthrough is reproducible from a seed and a fixed clock, and
regressions in tuning show up as failing assertions rather than as a feeling.

---

## 15. Setup prerequisites

- Go 1.26.5 ✅ — version pinned in `go.mod`
- Node 26.6.0 ✅ · npm 11.18.0 ✅ — **Vite 8 requires ≥22.12**, which the previous 22.2.0 failed
- Postgres (local Docker for dev) — `compose.yaml`, started by `make db`
- Tiled (level authoring)
- Anthropic API key — **offline content authoring only**, not needed to run the game
- Fly.io account

---

## 16. Amendments required to `HighLevelOverview.md`

Where this document and the design document disagree.

**Applied:**

| § | Change |
| --- | --- |
| §3.14 | Rules are composed `when → then` from an unlocked vocabulary, not written as prose. Playbook rename completed |
| §3.10 | Free-text judgement → authored options carrying hidden factor deltas; free text noted as a V2 addition |
| §3.15 | Daily conflicts are authored pathways per environment, finite, with the content cost stated |
| §3.9 | Challenge selection is a deterministic server-side heuristic, not LLM-assisted, and reads no player-written text |
| §3.4, §3.6 | Interview is an authored challenge; character dialogue is authored trees. Live AI interview and generated dialogue deferred to V2 in that document's §5 deferral table |

**Outstanding — deliberately not applied:**

| § | Still says | Should say | Status |
| --- | --- | --- | --- |
| §3.7, §5 | Human-to-human interaction deferred to V2 | **In-game** interaction still V2. A global chat box ships in V1 **behind a feature flag**; no DMs | skipped |
| §3.13 | Onboarding deferred; V1 assigns a random avatar | Onboarding is **in V1** — 2–3 minutes, account required, includes initial playbook selection | skipped |
| §3.5 | Emotional stability grown by "behavior in interactions; **in-app chat history**" | Committed choices across scenarios. Chat feeds nothing (§7) | pending |
| §6.1 | "Accounts" listed as open | **Decided:** account required up front, OAuth (§9) | pending |
| §6.1 | "Chat scoring" listed as open | **Dissolved:** nothing the player writes is analysed (§7) | pending |

---

## 17. To settle during implementation

Design is decided; these are the numbers and structures that come out of building it.

- **The playbook condition/action vocabulary** (§6) — the largest remaining design artifact, since it
  defines the space of thinkable strategies.
- **Biomarker formulas** — how routines and actions move each metric, and how the panel composes into
  biological age (§3.16).
- **Karma weights** — how the factors combine, and the shape of the money curve past $1M (§3.5).
- **Karma decay curve** per environment, and how decay tracks mastery rather than elapsed time
  (§3.15).
- **Event release policy** — the heuristic deciding what surfaces, when, and how much after an
  absence (§4).
- **Earning cap fall-off** — hard stop or steep drop past 3 paid actions per day (§3.4).
- **Job requirements and experience routes** — the capability floor and the days of previous work
  each posting asks for, now authored per job in `content/jobs` (§3.14; tiers withdrawn 2026-08-15).
- **Challenge roster** — which 6–8 families, their ~4 variants each, and the 3–4 mechanic types they
  need (§5).
- **Relationship state model** — the per-character variables daily friction is generated from
  (§3.15, §6.1).
- **City identity** — which city, and its district list (§3.12).
