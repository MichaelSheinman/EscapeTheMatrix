# Ralph loop status

## Pass 1 — issue #9 (Routines) — DONE

Commit `0b482f5` on `main`. Sleep / workout / diet / organization configured as one
`jsonb` unit, feeding the pure core; `routine.set` + `routine.live` commands, a
14-day forecast, and an **R** screen in the client. `make check` green.

Notes for the next agent:

- **DESIGN.md does not exist.** The living spec is `Documents/HighLevelOverview.md`
  (game design, §-numbered) and `Documents/technical.md` (architecture, also
  §-numbered). `README.md` is the developer-facing doc and gets a prose section per
  module — follow that convention when you land one.
- **Do not stage `ui/`, `preview/` or `Documents/`.** The user works there in
  parallel. He committed his in-flight work as `a213f98 "Pre-ralph check point"`
  just before this pass, so the tree happened to be clean — do not rely on that.
  Stage explicit paths, never `git add -A`.
- **`ui/components.css` has uncommitted-at-times additions.** Only use CSS classes
  present in `git show HEAD:ui/components.css`. `.ask` and `.question` were *not*
  committed at the time of this pass; `.rule`, `.rule-line`, `.kw`, `.token`,
  `.locked-slot`, `.slots`, `.spark`, `.notice` were.
- **Adding a socket message** = define it with `realtime.Define` in `internal/api`,
  then `make generate` (rewrites `web/src/realtime/messages.gen.ts`), then add a
  reducer in `web/src/world/state.ts` — the reducer table is a mapped type, so it
  stops compiling until you do.
- **Adding a migration** = bump `const schemaVersion` in
  `internal/api/commands_test.go`. It is now `7`.
- **Postgres integration tests**: `make db` first. Never call `t.Fatalf` inside a
  `store.InTx` closure — `Goexit` skips the rollback and the run hangs until the
  pool gives up. `internal/routine/routine_test.go` shows the pattern: `Begin`,
  then `t.Cleanup(rollback)`.
- **Seams left for later tickets**: `#10` (daily rollover) calls
  `routine.Routines.Day(ctx, tx, account, on)` per missed day. The `#32` gate
  seam is closed, not pending: diagnosis is out of V1 and `routine.New()` takes
  no gate — don't re-open one.
- `simulation.Day` gained an `Order` field. Its zero value is neutral by design —
  keep it that way, or every hand-narrated day starts being charged for an
  arrangement it never described.

## Pass 2 — issue #10 (Daily rollover) — DONE

Commit `7b588d6` on `main`. Per-player rollover at each player's local midnight,
on a River queue running in the same binary; catch-up on sign-in and on
`GET /api/world`; dormancy gate; bounded 30-day window. `make check` green.

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. `README.md` gets a prose section per module — I added
  "The daily rollover".
- **`schemaVersion` in `internal/api/commands_test.go` is now `8`.** Bump it with
  every migration or `TestTheSchemaVersionIsWhatTheMigrationsAddUpTo` fails.
- **There are now TWO migration systems.** goose owns `migrations/`; River owns
  its own tables and its own ledger, run by `jobs.Migrate` at boot in
  `cmd/server` and again in `internal/store/storetest`. Do not put River SQL in
  `migrations/`.
- **`internal/store/storetest` imports `internal/jobs`.** That means any test
  compiled *into* `internal/jobs` cannot import storetest — `jobs_test.go` is an
  external test package (`package jobs_test`) for that reason. Keep `internal/jobs`
  free of game imports beyond `simulation` and `store`, or the cycle comes back.
- **`health.Lived` changed signature**: `(ctx, tx, account, on simulation.Date,
  day simulation.Day)`. It no longer takes a zone and no longer reads the clock
  for the date. Guard is `!lastDay.Before(on)` — strictly forward.
- **Seams left for later tickets**: `rollover.World` is a three-method interface
  (`Replenish`/`Decay`/`Recompute`) bound to `rollover.Unbuilt{}` in
  `internal/api/api.go`. #21 (backlog), #15 (Karma decay), #29 (environment
  ceilings) each replace that one argument — do not edit `internal/rollover`.
  `rollover.Announce` is how the module pushes to the socket without knowing
  view shapes; `internal/api/rollover.go` implements it.
- **The rollover never touches today.** It rolls through *yesterday*, so
  `routine.live` / `health.live` still own the day in progress. If you change
  that, `TestADayAlreadyLivedByHandIsNotLivedAgain` is where it shows.
- **Adding a socket message** still means `realtime.Define` → `make generate` →
  a reducer in `web/src/world/state.ts`. I added `rollover.ran`.
- **River is chatty at debug.** `internal/jobs` floors its logger at info
  (`quiet(log, slog.LevelInfo)`); the dev logger runs at debug, and without that
  the poll loop buries everything.
- **The user's parallel work under `preview/`, `ui/`, `Documents/` was clean
  this pass** (he had committed it). Do not rely on that — still stage explicit
  paths, never `git add -A`.

## Pass 3 — issue #11 (Tiled pipeline — one walkable painted location) — DONE

Commit `9ff905c` on `main`. One painted location authored in Tiled: a plate plus
an object layer the renderer reads and never draws — walkable polygon with
holes, depth baselines, named trigger zones, portals, spawn. Visibility-graph
pathfinding, depth by ground-contact y, `world.standing` reported into the
store. `make check` green (86 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §10.3 is the section this pass implemented. `README.md`
  gets a prose section per module — I added "The painted world".
- **Levels live in `web/public/locations/<key>/`** — a `.tmj` (Tiled JSON) and
  its plate. Vite copies `public/` into `dist/`, which the Go binary embeds, so
  a level is served with no build step of its own. `internal/api/assets.go` now
  registers `.tmj` as `application/json`.
- **`web/src/render/level/` has no Pixi in it** and must not gain any. It is
  `geometry.ts` (polygons/holes), `navmesh.ts` (visibility graph + A*),
  `tiled.ts` (the reader and the `Level` type), `locations.ts` (place key → map
  URL). Pixi stops at `render/scene.ts`, which is §10.4's thin interface —
  `drawScene`, `moveCamera`, `playEffect` all exist on it now.
- **The Tiled reader is deliberately strict.** Unknown/absent object class,
  rotation, ellipse, polyline, a second floor or spawn, an occluder outside the
  painting, a trigger with no `key` — all throw at load. If you add a new kind
  of authored object you MUST add its class to the switch in `readMap`, or every
  existing map stops loading the moment somebody draws one.
- **Pixi display objects work in Node.** Only `Application.init` needs a GPU, so
  `render/scene.test.ts` builds the scene, mounts the shipped level and walks it
  under Vitest. This is the pattern for testing anything in the renderer — use
  it rather than assuming the world layer is untestable.
- **Tests load the level that ships**, via `import x from '../../public/.../x.tmj?raw'`.
  Vite's `?raw` works from `public/` in Vitest and typechecks (`vite/client`
  declares `*?raw`). There is no `@types/node` in `web/`; do not reach for `fs`.
- **`world.standing` is the only store slice the server did not state.** Written
  by the renderer via `patch.standing`, only when it changes. It carries zone
  keys and portals — geometry, not rules. #21/#22 read from it; do not start
  deciding *what happens* in a zone inside `render/`.
- **The canvas dispatches no command right now.** A tap walks, which changes
  nothing server-side. §10.1's "canvas input goes through the same command
  layer" is still the rule and is documented as such — **#12 is where it comes
  back**, by wiring the authored portals to `character.move`. The portals are
  already loaded, typed and reported; `locations.ts`'s flat `PAINTED` lookup is
  what #12 replaces.
- **`AT_THE_DOOR` in `locations.ts`** is the location drawn before sign-in. The
  character sprite is hidden until `session.status === 'in'`.
- **Only `east-side` is painted.** Any other district draws bare ground and a
  "not painted yet" line — that is #35, not a bug.
- **The plate is placeholder art** generated once and committed (31 KB, flat
  colour, 1920×1088). #34–#39 replace it. If you regenerate it, the map's
  `imagewidth`/`imageheight` and every occluder frame are in plate pixels and
  must move with it.
- **Backquote toggles the logic view** (`renderer.showLogic`, bound in
  `main.tsx`). Useful for checking any level you author.
- No migrations this pass, so `schemaVersion` in `internal/api/commands_test.go`
  is still `8`.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still stage
  explicit paths, never `git add -A`.

## Pass 4 — issue #12 (Location graph and portals) — DONE

Commit `2a4e679` on `main`. The city as a graph the server owns: layers, places,
ways, breadth-first routes, a transit line with a flat fare, and portals that
are the *placement* of a way rather than the way itself. Walking onto a door
sends `character.move`; a second painted location (the apartment) makes the
whole thing walkable end to end. `make check` green (106 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.12 is the section this pass implemented. `README.md`
  gets a prose section per module — I added "Getting about".
- **`schemaVersion` in `internal/api/commands_test.go` is now `9`.** Migration
  `00009_places.sql` renames `character.district` → `character.place` (any place
  key, any layer) and moves `home` from `east-side` to `east-side-apartment`.
- **`character.New` now takes `(clock, character.Encounters)`.** Bound to
  `character.Quiet{}` in `internal/api/api.go`. **#21/#22 replace that one
  argument** — every walked step is already offered to it, inside the command's
  transaction. Do not edit `internal/character/travel.go` to add events; bind a
  real `Encounters` and put the rules in your own package.
- **`internal/city` is the new pure package.** Authored geography — places,
  layers, `streets`, `doors` derived from `In`, one transit `line`, and
  `Route`/`Cost`. It imports nothing from this repo but `simulation.Money`, and
  it must stay that way: it is the geographic counterpart of
  `internal/simulation`. Adding a district is two lines and **no migration**.
  It `panic`s at init for an unreachable place or a line calling somewhere with
  no stop, which is the right time for authored content to fail.
- **`character.MoveTo` is gone.** `character.Travel(ctx, tx, account, to, mode)`
  replaces it and returns a `Trip` (steps, fare, chances). `character.At` and
  `.Home` are `city.Place`, not the deleted `character.Place`.
- **The city map is a *view*, not a place.** Deliberate: a standable overworld
  would be a free teleport between districts, which is what the fare exists to
  buy. It is the **M** screen in `App.tsx`. If #31 (trips and relocation) wants
  another city, it wants another set of places, not a place called "the map".
- **`character.move` input is `{to, by}`** — `by` is `"foot"` or `"transit"`,
  required, never defaulted (one of them costs money). 422 for nowhere and for
  an unknown mode; **409** for a ride with no stop at this end and for a fare
  that cannot be paid.
- **`world.places` now carries the whole city**, each place with `layer`, `in`,
  `stop` and its own `ways`. The client reads it through `web/src/world/city.ts`
  — lookups and groupings only. **Do not add route-finding or fare arithmetic
  there**: a screen that priced a journey itself would eventually quote a price
  the server did not charge. Offer the ways the server sent, and ask for the rest.
- **Two painted locations**: `east-side` and `east-side-apartment`.
  `render/level/locations.ts` is no longer the location graph — it answers only
  "has anybody painted this", which is the one question the server cannot.
  #35 adds entries there as it adds paintings.
- **Walking onto a portal fires `onPortal` once**, guarded by the portal you
  arrived on. If you touch `scene.ts`'s `stopped()` / `onPortal` bookkeeping,
  `scene.test.ts`'s "walking onto a way out" block is where the loop comes back.
- **The renderer's LRU + preload live inside `startRenderer`** and are not unit
  tested — they need `Application.init`, which needs a GPU. `BUDGET` is 6 and
  eviction calls `Assets.unload`. If that logic grows, extract the holding
  policy into `render/level/` where it can be driven in Node.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still
  stage explicit paths, never `git add -A`.

## Pass 5 — issue #13 (Effects quality tier and asset pipeline) — DONE

Commit `25217c1` on `main`. Two switchable effects tiers — Low is the scene
unchanged, High is an object built over it (authored lights, bloom, grade,
grain, vignette, motes) — plus a frame governor that demotes a machine that
cannot hold it. Plates baked to WebP, content ETags for unfingerprinted assets,
and the texture budget extracted as pure arithmetic. `make check` green (169
client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §10.4 is the section this pass implemented. `README.md`
  gets a prose section per module — I added "Two tiers, and the plates
  underneath them".
- **Pixi now lives in three files**, not two: `render/renderer.ts`,
  `render/scene.ts`, `render/effects.ts`. `render/level/` still has none and
  must keep none — `holding.ts` (the texture budget) went there because it is
  arithmetic over strings.
- **`web/vitest.setup.ts` is new and is wired through `vite.config.ts`**, which
  now imports `defineConfig` from `vitest/config` rather than `vite`. It sets a
  Pixi `DOMAdapter` whose canvas has no WebGL context. **Constructing any Pixi
  `Filter` in Node needs this** — `GlProgram` asks the environment for a canvas
  to sniff shader precision. It is not jsdom and should not become jsdom.
- **`Tier` is now an overloaded word.** `world/state.ts` has `Tier` for
  instrumentation (§3.16) and imports §10.4's as `EffectsTier`. If you add a
  third meaning, alias at the import site rather than renaming either.
- **The tier table is `render/quality.ts`'s `EFFECTS`.** A new effect belongs in
  a row there and in `effects.ts`, never as an `if` inside `scene.ts` — that is
  the whole reason Low cannot drift from what it promises.
- **`Scene` gained `setQuality(tier)`.** The scene keeps three empty containers
  (`glow`, `air` in the camera; `edge` in the view) that `effects.ts` fills.
  Camera children are now `bare, plate, marks, floor, glow, air, logic` — a test
  that indexes into them is `scene.test.ts`'s `layers()`.
- **Plates are `.webp` and a test enforces it.** `tiled.test.ts`'s "ships its
  painting compressed" fails if any shipped map names a plate that is not
  `.webp`. #39 replaces the *contents*; the path is unchanged.
- **`internal/api/assets.go` now ETags everything outside `assets/`.** Lazily
  hashed, cached per path, safe because the embedded FS cannot change while the
  process lives. `assets/` still gets immutable year-long caching.
- **A new authored Tiled class: `light`.** Drawn as the area it reaches;
  optional `color` (`#rrggbb` or Tiled's `#aarrggbb`), `intensity`, `flicker`.
  `Level.lights` is read at Low too and simply not drawn. Per pass 3's rule: any
  further authored object class must be added to `readMap`'s switch or every map
  breaks the moment somebody draws one.
- **`Boot` gained `onFrames` and `onDemoted`**, wired in `main.tsx` to
  `patch.frames` / `patch.demoted`. The governor lives in the renderer and
  decides nothing about the world — it says "this machine cannot hold it" and
  the app patches the store.
- **The remembered tier is `localStorage['etm.quality']`**, read once in
  `main.tsx` *before* `startRenderer` so the first frame is drawn at the right
  tier. `patch.cleared` preserves `quality` through sign-out.
- **Not visually verified in a browser.** The browser tooling wanted a device
  selection an automated pass cannot make. Everything is covered by tests that
  build real Pixi objects, but the lit tier has not been looked at on a GPU. If
  you have a browser, that is fifteen minutes well spent — and the plates
  themselves are still #34–#39's placeholders.
- No migrations this pass, so `schemaVersion` in `internal/api/commands_test.go`
  is still `9`.
- The user's `preview/`, `ui/` and `Documents/` were dirty this pass and were
  left alone. Stage explicit paths, never `git add -A`.

## Pass 6 — issue #15 (Factor module — Karma and the hidden factors) — DONE

Commit `11ce3f5` on `main`. Progression with no level bar: five factors composed on
read into one Karma, two revealed and three that never cross the wire; money's
curve flattening past $1M; §3.15's per-environment decay wired into the rollover.
`make check` green (175 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.5 and §3.15 are the sections this pass implemented.
  `README.md` gets a prose section per module — I added "Where you stand".
- **`schemaVersion` in `internal/api/commands_test.go` is now `10`.** Migration
  `00010_factors.sql` adds `factor_credit` and `factor_tenure`.
- **`rollover.Unbuilt{}` is gone from `internal/api/api.go`.** It is now
  `work{factors: factors}`, a struct in `internal/api/rollover.go` implementing
  `rollover.World` by composition. **#21 fills in `Replenish`, #29 fills in
  `Recompute`** — one method each, in that file, and neither touches
  `internal/rollover`. Do not re-bind the whole interface.
- **`internal/simulation/factor.go` is where every progression number lives.**
  `Standard()` is a *function* on purpose — a package var could be reassigned and
  the core is deterministic. `Weights` is a value, so a tuning pass is
  `Weights{...}.Read(standing)` in `factor_test.go`.
- **Naming collision to know about:** the money factor's Go constant is
  `simulation.Wealth`, because `simulation.Money` is already the type of an
  amount. Its `Key()` is `"money"`. `simulation.Health` is the factor, not the
  package.
- **The veil is in `internal/factor`, not in `internal/api` and not in React.**
  `Factors.Of` returns `[]Slot`; the three hidden ones have empty key, empty
  name and a nil value. `internal/api/factor_test.go` reads the *whole world
  snapshot as a string* and fails if `intelligence`/`stability`/`kindness`
  appears anywhere. If you add any view that touches factors, that test is what
  catches you.
- **There is deliberately no factor command**, and a test asserts four plausible
  names all 404 (§7: scored from committed choices, never from typing). **#17
  (endorsement), #24 (challenges), #30 (conflicts) call
  `factors.Record(ctx, tx, account, simulation.Grant{...})`** inside their own
  transaction. `Record` applies familiarity itself — do not scale rewards at the
  call site. Negative grants are allowed and are *not* discounted.
- **`health.Health` gained `Body(ctx, tx, account)`** — a narrow read returning
  just `simulation.Body`, for `factor.Panel`. `character.Characters` already
  satisfied `factor.Purse` via `Balance`.
- **`factor.Environment` is bound to `internal/api`'s `district`**, which is the
  player's district (or the district a place sits `In`). **#29 replaces that one
  binding** with real populations. Wear is kept per environment key, so returning
  to a worn-out place does *not* refresh it — that is deliberate.
- **`simulation.Ceilings` exists, is applied to hidden-factor readings, and is
  always the zero value today.** Zero means *no* ceiling (same convention as
  `simulation.Order`). #29 authors them; `Factors.Of` is where they get passed in.
- **New socket message `factor.stood`, published with NO `Origin`.** Karma is a
  reading of everything at once, so no command's response carries it and the
  acting tab has to hear it over the socket like everyone else. `s.stood(ctx, tx,
  account)` is called at the tail of `health.live`, `health.buy`,
  `character.work`, `character.move`, `routine.live` and from `Rolled`. **A new
  command that moves money or the body must call it too** — there is no hook in
  `internal/command` doing it for you.
- **The client's `stood` was already taken** (the renderer's `Standing`), so the
  Karma reducer is `composed(world, karma)` in `web/src/world/state.ts`. The
  screen is `Standing` in `App.tsx` under the **K** key, using the already-
  committed `.slots` / `.locked-slot` / `.decay-row` / `.env-chip` classes.
- **Leaderboards are #16 and were not touched.** §3.5 describes a distributed
  board (one overall, one per factor) — note that a per-factor board for a hidden
  factor would undo this whole pass, so read §3.5 carefully before shipping one.
- **Prettier is not enforced in this repo.** `make check` runs `tsc`, not
  `prettier`; existing committed files fail `prettier --check`. Do not run
  `prettier --write` over the tree.
- The user's `preview/`, `ui/` and `Documents/` were clean by the time this pass
  committed (they were dirty at its start). Still stage explicit paths, never
  `git add -A`.

## Pass 7 — issue #16 (Leaderboards) — DONE

Commit `5332a72` on `main`. §3.5's distributed board — one overall plus one per
factor, hidden ones included — rebuilt by a River periodic job every five
minutes, paginated GETs, and `board.moved` over the socket. `make check` green
(181 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.5 (the board) and §3.4 (the job type) are what this pass
  implemented. `README.md` gets a prose section per module — I added "The boards".
- **`schemaVersion` in `internal/api/commands_test.go` is now `11`.** Migration
  `00011_boards.sql` adds `factor_board` and `factor_board_refresh`.
- **The leaderboards are inside `internal/factor`, not a package of their own** —
  technical.md's module map says so, and the reason is load-bearing: ranking a
  hidden factor means *reading* it, so the sort happens inside the veil and only
  a rank comes out. Do not move them.
- **The veil on this path is three separate structural facts**, all tested:
  sealed boards are keyed `slot-3`/`slot-4`/`slot-5` (never the factor's name, so
  `/api/boards/kindness` is a **404**); `factor_board.value` is NULL for them, so
  a hidden *reading* is stored nowhere in the schema; and `internal/api/board_test.go`
  reads every board payload as text and fails on `intelligence`/`stability`/`kindness`.
  **The `board` column stores the wire key**, deliberately — a table whose rows
  could be selected straight into a response should not contain the word "kindness".
- **`factor.New` now takes four arguments** — `(Panel, Purse, Environment, Roster)`.
  `factor.Alone{}` is a Roster of nobody, for tests. The real one is `roster{accounts}`
  in `internal/api/board.go`.
- **`factor.Purse` is now bound to `purse{characters}` in `internal/api/factor.go`**,
  not to `characters` directly. It calls `characters.Ensure`, because the rebuild
  composes progression for *every* account and one account whose first wake failed
  would otherwise take every board down with it. Do not un-bind it.
- **`account` gained `Profile`, `Everyone` and `Profiles`.** `Profile` is name +
  avatar and no email, on purpose — a board is a public surface. Account ids are
  not sent to the client either (#17's endorsement should not inherit a handle
  from a list anybody can page through).
- **`jobs.New` now takes a fourth argument**, `jobs.Boards` — bound to `app`
  (`*api.Server`, method `RefreshBoards`) in `cmd/server`. There is a second queue
  (`boards`, one worker) and a River `PeriodicJob` at `jobs.Cadence` (5 min) with
  `RunOnStart`. **River requires `available`, `pending`, `running` and `scheduled`
  in any custom `UniqueOpts.ByState`** — omitting `running` made the insert fail
  silently and cost me a debugging round.
- **New socket message `board.moved`, published with NO `Origin`**, and only to
  accounts that actually moved — an unchanged rebuild publishes nothing, which is
  what keeps this from being a five-minute broadcast to everybody. Reducer is
  `ranked(...)` in `web/src/world/state.ts`.
- **The boards are deliberately NOT in the world snapshot.** §10.2 names them the
  one pull-shaped surface in this client. `world.boards` is null until the **L**
  screen fetches `/api/boards`; a *page* of other people is component state, not
  world state (`loadBoardPage` returns it rather than patching). A test asserts
  the snapshot has no `boards` key — do not "helpfully" add one.
- **The rebuild is O(players) queries** — `panel.Body` and `purse.Balance` per
  account, through the same narrow interfaces a single standing uses, plus one
  bulk read of `factor_credit` (this module's own table). That is the honest cost
  of §3.1's boundary and is why it runs on a schedule. A population that outgrows
  it wants a *windowed* rebuild, not a join into somebody else's tables.
- **Ties break on (score, account id)** so a rank is a position rather than a
  score class. `TestTiesDoNotFlapBetweenRebuilds` is where that is held.
- **Screen keys are now s/h/r/m/k/l.** `l` is the boards. The CSS is the already-
  committed `.board` / `.rank` / `.board.you` / `.tabs a.active` vocabulary — it
  was already in `ui/components.css` and fits a leaderboard exactly.
- **#17 (peer endorsement) is the natural next one here**: it calls
  `factors.Record(...)` for Kindness, and §3.5 pairs it with leaderboards
  explicitly. It will want a way to address another player that the board
  deliberately does not hand out.
- The user's `preview/`, `ui/` and `Documents/` were clean by the time this pass
  committed. Still stage explicit paths, never `git add -A`.

## Pass 8 — issue #17 (Peer endorsement) — DONE

Commit `27174c9` on `main`. §3.5's "I like this user" as an **offer** rather than a
button: the server names one other player now and then, four independent
anti-farming rules (two of them schema constraints), the credit landing on the
player who was liked, and `social.liked` — the first message in this game that is
literally §8's "another player's action affecting you". `make check` green (185
client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.5's "Notes" bullet on peer endorsement and §5's V1/V2
  table are what this pass implemented. `README.md` gets a prose section per
  module — I added "Saying you like somebody".
- **`schemaVersion` in `internal/api/commands_test.go` is now `12`.** Migration
  `00012_endorsement.sql` adds `endorsement` and `endorsement_offer`.
- **`internal/social` is the new module** — technical.md's module map calls it
  "peer endorsement, leaderboard identity". It owns its two tables, takes the
  caller's transaction, and reaches out through exactly two interfaces:
  `social.Crowd` (who is playing) and `social.Credits` (where the grant goes).
  Both are bound in `internal/api/social.go` as `crowd{accounts}` / `grants{factors}`.
- **`social.Credits.Record` returns only an `error`, deliberately.**
  `factor.Record` hands back the *recipient's* composed standing, and
  `internal/social` must not be able to see it — the endorser is mid-command and
  a standing that reached this module could reach their response. Do not widen
  that interface.
- **Two rules are enforced by the schema, not by Go**: `CHECK (from_account <>
  to_account)` and `PRIMARY KEY (from_account, to_account)`. If you touch
  candidate selection, those are the backstop and `social_test.go`'s `refuses`
  helper is how they are asserted — **inside a savepoint**, because a constraint
  violation aborts the whole transaction and a second bare check would then pass
  for the wrong reason.
- **An offer is drawn by `GET /api/world` and by nothing else.** `s.looked` in
  `internal/api/social.go`, called inside the snapshot's transaction. There is no
  job. If you add one, the cadence stops meaning what it says — a fortnight away
  would accrue a fortnight of strangers.
- **The world snapshot now publishes a socket message** (`social.offered`), and
  only when a draw actually happened. That is new for a GET; the precedent is
  `s.wake` → `rollover.ran`. A message per page load would be the polling §8
  exists instead of, and `internal/api/social_test.go` asserts exactly one for
  two page loads.
- **Three new socket messages**: `social.offered` (no origin), `social.answered`
  (origin = acting tab), `social.liked` (no origin, **addressed to a different
  account**). The last one is the first cross-player message in the codebase —
  if you publish to somebody else's account, publish their *whole* restated view
  rather than a delta, and remember `factor.stood` for them too.
- **The endorser's own Karma does not move**, and a test asserts it. §7: a factor
  a player can award themselves measures the button. If a future ticket wants to
  reward the giver, that is a design change, not a fix.
- **Candidate ordering is fewest-endorsements-received first**, tie-broken by
  roster order (oldest account). It is O(players), like the board rebuild, and for
  the same reason: the account table is not this module's to select from. A
  population that outgrows it wants a windowed pool, not a join.
- **Tuning knobs are `social.Cadence` (20h), `Window` (8h), `Cap` (5), `Span`
  (7d), `Credit` (30), `Remembered` (5)** — constants, not config, because a
  server running a different cadence would be a different game and the boards
  those populations share would not mean the same thing.
- **The client slice is `world.endorsement`, and it IS in the boot snapshot** —
  unlike `world.boards`. An offer plus a few names is the player's own world; a
  board is a page of other people (§10.2). `Snapshot` in `web/src/world/state.ts`
  is hand-written, so it had to be updated by hand.
- **The UI lives on the boards screen (`L`)** — `Somebody()` in `App.tsx` — and
  `BoardsHint()` changes the key-bar label when an offer is waiting, because the
  shell has no notification area and an offer lapses in eight hours.
- **CSS used is all committed vocabulary**: `.panel*`, `.ask` / `.ask-n` /
  `.ask-text` / `.ask-note`, `.btn` / `.btn small` / `.btn-row`, `.note first` /
  `.note last`. (`.note`, `.first`, `.last` have no rule in `ui/` at all — App.tsx
  has always used them that way; not something this pass introduced.)
- **#33 (onboarding) is the natural next one**, or #21/#22 (events) which are the
  larger seam — `character.Quiet{}` and `rollover.work.Replenish` are both still
  waiting on them.
- The user's `preview/`, `ui/` and `Documents/` were clean by the time this pass
  committed. Still stage explicit paths, never `git add -A`.

## Pass 9 — issue #18 (Playbook data model) — DONE

Commit `7616bc8` on `main`. §3.14's playbook as a data model: a rule is a sentence
composed from an authored vocabulary loaded out of `content/`, at most two
conditions with no nesting, rule sets scoped per environment, per-player unlocks,
and no draft/save anywhere. `make check` green (188 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.14 (the playbook), §3.15 (rule sets), technical.md §6
  (why structured beats free text) and §5.2 (content model) are what this pass
  implemented. `README.md` gets a prose section per module — I added "The
  playbook, which is rules for judgement".
- **`schemaVersion` in `internal/api/commands_test.go` is now `13`.** Migration
  `00013_playbook.sql` adds `playbook_rule` and `playbook_unlock`.
- **There is now a `content/` Go package at the repo root**, holding `//go:embed
  playbook` — the same trick `migrations/` uses, because embed cannot reach
  upward. technical.md's module map calls it `internal/content`; it *cannot* live
  there and hold the files, so the package sits beside the data. It knows nothing
  about what any file means — the module that owns a kind of content parses its
  own (§3.1). Add `//go:embed <dir>` lines as new content kinds land.
- **New dependency: `gopkg.in/yaml.v3`.** First non-infrastructure dep in the
  tree. §5.2 names `.yaml` explicitly, and `decode` in `vocabulary.go` sets
  `KnownFields(true)` so a misspelt field is a failed build. Use that helper for
  any further content loading rather than `yaml.Unmarshal`.
- **The vocabulary is validated in `init()` and panics**, exactly like
  `internal/city`. `parse(fs.FS)` is the testable half — `vocabulary_test.go`
  drives it over `fstest.MapFS`. **If you add a field to a content file you must
  add it to the `*File` struct**, or strict decoding refuses the whole tree at
  boot and every test in the repo fails at once.
- **YAML flow mappings and commas.** `says: say yes, this once` inside `{ ... }`
  is two fields. Every `says` in `content/playbook/` is quoted for that reason;
  keep it that way when authoring.
- **Term ids are `question/kind/key`** (`help/condition/fee`). Keys are unique
  only inside one pool of one question — `voice` has both a condition and an
  action called `leave`, deliberately, and it is the case that would break a flat
  term table. `playbook_unlock.term` stores the full id; the rule columns store
  the bare key, because the question is already a column.
- **The starting vocabulary is `start: true` in content and is NOT written to the
  table.** `playbook_unlock` holds only what was *earned*, so adding a starting
  term reaches players who already exist. Do not "seed" it on first read.
- **`playbook.Where` is bound to `districts{}` in `internal/api/playbook.go`.**
  #29 replaces that one binding with real populations, exactly as it replaces
  `factor.Environment`'s. Do not edit `internal/playbook` for it.
- **`Playbooks.InForce(ctx, tx, account, environment)` is the seam #19 reads** —
  complete rules only, in authored question order, which is what makes "the first
  one checked wins" sayable. `Playbooks.Unlock(ctx, tx, account, ids...)` is the
  seam struggle-gating calls (#25, #28, #32); there is deliberately no command.
- **A blank is a first-class state.** `Ref{}` is a hole, `Sentence.Complete()` is
  the only thing that decides whether a rule runs, and `conditions` is stored
  rather than inferred — one condition is a complete sentence, two with the
  second blank is a player mid-edit. If you touch `resolve`/`settled`, that
  distinction is what `TestASecondConditionStillBlankKeepsTheRuleOutOfForce`
  holds.
- **Stored parts are re-resolved against the vocabulary on every read.** A term
  renamed out of content, or one the player turns out not to have, reads back as
  a blank rather than as itself — the rule stops being in force and the question
  comes to the player again. That is the safe direction and it is tested.
- **The veil is `Question.only(unlocked)` inside `internal/playbook`**, and
  `internal/api/playbook_test.go` reads the whole world snapshot as text and
  fails if a locked term's `says` appears. Value lists are *inlined per term* on
  the wire for this reason — a shared table of thresholds would say that a term
  nobody has unlocked exists. Do not "optimise" that into one table.
- **New socket message `playbook.composed`** (origin = acting tab). Reducer is
  `ruled(...)` in `web/src/world/state.ts`; the store patch is `patch.playbook`
  and the action is `composeRule` in `world/actions.ts`. `world.playbook` **is**
  in the boot snapshot (unlike `world.boards`) — `Snapshot` in `state.ts` is
  hand-written and had to be updated by hand, and `state.test.ts`'s `snapshot`
  fixture too.
- **No screen was built. #20 is the playbook UI** and owns it —
  `preview/src/panels.js`'s `builder` is the reference implementation the ticket
  points at, and everything it needs is now on the wire: `{value}` survives
  intact in `says` so a menu can go *inside* the sentence, `n` is the authored
  question number, and `inForce` is stated rather than derived.
- **#19 (matcher) is the natural next one.** Value options carry an `Amount` and
  their list carries a `Unit` and a `Compare` (`at-least`/`at-most`/`quantity`)
  — deliberately server-side only, so the matcher needs no content migration and
  the client cannot predict it.
- The user's `preview/`, `ui/` and `Documents/` were clean by the time this pass
  committed. Still stage explicit paths, never `git add -A`.

## Pass 10 — issue #19 (Deterministic matcher and rule-fire records) — DONE

Commit `0e3bf9e` on `main`. §3.14's "it happens without you": a pure matcher over the
rules in force where the player is standing, three-valued conditions, cross-question
disagreement detected before anything fires, and rule-fire records that keep the
sentence as it stood. `make check` green (188 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.14 (the matcher), §3.6 (diagnosis evidence), technical.md §4
  (three escalation conditions) and §6 (why structured beats free text) are what this
  pass implemented. `README.md` gets a prose section per module — I added "When a rule
  fires", and trimmed the "the matcher is not here" paragraph out of the playbook one.
- **`schemaVersion` in `internal/api/commands_test.go` is now `14`.** Migration
  `00014_playbook_fire.sql` adds `playbook_fire` and `playbook_fire_rule`.
- **`internal/playbook/match.go` is pure and a test enforces it.**
  `TestTheMatcherIsPure` parses that one file and fails on any import with a dot in
  it, plus `time.Now`/`Since`. It could not go in `internal/simulation` — that
  package imports *nothing*, and a matcher is defined over the authored vocabulary.
  If you add matcher logic, it goes in that file or it goes somewhere that has to
  justify itself.
- **Actions now carry an authored `stance`** (`yield`/`resist`/`bargain`/`defer`/
  `open`/`withdraw`) in `content/playbook/questions/*.yaml`. It is the only field two
  questions have in common, and it exists because **a moment can put more than one
  question at once** — that is the only way two rules ever meet, since there is one
  rule per question per environment. Adding a question means giving every action a
  stance and offering **two distinct postures among the starting actions**, or `parse`
  panics at boot. `Stance` never crosses the wire, like `Value.Amount`.
- **Conditions are three-valued** (`Met`/`Unmet`/`Unsaid`, zero value `Unsaid`) and
  the join is Kleene logic. Silence does not spread: `and` is unmet the moment any
  condition is. Do not "simplify" this to a bool — the whole point is that a moment
  which never mentioned a fact must not fire somebody's `otherwise`.
- **`Decide(rules, situation)` executes nothing.** It returns a `Decision`; the caller
  carries out the action and then calls `Record`. That is what makes "detected before
  either fires" structural rather than careful.
- **The seam #28/#30 call** is `Playbooks.Weigh(ctx, tx, account, environment, s)` →
  act → `Playbooks.Record(ctx, tx, account, at, environment, decision, outcome)`, and
  `Playbooks.Settle(...)` for an outcome known later (once — a second settle is
  `ErrNoSuchFire`). `at` is passed in, not read from a clock, because the rollover
  replays days. **Nothing generates situations today**, so the log is always empty in
  a running server; that is #30, not a bug.
- **A `Situation` is built with `Arises(key, questions...)` then `.So(termID, bool)` /
  `.Reading(termID, amount)`.** Facts are addressed by the *full* term id
  (`help/condition/fee`), the same id space unlocks use. `Weigh` refuses a situation
  with any `Faults()` (unknown term, or a numeric term stated as a plain truth) —
  `Decide` itself stays total and treats them as unsaid.
- **There is no socket message for a fire.** Deliberate: there is no publisher. When
  #30 lands it will want one (a rule firing while the player is away is the clearest
  push in the game), and adding it is `realtime.Define` → `make generate` → a reducer
  in `web/src/world/state.ts`.
- **`GET /api/playbook/log?page=N` is the second pull-shaped surface**, after the
  boards. A test asserts the world snapshot carries no log — history is unbounded and
  a world model is not. `internal/api/playbook_test.go`'s
  `TestTheLogCarriesNoAmountAndNoPosture` reads the whole body as text and fails if a
  threshold amount or a stance key appears; that is the veil test for this surface.
- **No client screen.** #20 owns the playbook UI and now has both halves on the wire —
  the builder's vocabulary and the log's `says`/`did`/`met`/`held` per weighed rule.
- **`Foresee(rules, puts...)` is the static analysis** (`Playbooks.Foresaw` scopes it
  to one rule set). It needs the *questions a moment would put*: without them, every
  pair of rules is trivially "could disagree" and the answer is noise. #30's authored
  situations declare which questions they raise, which is the input this wants.
- The user's `preview/`, `ui/` and `Documents/` were clean by the time this pass
  committed. Still stage explicit paths, never `git add -A`.

## Pass 11 — issue #20 (Playbook UI) — DONE

Commit `8919e3d` on `main`. §3.14's screen: the editor is the sentence, every part
opening a short list in place; environment tabs above everything; statically
detected conflicts surfaced beside the rules; `playbook.clear` for taking an
answer back; the log fetched on open with §3.6's weekly counts. `make check`
green (189 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.14, §3.15, §3.6 and technical.md §6/§10.2 are what this
  pass implemented. `README.md` gets a prose section per module — I added
  "Composing one, and reading back what it did", and replaced the "the screen is
  #20" pointer at the end of the playbook section.
- **No migrations this pass**, so `schemaVersion` in `internal/api/commands_test.go`
  is still `14`.
- **`playbook.clear` is a second command and it is not a compose with nulls.** A
  stored sentence full of blanks is a rule left unfinished; no row is a question
  never answered (`opening()` in `internal/playbook/playbook.go`). Both are out of
  force, only one is true, and `TestClearingAnAnswerIsNotTheSameAsBlankingIt` is
  where that distinction lives. Clearing something nobody answered is a 200.
- **`ruleSetView` now carries `conflicts` and `idle`**, composed from the new
  `playbook.Set.Audit()` (one line in `match.go` over the existing pure `Foresee`).
  It runs with *no* `puts`, i.e. every question the set covers, because nothing
  authors situations yet — **when #28/#30 land they should pass the questions their
  moments raise**, and `Foresaw(ctx, tx, account, environment, puts...)` already
  takes them. The explanatory sentence is written by `clash`/`idle` in
  `internal/api/playbook.go`, never by the client.
- **The audit's veil is the same as the log's**: no stance, no amount.
  `TestTheAuditNamesNoPosture` reads the whole world snapshot as text. If you add
  anything to the audit, that test is what catches a posture leaking.
- **The screen is `PlaybookScreen` in `web/src/App.tsx` on the `P` key**, with
  `Choices` / `Meeting` / `Answer` / `Record` / `Moment` / `PlaybookHint` beside
  it. `Screen` and `KEYS` gained `'playbook'`; screen keys are now s/h/r/p/m/k/l.
  The CSS is all committed `@etm/ui` vocabulary (`.sentence`, `.clause`,
  `.editable`, `.blank`, `.num`, `.menu`, `.menu-item`, `.menu-rule`, `.question`,
  `.ask`, `.log-row`, `.tabs`, `.notice`) — `preview/src/panels.js`'s `builder` is
  the reference rendering and it matched without a single new class.
- **Menu state is one string in component state** (`'c0'`, `'num:c0'`, `'join'`…).
  The editables and menus `stopPropagation`; the overlay body's own click clears
  it. `say()` returns early while a command is in flight — a second pick would
  compose from the sentence as it stood before the first answer landed.
- **Choosing a parameterised term fills in its first value.** The server refuses a
  term that takes a number with none, so a menu that did not would be offering a
  choice that does nothing. `HOLE = '{value}'` in App.tsx is the one string this
  client shares with the vocabulary, and it is shared so the number can be a menu
  *inside* the sentence.
- **`PlaybookLog` / `Fire` / `Considered` / `PlaybookTrend` are types in
  `state.ts` but NOT slices of `World`** — same as `BoardPage`. History is
  unbounded; `loadPlaybookLog` returns it and `Record` holds it in component state.
  Do not "helpfully" put the log in the snapshot.
- **Nothing generates moments yet**, so the log panel is always empty in a running
  server — that is #30, not a bug. When #30 lands, a socket message for a fire is
  the obvious addition (`realtime.Define` → `make generate` → a reducer), and the
  screen would want to re-fetch on it.
- **Not looked at in a browser.** Sign-in is Google OAuth with no dev bypass, and
  `web/` has no DOM test setup — the client suite is pure reducers plus
  Pixi-in-Node, and adding jsdom/testing-library is a dependency decision rather
  than part of this ticket. The markup typechecks and builds; it has not been seen
  rendered. Same standing caveat pass 5 left about the lit effects tier.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass (he had
  committed them). Still stage explicit paths, never `git add -A` — and note that
  `npm run build` inside `web/` deletes `web/dist/.gitkeep`; `make` restores it,
  a bare npm build does not.

## Pass 12 — issue #21 (Event backlog and lifecycle) — DONE

Commit `b536cd2` on `main`. §4's backlog: things generated ahead of demand, released
by a server decision rather than a timer, placed only where authored rules allow, and
never destroyed by being declined. `make check` green (193 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §4 (the whole section), §3.9, §3.12 and §10.3 are what this
  pass implemented. `README.md` gets a prose section per module — I added "What the
  world has ready for you".
- **`schemaVersion` in `internal/api/commands_test.go` is now `15`.** Migration
  `00015_events.sql` adds `event_backlog` and `event_record`.
- **`internal/city` gained zones.** `Place.Zones`, a *closed* vocabulary (`zones` map
  in `city.go`, validated at init), plus `city.Zone`/`ZonesIn`/`Spot`. This is §10.3's
  "placement conditions and trigger zones are the same objects": the city names the
  spots, the paintings draw them, events are authored against them. Adding a place
  now **requires** zones or init panics. `internal/city/zone_test.go` reads every
  shipped `.tmj` and fails on a trigger key the city does not name — that test is what
  catches a painter renaming a trigger and silently taking a family of events out of
  the game. It is one-directional on purpose (unpainted places have zones already).
- **`character.Quiet{}` is gone from `internal/api/api.go`.** The walk's encounter
  surface is now `walks := &encounters{}`, a pointer bound *before* `character.New`
  and completed after the server exists (`walks.events, walks.announce = backlog,
  s.offered`). That knot is construction-only: internal/event needs the character's
  purse and internal/character needs the backlog. Do not try to untie it by moving
  `character.New` — it needs a value at construction.
- **`rollover.work` now has `Replenish` filled in** (in `internal/api/event.go`, not
  rollover.go — each module's binding lives beside its own transport). **`Recompute`
  is the last stub; #29 fills it.**
- **#22 (release policy) is the natural next one and its seam is ready.**
  `event.Policy` is one method over `event.Attention` (standing / released in a
  rolling day / since the last / kind / offered+passed of that kind in a fortnight).
  It runs **after** placement and revalidation, deliberately — a policy can never
  place an event, only refuse the moment. `event.Steady{}` is bound today and reads
  neither the decline signal nor time away; both are on `Attention`, unread. Time away
  and "what would serve the player next" need internal/account's activity, i.e.
  widening that struct and binding a second interface. Replace the third argument to
  `event.New` in `api.go`; do not edit `internal/event/event.go` for it.
- **`Events.Resolve(ctx, tx, account, id, at)` is the seam #23/#28/#30 call.** An
  event authored `awaits: true` stays `accepted` with a pending record until then.
  `content/events/a-shift-going-spare.yaml` is the one that exercises it.
- **Content lives in `content/events/*.yaml`** and `content/content.go` now embeds
  `playbook events`. Strict decoding, so a new field means adding it to `eventFile`
  in `catalogue.go` first. The loader validates the catalogue *as a whole*: a
  successor naming nothing, a cycle, two events sharing a successor, and a cooldown
  on an event that is only ever chained are all failed builds. `Event.Chained` is
  **derived** (something else names it as `then`) — never author it.
- **Declining is four different words**, and keeping them apart is the ticket:
  `declined` (button), `left` (walked away), `lapsed` (window passed), `dropped` (the
  world gave up, never shown). All but `dropped` return the row to `waiting`. If you
  touch `unoffer`, `TestDecliningReturnsTheEventToTheBacklog` asserts the *same row
  id* comes back after `Rest` — a requeue that made a new row would be consumption
  with extra steps.
- **The backlog never crosses the wire**, and `internal/api/event_test.go`'s
  `TestTheWorldSnapshotCarriesNoBacklog` reads the whole body as text to hold that.
  Do not add a "3 waiting" count to any screen — §4's sandbox depends on not being a
  quest list. `world.events` is `{offer}` and a client test asserts it has exactly
  that one key.
- **`GET /api/events/log?page=N` is the third pull-shaped surface**, after the boards
  and the playbook log. History is unbounded; it is not in the snapshot.
- **Two new socket messages**: `event.offered` (**no** Origin — the release is the
  world's decision, not the walking tab's) and `event.answered` (Origin set).
- **Screen keys are now s/h/r/p/e/m/k/l.** `e` is `EventsScreen` in `App.tsx`, with
  `EventsHint` and `Happened` beside it. All committed `@etm/ui` classes (`.ask`,
  `.log-row`, `.tag`, `.btn`, `.panel*`, `.note first`) — no new CSS.
- **The offer window is 45 minutes** (`event.Window`) and `Rest` is 6 hours, so an
  api-level test cannot wait one out; move the clock in `internal/event`'s tests
  instead (`clock.Stepping`). The api tests use the system clock and only assert
  transport.
- **Not looked at in a browser.** Same standing caveat as passes 5 and 11 — OAuth
  sign-in with no dev bypass and no DOM test setup in `web/`.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass (he had
  committed them). Still stage explicit paths, never `git add -A`.

## Pass 13 — issue #22 (Event release policy and escalation) — DONE

Commit `04ce26d` on `main`. §4's release decision (`event.Attentive` replaces the
`Steady` floor) and its other half: escalation on all three conditions, delivered
through the ordinary offer surface. `make check` green (193 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §4 is what this pass implemented, plus §3.2's "no numbers
  leave", §3.16 (why Health maps to no event family) and §3.15.
- **`schemaVersion` in `internal/api/commands_test.go` is now `16`.** Migration
  `00016_release.sql` adds `account.last_away`, `playbook_fire.raised_at` and a
  partial index whose predicate is `playbook.Because` restated in SQL. If you add
  a fourth escalation condition, change **both** or the world will decide on
  escalations it can never find.
- **`event.New` takes four arguments now**: clock, `Purse`, `Bearings`, `Policy`.
  `Bearings` is `Away` + `Lagging`, bound in `internal/api/event.go` as
  `bearings{accounts, factors}`. `event.Here{}` is the do-nothing double for
  tests about placement.
- **`rollover.World` gained `Escalate`**, and it is the one piece of the day that
  runs **before** `Advance`'s "is a day due" early return. A test double for
  `World` needs the extra method (`internal/rollover/rollover_test.go`'s
  `recording`).
- **`Playbooks.Settle` now returns `(Escalation, bool, error)`.** #28 and #30 call
  it when a job situation or a conflict settles; the bool is §4's third condition
  firing. They do not have to act on it — the rollover picks it up from
  `Unraised`.
- **New content field `raised: true`** (`internal/event/catalogue.go`). A raised
  event is never generated and never chained to — both are failed builds — and
  only `Events.Raise` can queue one. The three shipped ones are
  `content/events/{nothing-covered-it,two-answers-disagreed,it-went-badly}.yaml`,
  one per `playbook.Cause`, mapped in `internal/api/rollover.go`'s `escalations`.
  Author a fourth cause and you must author its event or it stays unraised
  forever (deliberately — losing it silently would be worse).
- **A raised event beats weight in `candidate`**, and goes past `Depth`, the daily
  cap and the cooling. It does **not** go past placement, the offer window, or
  one-thing-at-a-time. Do not add a fourth exemption without reading the
  catch-all-rule argument in `escalation.go`.
- **`Attention.Since` is measured from when the last offer was *done with*** —
  answered, walked away from, or lapsed — not from when it was offered, because
  `event_record.at` is the answer's timestamp. `SinceKind` is the same gap for one
  family and is never smaller than `Since`.
- **`account.Seen` now writes `last_away` too.** It only rewrites it when the gap
  it observes is at least `account.Sitting` (4h), so "somebody who has just come
  back" lasts as long as the sitting rather than one page load. Anything that
  wants time away must read `Accounts.Away`, never `now() - last_seen_at`.
- **`factor.Lagging` returns factors and no scores.** Do not add a variant that
  returns numbers; §3.2's hidden three stay unquantified outside internal/factor,
  and `TestLaggingNamesTheFactorsBehindAndNoNumbers` is the guard.
- **`TestNoRecommendationCrossesTheWire`** (`internal/api/escalation_test.go`)
  reads the world snapshot as text and fails on "lagging", "recommend",
  "suggest", "backlog", "escalat", "away", "unraised", "cooling", "attention".
  Adding a field with one of those words in its JSON name will fail it — that is
  the ticket, not a flaky test.
- **`work` in `internal/api/rollover.go` now holds four things**
  (`factors, events, playbooks, clk`). `#29` still fills `Recompute`, which is the
  last stub on `rollover.World`.
- **Not looked at in a browser.** Same standing caveat as passes 5, 11 and 12 —
  OAuth sign-in with no dev bypass and no DOM test setup in `web/`. No client
  change was needed for this ticket and none was made.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still stage
  explicit paths, never `git add -A`, and never stage `RALPH_STATUS.md`.

## Pass 14 — issue #23 (Challenge framework) — DONE

Commit `3c1b32e` on `main`. §5's framework: a catalogue loaded from repo files and
validated at boot, a mechanic-type registry, runs checkpointed at scene boundaries,
and hot reload in development. `make check` green (193 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §5 (all of it), §5.1/5.2/5.3, plus §3.10 and §4's
  availability-vs-offer distinction are what this pass implemented. `README.md`
  gets a prose section per module — I added "What a challenge runs on".
- **`schemaVersion` in `internal/api/commands_test.go` is now `17`.** Migration
  `00017_challenges.sql` adds `challenge` (catalog) and `challenge_run`.
- **No mechanic type ships, and `content/challenges/` holds only a README.**
  That is deliberate — §5.2's "extract the type on the second instance, not the
  first". **#24 writes the first challenge as Go** (register a `Mechanic` from an
  init in internal/challenge, or a subpackage that imports it), **#25 extracts the
  type.** `internal/challenge/catalogue_test.go` has `staged` and `rooted`, two
  test mechanics that show the shape.
- **What a mechanic must produce is `challenge.Plan`** — `Opening`, `[]Scene` with
  `[]Option` (each `Then`, `Weight`, `Deltas []simulation.Grant`), and `[]Bucket`
  (`AtLeast` + `Reaction`). `Plan.check()` is the framework's validation and it is
  strict: no cycles, no unreachable scene, at least one scene with >1 option, no
  delta on Health or Money, and **bucket bounds against the graph's own worst and
  best path**. Add a field to `Plan` and you must decide what `check` says about it.
- **`parse(fsys, types)` takes the registry as an argument** so tests supply their
  own mechanics without registering into the process. `Load(fsys)` uses the global.
- **`challenge_run` has no `score` column.** The score is `tally(plan, trail)`,
  computed on read (§3.3) — and `internal/factor`'s
  `TestNothingInTheDatabaseHoldsAKarma` greps `information_schema` for
  `%karma%`/`%score%`, so **do not add one**. The *bucket* is stored, because that
  is what the player was told.
- **`TestTheCatalogueIsPublishedSoARunCanReferenceIt` reads
  `information_schema.columns` for `challenge`** and fails on any column outside
  {family, variant, mechanic, code, published_at, retired_at}. Adding a `name` or
  a scene count there is the AC "the database stores runs, never definitions"
  breaking, not a flaky test.
- **`event.Resolve` is now called from somewhere.** A run begun with a non-zero
  `opened` (the `event_backlog` id) settles it on completion. `#28`/`#30` still
  have that seam free.
- **The `opens` link is validated across two modules**: internal/challenge imports
  internal/event and refuses a challenge behind an event that is not `awaits`, or
  two challenges behind one event. `a-shift-going-spare` is the only shipped
  `awaits: true` event, so it is what tests use.
- **Boot order in `cmd/server`**: `api.New` → `app.PublishContent(ctx)` →
  `go app.WatchContent(ctx)` → queue → listener. `WatchContent` is a no-op in
  production and when `content/` is not on disk.
- **The watcher reloads once before its loop**, so a dev process picks up the
  working copy even if it was started from a stale build. Tests that drive it must
  stop it *and wait for it* before restoring the global catalogue —
  `internal/challenge/watch_test.go` shows the pattern.
- **No transport, no client change, nothing looked at in a browser.** Same standing
  caveat as passes 5, 11, 12 and 13 (OAuth with no dev bypass, no DOM test setup in
  `web/`) — but there was also nothing to look at: #24 brings the playable surface.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still stage
  explicit paths, never `git add -A`, and never stage `RALPH_STATUS.md` (it is
  untracked on purpose).

## Pass 15 — issue #24 (First challenge to playable) — DONE

Commit `782abf3` on `main`. §3.10's worked example authored end to end — the argument
on the stoop — plus the transport and the screen #23 deliberately left out. `make
check` green (197 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §5.1/5.2/5.3, §3.10 and §4's availability-vs-offer split are
  what this pass implemented. `README.md` gained "The argument on the stoop".
- **`schemaVersion` is still `17`.** No migration: #23's `challenge` and
  `challenge_run` needed nothing added.
- **#25 is the direct successor and this pass is its raw material.** The mechanic
  is `the_stoop` in `internal/challenge/stoop.go` — an `argument()` script of
  `act`/`position` values, projected into a `Plan` by `Configure` and into words
  by `Say`. Extracting `dialogue_branch` means moving `argument()`/`outcomes()`
  into a `dialogue.yaml` schema and leaving the projection behind as the type.
  `Configure` currently **refuses any file beside `challenge.yaml`** — that
  refusal is the line #25 deletes.
- **The built-in mechanic registers from `catalogue.go`'s `init`, not its own
  file's.** Within a package `init` runs in file-name order and `catalogue.go`
  loads the content, so a registration in `stoop.go` would be too late; a
  separate package would be later still (an import is initialised in full before
  its importer). If #25 moves the type into `internal/challenge/dialogue/`, that
  ordering has to be solved first — the load at init is the constraint.
- **The framework grew one seam: `challenge.Narrator` / `challenge.Say(run)`**
  (`internal/challenge/telling.go`), plus `Plan.Ahead`. `Told` carries text, the
  keys a choice is made by, and `Stage`/`Stages`. **Adding any other numeric or
  judgement-shaped field to `Told` fails `TestNothingToldIsAGrade`**, which
  enumerates the struct's fields — that is the ticket, not a brittle test.
- **`stoop.Say` builds its choices by walking `run.Scene.Options`** (the plan's
  list) and attaching words by key, so the words and the plan cannot come apart.
  Keep that shape in any mechanic.
- **The two commands are `challenge.begin` and `challenge.choose`.** `begin`
  refuses (409) unless the challenge's own entry point stands where the player is
  *and* they are not mid-run in something else. There is deliberately no abandon
  command.
- **Accepting an `awaits` event begins the run in the same transaction**
  (`Server.opened` in `internal/api/challenge.go`), and `eventResult` gained a
  nullable `challenge`. #28/#30 will want the same seam for job situations and
  conflicts — copy `opened`, do not generalise it yet.
- **`build()` in `internal/api/commands_test.go` now calls `PublishContent`**, as
  cmd/server does. Without it `challenge_run`'s foreign key has nothing to point
  at and every api test that begins a challenge fails on the FK.
- **`event.Quiet` is 30 minutes of real time**, so at most one event is ever
  released inside one test run. `TestAcceptingTheEventBehindItOpensTheRun`
  therefore *arranges* the backlog (deletes every other row) and then walks out
  and back, rather than hoping the policy picks its event. Copy that pattern.
- **The client keeps a finished run through a hydration** (`challengeAfter` in
  `web/src/world/state.ts`): the snapshot stops carrying a run the moment it
  finishes, and dropping the reaction off screen would take back the ending of a
  six-minute endeavour. It is presentation memory, like `crossings` — no rule is
  computed there.
- **Only CSS classes present in `git show HEAD:ui/*.css` were used.** The scene
  vocabulary (`scene-head`, `stage-dots`, `speech`, `reply`, `reply-who`,
  `option`, `option-lead`, `option-text`, `commit`, `ask`) was already committed
  and the screen is built from it. `.note first` is used elsewhere in App.tsx and
  is still undefined in committed CSS.
- **Not looked at in a browser.** Same standing caveat as passes 5 and 11–14 —
  OAuth with no dev bypass, no DOM test setup in `web/`. The transport is covered
  end to end in `internal/api/challenge_test.go`.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still stage
  explicit paths, never `git add -A`, and never stage `RALPH_STATUS.md`.

## Pass 16 — issue #25 (Extract the first mechanic type) — DONE

Commit `a178259` on `main`. §5.2's line drawn: `dialogue_branch` lifted out of
`stoop.go` with a typed config schema, the stoop's words moved into
`dialogue.yaml` unchanged, and a second variant (`resolve_conflict/b`, the Sunday
at the laundromat) shipped as two YAML files with no Go. `make check` green (197
client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §5.2 (all of it), §5.1, §5.3, §3.10 and §4's
  availability-vs-offer split are what this pass implemented. `README.md`'s two
  challenge sections were rewritten; `content/challenges/README.md` is now the
  **authoring guide** and carries `dialogue_branch`'s whole schema.
- **`schemaVersion` in `internal/api/commands_test.go` is still `17`.** No
  migration: nothing about the extraction touched SQL.
- **`internal/challenge/stoop.go` is gone.** Its content is
  `content/challenges/resolve_conflict/a/dialogue.yaml`, byte-identical (a
  throwaway test held the two plans and scripts equal field for field before the
  file was deleted). The one intended difference is that an option's grants come
  out in `simulation.Factors` order now, because `moves` is a YAML map and Go
  iterates a map at random; they are summed per factor before payment, so nothing
  awarded changed.
- **`Mechanic.Configure` changed signature**: it returns `(Configured, error)`
  rather than `(Plan, error)`. `Configured` has one method, `Plan() Plan`, and the
  definition carries the whole thing as `Definition.Made`. A mechanic is a
  singleton shared by every challenge naming it, so a challenge's words live on
  what Configure returned — **do not add a map of code→script to a mechanic**, it
  breaks in half across a hot reload. `Narrator` hangs off `Configured` now, not
  off `Mechanic`, and `Say` no longer reads the registry.
- **Adding a challenge of an existing shape is content only.** Two YAML files
  under `content/challenges/<family>/<variant>/`, nothing in `internal/`. If you
  find yourself editing Go to add a challenge, either it is a new mechanic type
  (which is code, correctly) or the type is missing something and it goes on the
  type for every challenge, not yours. `content/challenges/README.md` says this
  in full; #26–#28, #30–#32 should all read it before writing a line.
- **`internal/challenge/authored_test.go` (was `stoop_test.go`) runs over every
  shipped challenge**, via `shipped(t)`. Note `run_test.go` already has an
  `authored(t, variant)` helper — different thing, do not collide with it. Add a
  challenge and these tests apply to it immediately: five scenes on every
  passage, ≥100 ways through, four readings within a point of each other, every
  outcome reached, 700–2400 words a passage, unique `taken` per option.
- **The bucket floors are set against the plan's own bounds and the loader checks
  them.** Both shipped challenges run −2..13. If you author a third, print the
  distribution before choosing floors — an unreachable band is a failed build,
  and a band nothing lands in fails `TestEveryOutcomeIsReachedBySomePassage`.
- **`resolve_conflict/b` stands in `east-side-laundromat` / `seats`**, which is
  one free walk from `east-side`. It is unpainted (#35), which does not matter —
  the doorway resolves by *place*; the zone is only where a painting would draw
  the marker.
- **`TestAChallengeThatIsOnlyContentIsPlayable`** in `internal/api` walks in,
  plays a whole passage over the real commands and walks out. Copy it if you add
  a surface that has to be shown to work end to end.
- **Not looked at in a browser.** Same standing caveat as passes 5 and 11–15 —
  OAuth with no dev bypass, no DOM test setup in `web/`. **No client change was
  needed**, which is itself the point: the screen is built from `Told` and never
  learns which mechanic type it is looking at.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass (his
  in-flight edits were gone from the tree by the time I staged). Still stage
  explicit paths, never `git add -A` at the repo root, and never stage
  `RALPH_STATUS.md`.

## Pass 17 — issue #27 (Passive job tiers) — DONE

Commit `ea428cf` on `main`. §3.14's ladder in the pure core, `internal/career`
owning the job held, a wage paid by the day ending through a new
`rollover.Employer` seam, and the interview for the first step up authored as
two YAML files. `make check` green (202 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.14 (all of it), §3.4's economy paragraphs, §3.5's veil
  and §17's "job tier thresholds" are what this pass implemented. `README.md`
  gained "The job you hold, and the ladder above it".
- **`schemaVersion` in `internal/api/commands_test.go` is now `18`.** Migration
  `00018_career.sql` adds `career` (tier, reached, since, paid_through).
- **`internal/jobs` is the River queue, not employment.** The employment module
  is `internal/career`. Do not confuse them.
- **`simulation.Project` changed signature** — it takes a `simulation.Post` now.
  A forecast that ignored the job would promise a body the rollover will never
  produce, so the routines screen and the rollover run the same `Working` over
  the same days. `api.forecast` gained the same parameter.
- **`rollover.New` gained an `Employer` argument** (before `World`). The stub is
  `rollover.Unemployed{}`; `internal/api` binds `employed{careers: ...}`. It is
  called for *every* day walked, including days the body has already had —
  paying is idempotent in the date on the career side, and a wage skipped
  because the body was ahead would be a day worked for nothing.
- **`api`'s `stopped(t, s, at)` now rebuilds `s.careers` and `s.rollovers`.**
  It previously left the rollover on the real clock, which meant no api test
  using it had ever advanced a day. Any test that moves the clock and expects
  days to be lived should also call the new `upTo(t, st, account, "YYYY-MM-DD")`
  — the sign-in happens before the clock is stopped, so the rollover ledger is
  stamped with the *real* day and will otherwise eat one of your days.
- **A wage must never go through `character.Work`.** `character.Wage` exists so
  that structural income does not spend the paid-action allowance (§3.4), and
  `internal/api`'s `payroll` adapter is what makes the capped path unreachable
  from `internal/career`. Do not hand the career module the character module.
- **`factor.Ready` / `factor.Wanting` are the veil for a requirement.** `Ready`
  is a boolean over all five floors; `Wanting` describes only the two revealed
  ones and returns a zero `Want` for each hidden one — no name, no value, **no
  met flag**. A blanked chip that flipped would name the factor. If you add a
  gate anywhere else, use these rather than reading a Reading.
- **The interview seam is in `internal/api`, not in `internal/challenge`.**
  `chooseInChallenge` sees `run.Done` and calls `s.hired(code, run.Outcome.Key)`;
  `challengeSurface` and `beginChallenge` both ask `careers.Sitting(code)`.
  `internal/challenge` was not edited for this and must not be — #28/#30 should
  copy the same shape rather than growing an interface on the framework.
- **Which outcome bands hire is `career.Role.Offered`**, not a threshold in the
  content. `interview/a`'s `offered` and `start-monday` are the two.
- **One place hires one role.** `career.Hiring(place)` returns the first match in
  `Roles` order, and `TestNoPlaceHiresTwoRoles` is what stops that being decided
  by list order. Warehouse nights is the loading bay's `dock`; the dispatch desk
  is the warehouse's `desk`.
- **Tiers 3–5 exist on the ladder with real numbers and no way in.** That is #28:
  give `Operations` a `Hired` spot and/or an `Interview`, and the whole surface
  already renders them. `simulation.Delegated` is the line.
- **The health floors are tight on purpose.** Tier 2 asks for a health reading of
  45, and a fortnight of nights lived carefully reads ~37 after 60 days — the
  job at the bottom takes the thing the step out of it is hired against. If you
  retune `simulation.Rungs()`, run `TestTheSameJobAgesTwoRoutinesDifferently`
  and check the numbers it logs before believing the ladder is still climbable.
- **The client screen is key `J`.** It is built from `@etm/ui`'s committed
  `Posting` / `LadderRow` / `.req` vocabulary, which the user had already
  designed — `.posting`, `.posting-head/-role/-where/-pay/-foot`, `.req` with
  `.met/.unmet/.hidden`, `.ladder-row` with `.current/.future`. `.right` is used
  because `ui/index.js`'s `Posting` uses it, and is not yet defined in committed
  CSS; `.note` was already in that state before this pass.
- **`rollover.ran` now carries the character.** The rollover is the only thing
  that moves a balance with nobody asking, so a tab left open overnight would
  otherwise show yesterday's money.
- **Not looked at in a browser.** Same standing caveat as passes 5 and 11–16 —
  OAuth with no dev bypass, no DOM test setup in `web/`. The transport is
  covered end to end in `internal/api/career_test.go`.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still stage
  explicit paths, never `git add -A` at the repo root, and never stage
  `RALPH_STATUS.md`.

## Pass 18 — issue #28 (Active job tiers and playbook delegation) — DONE

Commit `483c1d4` on `main`. §3.14's delegated half: `internal/situation` produces a
day's authored moment for a tier-3+ role during the rollover, the player's own
rules settle it or it waits as a live decision, and the log reads back all four
of what happened / which rule fired / what was done / what came of it. Tier 3
opened with a new interview. `make check` green (207 client tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.14 (all of the "active jobs are delegated" half), §4's
  three escalation conditions and its "authored pathways" line, §3.5's granted
  factors and §3.15's environment scoping are what this pass implemented.
  `README.md` gained "The job that runs while you are somewhere else".
- **`schemaVersion` in `internal/api/commands_test.go` is now `19`.** Migration
  `00019_situation.sql` adds `job_situation`.
- **`internal/situation` is the delegated half of a job; `internal/career` is the
  job held.** Do not confuse them, and do not hand either one the other — the
  seam is `situation.Holding`, which is one `simulation.Tier`.
- **`rollover.World` gained `Delegated`** (per day, in the day loop after
  `r.day` and before `Decay`). `Unbuilt` has a no-op; `internal/api`'s `work`
  binds it. Any test with its own `World` needs the method — `rollover_test.go`'s
  `recording` now has it.
- **`internal/api`'s `stopped(t, s, at)` now also rebuilds `s.situations`.** Same
  reason it rebuilds `s.careers` and `s.rollovers`: a module left on the real
  clock files a caught-up Tuesday under today.
- **A role that is waiting produces nothing new.** That is the mechanic, not an
  optimisation — `Situations.Day` inserts, then checks for an older waiting row
  and deletes its own insert if there is one. It is what bounds a long absence
  (§4) and what makes composing rules pay. Do not "fix" it into a queue.
- **A waiting moment marks its own fire record raised.** The moment *is* the
  escalation and is already in front of the player, so `work.Escalate` must not
  also queue `nothing-covered-it` for it. The third condition (fired, went
  badly) is deliberately left unraised and still goes through the backlog.
- **`playbook.Stances` is now exported** (ordered, least committal first) because
  a situation is answered by *posture* rather than by an action term. There is a
  test that it and the unexported `stances` map are one set — add a seventh
  stance to one and the build fails.
- **Adding a situation is content only**: one YAML file in `content/situations/`,
  named for its key. The loader refuses a passive tier, an unknown question, a
  term stated the wrong way, a fact about a question the moment does not put, a
  `moves` line naming health or money, and — the one that bites — **any posture
  the moment's questions can reach with no answer written for it**. Reachable
  postures per question today: `help` {yield,resist,bargain}, `voice`
  {yield,resist,open,withdraw}, `money` {yield,resist,defer,bargain}, `overtime`
  {yield,resist,bargain}.
- **`pick` uses FNV + a 32-bit avalanche.** Do not remove the finalizer: FNV's
  low bits are a bijection of the previous low bits, so `% 4` without it hands
  two accounts the same year of mornings. There is a test.
- **`internal/situation/catalogue_test.go` runs over every shipped situation**,
  like `internal/challenge/authored_test.go` does for challenges. Two of its
  assertions are the ticket: yielding must backfire *and cost something* at tier
  3, and every moment must have a `good` answer paying into Intelligence or
  Stability.
- **Tier 3 is open; tiers 4 and 5 are not.** `career_test.go`'s
  `TestTheLadderOpensOneRungAtATime` states the frontier as a constant (`4`) —
  move it when you open the next rung, and give `Regional` a `Hired` spot and an
  interview. Situations for tiers 4 and 5 are already authored and waiting.
- **`interview/b` ("The fourth floor") is the third authored challenge**, at
  `downtown-office`/`desk`. Same topology and the same weight distribution as
  `interview/a`, so the buckets sit at −2/4/9/11 and every band is reachable.
  `career.Operations.Offered` is `["the-job", "the-mandate"]`.
- **A test that wants a particular situation should compute its day rather than
  walk to it** — `internal/situation/situation_test.go`'s `arise` shows the
  shape, and it keeps every other moment in the pool out of the ledger.
- **The client screen is still key `J`.** The waiting moment is a panel at the
  top of the job overlay (`OnYourDesk` in `App.tsx` — `Waiting` was already
  taken by the session splash) and takes the key hint ahead of everything else.
  The world snapshot gained a `situations` key and `rollover.ran` carries it,
  because the rollover is the only thing that can produce one.
- **Not looked at in a browser.** Same standing caveat as passes 5 and 11–17.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still
  stage explicit paths, never `git add -A`, and never stage `RALPH_STATUS.md`.

## Pass 19 — issue #29 (Environment module — populations, ceilings, decay) — DONE

Commit `4fe2e50` on `main`. §3.15's inversion: an environment is a **population**,
not a place. `internal/environment` is a new pure package over
`content/environments/*.yaml`; ceilings are applied to the reading (money in
dollars, stability/kindness per population, intelligence by breadth); decay is
keyed by population; nothing may be labelled. `make check` green (207 client
tests).

Notes for the next agent:

- **The design doc is `Documents/technical.md` + `Documents/HighLevelOverview.md`.**
  Still no DESIGN.md. §3.15 in full (populations, the factor-affordance table,
  decay, "never labelled") is what this pass implemented. `README.md` gained
  **"Who you are among"**.
- **`schemaVersion` in `internal/api/commands_test.go` is now `20`.** Migration
  `00020_environments.sql` changes no table shape — it *remaps the meaning* of
  four `environment` columns from district keys to population keys
  (`east-side`→`the-landing`, `riverfront`→`the-lifts`) and deletes wear and
  rules for the two districts nobody lives in. History (`playbook_fire`,
  `job_situation`) is deliberately kept.
- **The environment keys are now `the-landing` and `the-lifts`.** Anything that
  hard-codes `east-side` as an *environment* is wrong; `east-side` is still a
  district and still a place key. Several tests were updated for this — check
  which sense you mean before typing a key.
- **`internal/environment` is pure**, like `internal/simulation` and
  `internal/city`: no db, no clock, package-level catalogue built in `init`,
  `parse` takes an `fs.FS` so the failure paths are testable. Adding a
  population is one YAML file and nothing else.
- **`environment.Labelled` is a load-bearing boot check.** It refuses any
  player-facing string containing a stage name, register, framework word or
  developmental colour. It matches **whole words**, so "stages" as a verb and
  "amber" as a name are both caught — that is intentional; rephrase rather than
  weaken the list. Exported so #30 and #31 can hold their own copy to it.
- **The population follows `character.Home`, not `character.At`.** The binding is
  `population` in `internal/api/factor.go` (it replaced `district`, which is
  gone). Do not "fix" it to follow the player's feet — it would make the ceiling
  avoidable by walking downtown, which is the whole of §3.15's pressure.
- **`internal/factor.compose` is now the ONE place a Karma is composed**, and
  therefore the one place ceilings are applied. `Of`, `reading` and `board.go`
  all go through it. A second composition path is how a reading escapes its
  ceiling — do not add one.
- **`simulation.Ceilings.Intelligence` is NOT authored.** It is
  `simulation.Reach(sets)` where `sets` counts `factor_tenure` rows with
  `worked >= simulation.RuleSet`. A YAML file that writes an `intelligence:`
  affordance fails the strict decode, and there is a test. Zero still means *no
  ceiling* everywhere, which is why `Reach(0)` returns `Reach(1)`.
- **`rollover.World.Recompute` is now deliberately a no-op and says so.** A
  ceiling turned out to be a pure function of authored content plus one tenure
  row, so it is composed on read (§3.3). Do not delete the seam; do not fill it
  in without a stored ceiling to fill.
- **Only `the-landing` is reachable.** `internal/city/city_test.go` asserts the
  city has exactly one home "until §3.12's relocation (#31)". **#31 is the
  ticket that adds the second home** — put it in `riverfront` so `the-lifts`
  becomes live, and move that test's `homes != 1` assertion when you do.
- **#30 has what it needs**: `Environment.People()`, `Person.Found`,
  `Person.Does`, `Environment.Holds()`. What does not exist yet is any record of
  what a person is owed — `internal/situation`'s `pick` comment names that gap.
- The client's playbook set in force now comes from `karma.where` (the server's
  key), not from a district derived on the client. `karmaView` gained `among`;
  `make generate` was run.
- **Not looked at in a browser.** Same standing caveat as passes 5 and 11–18.
- The user's `preview/`, `ui/` and `Documents/` were clean this pass. Still stage
  explicit paths, never `git add -A`, and never stage `RALPH_STATUS.md`.

## Pass 20 — issue #30 (Daily conflicts) — IN PROGRESS

## Pass 21 — issue #32 (Diagnosis scenes) — DONE

Commit `8a7917a` on `main`. §3.6's struggle-gated unlocking: the player points at
their own readings, names the cause of a failure the world already escalated, and
the naming is what grants the tool. `make check` green (235 client tests).
**Looked at in a browser** — the first pass since #11 that was.

Notes for the next agent:

- **`schemaVersion` in `internal/api/commands_test.go` is now `26`.** #31's
  `00025_relocation.sql` was uncommitted in the tree while this landed, so 25 was
  left for it. The assertion is `max(version_id)`, not a count, so a gap is fine
  and both numbers are consistent whichever lands first.
- **`internal/api/api.go` binds `routine.New(gate{diagnoses:...})`.** The line #9
  left. If you need routines unlocked in a test, `everything(t, st)` in
  `internal/api/diagnosis_test.go` puts the conclusions in by fiat — six existing
  tests already use it, and any new test that calls `routine.set` will need it.
- **`internal/diagnosis` has no notion of correctness and must not grow one.**
  `TestNothingInThisPackageKnowsWhatIsTrue` walks the exported surface by
  reflection and fails on a field or method named for it. That is the whole of
  §3.6's second rule; do not "fix" the test.
- **A failure is one of §4's three escalation conditions, read back** — see
  `playbook.Failed`. Nothing about a *body* is ever an occasion, deliberately:
  `bodyLedger.Failed` returns nothing and says why. A body is evidence, not a
  moment.
- **Adding a ledger** = implement `diagnosis.Ledger`, bind it in `api.New`, and
  add its keys to `observable()` in `internal/api/diagnosis.go`. A cause speaking
  from a key nothing produces panics at boot, on purpose — an unreachable cause
  looks exactly like a player who has not struggled enough yet.
- **The observation *sentences* live with the rows.** `health.Observed` and
  `playbook.Observed` are pure functions over types those packages already
  export, so their thresholds are testable without a database. Do not move them
  into `internal/diagnosis`; it would become a second copy of both.
- **`playbook.Unlock` had no production caller before this.** Every term without
  `start: true` was unreachable in the running game. If you author a locked term,
  something has to grant it — today that is a cause in `content/diagnoses/causes/`.
- **Adding a routine** means adding a cause that unlocks it, or
  `TestEveryRoutineCanBeReached` fails. That is the point: a `Learn` sentence with
  no way to earn it is a promise the content cannot keep.
- **Not art-passed.** The scene uses the existing panel/button vocabulary; the
  selected-evidence state is a faint outline. Left for #38.
- **This pass shared the working tree with an agent on #31.** Their files
  (`internal/city`, `internal/character`, `internal/career`, `content/environments`,
  `web/src/screens/*`, `ui/`, `Makefile`) were uncommitted throughout and were
  **not** staged — the commit was built by diffing a pre-pass snapshot per shared
  file and applying only those hunks onto HEAD. Do the same if it happens again;
  `git add <path>` on a shared file would have taken their work with it.
