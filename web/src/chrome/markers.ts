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

/** The screens a marker may open. A subset of the app's own screen names. */
export type MarkerScreen =
  | 'playbook'
  | 'work'
  | 'body'
  | 'map'
  | 'closet'
  | 'events'
  | 'rules'
  | 'among'

const SCREENS: ReadonlySet<string> = new Set([
  'playbook',
  'work',
  'body',
  'map',
  'closet',
  'events',
  'rules',
  'among',
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
    const kind = m.kind === 'use' && !screen ? 'sign' : m.kind
    return {
      kind,
      label: m.label,
      tip: m.tip,
      ...(m.hint ? { hint: m.hint } : {}),
      at: m.at,
      ...(screen && kind === 'use' ? { screen } : {}),
    }
  })
}

/**
 * Every marker for what is on screen: the authored doorways, then the
 * client-side furnishings for that location.
 */
export function markersFor(view: SceneView, places: Place[]): WorldMarker[] {
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

  const set = furnishings(view.key)
  return doors.concat(set.map((marker, i) => ({ ...marker, id: `${view.key}:${i}` })))
}

/** The nearest marker the O key could act on, or null if nothing is in reach. */
export function nearestTo(markers: WorldMarker[], player: Point | null): WorldMarker | null {
  if (!player) return null
  let best: WorldMarker | null = null
  let bestGap = REACH
  for (const marker of markers) {
    if (!marker.go && !marker.screen) continue
    const gap = Math.hypot(marker.at.x - player.x, marker.at.y - player.y)
    if (gap < bestGap) {
      bestGap = gap
      best = marker
    }
  }
  return best
}
