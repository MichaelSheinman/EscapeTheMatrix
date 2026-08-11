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
  Career,
  Challenge,
  Character,
  Endorsement,
  EventLog,
  Conflicts,
  Events,
  Forecast,
  Health,
  Living,
  Phone,
  Playbook,
  PlaybookLog,
  PlaybookRef,
  Push,
  Routines,
  Situations,
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

/**
 * Take rooms somewhere else (§3.15).
 *
 * Not a journey, and deliberately not `moveTo` with a flag on it. That command
 * changes where the character is standing; this changes who they live among,
 * and everything the design hangs off the second — which rule set is in force,
 * whose friction arrives in the morning, what the reading is capped at — follows
 * from home and never from feet.
 *
 * It moves nobody. The player signs for the rooms and is still standing where
 * they were when they did it.
 */
export async function relocate(to: string): Promise<void> {
  await send(
    'character.relocate',
    { to },
    (result: { character: Character; living: Living; conflicts: Conflicts }) =>
      patch.relocated(result.character, result.living, result.conflicts),
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
 * Compose one rule of the playbook (§3.14).
 *
 * The whole sentence goes at once, blanks included, and there is nothing to
 * press afterwards — editing the sentence is the change, and it is what the
 * character does from the next time the situation comes up. A commit step would
 * only be honest if the player could sit and think in a paused world, and they
 * cannot: the world is running behind the screen either way.
 *
 * Every argument is a key out of the vocabulary the server sent. `null` in
 * `when`, `then` or `otherwise` is a part still blank, which is a state a rule
 * is allowed to be in and the reason nothing partial ever runs.
 */
export async function composeRule(rule: {
  environment: string
  question: string
  when: (PlaybookRef | null)[]
  join: string
  then: PlaybookRef | null
  otherwise: PlaybookRef | null
}): Promise<void> {
  await send('playbook.compose', rule, (result: { playbook: Playbook }) =>
    patch.playbook(result.playbook),
  )
}

/**
 * Take one answer back (§3.14).
 *
 * A command of its own rather than composing a sentence of nulls, because the
 * two mean different things: a stored sentence full of blanks is a rule somebody
 * wrote and did not finish, and no rule at all is a question they have never
 * answered. Both are out of force and only one of them is true — and a client
 * that had to blank four parts one at a time to say "I take that back" would
 * store the first meaning on its way to the second.
 *
 * It reaches one question in one environment. §3.15's whole mechanism is that
 * the same question has different answers in different places, so a clear that
 * took the answer everywhere would be a way to lose a week's thinking at once.
 */
export async function clearRule(environment: string, question: string): Promise<void> {
  await send('playbook.clear', { environment, question }, (result: { playbook: Playbook }) =>
    patch.playbook(result.playbook),
  )
}

/**
 * What the rules did while the player was elsewhere (§3.14), and what a week of
 * it adds up to (§3.6).
 *
 * A GET, and returned rather than patched into the store — the second
 * pull-shaped surface in this client after the leaderboards, and one for the
 * same reason: history grows for as long as somebody plays, and a world model is
 * bounded (§10.2). A year of moments in the boot snapshot would put the last of
 * them out of reach forever.
 *
 * Null when it could not be read. The screen says it has no log rather than
 * inventing an error state, because the socket's own status already says whether
 * the server is there.
 */
export async function loadPlaybookLog(page: number): Promise<PlaybookLog | null> {
  try {
    const res = await fetch(`/api/playbook/log?page=${page}`, { headers: json, cache: 'no-store' })
    if (!res.ok) return null
    return (await res.json()) as PlaybookLog
  } catch {
    return null
  }
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
 * Say you like the player the game offered you (§3.5).
 *
 * The only argument is the offer's own id, and that is the shape of the feature
 * rather than an optimisation: there is nobody to look up and nothing to type,
 * because the server named one person and the answer is about that. Which is
 * also why the leaderboards can go on sending names and never account ids — the
 * one thing that needed a handle turns out not to need one.
 *
 * The player who was liked is told over the socket. They did not ask, they were
 * not here, and they do not have to do anything about it: endorsement is
 * asynchronous by construction, which is why §5 keeps it in V1 while everything
 * else player-to-player waits.
 */
export async function endorse(offer: number): Promise<void> {
  await send('social.endorse', { offer }, (result: { endorsement: Endorsement }) =>
    patch.endorsement(result.endorsement),
  )
}

/**
 * Let the offer go (§3.5).
 *
 * It buys no replacement — the next one comes on the server's cadence either
 * way — and it tells the other player nothing. What it buys is not having to
 * press a button you mean nothing by, which is the difference between a signal
 * and a habit.
 */
export async function passOnOffer(offer: number): Promise<void> {
  await send('social.pass', { offer }, (result: { endorsement: Endorsement }) =>
    patch.endorsement(result.endorsement),
  )
}

/**
 * Take up what the world put in front of you (§4).
 *
 * The only argument is the offer's own id, and that is the shape of the feature
 * rather than an economy: release is a server decision, so there is nothing to
 * choose between and no event key a client could name. What the response says
 * is either what it came to, or that it opened something which finishes later —
 * a challenge, a job situation, a conflict — in which case nothing has happened
 * yet and the screen says so rather than inventing an outcome.
 */
export async function acceptEvent(event: number): Promise<string> {
  let after = ''
  await send(
    'event.accept',
    { event },
    (result: { events: Events; after: string; challenge: Challenge | null }) => {
      patch.events(result.events)
      after = result.after
      // §4's doorway: an event authored to open a challenge begins its run in
      // the same transaction as the acceptance, so saying yes and being inside
      // the thing you said yes to are one round trip. Most events carry no
      // challenge and this is null, which is the ordinary case.
      if (result.challenge) patch.challenge(result.challenge)
    },
  )
  return after
}

/**
 * Let it go (§4, §3.9).
 *
 * **Declining does not consume it.** The event goes back into the world's
 * backlog and may be put to the player again later — which is why nothing here
 * says it is gone, and why the button is worded as walking away rather than as
 * refusing. Walking out of the place it was put has exactly the same effect and
 * needs no command at all.
 */
export async function declineEvent(event: number): Promise<void> {
  await send('event.decline', { event }, (result: { events: Events }) => {
    patch.events(result.events)
  })
}

/**
 * Walk up to a challenge that is standing there and begin it (§5).
 *
 * One command for starting and for resuming, because they are one thing:
 * §5.1 makes save and resume mandatory, so a player opening a challenge does
 * not know — and does not have to know — which of the two they are doing. The
 * server answers with the scene they are standing in either way.
 */
export async function beginChallenge(challenge: string): Promise<void> {
  await send('challenge.begin', { challenge }, (result: { challenge: Challenge }) => {
    patch.challenge(result.challenge)
  })
}

/**
 * Commit to one of the authored positions (§3.10).
 *
 * The scene travels with the option because an option key is only unique within
 * its scene, and because naming it is what makes a stale tab's answer refusable
 * rather than applied twice — the run has moved on, and the reply says so.
 *
 * What comes back is the next scene, or the ending: an option with nothing
 * after it finishes the run, and the response carries the reaction authored for
 * the outcome the whole passage landed in. Nothing about how it was scored
 * comes back with it, ever (§5.1).
 */
export async function chooseInChallenge(
  challenge: string,
  scene: string,
  option: string,
): Promise<void> {
  await send(
    'challenge.choose',
    { challenge, scene, option },
    (result: { challenge: Challenge }) => patch.challenge(result.challenge),
  )
}

/**
 * Take the job that is being taken on where you are standing (§3.14).
 *
 * Named, it is the board's door (2026-08-08): a posting on the J screen,
 * answered from wherever the player is standing, the way people actually get
 * hired. With no argument it is the map's — take whatever is being taken on
 * right here — kept because a vacancy you are standing in front of is still a
 * door. The server refuses both for the same reasons, so neither is a way
 * around the other.
 */
export async function takeJob(role?: string): Promise<void> {
  await send('job.take', role ? { role } : {}, (result: { career: Career; character: Character }) => {
    patch.career(result.career, result.character)
  })
}

/**
 * Give the job up.
 *
 * The one command in this client that undoes a standing arrangement, and it
 * exists because §3.14's ladder only teaches its trade if walking back down it
 * is a real option. What it does not undo is the interview: a challenge is once
 * ever (§5.1), so the rung stays reached and the job can be walked back into.
 */
export async function leaveJob(): Promise<void> {
  await send('job.leave', {}, (result: { career: Career; character: Character }) => {
    patch.career(result.career, result.character)
  })
}

/**
 * Decide one of the moments your job produced while you were elsewhere
 * (§3.14's step 4, by hand).
 *
 * The one command this client has for delegated work, and there is deliberately
 * no second: nothing here asks a role for work, because §3.14's loop begins "it
 * happens without you" and a button that produced a situation would be a job
 * with a lever on it.
 *
 * What comes back is the sentence the content wrote for the answer — the only
 * account of how it went the player gets — and the surface behind it, because a
 * role that was waiting starts producing work again the moment it stops.
 */
export async function answerSituation(situation: number, answer: string): Promise<string> {
  let came = ''
  await send(
    'situation.answer',
    { situation, answer },
    (result: { situations: Situations; came: string; did: string }) => {
      patch.situations(result.situations)
      came = result.came
    },
  )
  return came
}

/**
 * Settle one of the conflicts the people you live among produced while you were
 * getting on with your life (§3.15's step 3, by hand).
 *
 * The one command this client has for daily conflicts, and there is deliberately
 * no second: nothing here asks the neighbourhood for friction, because friction
 * is what living somewhere *is* and a button for it would be a doorbell with
 * your own finger on it.
 *
 * What comes back is the sentence the content wrote for the answer, and the
 * whole surface behind it — because settling one moves what stands between the
 * two of you, and that is what tomorrow's conflict will follow from.
 */
export async function answerConflict(conflict: number, answer: string): Promise<string> {
  let came = ''
  await send(
    'conflict.answer',
    { conflict, answer },
    (result: { conflicts: Conflicts; came: string; did: string }) => {
      patch.conflicts(result.conflicts)
      came = result.came
    },
  )
  return came
}

/**
 * What the world has put in front of this player before, and what they did
 * about it (§4).
 *
 * A GET and a page, for the third time in this client and for the same reason
 * as the other two: history grows for as long as somebody plays and a world
 * model is bounded (§10.2). Note what cannot be fetched — the backlog. A client
 * that could read what was queued could walk towards it.
 */
export async function loadEventLog(page: number): Promise<EventLog | null> {
  try {
    const res = await fetch(`/api/events/log?page=${page}`, { headers: json, cache: 'no-store' })
    if (!res.ok) return null
    return (await res.json()) as EventLog
  } catch {
    return null
  }
}

/**
 * The phone: everybody this player has had anything with, and what (§3.15).
 *
 * A GET and no page, unlike the three fetched surfaces above it. Threads are
 * bounded by the population rather than by time — six people, not six hundred —
 * and the server bounds how far back each one reads, so what comes back is
 * already the size of a phone. A conversation paged backwards is not a
 * conversation.
 *
 * Nothing is thrown. The screen either has threads or does not.
 */
export async function loadPhone(): Promise<Phone | null> {
  try {
    const res = await fetch('/api/phone', { headers: json, cache: 'no-store' })
    if (!res.ok) return null
    return (await res.json()) as Phone
  } catch {
    return null
  }
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

/**
 * Walk out of a challenge (§5).
 *
 * It puts the challenge back to never having happened rather than spending it:
 * the run is deleted, so it can be met again. Nothing is taken back because
 * nothing was paid — a challenge moves factors and money on completion and only
 * on completion, so half a run has moved nothing.
 */
export async function abandonChallenge(challenge: string): Promise<void> {
  await send('challenge.abandon', { challenge }, (result: { challenge: Challenge }) => {
    patch.challenge(result.challenge)
  })
}

/* ── the developer's toolbar ──────────────────────────────────────────────
 *
 * Four commands that only exist outside production — the server does not
 * register them there, so these four functions get a 404 and the toolbar says
 * so. That is deliberately the *only* thing gating them on this side: a client
 * that decided for itself whether it was a development build would be a client
 * somebody could talk into being one.
 *
 * They are here rather than in the component for the same reason every other
 * command is: this file is the one path into the command layer, and a panel
 * that sent its own would have its own idea of what the answer means.
 */

/** What the toolbar shows: every authored challenge, and this player's state for it. */
export type DevChallenge = {
  challenge: string
  name: string
  mechanic: string
  gate: string
  opens: string
  places: string[]
  on: boolean
  played: boolean
  running: boolean
}

export type DevState = {
  challenges: DevChallenge[]
  notice: string
}

/**
 * Apply a toolbar command's answer.
 *
 * It carries the two player surfaces it just moved, so enabling the fire puts
 * it on screen without a reload — the same restate-your-own-slice rule every
 * other command in this client follows.
 */
function applyDev(result: {
  challenges: DevChallenge[]
  challenge: Challenge
  events: Events
  notice: string
}): void {
  patch.challenge(result.challenge)
  patch.events(result.events)
  patch.dev({ challenges: result.challenges, notice: result.notice })
}

/** Read the catalogue. Silently does nothing where the commands do not exist. */
export async function devChallenges(): Promise<void> {
  await send('dev.challenges', {}, applyDev)
}

/** Open a challenge's gate, and raise its doorway if it has one. */
export async function devEnable(challenge: string): Promise<void> {
  await send('dev.enable', { challenge }, applyDev)
}

/** Shut the gate again. */
export async function devDisable(challenge: string): Promise<void> {
  await send('dev.disable', { challenge }, applyDev)
}

/** Put one challenge back to never having happened, so it can be played again. */
export async function devReset(challenge: string): Promise<void> {
  await send('dev.reset', { challenge }, applyDev)
}

/** The world as the server holds it right now. */
async function snapshot(): Promise<Snapshot> {
  const res = await fetch('/api/world', {
    headers: { Accept: 'application/json', ...deviceZone() },
    cache: 'no-store',
  })
  if (!res.ok) throw new Error(`the world could not be read (${res.status})`)
  return (await res.json()) as Snapshot
}

/**
 * The IANA timezone this device reports, as a header for the server to keep.
 *
 * §3.3 measures the player's whole day in their own timezone, and the browser
 * is the only party that knows it — sign-in is a redirect from Google and
 * cannot say. It rides the snapshot rather than being a command because it is
 * not a decision the player makes; it is a fact about their device, restated
 * on every arrival and written server-side only when it changed.
 */
function deviceZone(): Record<string, string> {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
    return zone ? { 'Etm-Timezone': zone } : {}
  } catch {
    return {}
  }
}
