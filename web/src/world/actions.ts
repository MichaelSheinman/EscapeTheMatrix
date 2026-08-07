/*
 * Everything the client does *to* the server, and the lifecycle that follows
 * from who it turns out to be.
 *
 * This is the one path from either tree into the command layer (§10.1): the
 * button in a React panel and a tap on the canvas call the same function here,
 * so there is no second way to send a command with its own idea of what to do
 * with the answer. `../commands.ts` is still the only place that knows the
 * transport; this is the only place that knows what a result means to the
 * world.
 *
 * It runs outside React, and is started once from `main.tsx`. A component
 * mounting or unmounting must not be able to open or close the socket, for the
 * same reason it must not be able to create or destroy the renderer (§10.1) —
 * the connection belongs to the app, not to whatever happens to be on screen.
 */

import { command, CommandError, newKey, type ProbeResult } from '../commands'
import { connect, type Stream } from '../realtime/client'
import { currentPlayer, signOut as endSession, takeSignInFailure } from '../session'
import { patch, world } from './store'
import type {
  BoardPage,
  Boards,
  Character,
  Crossing,
  Forecast,
  Health,
  Push,
  Routines,
  Snapshot,
  Trip,
  Way,
} from './state'

/** The live socket, when there is a signed-in player to have one for. */
let stream: Stream | null = null

/**
 * Messages that arrived while a hydration was in flight.
 *
 * Null when nothing is being hydrated, which is almost always. See `resync`.
 */
let held: Push[] | null = null

/**
 * Boot: find out who this is, and if it is anybody, join the world.
 *
 * Called once, before React renders anything. The identity survives a reload
 * because the credential is a cookie the server issued (§9) — this asks for the
 * answer rather than storing it.
 */
export async function start(): Promise<void> {
  // Read before the fetch, so a failed sign-in attempt is still reported even
  // though the answer that follows it is simply "signed out".
  const notice = takeSignInFailure()

  try {
    const player = await currentPlayer()
    if (!player) {
      patch.session({ status: 'out', notice })
      return
    }
    patch.session({ status: 'in', player })
    await enter()
  } catch {
    patch.session({
      status: 'out',
      notice: notice ?? 'The server could not be reached.',
    })
  }
}

/** Teardown: the page is going away, or a dev reload is replacing this module. */
export function stop(): void {
  stream?.close()
  stream = null
  held = null
}

/**
 * Send a probe, and apply what comes back.
 *
 * `key` is the idempotency key, one per *intent* (§8): pass the same one back
 * to retry the same probe, and a new one to make another.
 *
 * Nothing is returned and nothing is thrown. Both halves of the answer — what
 * it did to the world, and what became of this particular delivery — are
 * patched into the store, because the caller might be a button in a panel or a
 * tap on the canvas and the outcome has to be visible on both. A rejection
 * would put the handling back in whichever tree happened to send it.
 */
export async function probe(label: string, key: string = newKey()): Promise<void> {
  await send('probe', { label }, (result: ProbeResult) => patch.probed(result), key)
}

/**
 * Go somewhere: a street, a shop, or your own kitchen (§3.12).
 *
 * One function for all of §3.12's layers, because there is one question being
 * asked — take me there — and the city works out the route. `by` is the trade:
 * walking is free and is where things happen to you, the train takes the fare
 * and passes through nothing. Neither costs game time (§3.1).
 *
 * This is a command the server decides the outcome of rather than a camera move
 * the client could make on its own, which is why a tap on a portal ends up
 * here: §10.1's "canvas input dispatches through the same command layer",
 * exercised at last.
 */
export async function moveTo(to: string, by: Way['mode'] = 'foot'): Promise<void> {
  await send('character.move', { to, by }, (result: { character: Character } & Trip) =>
    patch.travelled(result.character, {
      by: result.by,
      steps: result.steps ?? [],
      fare: result.fare,
      chances: result.chances,
    }),
  )
}

/** Take a paid action. Three a day (§3.4), counted by the server. */
export async function work(): Promise<void> {
  await send('character.work', {}, (result: { character: Character }) =>
    patch.character(result.character),
  )
}

/**
 * Live a day: how you slept, whether you trained, how you ate (§3.16).
 *
 * A closed vocabulary rather than numbers, because it is what a routine will
 * produce once routines exist. One a day — the server refuses a second, since
 * one game day is one real calendar day.
 */
export async function liveADay(day: {
  sleep: string
  training: string
  diet: string
}): Promise<void> {
  await send('health.live', day, (result: { panel: Health; crossed: Crossing[] }) =>
    patch.panel(result.panel, result.crossed ?? []),
  )
}

/**
 * Set the routines: the rules for the body (§3.4).
 *
 * The whole configuration goes at once, because it is one decision — deciding to
 * sleep more is deciding what that costs the evening — and because a per-routine
 * command would let this client assemble a set of rules the player never looked
 * at. Every value is a key from the vocabulary the server sent; nothing here is
 * typed and nothing here is a number.
 */
export async function setRoutines(choices: Record<string, string>): Promise<void> {
  await send('routine.set', { choices }, (result: { routines: Routines; forecast: Forecast }) =>
    patch.routines(result.routines, result.forecast),
  )
}

/**
 * Live today by the routines rather than by answering for it.
 *
 * This is the arc §3.4 describes — the three choices are produced by the rules
 * instead of asked for — and it is the same seam the daily rollover will use
 * once it exists. Still one day per real day: the server refuses a second.
 */
export async function liveByRoutine(): Promise<void> {
  await send('routine.live', {}, (result: { panel: Health; crossed: Crossing[] }) =>
    patch.panel(result.panel, result.crossed ?? []),
  )
}

/**
 * Buy instrumentation.
 *
 * A money sink that buys insight rather than power: the same body, the same
 * numbers, the same history — now legible (§3.16).
 */
export async function buyInstrument(tier: string): Promise<void> {
  await send('health.buy', { tier }, (result: { panel: Health; money: number }) => {
    patch.panel(result.panel)
    // The purchase moved money too, and the response says how much is left
    // rather than this working it out.
    const character = world.getState().character
    if (character) patch.character({ ...character, money: result.money })
  })
}

/**
 * Load the leaderboards (§3.5).
 *
 * A GET rather than a command, because it writes nothing — and separate from the
 * boot snapshot, because §10.2 names leaderboards as the one genuinely
 * pull-shaped surface in this client. It is fetched when somebody opens the
 * screen, and the socket keeps it current from there.
 *
 * Nothing is thrown. The screen either has boards or does not, and a failure to
 * read them is not a reason to take the world away.
 */
export async function loadBoards(): Promise<void> {
  try {
    const res = await fetch(boardsPath, { headers: json, cache: 'no-store' })
    if (!res.ok) return
    patch.boards((await res.json()) as Boards)
  } catch {
    // Offline, or the session ended. The socket's own status says so already.
  }
}

/**
 * One page of one board (§3.5).
 *
 * Returned rather than stored, and that is the point: a page of *other people*
 * belongs to whatever is looking at it and is thrown away when that closes. The
 * world model is the player's world, and a hundred strangers' ranks are not part
 * of it — which is exactly the read-only paginated list §10.2 carved out.
 *
 * Null when it could not be read, including for a board the server does not
 * publish: the caller draws nothing rather than inventing an error state for a
 * key it should not have had.
 */
export async function loadBoardPage(key: string, page: number): Promise<BoardPage | null> {
  try {
    const res = await fetch(`${boardsPath}/${encodeURIComponent(key)}?page=${page}`, {
      headers: json,
      cache: 'no-store',
    })
    if (!res.ok) return null
    return (await res.json()) as BoardPage
  } catch {
    return null
  }
}

/** Where the boards are read from. */
const boardsPath = '/api/boards'

const json = { Accept: 'application/json' }

/**
 * Send a command, apply what it returns, and record what became of it.
 *
 * The one path every intent above goes through, so that the session-ended
 * handling, the in-flight state and the refusal are written once rather than
 * once per command. Auth is one middleware over the command route (§8), so any
 * command can be the one that discovers the session ended — expired, or signed
 * out on another device — and handling it here means every caller inherits it,
 * whether it was a button or a tap on the world.
 *
 * `apply` is the only part that differs between commands: what this particular
 * answer means to the world.
 */
async function send<T>(
  name: string,
  input: unknown,
  apply: (result: T) => void,
  key: string = newKey(),
): Promise<void> {
  patch.attempt({ status: 'sending', key })
  try {
    const sent = await command<T>(name, input, key)
    apply(sent.result)
    patch.attempt({ status: 'applied', key, replayed: sent.replayed })
  } catch (err) {
    if (err instanceof CommandError && err.isSignedOut) {
      ended('That session ended. Sign in again.')
      return
    }
    patch.attempt(
      err instanceof CommandError
        ? { status: 'refused', key, code: err.status, message: err.message }
        : { status: 'refused', key, code: 0, message: 'the server could not be reached' },
    )
  }
}

/**
 * End the session.
 *
 * Either the server revoked it or the request never landed, and neither leaves
 * this client anything it can trust — so both end at the door. Which of them
 * happened is not this app's call: the cookie is only worth what the server
 * says it is.
 */
export async function signOut(): Promise<void> {
  try {
    await endSession()
  } finally {
    ended(null)
  }
}

/** Join the world: hydrate it, and start being told about it. */
async function enter(): Promise<void> {
  await resync()
}

/** The session is over. Drop the connection and forget what it was for. */
function ended(notice: string | null): void {
  stop()
  patch.cleared(notice)
}

/**
 * Load the world over REST and connect the socket to it, in that order and
 * without a hole between them.
 *
 * §10.2 gives the two halves — REST hydrates, socket patches — and the order
 * they have to happen in is the interesting part. A socket opened *after* the
 * snapshot misses anything published while the snapshot was in flight; the
 * client asks for "from now on" and "now" has already moved. So the socket goes
 * first and its messages are held, the snapshot lands, and the held ones are
 * applied on top.
 *
 * The overlap is deliberate and harmless: a message published just before the
 * snapshot was taken is applied twice over — once inside the count the snapshot
 * carries, once as itself. It restates a number rather than adding to one,
 * because the client derives nothing (§10.2, rule 2), so the second telling
 * says exactly what the first did.
 *
 * The same function answers a socket gap. When a reconnect resumes from a point
 * the server's outbox no longer holds, the patches that follow cannot be
 * trusted to land on the right state (§8, rule 2) — and the answer is to load
 * the state again, which is this.
 */
async function resync(): Promise<void> {
  held = []

  if (!stream) {
    stream = connect({
      onStatus: patch.link,
      onMessage: (message) => {
        // The socket client handles the welcome itself and does not pass it on;
        // the guard is here so the world model's message union can exclude it
        // rather than every reducer having to.
        if (message.type === 'welcome') return
        if (held) held.push(message)
        else patch.pushed(message)
      },
      onGap: () => void resync(),
    })
  }

  try {
    patch.hydrated(await snapshot())
  } finally {
    // Even if the snapshot failed, the held messages are real and the socket is
    // still delivering. Dropping them because a fetch failed would be the
    // silent skip §8 exists to prevent.
    const waiting = held ?? []
    held = null
    for (const message of waiting) patch.pushed(message)
  }
}

/** The world as the server holds it right now. */
async function snapshot(): Promise<Snapshot> {
  const res = await fetch('/api/world', {
    headers: { Accept: 'application/json' },
    cache: 'no-store',
  })
  if (!res.ok) throw new Error(`the world could not be read (${res.status})`)
  return (await res.json()) as Snapshot
}
