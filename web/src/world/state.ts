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

import type { ServerMessage } from '../realtime/messages.gen'
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
  probes: number
  database: string
  schema: number
}

/* ── patches ───────────────────────────────────────────────────────────────
 *
 * Every one of these is `(world, thing) => world` and returns the *same* object
 * when nothing changed. That matters more here than in an ordinary app: the
 * renderer reads the store every frame and skips work by comparing references,
 * so a reducer that clones unconditionally would rebuild the scene sixty times
 * a second.
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

  if (server === world.server && probes === world.probes) return world
  return { ...world, server, probes }
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
