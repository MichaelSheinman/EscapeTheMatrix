/*
 * Which places in the city have been painted, and where their maps live.
 *
 * The city itself — every place, every way between them, and what a journey
 * costs — is the server's, sent with the world and read through
 * `../../world/city.ts`. **This is not that.** What is here is the one question
 * the server cannot answer, because it is a question about this build's assets:
 * has anybody painted that place yet?
 *
 * A place with no entry is not an error and is not hidden. The world draws the
 * ground, names the place, and says plainly that it has not been painted —
 * which is true, and stays true until §3.12's districts and their interiors are
 * built out (#35). Every place still has its ways out, because a way is a fact
 * about the city and a portal is only where that way was drawn (§10.3).
 */

/** Place key → the Tiled map to load for it. Served from `web/public`. */
const PAINTED: Readonly<Record<string, string>> = {
  'east-side': '/locations/east-side/east-side.tmj',
  'east-side-apartment': '/locations/east-side-apartment/east-side-apartment.tmj',
}

/** The map for a place, or null if nobody has painted it. */
export function mapFor(place: string): string | null {
  return PAINTED[place] ?? null
}

/** Every place with a painting, in no particular order. */
export function painted(): string[] {
  return Object.keys(PAINTED)
}

/**
 * What the world shows before anybody has signed in.
 *
 * The canvas is created at boot and outlives every screen including the door
 * (§10.1), so there is always a location on it. This is a street, not a player
 * — nothing behind the door is anybody's, and the character who would walk it
 * is not drawn until there is one.
 */
export const AT_THE_DOOR = 'east-side'
