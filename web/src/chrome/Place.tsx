/*
 * Where you are, over the world.
 *
 * Both of these are chrome above a canvas that is still running (§10.1): the
 * chip that names the spot under your feet, and the line in the key bar for a
 * way you are standing in. Neither holds any city knowledge — the names come
 * from the world the server sent, so a place renamed on the server is renamed
 * here without a deploy.
 */

import { named } from '../world/city'
import { useWorld } from '../world/react'

/**
 * Where you are, and where home is.
 *
 * @etm/ui calls this the only orientation the shell gives you — the place names
 * itself in the world through its own signage, and this is the fallback for
 * before there is any.
 */
export function Where() {
  const character = useWorld((w) => w.character)
  const standing = useWorld((w) => w.standing)

  if (!character) {
    return (
      <div className="place-chip">
        <span className="place-name">The world</span>
        <span className="place-where">nobody signed in</span>
      </div>
    )
  }

  // The zone the world layer reports is the more particular answer, and the
  // place is the one it sits inside: standing on your own stoop is standing
  // in Wilmington. `zones` arrives smallest first, so the first is the spot.
  const spot = standing?.zones[0] ?? null

  return (
    <div className="place-chip">
      <span className="place-name">{spot ? spot.name : character.at.name}</span>
      <span className="place-where">
        {spot
          ? character.at.name
          : character.at.key === character.home.key
            ? 'home'
            : `home is ${character.home.name}`}
      </span>
    </div>
  )
}

/**
 * The ways out of the spot the character is standing on.
 *
 * A portal is an authored object in the map (§10.3) and it is where a way *is*:
 * walk onto it and you go through it. Which is why this says "you are standing
 * on one" rather than offering a button — the input is the world, and it goes
 * through the same command layer a button would (§10.1).
 */
export function Ways() {
  const standing = useWorld((w) => w.standing)
  const places = useWorld((w) => w.places)
  if (!standing || standing.portals.length === 0) return null

  return (
    <>
      {standing.portals.map((portal) => (
        <span className="keybar-item" key={portal.key}>
          <span className="keybar-label">
            you are in the way through to {named(places, portal.to).name}
          </span>
        </span>
      ))}
    </>
  )
}
