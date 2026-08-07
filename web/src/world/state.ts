/*
 * The client's world model, and the only functions allowed to change it.
 *
 * technical.md §10.2: one normalised world-model that REST hydrates at boot,
 * socket messages patch, React subscribes to, and the renderer reads directly
 * in its own frame loop. This file is the model and the patches; `store.ts` is
 * the one instance of it, `react.ts` is how components read it, and
 * `../render/` is how the canvas does.
 *
 * Two rules from §10.2 shape everything below.
 *
 * 1. **Patches are typed domain messages, not generic diffs.** One reducer per
 *    message type, in a table the type system checks for completeness — a
 *    message added on the server becomes a compile error here rather than one
 *    silently ignored. A generic diff would make every socket message an
 *    accident instead of a decision.
 *
 * 2. **The client computes no game rules.** Every number in here was stated by
 *    the server. Nothing is derived, predicted, or incremented locally — which
 *    is what keeps `probes.count` honest when a command's response and a
 *    socket message disagree about it: the newer statement simply wins, and
 *    there is no local arithmetic to have drifted in between.
 *
 * The reducers are pure and take no dependency on the store, which is what
 * makes them the part worth unit-testing (`state.test.ts`).
 */

import type { HealthLived, RoutineSet, ServerMessage } from '../realtime/messages.gen'
import type { ProbeResult } from '../commands'
import type { Player } from '../session'

/** Who the server says we are (§9). Unknown until it has answered once. */
export type Session =
  | { status: 'unknown' }
  | { status: 'out'; notice: string | null }
  | { status: 'in'; player: Player }

/** What the push channel is doing (§8), as something the screen can say. */
export type Link = 'connecting' | 'live' | 'offline'

/**
 * What the renderer is doing.
 *
 * WebGL2 is a hard floor (§10.4): Pixi v8 removed the Canvas2D fallback, so a
 * device without it does not degrade, it fails. That is a state the world model
 * carries rather than an exception thrown into the void, because the only
 * useful response to it is a sentence on screen.
 */
export type Renderer = 'starting' | 'running' | 'unsupported'

/**
 * One probe this client has been told about, and how it heard.
 *
 * `by` is the difference §8's rule 3 draws: a command's response tells you
 * about your own action, and the socket tells you about the world's. Keeping it
 * means the renderer can draw the two differently, which is the whole visible
 * proof that both paths land in the same store.
 */
export type Mark = {
  /** Which one this was, counting from the moment this client loaded. */
  n: number
  label: string
  notedAt: string
  by: 'you' | 'elsewhere'
}

/**
 * The walking skeleton's domain, and the shape every later module copies: a
 * number the server stated, and whatever presentation memory hangs off it.
 */
export type Probes = {
  /** Rows in `skeleton_probe`, as the server last said. Never computed here. */
  count: number
  /** How many this client has learned about since it loaded. */
  seen: number
  /** The most recent of those, oldest first. Bounded — see `REMEMBERED`. */
  marks: Mark[]
}

/** Somewhere in the city. Authored vocabulary, sent by the server (§17). */
export type Place = {
  key: string
  name: string
  note: string
}

/**
 * Where in the painted location the player is standing (§10.3).
 *
 * Not a thing the server said, and the one slice of the model that is not.
 * That is deliberate and narrow: a painted location is authored data the client
 * loads directly (§10.3), and which authored rectangle a walk ended inside is a
 * fact about that data — geometry, not a rule. §10.2's "the client computes no
 * game rules" is untouched, because nothing here decides what *happens* in a
 * zone. Deciding that is the event system's (#21, #22), and the keys below are
 * the vocabulary it will be asked in: §3.9's placement conditions and a
 * physical spot in the world are the same object.
 *
 * Null when the world is not drawing a location — before boot, or in a district
 * nobody has painted yet.
 */
export type Standing = {
  /** The location's own key, as its map file names itself. */
  level: string
  /** The trigger zones containing the player, most specific first. */
  zones: { key: string; name: string }[]
  /** The ways out this spot offers. #12 makes them lead somewhere. */
  portals: { key: string; name: string; to: string }[]
}

/**
 * The player: money, where they are standing, and where they live (§3.3).
 *
 * `money` is cents all the way to the screen — formatting it is presentation,
 * and a float on the wire arrives rounded. `earningsLeft` is derived by the
 * server on read (§3.4's cap against the player's own day), so it is a number
 * this client was told rather than one it worked out.
 */
export type Character = {
  money: number
  at: Place
  home: Place
  earningsTaken: number
  earningsLeft: number
}

/** One level of instrumentation: what the player has bought the ability to see. */
export type Tier = {
  key: string
  name: string
  buys: string
  price: number
  owned: boolean
  forSale: boolean
}

/**
 * One readout on the health panel (§3.16).
 *
 * A metric the player has no instrument for still arrives — named, explained,
 * and carrying the tier that would read it — with `value` null and no history.
 * That is the design rather than an omission: what is withheld is the reading,
 * not the idea that the measurement exists.
 */
export type Metric = {
  key: string
  name: string
  unit: string
  means: string
  tier: string
  tierName: string
  lower: boolean
  decimals: number
  visible: boolean
  value: number | null
  history: number[]
}

/** The body, and what this player can see of it. */
export type Health = {
  /** Visible at every tier, composed by the server on read (§3.5, §3.3). */
  biologicalAge: number
  livedToday: boolean
  tiers: Tier[]
  metrics: Metric[]
}

/**
 * One answer a routine will take (§3.4).
 *
 * `says` is the rule as a sentence, because a routine *is* a rule and the
 * playbook composes in sentences too (§3.14) — one system, one interface
 * language. `costs` is the honest half: nothing in this game is labelled good or
 * bad (§3.2), so the price is stated and the verdict is the player's.
 */
export type RoutineOption = {
  key: string
  says: string
  costs: string
  chosen: boolean
}

/**
 * One routine: a standing question about the body, and the answers it will take.
 *
 * `learn` is what the player has still to go through before they may configure
 * it (§3.6, struggle before mastery). It arrives whether or not the routine is
 * locked, because it is the sentence that says *why* — nothing about the gate is
 * for this client to invent.
 */
export type Routine = {
  key: string
  name: string
  asks: string
  learn: string
  unlocked: boolean
  chosen: string
  options: RoutineOption[]
}

/** The rules the player has set for their own body, as one unit. */
export type Routines = {
  /** Whether they have ever set them. Not the same as having chosen defaults. */
  set: boolean
  routines: Routine[]
}

/** One metric's arc under the rules currently in force. */
export type ForecastMetric = {
  key: string
  /** Empty for a metric this player has no instrument for — the panel's rule. */
  values: number[]
}

/**
 * What the routines would do over the next fortnight, if they were kept.
 *
 * Composed by the server on read and never stored (§3.3), and instrumented
 * exactly like the panel: a projection is a reading of a body too. Biological
 * age is the exception at both ends, visible at every tier.
 */
export type Forecast = {
  days: number
  biologicalAge: number[]
  metrics: ForecastMetric[]
}

/** A metric that moved between named bands — the simulation, saying so. */
export type Crossing = {
  metric: string
  from: string
  to: string
  worse: boolean
}

/**
 * The last command this client sent, whatever became of it.
 *
 * In the world model rather than in a component, because the canvas sends the
 * same commands the panels do (§10.1) — a tap that failed has to be able to say
 * so on a surface the tap did not come from. Holding the key is also the whole
 * of what a client must do to make its retries safe (§8): the same key is the
 * same intent, and a new one is a new action.
 */
export type Attempt =
  | { status: 'idle' }
  | { status: 'sending'; key: string }
  | { status: 'applied'; key: string; replayed: boolean }
  | { status: 'refused'; key: string; code: number; message: string }

/**
 * What the server says it is running on.
 *
 * Null until the world has been hydrated once. Stated by the server rather than
 * detected here — like every other number on screen (§10.2).
 */
export type Server = { database: string; schema: number }

export type World = {
  session: Session
  link: Link
  renderer: Renderer
  attempt: Attempt
  server: Server | null
  /** Null until the world has been hydrated once. */
  character: Character | null
  /** The city, as the server says it is. Empty until hydration. */
  places: Place[]
  /** Where in the painted location they are standing. Null until they are. */
  standing: Standing | null
  /** Null until the world has been hydrated once. */
  health: Health | null
  /** The rules set for the body, and where they lead. Null until hydration. */
  routines: Routines | null
  forecast: Forecast | null
  /**
   * What the last day lived moved across, newest first.
   *
   * Presentation memory of an effect the pure core reported (§3.1), kept so
   * the panel can say what changed rather than only what is.
   */
  crossings: Crossing[]
  probes: Probes
}

/**
 * How many marks the client keeps.
 *
 * This is presentation memory, not state: the count above is the truth, and
 * these are the individual events behind the last stretch of it. Unbounded, a
 * long session would hand the renderer thousands of sprites describing
 * something the number already says.
 */
export const REMEMBERED = 48

export const initialWorld: World = {
  session: { status: 'unknown' },
  link: 'connecting',
  renderer: 'starting',
  attempt: { status: 'idle' },
  server: null,
  character: null,
  places: [],
  standing: null,
  health: null,
  routines: null,
  forecast: null,
  crossings: [],
  probes: { count: 0, seen: 0, marks: [] },
}

/**
 * What `GET /api/world` answers — the boot hydration of the model above (§10.2).
 *
 * Hand-written, unlike the socket's message types, which are generated from the
 * Go structs (§8, rule 1). Generation covers the push protocol because that is
 * where a silent mismatch turns into a message quietly ignored; this one is a
 * single read whose Go definition is `internal/api/world.go` and whose failure
 * mode is the whole screen being empty. Keep the two in step by hand.
 */
export type Snapshot = {
  character: Character
  places: Place[]
  health: Health
  routines: Routines
  forecast: Forecast
  probes: number
  database: string
  schema: number
}

/* ── patches ───────────────────────────────────────────────────────────────
 *
 * Every one of these is `(world, thing) => world`, and each keeps the *same*
 * object for the slices it did not change. That matters more here than in an
 * ordinary app: the renderer reads the store every frame and skips work by
 * comparing references, so a reducer that clones unconditionally would rebuild
 * the scene sixty times a second.
 *
 * Hydration is the one that restates whole slices rather than comparing them —
 * it is the server saying everything it knows, and it happens twice in a
 * session: at boot, and after a socket gap.
 */

/** Boot, and rehydration after the socket reports a gap it cannot replay. */
export function hydrated(world: World, snapshot: Snapshot): World {
  const server =
    world.server?.database === snapshot.database && world.server.schema === snapshot.schema
      ? world.server
      : { database: snapshot.database, schema: snapshot.schema }

  const probes =
    world.probes.count === snapshot.probes
      ? world.probes
      : { ...world.probes, count: snapshot.probes }

  // The character and the city are replaced wholesale rather than compared,
  // because a hydration is the server restating everything it knows and there
  // is nothing here a stale reference would save.
  return {
    ...world,
    server,
    probes,
    character: snapshot.character,
    places: snapshot.places,
    health: snapshot.health,
    routines: snapshot.routines,
    forecast: snapshot.forecast,
  }
}

/**
 * A probe this tab just made, from the command's own response.
 *
 * §8, rule 3: the acting client applies the response directly and does not wait
 * for a socket echo — which is also why the server does not send it one. This
 * is that half, and `probe.recorded` below is the other.
 */
export function probed(world: World, result: ProbeResult): World {
  return {
    ...world,
    probes: remember(world.probes, result.probes, {
      label: result.label,
      notedAt: result.notedAt,
      by: 'you',
    }),
  }
}

/** The character as a command that changed it reported them back. */
export function characterChanged(world: World, character: Character): World {
  return { ...world, character }
}

/**
 * The world layer, saying where in the painting the player ended up (§10.3).
 *
 * Written only by the renderer, and only when the answer changed — a walk
 * crossing a zone boundary, a location arriving, a district with no painting.
 * The comparison happens there, because the renderer is the only thing looking
 * every frame; this stores what it decided.
 */
export function stood(world: World, standing: Standing | null): World {
  return { ...world, standing }
}

/**
 * The panel as the server last stated it, and what today crossed.
 *
 * One function for every way the body can change — living a day, buying an
 * instrument — because in each case the server restates the whole panel and
 * this client stores it. There is nothing to merge and nothing to derive: an
 * instrument bought changes which metrics carry a reading, and a day lived
 * changes the readings, and both arrive as the same thing.
 */
export function panelChanged(world: World, health: Health, crossings: Crossing[] = []): World {
  return { ...world, health, crossings: crossings.length > 0 ? crossings : world.crossings }
}

/**
 * The rules, and where they lead, as the server last stated them.
 *
 * One patch for both, because the forecast is a reading of the body *under* the
 * rules — there is no moment at which one of them is current and the other is
 * not, and sending them apart would invite a screen showing a fortnight that
 * belongs to a configuration the player has already replaced.
 */
export function routinesChanged(world: World, routines: Routines, forecast: Forecast): World {
  return { ...world, routines, forecast }
}

/**
 * The world after the daily rollover advanced it (§3.3).
 *
 * The panel and the forecast together, for the same reason `routinesChanged`
 * sends them together: the forecast is a reading of the body, and a fortnight
 * projected from a body that has since lived four days is a fortnight about
 * nobody. The rules themselves are untouched — a rollover applies them, it does
 * not change them.
 *
 * `crossings` is whatever the *last* day applied moved across, not every day of
 * a catch-up. A threshold crossed three weeks ago is not news.
 */
export function rolledOver(
  world: World,
  health: Health,
  forecast: Forecast,
  crossings: Crossing[],
): World {
  return { ...panelChanged(world, health, crossings), forecast }
}

/**
 * The reducer table: one entry per message the socket can deliver.
 *
 * The mapped type is the point. `Push` is every server message except the
 * connection's own welcome — which the socket client handles itself and never
 * hands over — so adding a message type on the server regenerates that union,
 * and this object stops compiling until it has a reducer. That is the
 * "deliberate API decision" §10.2 asks every socket message to be.
 */
export type Push = Exclude<ServerMessage, { type: 'welcome' }>

type Reducers = {
  [M in Push as M['type']]: (world: World, payload: M['payload']) => World
}

const reducers: Reducers = {
  // Where the player is, changed somewhere else — the same tab open twice, or
  // a phone in a pocket. The command's own response tells the tab that walked
  // (§8, rule 3); this tells the others.
  'character.moved': (world, payload) =>
    world.character === null
      ? world
      : { ...world, character: { ...world.character, at: payload.at } },

  'character.earned': (world, payload) =>
    world.character === null
      ? world
      : {
          ...world,
          character: {
            ...world.character,
            money: payload.money,
            earningsTaken: payload.earningsTaken,
            earningsLeft: payload.earningsLeft,
          },
        },

  // A day lived somewhere else, and an instrument bought somewhere else. Both
  // restate the panel, because both change what it reads.
  'health.lived': (world, payload) =>
    panelChanged(world, received(payload.panel), payload.crossed ?? []),
  'health.bought': (world, payload) => panelChanged(world, received(payload.panel)),

  // The rules changed somewhere else. Both halves are restated by the server,
  // so this stores rather than merges.
  'routine.set': (world, payload) =>
    routinesChanged(world, receivedRoutines(payload.routines), receivedForecast(payload.forecast)),

  // The world moved on its own: the daily rollover, at this player's local
  // midnight (§3.3). The only message no command of theirs caused, which is why
  // it is the clearest case for the socket existing at all — a tab left open
  // overnight would otherwise still be showing yesterday's body.
  //
  // The panel and the forecast are both restated, because a body that has moved
  // is a body whose readings and whose whole arc have moved with it.
  'rollover.ran': (world, payload) =>
    rolledOver(
      world,
      received(payload.panel),
      receivedForecast(payload.forecast),
      payload.crossed ?? [],
    ),

  'probe.recorded': (world, payload) => ({
    ...world,
    probes: remember(world.probes, payload.probes, {
      label: payload.label,
      notedAt: payload.notedAt,
      // It arrived over the socket, so by definition this tab did not do it:
      // the originating session's own messages are skipped server-side (§8,
      // rule 4). Somewhere else means another tab, or another device.
      by: 'elsewhere',
    }),
  }),
}

/** Apply one socket message through its own reducer. */
export function applyMessage(world: World, message: Push): World {
  // The one cast in the file, and it is confined to this line. The table above
  // is what the type system checks — each reducer's payload is correlated with
  // its message type there — but an indexed lookup on a union loses that
  // correlation, and there is no way to express "this key's function takes this
  // key's payload" at the call site.
  const reduce = reducers[message.type] as (world: World, payload: unknown) => World
  return reduce(world, message.payload)
}

/**
 * A panel off the wire, as a panel.
 *
 * An empty Go slice is `null` in JSON rather than `[]`, which the generated
 * types state honestly and which nothing downstream should have to think about.
 * This is the one place that is turned back into a list.
 */
function received(panel: HealthLived['panel']): Health {
  return {
    biologicalAge: panel.biologicalAge,
    livedToday: panel.livedToday,
    tiers: panel.tiers ?? [],
    metrics: (panel.metrics ?? []).map((m) => ({ ...m, history: m.history ?? [] })),
  }
}

/** The routines off the wire, with Go's nil slices read back as empty lists. */
function receivedRoutines(routines: RoutineSet['routines']): Routines {
  return {
    set: routines.set,
    routines: (routines.routines ?? []).map((r) => ({ ...r, options: r.options ?? [] })),
  }
}

function receivedForecast(forecast: RoutineSet['forecast']): Forecast {
  return {
    days: forecast.days,
    biologicalAge: forecast.biologicalAge ?? [],
    metrics: (forecast.metrics ?? []).map((m) => ({ ...m, values: m.values ?? [] })),
  }
}

/**
 * Record a probe the server stated, whichever way it said it.
 *
 * The count comes from the message; the mark is this client's memory of having
 * been told. Note that the two are independent — the count is the whole table
 * and can move by more than one between marks, because other players probe too.
 */
function remember(
  probes: Probes,
  count: number,
  mark: Omit<Mark, 'n'>,
): Probes {
  const seen = probes.seen + 1
  const marks = [...probes.marks, { ...mark, n: seen }]
  return {
    count,
    seen,
    marks: marks.length > REMEMBERED ? marks.slice(marks.length - REMEMBERED) : marks,
  }
}
