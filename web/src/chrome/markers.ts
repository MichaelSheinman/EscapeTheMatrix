/*
 * What the markers over the world say.
 *
 * The *where* is authored in the maps — portals and trigger zones are read out
 * of the level (§10.3) — and the *what to say* is here: a label, one sentence
 * in the world's voice, and what acting on it opens. Every location explains
 * itself; a labelled point that will not say what it is for reads as a
 * destination and then refuses to be one.
 *
 * Furnishings are client-side markers for things the placeholder plates do not
 * author yet — the board on the wall, the computer, the closet. They are pinned
 * at plate coordinates the same way a zone is, and they go away location by
 * location as #35's interiors author real objects to hang them on.
 */

import type { SceneView } from '../render/scene'
import { mapFor } from '../render/level/locations'
import type { Point } from '../render/level/geometry'
import type { Place } from '../world/state'
import { GENERATED } from './markers.gen'

/**
 * The screens a marker may open. A subset of the app's own screen names.
 *
 * `among` is deliberately absent while the people-you-live-among screen is out
 * of the build: the Saturday market is still authored in the generated set, and
 * a name this list does not know falls back to a sign below — so the market
 * stays on the map, still says what it is, and does not open anything.
 */
export type MarkerScreen =
  | 'playbook'
  | 'work'
  | 'body'
  | 'map'
  | 'closet'
  | 'events'
  | 'rules'

const SCREENS: ReadonlySet<string> = new Set([
  'playbook',
  'work',
  'body',
  'map',
  'closet',
  'events',
  'rules',
])

export type WorldMarker = {
  id: string
  /**
   * A door, a thing you can use, only a name on a building — or somewhere the
   * design calls for and the build has not reached, which is shown and named
   * rather than left off the map, and cannot be entered.
   */
  kind: 'portal' | 'use' | 'sign' | 'soon'
  label: string
  /** One sentence, in the world's voice. */
  tip: string
  /** A short clause on the label — what acting on it gets you. */
  hint?: string
  at: Point
  /** A portal's destination — walking in or pressing O goes there. */
  go?: string
  /** The overlay the O key opens. */
  screen?: MarkerScreen
  /**
   * The challenge item this marker *is* — the same key the run offers as an
   * option, so pressing O on it commits that choice.
   *
   * The third thing a marker can do, beside going somewhere and opening a
   * screen, and the only one that changes what a run has committed. It exists
   * because a room full of things you can only interact with through a panel is
   * a room that may as well be a list, and §3.12's whole argument is that
   * walking is where things happen.
   */
  item?: string
}

/** How far the O key reaches, and how near "near" is, in plate pixels. */
export const REACH = 130

/** What stepping through each doorway means, keyed by where it leads. */
const DOORWAYS: Record<string, { tip: string; hint?: string }> = {
  'east-side-apartment': {
    tip: 'Up three flights to your own room. The board, the closet, the computer and the bed are all up there.',
    hint: 'home',
  },
  'east-side': {
    tip: 'Down three flights and out onto the East Side.',
    hint: 'the street',
  },
  downtown: {
    tip: 'The long blocks west, and the offices past them.',
  },
}

/**
 * Client-side markers per painted location, generated from the same scene data
 * the plates are baked from (tools/bake/gen-tmj.mjs) — so a label sits exactly
 * on the thing it names. A generated screen name that this build does not know
 * falls back to a sign: the place still explains itself, it just does nothing.
 */
function furnishings(key: string): Omit<WorldMarker, 'id'>[] {
  return (GENERATED[key] ?? []).map((m) => {
    const screen = m.screen && SCREENS.has(m.screen) ? (m.screen as MarkerScreen) : undefined
    // A marker naming an item stays usable without naming a screen: what it
    // opens is nothing, and what it does is commit a choice.
    const kind = m.kind === 'use' && !screen && !m.item ? 'sign' : m.kind
    return {
      kind,
      label: m.label,
      tip: m.tip,
      ...(m.hint ? { hint: m.hint } : {}),
      at: m.at,
      ...(screen && kind === 'use' ? { screen } : {}),
      ...(m.item ? { item: m.item } : {}),
    }
  })
}

/**
 * Every marker for what is on screen: the authored doorways, then the
 * client-side furnishings for that location.
 */
export function markersFor(
  view: SceneView,
  places: Place[],
  /*
   * Which challenge items are still on offer, if a run is in progress.
   *
   * The room is painted with every item in it, because the plate is baked once
   * and a burning flat does not have three versions. What changes is which of
   * them you can still take — so the markers are filtered against the run's own
   * choices, and a thing already in the bag stops being something to walk up to.
   *
   * Undefined means "no run": every item marker is dropped, which is what makes
   * the flat inert when nobody is in a challenge in it. The list of choices is
   * the server's, so this cannot offer something `challenge.choose` would
   * refuse.
   */
  offering?: ReadonlySet<string>,
): WorldMarker[] {
  const doors: WorldMarker[] = view.portals.map((portal) => {
    const said = DOORWAYS[portal.to]
    const there = places.find((p) => p.key === portal.to)
    // A door to somewhere unbuilt is shown and named, and does not open —
    // stepping into nothing is how a player gets stuck.
    if (!mapFor(portal.to)) {
      return {
        id: `portal:${portal.key}`,
        kind: 'soon' as const,
        label: portal.name,
        tip: said?.tip ?? `${there?.name ?? portal.name}. Nobody has built the inside of this yet.`,
        at: portal.at,
      }
    }
    return {
      id: `portal:${portal.key}`,
      kind: 'portal' as const,
      label: portal.name,
      hint: said?.hint,
      tip: said?.tip ?? `Through here to ${there?.name ?? portal.to.replace(/-/g, ' ')}.`,
      at: portal.at,
      go: portal.to,
    }
  })

  const set = furnishings(view.key).filter(
    (marker) => !marker.item || (offering?.has(marker.item) ?? false),
  )
  return doors.concat(set.map((marker, i) => ({ ...marker, id: `${view.key}:${i}` })))
}

/** The nearest marker the O key could act on, or null if nothing is in reach. */
export function nearestTo(markers: WorldMarker[], player: Point | null): WorldMarker | null {
  if (!player) return null
  let best: WorldMarker | null = null
  let bestGap = REACH
  for (const marker of markers) {
    if (!marker.go && !marker.screen && !marker.item) continue
    const gap = Math.hypot(marker.at.x - player.x, marker.at.y - player.y)
    if (gap < bestGap) {
      bestGap = gap
      best = marker
    }
  }
  return best
}
