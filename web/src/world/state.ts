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

import type { HealthLived, ServerMessage } from '../realtime/messages.gen'
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
  /** Null until the world has been hydrated once. */
  health: Health | null
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
  health: null,
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
