/*
 * Reading the city the server sent.
 *
 * technical.md §10.2, rule 2: the client computes no game rules. Everything
 * in this file is a lookup or a grouping over `world.places` — which place is
 * that key, what leads out of here, which interiors belong to which street.
 * Nothing here decides a fare, finds a route, or works out whether a journey is
 * allowed. Those are the city's (`internal/city`), and asking is a command.
 *
 * That is a sharper line than it looks. A screen *could* walk the ways and add
 * up a route itself, and it would be right almost all of the time — and the
 * first time it was wrong the player would be told a price the server then did
 * not charge. So the screens below offer only what the server already said: the
 * ways out of where you are stand, with their own fares on them, and for
 * anywhere else a button that asks and lets the answer come back.
 */

import type { Place, Way } from './state'

/** One place by key, or null. */
export function placeIn(places: Place[], key: string): Place | null {
  return places.find((p) => p.key === key) ?? null
}

/** Where a way leads, named — or its bare key if the city no longer has it. */
export function named(places: Place[], key: string): Place {
  return placeIn(places, key) ?? { key, name: key, note: '', layer: 'district', in: '', stop: false, ways: [] }
}

/** The ways out of a place, taken one way. */
export function waysBy(place: Place | null, mode: Way['mode']): Way[] {
  return (place?.ways ?? []).filter((w) => w.mode === mode)
}

/** The way from a place to another, if there is a direct one. */
export function wayTo(place: Place | null, to: string, mode: Way['mode']): Way | null {
  return (place?.ways ?? []).find((w) => w.to === to && w.mode === mode) ?? null
}

/**
 * Everywhere one step from here, whatever the mode, each named once.
 *
 * What the renderer preloads (§10.4's "adjacent-location preload"): the places
 * the player could be looking at next. Adjacency rather than a radius, because
 * a radius on a city this small is the whole city, which is the texture budget
 * the same section asks for.
 */
export function neighbours(place: Place | null): string[] {
  const out: string[] = []
  for (const way of place?.ways ?? []) {
    if (!out.includes(way.to)) out.push(way.to)
  }
  return out
}

/** The streets, in the order the city authored them. */
export function districts(places: Place[]): Place[] {
  return places.filter((p) => p.layer === 'district')
}

/** What is inside a street: its home first, then everything else. */
export function inside(places: Place[], district: string): Place[] {
  const within = places.filter((p) => p.in === district)
  return [...within.filter((p) => p.layer === 'home'), ...within.filter((p) => p.layer !== 'home')]
}

/** The district a place is in — itself, if it is one. */
export function district(place: Place | null): string {
  if (!place) return ''
  return place.layer === 'district' ? place.key : place.in
}
