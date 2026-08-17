/*
 * The one store, and the only things allowed to write to it.
 *
 * The store is plain TypeScript rather than React state (§10.1). If the world
 * lived in React the renderer could only read it at React's cadence, which is
 * the coupling §10.1 rejected `@pixi/react` to avoid. So it is
 * `zustand/vanilla`: `getState`, `setState`, `subscribe`, and no framework.
 * React reads it through `useSyncExternalStore` (`react.ts`) and the renderer
 * reads it directly every frame and subscribes to nothing. Neither owns it and
 * neither can starve the other.
 *
 * `setState` is not exported. Everything that changes the world goes through one
 * of the named patches below, each a domain event rather than a shape (§10.2).
 */

import { createStore } from 'zustand/vanilla'

import type { ProbeResult } from '../commands'
import { rememberTier, shelf, type Tier } from '../render/quality'
import {
  applyMessage,
  characterChanged,
  delegated,
  employed,
  framesMeasured,
  hydrated,
  initialWorld,
  listed,
  living,
  offered,
  panelChanged,
  probed,
  qualityChosen,
  qualityDemoted,
  released,
  relocated,
  routinesChanged,
  ruled,
  stood,
  travelled,
  underway,
  type Attempt,
  type Boards,
  type Career,
  type Challenge,
  type Character,
  type Conflicts,
  type Crossing,
  type DevConversations,
  type DevToolbar,
  type Endorsement,
  type Events,
  type Forecast,
  type Heading,
  type Health,
  type Link,
  type Living,
  type Look,
  type Phase,
  type Playbook,
  type Push,
  type Renderer,
  type Routines,
  type Session,
  type Situations,
  type Snapshot,
  type Standing,
  type Trip,
  type World,
} from './state'

/**
 * The world. Read it with `getState()` — from a frame loop, a component, or the
 * console — and write it only through `patch`.
 */
export const world = createStore<World>(() => initialWorld)

/**
 * Where this browser remembers the outfit, and which shape it is in. A `Look`
 * written by one build is read by whatever build the player loads next, and the
 * reader is `JSON.parse` followed by a cast. Bumping the suffix when the shape
 * changes makes a stale entry miss by name rather than crash, and the outfit
 * starts over.
 *
 * `main.tsx` reads it, and carries the unversioned `etm.look` forward once
 * before dropping it.
 */
export const LOOK_KEY = 'etm.look:v1'

/**
 * Every write there is. The four small ones carry client-side facts the server
 * has no opinion about. The rest carry the server's statements, and each names
 * how it was told: hydrated over REST, applied from a command's response, or
 * pushed down the socket.
 */
export const patch = {
  /** The server answered `GET /api/auth/session`, or a command 401'd. */
  session(session: Session) {
    world.setState({ session })
  },

  /** The socket connected, dropped, or is trying again. */
  link(link: Link) {
    world.setState({ link })
  },

  /** The renderer started, or the device cannot run it at all (§10.4). */
  renderer(renderer: Renderer) {
    world.setState({ renderer })
  },

  /**
   * The character's clock crossed a boundary: night from 8 PM, day from 6 AM
   * (§3.1). Written by `clock.ts` on its slow tick, and by nothing else.
   */
  phase(phase: Phase) {
    world.setState({ phase })
  },

  /**
   * The held arrow keys changed. Null when the last of them lifted.
   *
   * Written on key transitions only, never per frame — the renderer reads it at
   * its own cadence like everything else in the store (§10.1).
   */
  heading(heading: Heading | null) {
    world.setState({ heading })
  },

  /**
   * The player dressed themselves (client-side, cosmetic — see `Look`).
   *
   * Remembered in this browser like the effects tier, and for the same shape of
   * reason: the server has no appearance model yet, so there is nothing for it
   * to have an opinion about.
   */
  look(look: Look) {
    try {
      localStorage.setItem(LOOK_KEY, JSON.stringify(look))
    } catch {
      // A browser refusing storage refuses the memory, not the outfit.
    }
    world.setState({ look })
  },

  /**
   * Draw at one of §10.4's two tiers from now on. Remembered in this browser as
   * well as applied, because the tier is an answer about this machine and the
   * server has no opinion about it. `chosen` is false only at boot, when what is
   * being restored is a decision somebody already made.
   */
  quality(tier: Tier, chosen = true) {
    rememberTier(shelf(), tier)
    world.setState(qualityChosen(world.getState(), tier, chosen))
  },

  /**
   * The renderer's frame clock says this machine cannot carry the tier (§10.4).
   *
   * Remembered too, so a machine that has been thrown back once does not spend
   * the first three seconds of every session being thrown back again.
   */
  demoted() {
    const dropped = qualityDemoted(world.getState())
    rememberTier(shelf(), dropped.quality.tier)
    world.setState(dropped)
  },

  /** How fast the world is drawing. Ignored when the number has not moved. */
  frames(fps: number) {
    world.setState(framesMeasured(world.getState(), fps))
  },

  /** A command left, landed, or was refused — whoever sent it. */
  attempt(attempt: Attempt) {
    world.setState({ attempt })
  },

  /** `GET /api/world` — the boot snapshot, and the answer to a socket gap. */
  hydrated(snapshot: Snapshot) {
    world.setState(hydrated(world.getState(), snapshot))
  },

  /** The response to a probe this tab sent (§8, rule 3). */
  probed(result: ProbeResult) {
    world.setState(probed(world.getState(), result))
  },

  /**
   * The character, as a command that changed them reported back. Every command
   * that touches the character returns all of them, so this is one patch rather
   * than one per command (§10.2).
   */
  character(character: Character) {
    world.setState(characterChanged(world.getState(), character))
  },

  /**
   * A journey, as the command that made it reported it back (§3.12). The pair
   * rather than either half: a fare paid and a place arrived at are one event.
   */
  travelled(character: Character, trip: Trip) {
    world.setState(travelled(world.getState(), character, trip))
  },

  /**
   * Somewhere else is home (§3.15). The character carries the new address, the
   * slice carries which of the city's homes is yours, and the third is the
   * people themselves.
   */
  relocated(character: Character, where: Living, conflicts: Conflicts) {
    world.setState(relocated(world.getState(), character, where, conflicts))
  },

  /**
   * Where in the painted location the player is standing (§10.3) — the one write
   * that comes from the renderer rather than the server. It carries no game
   * rule: which authored rectangle a walk ended inside is a fact about authored
   * geometry.
   */
  standing(standing: Standing | null) {
    world.setState(stood(world.getState(), standing))
  },

  /**
   * The health panel, as a command that changed it reported back. Living a day
   * and buying an instrument are the same patch, because the server restates the
   * whole panel either way and buying one changes which metrics carry a reading
   * rather than what any says (§3.16).
   */
  panel(health: Health, crossings: Crossing[] = []) {
    world.setState(panelChanged(world.getState(), health, crossings))
  },

  /**
   * The rules for the body, and the fortnight they lead to (§3.4). One patch,
   * because the forecast is a reading of the body under the rules and there is
   * no instant at which one is current and the other is not.
   */
  routines(routines: Routines, forecast: Forecast) {
    world.setState(routinesChanged(world.getState(), routines, forecast))
  },

  /**
   * The rules for judgement, as the command that changed one reported back
   * (§3.14).
   *
   * The whole playbook rather than the rule that moved: the vocabulary and the
   * rules are one statement, and a patch carrying only the edited sentence
   * would leave them able to disagree about which words exist.
   */
  playbook(playbook: Playbook) {
    world.setState(ruled(world.getState(), playbook))
  },

  /**
   * The leaderboards, as `GET /api/boards` answered (§3.5).
   *
   * The one server statement that arrives from neither the boot snapshot nor a
   * command: a board is a read-only paginated list, fetched when somebody looks
   * at it (§10.2). It lives in the store rather than in the component that asked
   * because the socket patches it — a rank moves when somebody else plays.
   */
  boards(boards: Boards) {
    world.setState(listed(world.getState(), boards))
  },

  /**
   * Peer endorsement, as the command that answered an offer reported back
   * (§3.5).
   *
   * Liking somebody and passing on them are one patch, because the server
   * restates the whole surface either way — what changed is which of them is
   * now true about a stranger, and that is the server's sentence to write.
   */
  endorsement(endorsement: Endorsement) {
    world.setState(offered(world.getState(), endorsement))
  },

  /**
   * What the world has put in front of the player, as an answer's response
   * restated it (§4).
   *
   * Accepting and declining are one patch, because the server restates the whole
   * surface either way — and because what a decline leaves behind is not a
   * changed offer but the absence of one. The event itself is still in the
   * backlog, which is the server's business and never this client's.
   */
  events(events: Events) {
    world.setState(released(world.getState(), events))
  },

  /**
   * The challenge surface, as the command that moved it reported back (§5).
   *
   * Beginning, committing to a position and finishing are one patch: the server
   * restates the whole surface for each of them, and a client holding a run and
   * a doorway from two different moments would be offering a way into something
   * it is already inside.
   */
  challenge(challenge: Challenge) {
    world.setState(underway(world.getState(), challenge))
  },

  /**
   * The developer's toolbar answered (dev.go).
   *
   * A plain assignment rather than a reducer, because unlike everything else in
   * here it is not a slice of the world — it is a list of what is authored and
   * what this player has done with it, which no rule of the game is written
   * against. The two *world* surfaces a toolbar command moves are patched
   * through `challenge` and `events` above, so they go through the same
   * reducers a player's own command would.
   */
  dev(dev: DevToolbar) {
    world.setState({ dev })
  },

  /**
   * The toolbar's conversation half, as the command that moved it reported back.
   *
   * Its own assignment beside `dev` rather than merged into it, so a challenge
   * command and a conversation command cannot blank each other's list. The one
   * *world* surface a conversation command moves — the waiting conflict — is
   * patched through `conflicts` below, so it goes through the same reducer a
   * player's own answer would.
   */
  devTalk(devTalk: DevConversations) {
    world.setState({ devTalk })
  },

  /**
   * The job, as the command that moved it reported back (§3.14).
   *
   * The character comes with it because taking or leaving one changes what a
   * day is worth: the two arriving apart would be a balance and a wage that
   * disagree for one render.
   */
  career(career: Career, character: Character) {
    world.setState(employed(world.getState(), career, character))
  },

  /**
   * What a delegated role has waiting, as the command that answered one
   * reported back (§3.14).
   *
   * The whole surface rather than the moment that was settled: a role that was
   * waiting starts producing work again the second it stops, so there may well
   * be another one behind the one just answered.
   */
  situations(situations: Situations) {
    world.setState(delegated(world.getState(), situations))
  },

  /**
   * What living among people has come to, as the command that settled one
   * reported back (§3.15).
   *
   * The whole surface rather than the conflict that was settled, for the same
   * reason and one more: the counts behind it move every time one is answered,
   * and a screen showing yesterday's pattern beside today's decision would be
   * showing the evidence and the moment from two different days.
   */
  conflicts(conflicts: Conflicts) {
    world.setState(living(world.getState(), conflicts))
  },

  /** One message off the socket, through its own reducer. */
  pushed(message: Push) {
    world.setState(applyMessage(world.getState(), message))
  },

  /**
   * Forget the world.
   *
   * Signing out is the one moment the client is meant to know less than it did.
   * What is left behind after it is exactly the two things that are still true:
   * that nobody is signed in, and whatever the canvas is doing.
   */
  cleared(notice: string | null) {
    const { renderer, quality, look, phase } = world.getState()
    // What survives is exactly what is still true after signing out: what the
    // canvas is doing, and how it was asked to draw. The effects tier and the
    // outfit are preferences of this machine and nobody signed out of the
    // machine — and the sky is still whatever time it is.
    world.setState({
      ...initialWorld,
      renderer,
      quality,
      look,
      phase,
      session: { status: 'out', notice },
    })
  },
}
