import { describe, expect, it } from 'vitest'

import exported from '../../../public/locations/east-side-street/east-side-street.tmj?raw'
import { clearLine, contains, distance, inArea } from './geometry'
import { findPath } from './navmesh'
import { mapFor } from './locations'
import { portalsAt, readMap, resolve, zonesAt } from './tiled'

/*
 * Reading a map somebody else's editor wrote.
 *
 * The first half of this file loads **the map that ships** — the one under
 * `web/public`, imported here as text so the assertions are about the level the
 * game actually draws rather than about a fixture that agrees with it. A street
 * edited in Tiled into a state the renderer cannot use fails here, at build
 * time, rather than as an empty canvas in a browser.
 *
 * The second half is about the reader's refusals. Authored data with a
 * hand-written reader has to fail at the door: a trigger zone typed `triger`
 * has to be an error, because the alternative is a location quietly missing a
 * place for anything to happen and nothing saying so for months.
 */

const url = mapFor('east-side') ?? ''
const street = readMap(JSON.parse(exported), url)

describe('the East Side street, as it ships', () => {
  it('names itself, and the place it is', () => {
    expect(street.key).toBe('east-side-street')
    expect(street.name).toBe('Nostrand Avenue')
    // `internal/character/city.go` owns this vocabulary.
    expect(street.place).toBe('east-side')
  })

  it('resolves its painting against its own location', () => {
    expect(street.painting.url).toBe('/locations/east-side-street/plate.png')
    expect(street.extent).toEqual({ x: 0, y: 0, width: 1920, height: 1088 })
  })

  it('has a floor with the props cut out of it', () => {
    expect(street.region.outer).toHaveLength(8)
    expect(street.region.holes).toHaveLength(3)
    // The van is painted on the road, so the road is not walkable through it.
    expect(contains(street.region, { x: 1400, y: 620 })).toBe(false)
    // Behind it is, which is the point: that is where occlusion happens.
    expect(contains(street.region, { x: 1400, y: 540 })).toBe(true)
  })

  it('starts the player somewhere they could stand', () => {
    expect(contains(street.region, street.spawn)).toBe(true)
  })

  it('cuts every occluder out of the painting, with a baseline on the floor', () => {
    expect(street.occluders.map((o) => o.name)).toEqual([
      'the planter',
      'the van',
      'the lamp post',
    ])
    for (const occluder of street.occluders) {
      expect(occluder.frame.x).toBeGreaterThanOrEqual(0)
      expect(occluder.frame.y).toBeGreaterThanOrEqual(0)
      expect(occluder.frame.x + occluder.frame.width).toBeLessThanOrEqual(1920)
      expect(occluder.frame.y + occluder.frame.height).toBeLessThanOrEqual(1088)
      // A prop meets the ground somewhere inside its own picture.
      expect(occluder.baseline).toBeGreaterThan(occluder.frame.y)
      expect(occluder.baseline).toBeLessThanOrEqual(occluder.frame.y + occluder.frame.height)
    }
  })

  it('sorts its trigger zones most specific first', () => {
    expect(street.zones.map((zone) => zone.key)).toEqual([
      'home-door',
      'corner-shop',
      'transit-stop',
      'on-foot',
    ])
    for (let i = 1; i < street.zones.length; i++) {
      expect(street.zones[i].area.size).toBeGreaterThanOrEqual(street.zones[i - 1].area.size)
    }
  })

  it('answers where you are standing, particular before general', () => {
    expect(zonesAt(street, street.spawn).map((z) => z.key)).toEqual(['on-foot'])
    expect(zonesAt(street, { x: 1000, y: 800 }).map((z) => z.key)).toEqual([
      'transit-stop',
      'on-foot',
    ])
    expect(zonesAt(street, { x: 1000, y: 2000 })).toEqual([])
  })

  it('carries the ways out as authored objects', () => {
    expect(street.portals.map((p) => p.to)).toEqual(['downtown', 'east-side-apartment'])
    expect(portalsAt(street, { x: 1750, y: 800 }).map((p) => p.to)).toEqual(['downtown'])
    expect(portalsAt(street, street.spawn)).toEqual([])
  })

  it('keeps every trigger zone somewhere the player can reach', () => {
    // A zone nobody can stand in is a zone no event could ever fire in.
    for (const zone of street.zones) {
      const at = middle(zone.area.bounds)
      expect(inArea(zone.area, at), `${zone.key} does not contain its own middle`).toBe(true)
      expect(contains(street.region, at), `${zone.key} has no walkable middle`).toBe(true)
    }
  })

  it('can be walked from end to end, around what is on it', () => {
    const from = street.spawn
    const to = { x: 1750, y: 420 }
    const route = findPath(street.navmesh, from, to)

    expect(route.length).toBeGreaterThan(1)
    let length = 0
    for (let i = 1; i < route.length; i++) {
      expect(contains(street.region, route[i])).toBe(true)
      expect(clearLine(street.region, route[i - 1], route[i])).toBe(true)
      length += distance(route[i - 1], route[i])
    }
    expect(length).toBeGreaterThanOrEqual(distance(from, to))
  })
})

/* ── what the reader refuses ────────────────────────────────────────────── */

const floor = {
  class: 'floor',
  id: 1,
  name: 'floor',
  x: 0,
  y: 0,
  polygon: [
    { x: 0, y: 0 },
    { x: 100, y: 0 },
    { x: 100, y: 100 },
    { x: 0, y: 100 },
  ],
}

const spawn = { class: 'spawn', id: 2, name: 'spawn', point: true, x: 50, y: 50 }

function mapOf(objects: unknown[], properties: unknown[] = defaults()): unknown {
  return {
    type: 'map',
    infinite: false,
    properties,
    layers: [
      {
        type: 'imagelayer',
        name: 'painting',
        image: 'plate.png',
        imagewidth: 100,
        imageheight: 100,
        x: 0,
        y: 0,
      },
      { type: 'objectgroup', name: 'logic', objects },
    ],
  }
}

function defaults() {
  return [
    { name: 'key', type: 'string', value: 'somewhere' },
    { name: 'place', type: 'string', value: 'east-side' },
  ]
}

describe('a map the renderer cannot use', () => {
  it('is refused when it is not a Tiled map at all', () => {
    expect(() => readMap({ hello: true }, 'a.tmj')).toThrow(/not a Tiled map/)
    expect(() => readMap('nope', 'a.tmj')).toThrow(/not a Tiled map/)
  })

  it('is refused when it does not say which place it is', () => {
    expect(() => readMap(mapOf([floor, spawn], []), 'a.tmj')).toThrow(/"key" property/)
  })

  it('is refused when there is nowhere to walk', () => {
    expect(() => readMap(mapOf([spawn]), 'a.tmj')).toThrow(/class "floor"/)
  })

  it('is refused when there is nowhere to arrive', () => {
    expect(() => readMap(mapOf([floor]), 'a.tmj')).toThrow(/class "spawn"/)
  })

  it('is refused when an object has a class nobody reads', () => {
    const typo = { class: 'triger', id: 3, name: 'the stoop', x: 1, y: 1, width: 5, height: 5 }
    expect(() => readMap(mapOf([floor, spawn, typo]), 'a.tmj')).toThrow(/unknown class "triger"/)
  })

  it('is refused when an object has no class at all', () => {
    const stray = { id: 3, name: 'something', x: 1, y: 1, width: 5, height: 5 }
    expect(() => readMap(mapOf([floor, spawn, stray]), 'a.tmj')).toThrow(/no class/)
  })

  it('is refused when a trigger zone has no key for events to match on', () => {
    const zone = { class: 'trigger', id: 3, name: 'The stoop', x: 1, y: 1, width: 5, height: 5 }
    expect(() => readMap(mapOf([floor, spawn, zone]), 'a.tmj')).toThrow(/"key"/)
  })

  it('is refused when an occluder does not say where it meets the ground', () => {
    const prop = { class: 'occluder', id: 3, name: 'a bin', x: 1, y: 1, width: 5, height: 5 }
    expect(() => readMap(mapOf([floor, spawn, prop]), 'a.tmj')).toThrow(/"baseline"/)
  })

  it('is refused when an occluder reaches outside the painting', () => {
    const prop = {
      class: 'occluder',
      id: 3,
      name: 'a bin',
      x: 90,
      y: 1,
      width: 40,
      height: 5,
      properties: [{ name: 'baseline', type: 'float', value: 4 }],
    }
    expect(() => readMap(mapOf([floor, spawn, prop]), 'a.tmj')).toThrow(/outside the painting/)
  })

  it('is refused when an object is rotated, because a frame is not', () => {
    const zone = {
      class: 'trigger',
      id: 3,
      name: 'The stoop',
      rotation: 30,
      x: 1,
      y: 1,
      width: 5,
      height: 5,
      properties: [{ name: 'key', type: 'string', value: 'stoop' }],
    }
    expect(() => readMap(mapOf([floor, spawn, zone]), 'a.tmj')).toThrow(/rotated/)
  })

  it('is refused when it has two floors, or two ways to arrive', () => {
    expect(() => readMap(mapOf([floor, floor, spawn]), 'a.tmj')).toThrow(/second floor/)
    expect(() => readMap(mapOf([floor, spawn, spawn]), 'a.tmj')).toThrow(/second spawn/)
  })
})

describe('a map an older editor wrote', () => {
  it('is read anyway — Tiled 1.9 renamed an object’s type to its class', () => {
    const old = [
      { ...floor, class: undefined, type: 'floor' },
      { ...spawn, class: undefined, type: 'spawn' },
    ]
    expect(readMap(mapOf(old), 'a.tmj').spawn).toEqual({ x: 50, y: 50 })
  })

  it('is read out of group layers too', () => {
    const grouped = {
      type: 'map',
      infinite: false,
      properties: defaults(),
      layers: [
        {
          type: 'group',
          name: 'everything',
          layers: [
            {
              type: 'imagelayer',
              image: 'plate.png',
              imagewidth: 100,
              imageheight: 100,
              x: 0,
              y: 0,
            },
            { type: 'objectgroup', name: 'logic', objects: [floor, spawn] },
          ],
        },
      ],
    }
    expect(readMap(grouped, 'a.tmj').region.outer).toHaveLength(4)
  })

  it('reads a rectangle zone as its four corners', () => {
    const zone = {
      class: 'trigger',
      id: 3,
      name: 'The stoop',
      x: 10,
      y: 10,
      width: 20,
      height: 20,
      properties: [{ name: 'key', type: 'string', value: 'stoop' }],
    }
    const [only] = readMap(mapOf([floor, spawn, zone]), 'a.tmj').zones
    expect(only.area.points).toHaveLength(4)
    expect(only.area.size).toBe(400)
    expect(inArea(only.area, { x: 20, y: 20 })).toBe(true)
    expect(inArea(only.area, { x: 40, y: 20 })).toBe(false)
  })
})

describe('an image path, relative to the map that names it', () => {
  it('is resolved against the map’s own folder', () => {
    expect(resolve('/locations/street/street.tmj', 'plate.png')).toBe(
      '/locations/street/plate.png',
    )
    expect(resolve('/locations/street/street.tmj', '../shared/plate.png')).toBe(
      '/locations/shared/plate.png',
    )
  })

  it('is left alone when it is already absolute', () => {
    expect(resolve('/locations/street/street.tmj', '/art/plate.png')).toBe('/art/plate.png')
    expect(resolve('/locations/street/street.tmj', 'https://x/p.png')).toBe('https://x/p.png')
  })
})

function middle(box: { x: number; y: number; width: number; height: number }) {
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}
