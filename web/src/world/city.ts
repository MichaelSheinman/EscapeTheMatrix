/*
 * Reading the city the server sent.
 *
 * technical.md §10.2, rule 2: the client computes no game rules. Everything
 * in this file is a lookup or a grouping over `world.places` — which place is
 * that key, what leads out of here. Nothing here finds a route or works out
 * whether a journey is allowed. Those are the city's (`internal/city`), and
 * asking is a command.
 */

import type { Place } from './state'

/** One place by key, or null. */
export function placeIn(places: Place[], key: string): Place | null {
  return places.find((p) => p.key === key) ?? null
}

/** Where a way leads, named — or its bare key if the city no longer has it. */
export function named(places: Place[], key: string): Place {
  return placeIn(places, key) ?? { key, name: key, note: '', layer: 'district', in: '', stop: false, ways: [] }
}

/**
 * Everywhere one step from here, each named once.
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
