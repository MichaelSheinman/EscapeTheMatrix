import { useEffect, useState } from 'react'

import { moveTo } from '../world/actions'
import { useWorld } from '../world/react'

import './house.css'

/*
 * The house on the strip.
 *
 * A separate game — The House, by Artur Kot, vendored under
 * web/public/the-house — standing in a city interior. Walking in opens it over
 * the world; the button closes it and leaves the character where they were.
 *
 * An iframe rather than a port. The game is a 2013 jQuery application that
 * paints its own rooms out of DOM and CSS and walks its character over its own
 * grid, and none of that is expressible through the renderer. What makes the
 * seam cheap is that it needs nothing from us: no props, no state, no bridge.
 * It is served as files and it runs.
 */

/** The city place this stands in: the bodega in Wilmington. */
const HOUSE = 'wilmington-house'

/*
 * The trailing slash is load-bearing.
 *
 * js/audio.js builds every sound URL from `window.location.pathname`. Served as
 * `/the-house/index.html` the game asks for `/the-house/index.htmlsound/*.mp3`,
 * which 404s without an error anybody sees — the game runs, silently.
 */
const GAME = '/the-house/'

/*
 * TODO(per-character): the save is per browser.
 *
 * The game keeps its progress under the `jStorage` key in localStorage, and
 * that key belongs to the origin rather than to whoever is signed in. Two
 * characters on one browser share one house, and the ending calls
 * `$.jStorage.flush()`, which resets it for both. Making it per character means
 * this component reading that key out when the iframe unmounts and writing it
 * back when the next one mounts, against storage the server holds.
 *
 * TODO(completion): nothing in our world knows the house was finished.
 * `played` gains `darkness_retract3` when the last puzzle is solved, about
 * fifteen seconds before the flush. Polling for it while the frame is up is
 * what would let the city react to somebody who got out.
 */
export function TheHouse() {
  const at = useWorld((w) => w.character?.at.key ?? null)
  /*
   * The street this stands on, which is where leaving puts you.
   *
   * Read off the place rather than named here, so the way out is the city's
   * own account of where the house sits. Closing the game without walking out
   * leaves a character standing in a room nobody has painted, which is the
   * placeholder under the iframe and reads as somebody else's flat.
   */
  const street = useWorld((w) => w.character?.at.in ?? null)
  const inside = at === HOUSE
  /*
   * Closed by hand, this visit.
   *
   * Without it the button does nothing: the character is still standing in the
   * house, so the next render puts the game straight back up. Walking out
   * clears it, so the door works twice.
   */
  const [shut, setShut] = useState(false)

  useEffect(() => {
    if (!inside) setShut(false)
  }, [inside])

  if (!inside || shut) return null

  return (
    <div className="house">
      <iframe className="house-game" src={GAME} title="The house" />
      <button
        type="button"
        className="house-close"
        onClick={() => {
          setShut(true)
          if (street) void moveTo(street, 'foot')
        }}
      >
        Leave
      </button>
    </div>
  )
}
