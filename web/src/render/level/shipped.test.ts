import { describe, expect, it } from 'vitest'

import apartment from '../../../public/locations/east-side-apartment/east-side-apartment.tmj?raw'
import street from '../../../public/locations/east-side/east-side.tmj?raw'
import { contains } from './geometry'
import { readMap, type Level } from './tiled'

/*
 * The locations that actually ship, which are generated (tools/bake) rather
 * than drawn in Tiled — so what is held here is invariants of the generation,
 * not coordinates: a regenerated map with a nudged margin should pass, and a
 * map a character could get stuck in should not.
 */

/** The zone vocabulary the city names (internal/city). A key outside it is a
 * family of events silently unplaced — the Go side holds the same line. */
const ZONES = new Set([
  'on-foot',
  'transit-stop',
  'home-door',
  'corner-shop',
  'doorway',
  'counter',
  'seats',
  'desk',
  'dock',
  'at-home',
  'bed',
  'kitchen',
  'window',
])

const SHIPPED: Record<string, Level> = {
  'east-side': readMap(JSON.parse(street), '/locations/east-side/east-side.tmj'),
  'east-side-apartment': readMap(
    JSON.parse(apartment),
    '/locations/east-side-apartment/east-side-apartment.tmj',
  ),
}

describe.each(Object.entries(SHIPPED))('%s, as generated', (key, level) => {
  it('is the place it says it is', () => {
    expect(level.place).toBe(key)
    expect(level.key).toBe(key)
  })

  it('ships its painting compressed, beside the map', () => {
    expect(level.painting.url).toBe(`/locations/${key}/plate.webp`)
  })

  it('starts the character on the floor', () => {
    expect(contains(level.region, level.spawn)).toBe(true)
  })

  it('names its spots in the city’s own vocabulary', () => {
    expect(level.zones.length).toBeGreaterThan(0)
    for (const zone of level.zones) expect(ZONES.has(zone.key), zone.key).toBe(true)
  })

  it('has a way out whose doorway is standable', () => {
    expect(level.portals.length).toBeGreaterThan(0)
    for (const portal of level.portals) {
      const { x, y, width, height } = portal.area.bounds
      const middle = { x: x + width / 2, y: y + height / 2 }
      // The doorway itself must be walkable, or arriving through it would pull
      // the character somewhere the door is not — which is how a walk-in door
      // becomes a door nobody can come back out of.
      expect(contains(level.region, middle), `${portal.name} at ${middle.x},${middle.y}`).toBe(true)
    }
  })
})

describe('the pair of them', () => {
  it('lead to each other: the street into the flat, the flat back out', () => {
    const street = SHIPPED['east-side']
    const flat = SHIPPED['east-side-apartment']
    expect(street.portals.map((p) => p.to)).toContain('east-side-apartment')
    expect(flat.portals.map((p) => p.to)).toContain('east-side')
  })

  it('lights the flat, and one of the lights is the cold one', () => {
    const flat = SHIPPED['east-side-apartment']
    expect(flat.lights.length).toBeGreaterThanOrEqual(3)
    const window = flat.lights.find((light) => light.name === 'the window')
    expect(window?.color).toBe(0x3a6ba6)
  })
})
