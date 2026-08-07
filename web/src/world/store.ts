/*
 * The one store, and the only things allowed to write to it.
 *
 * technical.md §10.1 states the discipline rule this file exists to keep: **the
 * store is plain TypeScript, never React state.** If the world lived in React,
 * the renderer could only read it at React's cadence — which is the coupling
 * §10.1 rejected `@pixi/react` to avoid, arriving by another route. So the
 * store is `zustand/vanilla`, which is three functions — `getState`,
 * `setState`, `subscribe` — and no framework:
 *
 *   - React reads it through `useSyncExternalStore` (`react.ts`).
 *   - The renderer reads it directly, every frame, and subscribes to nothing
 *     (`../render/renderer.ts`).
 *
 * Neither owns it, and neither can starve the other.
 *
 * `setState` is not exported. Everything that changes the world goes through
 * one of the named patches below, each of which is a domain event rather than a
 * shape — §10.2's "typed domain messages, not generic diffs", applied to the
 * client's own writes as well as to the socket's.
 */

import { createStore } from 'zustand/vanilla'

import type { ProbeResult } from '../commands'
import {
  applyMessage,
  characterChanged,
  hydrated,
  initialWorld,
  panelChanged,
  probed,
  routinesChanged,
  stood,
  type Attempt,
  type Character,
  type Crossing,
  type Forecast,
  type Health,
  type Link,
  type Push,
  type Renderer,
  type Routines,
  type Session,
  type Snapshot,
  type Standing,
  type World,
} from './state'

/**
 * The world. Read it with `getState()` — from a frame loop, a component, or the
 * console — and write it only through `patch`.
 */
export const world = createStore<World>(() => initialWorld)

/**
 * Every write there is.
 *
 * The four small ones carry client-side facts the server has no opinion about
 * (which screen the player is looking at is not one of them — that is React's
 * own state, and it belongs there). The three below them carry the server's
 * statements, and each one names *how* it was told: hydrated over REST, applied
 * from a command's own response, or pushed down the socket.
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
   * The character, as a command that changed them reported back.
   *
   * Every command that touches the character returns the whole of them, so
   * this is one patch rather than one per command — the server states the
   * result and the client stores it, which is the shape §10.2 asks for.
   */
  character(character: Character) {
    world.setState(characterChanged(world.getState(), character))
  },

  /**
   * The world layer, saying where in the painted location the player is
   * standing (§10.3).
   *
   * The one write that comes from the renderer rather than from the server. It
   * carries no game rule — which authored rectangle a walk ended inside is a
   * fact about authored geometry — and it is the seam the event system reads
   * from when placement conditions become live (#21, #22).
   */
  standing(standing: Standing | null) {
    world.setState(stood(world.getState(), standing))
  },

  /**
   * The health panel, as a command that changed it reported back.
   *
   * Living a day and buying an instrument are the same patch, because the
   * server restates the whole panel either way — and buying one changes which
   * metrics carry a reading rather than what any of them says (§3.16).
   */
  panel(health: Health, crossings: Crossing[] = []) {
    world.setState(panelChanged(world.getState(), health, crossings))
  },

  /**
   * The rules for the body, and the fortnight they lead to (§3.4).
   *
   * One patch for the pair, because the forecast is a reading of the body under
   * the rules — there is no instant at which one is current and the other is
   * not.
   */
  routines(routines: Routines, forecast: Forecast) {
    world.setState(routinesChanged(world.getState(), routines, forecast))
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
    const { renderer } = world.getState()
    world.setState({ ...initialWorld, renderer, session: { status: 'out', notice } })
  },
}
