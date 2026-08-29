/*
 * Which places in the city have been painted, and where their maps live.
 *
 * The city itself — every place, every way between them, and what a journey
 * costs — is the server's, sent with the world and read through
 * `../../world/city.ts`. This is not that. What is here is the one question
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
  'wilmington': '/locations/wilmington/wilmington.tmj',
  'wilmington-apartment': '/locations/wilmington-apartment/wilmington-apartment.tmj',
  'wilmington-apartment-washroom':
    '/locations/wilmington-apartment-washroom/wilmington-apartment-washroom.tmj',
  /*
   * The one location here that is not a place.
   *
   * Every key above is somewhere in `internal/city`'s graph, and the renderer
   * reaches them by following where the character is. This one is named by a
   * *challenge* — a run of `house_fire/a` stands in it for its six minutes and
   * nobody can walk here before or after, because the city has no way into it
   * and never will.
   *
   * It is in the same table as the rest because a painting is a painting: it
   * loads, unloads and budgets identically, and the only difference is which
   * question puts it on screen.
   */
  'house-fire': '/locations/house-fire/house-fire.tmj',
  /*
   * SCAFFOLDING. The art here is a copy of the Wilmington apartment's, and it is
   * a stand-in for a painting of the house.
   *
   * What it buys is the way in: `chrome/markers` turns a door into a `soon`
   * marker that says nobody has built the inside yet unless this table names
   * its destination, so an entry is what makes the bodega openable. Nobody sees
   * it — `chrome/TheHouse` covers the viewport with the game for as long as
   * they are inside — so a wrong room under it is invisible rather than wrong
   * on screen.
   */
  'wilmington-house': '/locations/wilmington-house/wilmington-house.tmj',
  /*
   * SCAFFOLDING, the same way wilmington-house is: a copy of the apartment's
   * art standing in for the studio. chrome/TheStudio covers the viewport with
   * Bemuse for as long as anybody is inside.
   */
  'wilmington-dance': '/locations/wilmington-dance/wilmington-dance.tmj',
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
export const AT_THE_DOOR = 'wilmington'
