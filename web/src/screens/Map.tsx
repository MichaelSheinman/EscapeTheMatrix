import { CityAlmanac, CityMap, CITY_NAME, CITY_PLACE } from '../chrome/CityMap'
import { mapFor } from '../render/level/locations'
import { moveTo } from '../world/actions'
import { districts, inside, named, waysBy } from '../world/city'
import { useWorld } from '../world/react'
import { sending, type Place, type Trip, type Way } from '../world/state'
import { Overlay } from './Overlay'
import { spending } from './words'

/**
 * The city map (§3.12).
 *
 * §3.12's layers are home grid → interior → district → city map, and the last of
 * them is not a place: it is the district layer seen from above, a way of
 * choosing where to go rather than somewhere to go. Making it somewhere you
 * could stand would have made it a free ride between districts, which is
 * exactly the thing the fare below is meant to buy.
 *
 * Two kinds of offer, and the difference between them is the whole ticket:
 *
 *   - The ways out of here come with their own fares, because the server
 *     sent them attached to the place. A train is only ever offered where the
 *     network actually calls — you cannot ride out of your own kitchen.
 *   - Everywhere else is a walk, offered without a number on it. The client
 *     could add up a route itself and would be right nearly every time; the
 *     first time it was wrong the player would have been quoted a price nobody
 *     charged. So it asks, and the answer comes back with the journey in it
 *     (§10.2, rule 2).
 *
 * Nothing here unmounts the world. This is an overlay above a canvas that is
 * still running (§10.1) — walk somewhere from this panel and the scene behind it
 * changes underneath while the panel stays open.
 */
export function Map({ open, onClose }: { open: boolean; onClose: () => void }) {
  const character = useWorld((w) => w.character)
  const places = useWorld((w) => w.places)
  const trip = useWorld((w) => w.trip)

  if (!character) return null

  const here = character.at
  const onFoot = waysBy(here, 'foot')
  const byTransit = waysBy(here, 'transit')

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title={CITY_PLACE}
      sub="A port town on the Christina that never grew past its river. The cranes pay the east
            bank, billed hours pay the west, and the 6 is the only way across — which side you
            sleep on decides most of the rest."
    >
      <CityMap />

      <div className="stack">
        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">Standing in {here.name}</span>
            <span className="panel-note">{here.note}</span>
          </div>
          <div className="panel-body">
            <div className="btn-row">
              {onFoot.map((way) => (
                <WayButton key={`foot-${way.to}`} way={way} />
              ))}
            </div>

            {byTransit.length > 0 ? (
              <>
                <div className="btn-row">
                  {byTransit.map((way) => (
                    <WayButton key={`transit-${way.to}`} way={way} />
                  ))}
                </div>
                <p className="faint tiny">
                  The network calls here. A ride takes the fare and passes through nothing —
                  no street, no doorway, and nobody who might have stopped you.
                </p>
              </>
            ) : (
              <p className="faint tiny">
                No stop here. Leaving anywhere indoors begins on foot, however much money you
                have — which is the one thing a fare cannot buy.
              </p>
            )}
          </div>
        </div>

        {trip && <Journey trip={trip} />}

        {districts(places).map((street) => (
          <div className="panel" key={street.key}>
            <div className="panel-head">
              <span className="panel-title">{street.name}</span>
              <span className="panel-note">{street.note}</span>
            </div>
            <div className="panel-body">
              <div className="btn-row">
                <GoButton place={street} here={here.key} />
                {inside(places, street.key).map((place) => (
                  <GoButton key={place.key} place={place} here={here.key} />
                ))}
              </div>
            </div>
          </div>
        ))}

        <p className="faint tiny">
          The city is larger than what has been built. Anywhere greyed out is named and real
          — it opens as the districts are built out, one at a time.
        </p>

        {/* The almanac last: it is what this city is, and everything above
            it is somewhere you could go right now. */}
        <CityAlmanac />
      </div>
    </Overlay>
  )
}

/** One step out of where you are, with the price the server put on it. */
function WayButton({ way }: { way: Way }) {
  const places = useWorld((w) => w.places)
  const busy = useWorld(sending)
  // Somewhere unbuilt is named and stays shut — a way in with nothing on the
  // other side is how a player gets stuck.
  const built = mapFor(way.to) !== null
  return (
    <button
      className={`btn${built ? '' : ' spent'}`}
      onClick={() => void moveTo(way.to, way.mode)}
      disabled={busy || !built}
      title={built ? named(places, way.to).note : 'Not built yet.'}
    >
      {way.says}
      {way.fare > 0 && <span className="metric-sub"> · {spending(way.fare)}</span>}
      {!built && <span className="metric-sub"> · not built yet</span>}
    </button>
  )
}

/**
 * Anywhere in the city, on foot.
 *
 * No fare on it, because there is none to show: every walk in the city is free,
 * and the number of streets between here and there is the city's to know.
 */
function GoButton({ place, here }: { place: Place; here: string }) {
  const busy = useWorld(sending)
  const there = place.key === here
  // Somewhere unbuilt is named and stays shut — a place you could walk to but
  // not around in, or back out of, is how a player gets stuck.
  const built = mapFor(place.key) !== null
  return (
    <button
      className={`btn${there || !built ? ' spent' : ''}`}
      onClick={() => void moveTo(place.key, 'foot')}
      disabled={busy || there || !built}
      title={built ? place.note : 'Not built yet.'}
    >
      {there
        ? `You are in ${place.name}`
        : built
          ? `Walk to ${place.name}`
          : `${place.name} — not built yet`}
    </button>
  )
}

/**
 * The journey just made.
 *
 * `chances` is the honest half of the trade before the event system exists
 * (#21, #22): how many places the journey passed through where something could
 * have happened. Zero for a ride, always — and that difference is the only
 * thing the fare bought.
 */
function Journey({ trip }: { trip: Trip }) {
  const places = useWorld((w) => w.places)
  if (trip.steps.length === 0) return null

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">The way you came</span>
        <span className="panel-note">{trip.by === 'foot' ? 'on foot' : 'by transit'}</span>
      </div>
      <div className="panel-body">
        {trip.steps.map((step, i) => (
          <div className="metric-row" key={`${i}-${step.to}`}>
            <div>
              <div className="metric-name">{step.says}</div>
              <div className="metric-sub">{named(places, step.to).note}</div>
            </div>
            <div className="metric-value">{named(places, step.to).name}</div>
          </div>
        ))}

        <div className="metric-row">
          <div>
            <div className="metric-name">The fare</div>
            <div className="metric-sub">
              {trip.fare > 0 ? 'what it cost to skip what was on the way' : 'walking is free'}
            </div>
          </div>
          <div className="metric-value">{trip.fare > 0 ? spending(trip.fare) : 'nothing'}</div>
        </div>

        <div className="metric-row">
          <div>
            <div className="metric-name">Places passed through</div>
            <div className="metric-sub">
              {trip.chances > 0
                ? 'each one somewhere your life could have happened to you'
                : 'the fare bought the whole journey back'}
            </div>
          </div>
          <div className="metric-value">{trip.chances}</div>
        </div>
      </div>
    </div>
  )
}

/**
 * Where you are, and where home is.
 *
 * @etm/ui calls this the only orientation the shell gives you — the place names
 * itself in the world through its own signage, and this is the fallback for
 * before there is any. Both places come from the character, so a district
 * renamed on the server is renamed here without a deploy.
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
  // district is the one it sits inside: standing on your own stoop is standing
  // on the East Side. `zones` arrives smallest first, so the first is the spot.
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
        {` · ${CITY_NAME}`}
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
