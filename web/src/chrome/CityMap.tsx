/*
 * The city, seen from above — a schematic, not a place.
 *
 * §3.12: the map is the district layer looked at, never somewhere you can
 * stand. It shows the one city whole: its districts around the Christina, the
 * places inside them, and where you are — and clicking somewhere walkable is
 * the same `character.move` the buttons under it send. Somewhere unbuilt is
 * drawn faded and stays shut, the same rule the doors hold.
 *
 * The geography here is presentation: the server's city is a graph with no
 * coordinates, so where a district sits on this sheet is authored below, the
 * way the preview authors its fiction. The name is too — nothing server-side
 * names the city yet.
 */

import { mapFor } from '../render/level/locations'
import { moveTo } from '../world/actions'
import { district, districts, inside, placeIn } from '../world/city'
import { useWorld } from '../world/react'
import type { Place } from '../world/state'

/** What the city is called. Client-authored until the server names it. */
export const CITY_NAME = 'Wilmington'

/** And where it is, said in full where there is room for it. */
export const CITY_PLACE = 'Wilmington, Delaware'

/** Where each district sits on the sheet, in viewBox units (100 × 64). */
const SPOTS: Record<string, { x: number; y: number; w: number; h: number }> = {
  'east-side': { x: 60, y: 8, w: 34, h: 22 },
  downtown: { x: 22, y: 6, w: 30, h: 20 },
  'the-strip': { x: 4, y: 32, w: 26, h: 18 },
  riverfront: { x: 56, y: 42, w: 34, h: 18 },
}

/** The Christina, cutting the sheet between the banks. */
const RIVER = 'M -2 36 C 24 30, 40 34, 52 38 C 66 42, 84 38, 102 34 L 102 42 C 84 46, 66 50, 52 46 C 40 43, 24 39, -2 44 Z'

export function CityMap({ busy }: { busy: boolean }) {
  const places = useWorld((w) => w.places)
  const at = useWorld((w) => w.character?.at ?? null)
  const home = useWorld((w) => w.character?.home ?? null)

  if (places.length === 0) return null
  const here = district(placeIn(places, at?.key ?? ''))

  return (
    <svg
      viewBox="0 0 100 64"
      style={{ width: '100%', maxWidth: 820, height: 'auto', display: 'block', margin: '0 auto 14px' }}
      role="img"
      aria-label={`${CITY_NAME}, from above`}
    >
      {/* The river, and one quiet line about it. */}
      <path d={RIVER} fill="var(--sky, #cfe0ee)" opacity={0.8} />
      <text x={2.4} y={62.4} fontSize={1.8} fill="var(--text-muted)">
        the Christina · the 6 is the one way across
      </text>

      {districts(places).map((street) => (
        <District
          key={street.key}
          street={street}
          places={places}
          here={here}
          atKey={at?.key ?? ''}
          homeKey={home?.key ?? ''}
          busy={busy}
        />
      ))}
    </svg>
  )
}

/*
 * The almanac: what a resident would know about the city, as data.
 *
 * Deliberately observable facts — headcounts, rents, seats, turnout, what the
 * paper does — and never a word about what any of it means. The differences
 * between the city's populations are the game's to be discovered by living in
 * them, not a taxonomy to be read off a screen; nothing here names a type of
 * person, and it must stay that way.
 */
export function CityAlmanac() {
  return (
    <>
      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">About {CITY_NAME}</span>
          <span className="panel-note">the almanac</span>
        </div>
        <div className="panel-body">
          <p className="faint tiny mt0">
            A port city on the Christina, Delaware side — small enough to cross on foot in an
            afternoon, big enough that nobody has. Four districts, one bus line, and a river that decides which
            side of town your money lives on.
          </p>
          <div className="metric-row">
            <div>
              <div className="metric-name">People</div>
              <div className="metric-sub">last census, give or take</div>
            </div>
            <div className="metric-value">41,000</div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">Work</div>
              <div className="metric-sub">what the pay stubs say</div>
            </div>
            <div className="metric-value">the port · offices · nights</div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">Rent</div>
              <div className="metric-sub">a month, one room</div>
            </div>
            <div className="metric-value">$410 east · $980 river</div>
          </div>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">Who lives where</span>
          <span className="panel-note">as the streets tell it</span>
        </div>
        <div className="panel-body">
          <div className="metric-row">
            <div>
              <div className="metric-name">East Side</div>
              <div className="metric-sub">walk-ups; people stay for decades; favours are a currency</div>
            </div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">Downtown</div>
              <div className="metric-sub">clerks and commuters; almost nobody sleeps here</div>
            </div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">Riverfront</div>
              <div className="metric-sub">billed hours and tall glass; newest money in the city</div>
            </div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">The Strip</div>
              <div className="metric-sub">freight, night shifts, and the union hall's parking lot</div>
            </div>
          </div>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">Civics</span>
          <span className="panel-note">public record</span>
        </div>
        <div className="panel-body">
          <div className="metric-row">
            <div>
              <div className="metric-name">Council</div>
              <div className="metric-sub">seven seats, by bank of the river</div>
            </div>
            <div className="metric-value">4 east · 3 river</div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">Turnout</div>
              <div className="metric-sub">last municipal election</div>
            </div>
            <div className="metric-value">31%</div>
          </div>
          <div className="metric-row">
            <div>
              <div className="metric-name">The Ledger</div>
              <div className="metric-sub">the paper, most mornings</div>
            </div>
            <div className="metric-value">endorses more shifts at the port</div>
          </div>
          <p className="faint tiny mb0">
            The east bank votes for whoever protects the overtime; the river bank votes for
            whoever keeps the assessments down. Both sides call the 6 late.
          </p>
        </div>
      </div>
    </>
  )
}

function District({
  street,
  places,
  here,
  atKey,
  homeKey,
  busy,
}: {
  street: Place
  places: Place[]
  here: string
  atKey: string
  homeKey: string
  busy: boolean
}) {
  const spot = SPOTS[street.key] ?? { x: 36, y: 26, w: 28, h: 16 }
  const built = mapFor(street.key) !== null
  const current = here === street.key
  const within = inside(places, street.key)

  const go = (key: string, ok: boolean) => {
    if (!busy && ok && key !== atKey) void moveTo(key, 'foot')
  }

  return (
    <g opacity={built ? 1 : 0.55}>
      <rect
        x={spot.x}
        y={spot.y}
        width={spot.w}
        height={spot.h}
        rx={2.4}
        fill="var(--bg-surface)"
        stroke={current ? 'var(--bg-accent-solid)' : 'var(--border-default)'}
        strokeWidth={current ? 0.7 : 0.35}
        strokeDasharray={built ? undefined : '1.4 1'}
        style={{ cursor: built ? 'pointer' : 'not-allowed' }}
        onClick={() => go(street.key, built)}
      >
        <title>{built ? `${street.name} — ${street.note}` : `${street.name} — not built yet`}</title>
      </rect>

      <text
        x={spot.x + 2}
        y={spot.y + 3.4}
        fontSize={2.3}
        fontWeight={600}
        fill="var(--text-default)"
        pointerEvents="none"
      >
        {street.name}
      </text>
      <text
        x={spot.x + 2}
        y={spot.y + 5.9}
        fontSize={1.5}
        fill="var(--text-muted)"
        pointerEvents="none"
      >
        {street.note}
      </text>

      {/* The places inside, each a dot you can ask to walk to. */}
      {within.map((place, i) => {
        const px = spot.x + 3 + (i % 2) * (spot.w / 2 - 0.5)
        const py = spot.y + 9.4 + Math.floor(i / 2) * 3.6
        const openable = mapFor(place.key) !== null
        const isHome = place.key === homeKey
        return (
          <g key={place.key} style={{ cursor: openable ? 'pointer' : 'not-allowed' }}>
            <circle
              cx={px}
              cy={py}
              r={0.95}
              fill={openable ? 'var(--bg-accent-solid)' : 'var(--border-default)'}
              onClick={() => go(place.key, openable)}
            >
              <title>
                {openable
                  ? isHome
                    ? `${place.name} — home`
                    : place.name
                  : `${place.name} — not built yet`}
              </title>
            </circle>
            <text x={px + 1.7} y={py + 0.55} fontSize={1.45} fill="var(--text-muted)" pointerEvents="none">
              {place.name}
            </text>
          </g>
        )
      })}

      {/* You are here: on the place if it is drawn, on the district if not. */}
      {current && <You atKey={atKey} spot={spot} within={within} />}
    </g>
  )
}

function You({
  atKey,
  spot,
  within,
}: {
  atKey: string
  spot: { x: number; y: number; w: number; h: number }
  within: Place[]
}) {
  const index = within.findIndex((p) => p.key === atKey)
  const cx = index >= 0 ? spot.x + 3 + (index % 2) * (spot.w / 2 - 0.5) : spot.x + spot.w - 3
  const cy = index >= 0 ? spot.y + 9.4 + Math.floor(index / 2) * 3.6 : spot.y + 3
  return (
    <g pointerEvents="none">
      <circle cx={cx} cy={cy} r={2.1} fill="none" stroke="var(--bg-accent-solid)" strokeWidth={0.4}>
        <animate attributeName="r" values="1.6;2.6;1.6" dur="2s" repeatCount="indefinite" />
        <animate attributeName="opacity" values="0.9;0.3;0.9" dur="2s" repeatCount="indefinite" />
      </circle>
      <circle cx={cx} cy={cy} r={0.9} fill="var(--bg-accent-solid)" />
    </g>
  )
}
