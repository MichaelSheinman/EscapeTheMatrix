import { useEffect, useState } from 'react'

import { moveTo } from '../world/actions'
import { useWorld } from '../world/react'

import './house.css'

/*
 * The dance studio on the strip.
 *
 * A separate game — Bemuse, by Thai Pangsakulyanont and contributors, AGPL,
 * vendored under web/public/bemuse with its source in third_party/bemuse —
 * standing in a city interior. Walking in opens it over the world; the button
 * closes it and leaves the character where they were.
 *
 * The `.house` classes are the covering-a-game layout house.css defines, and
 * the reasons written there (iframe over everything, a close control in the
 * parent document) hold here unchanged.
 */

/** The city place this stands in: the dance studio on the east side. */
const STUDIO = 'east-side-dance'

const GAME = '/bemuse/'

export function TheStudio() {
  const at = useWorld((w) => w.character?.at.key ?? null)
  // The street this stands on, which is where leaving puts you — read off the
  // place, the same way chrome/TheHouse does.
  const street = useWorld((w) => w.character?.at.in ?? null)
  const inside = at === STUDIO
  // Closed by hand, this visit. Without it the button does nothing: the
  // character is still standing in the studio, so the next render puts the
  // game straight back up.
  const [shut, setShut] = useState(false)

  useEffect(() => {
    if (!inside) setShut(false)
  }, [inside])

  if (!inside || shut) return null

  return (
    <div className="house">
      <iframe className="house-game" src={GAME} title="Dance studio" />
      <button
        type="button"
        className="house-close above-bar"
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
