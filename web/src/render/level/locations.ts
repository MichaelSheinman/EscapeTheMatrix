/*
 * Which places in the city are painted, and where their maps live.
 *
 * `internal/character/city.go` holds the city as a closed vocabulary of place
 * keys; this is the client's answer to "is there a location behind that key
 * yet?" — one entry per painted location, and nothing for the rest.
 *
 * A place with no entry is not an error and is not hidden. The world draws the
 * ground and names the place, and says plainly that it has not been painted —
 * which is true, and stays true until §3.12's districts are built out (#35).
 *
 * **This is not the location graph.** #12 owns that: which location leads to
 * which, what an interior is, and what gets preloaded when you approach a door.
 * What is here is the flat lookup that ticket replaces, kept in one place so
 * replacing it is an edit rather than a search.
 */

/** Place key → the Tiled map to load for it. Served from `web/public`. */
export const PAINTED: Readonly<Record<string, string>> = {
  'east-side': '/locations/east-side-street/east-side-street.tmj',
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
