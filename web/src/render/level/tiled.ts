/*
 * A location, as Tiled exports it.
 *
 * technical.md §10.3: **Tiled is the authoring tool.** An image layer plus
 * object layers is a real visual editor for the walkable polygon, the depth
 * anchors, the portals and the trigger zones, and it exports JSON the renderer
 * loads directly — "which avoids building a level editor, which is otherwise a
 * genuine trap in this design." So this file is a reader for someone else's
 * format, and the format is the contract: `.tmj` is what Tiled writes, and
 * anything this file needs has to be something an author can draw.
 *
 * Two consequences worth stating, because both are choices:
 *
 *   1. **The layer names mean nothing. The object classes mean everything.**
 *      An author can reorganise the layer panel, split the triggers across
 *      three groups, or rename "walkable" — none of it reaches here. What is
 *      read is the *class* on each object, which is the field Tiled puts in
 *      front of you and the one that survives being moved.
 *
 *   2. **An object this file does not understand is an error, not a shrug.**
 *      A zone typed `triger` would otherwise load as a level with one fewer
 *      place for anything to happen, and nothing would say so until an event
 *      failed to fire months later. Authored data with a hand-written reader
 *      has to fail at the door.
 *
 * The result is a plain data `Level` with no Pixi in it. Building the sprites
 * is `../scene.ts`'s job; this is the part that can be tested against a JSON
 * fixture, and `tiled.test.ts` does exactly that.
 */

import {
  area,
  bounds,
  corners,
  inArea,
  toArea,
  type Area,
  type Point,
  type Polygon,
  type Rect,
  type Region,
} from './geometry'
import { buildNavmesh, type Navmesh } from './navmesh'

/** The painted image the whole location is drawn on. */
export type Painting = {
  /** Resolved against the map's own URL, the way Tiled stores it. */
  url: string
  width: number
  height: number
  x: number
  y: number
}

/**
 * A piece of the painting that draws *over* the character when they are behind
 * it (§10.3's depth-sort anchors).
 *
 * `frame` is a rectangle of the plate — the prop, cut out of the picture it is
 * already part of — and `baseline` is the y at which the prop meets the ground.
 * Sorting everything on the floor by that one number is the whole of depth
 * under a fixed camera: further up the image is further away.
 *
 * Nothing new is drawn. The cut-out is pixel-identical to what is underneath
 * it, so a character in *front* of a prop covers a copy of the plate that
 * matches the plate, and only a character *behind* it sees anything happen.
 * That is what lets a painted location have depth without a second set of art.
 */
export type Occluder = { name: string; frame: Rect; baseline: number }

/**
 * A named place inside the location where something can happen.
 *
 * §10.3: "**the same objects as event placement rules (§4)**". §3.9's placement
 * conditions — on foot downtown, at home late, at work — *are* these, which is
 * why `key` is authored rather than generated: it is the vocabulary the event
 * system will match against, and a key that changed when somebody nudged a
 * rectangle would silently unplace every event that named it.
 */
export type Zone = { key: string; name: string; area: Area }

/** A way out. #12 turns these into transitions; here they are authored data. */
export type Portal = { key: string; name: string; to: string; area: Area }

export type Level = {
  /** The map's own key, from its `key` property. Not the file name. */
  key: string
  name: string
  /** Which of `internal/character/city.go`'s places this location is. */
  place: string
  /** Where it was loaded from. */
  url: string
  /** The whole location, in plate pixels. */
  extent: Rect
  painting: Painting
  region: Region
  navmesh: Navmesh
  occluders: Occluder[]
  /** Sorted smallest first, so the most specific zone you are in comes first. */
  zones: Zone[]
  portals: Portal[]
  spawn: Point
}

/** Fetch and read a location. One network round trip; the JSON is the level. */
export async function loadLevel(url: string): Promise<Level> {
  const res = await fetch(url, { headers: { Accept: 'application/json' } })
  if (!res.ok) throw new Error(`${url}: the level could not be loaded (${res.status})`)
  return readMap(await res.json(), url)
}

/**
 * Read an exported map.
 *
 * Pure, and separate from the fetch above, because this is the part with
 * decisions in it — and the part a fixture can be handed in a test without a
 * server, a canvas, or a network.
 */
export function readMap(source: unknown, url: string): Level {
  // A function declaration rather than a `const`, so that the compiler treats a
  // call to it as the end of the branch and narrows what follows.
  function fail(why: string): never {
    throw new Error(`${url}: ${why}`)
  }

  const map = asRecord(source) ?? fail('not a Tiled map — the file is not a JSON object')
  if (map.type !== 'map') fail(`not a Tiled map — "type" is ${JSON.stringify(map.type)}`)
  if (map.infinite === true) fail('infinite maps are not supported — set a fixed size in Tiled')

  const key = text(property(map.properties, 'key')) ?? fail('the map needs a "key" property')
  const name = text(property(map.properties, 'name')) ?? key
  const place = text(property(map.properties, 'place')) ?? fail('the map needs a "place" property')

  const layers = flatten(map.layers)

  // ── the painting ──────────────────────────────────────────────────────
  const images = layers.filter((layer) => layer.type === 'imagelayer')
  if (images.length !== 1) {
    fail(`expected exactly one image layer, found ${images.length}`)
  }
  const image = images[0]
  const file = text(image.image) ?? fail('the image layer has no image')
  const painting: Painting = {
    url: resolve(url, file),
    width: number(image.imagewidth) ?? fail('the image layer has no width'),
    height: number(image.imageheight) ?? fail('the image layer has no height'),
    x: number(image.x) ?? 0,
    y: number(image.y) ?? 0,
  }
  const extent: Rect = {
    x: painting.x,
    y: painting.y,
    width: painting.width,
    height: painting.height,
  }

  // ── everything the layer over it says ─────────────────────────────────
  let floor: Polygon | null = null
  const holes: Polygon[] = []
  const occluders: Occluder[] = []
  const zones: Zone[] = []
  const portals: Portal[] = []
  let spawn: Point | null = null

  for (const layer of layers) {
    if (layer.type !== 'objectgroup') continue
    const objects = Array.isArray(layer.objects) ? layer.objects : []

    for (const raw of objects) {
      const object = asRecord(raw) ?? fail('an object layer holds something that is not an object')
      const named = text(object.name) ?? ''
      const id = number(object.id) ?? '?'
      const where = named ? `"${named}" (object ${id})` : `object ${id}`

      // Tiled 1.9 renamed an object's `type` to `class`; both spellings turn up
      // in the wild, and a map exported by an older editor is still a map.
      const kind = text(object.class) ?? text(object.type) ?? ''

      if (number(object.rotation)) {
        fail(`${where} is rotated — draw it as a polygon instead, so what is authored is what is read`)
      }
      if (object.ellipse === true) fail(`${where} is an ellipse — draw it as a polygon instead`)
      if (object.polyline) fail(`${where} is a polyline, which encloses nothing`)

      switch (kind) {
        case 'floor': {
          if (floor) fail(`${where} is a second floor — a location has one walkable outline`)
          floor = outline(object, where, fail)
          break
        }
        case 'hole': {
          holes.push(outline(object, where, fail))
          break
        }
        case 'occluder': {
          const baseline =
            number(property(object.properties, 'baseline')) ??
            fail(`${where} has no "baseline" — the y at which it meets the ground`)
          const frame = box(object, where, fail)
          if (!within(frame, extent)) {
            fail(`${where} reaches outside the painting — an occluder is a piece of the plate`)
          }
          occluders.push({ name: named || 'a prop', frame, baseline })
          break
        }
        case 'trigger': {
          const zoneKey =
            text(property(object.properties, 'key')) ??
            fail(`${where} has no "key" — the name event placement will match on`)
          zones.push({ key: zoneKey, name: named || zoneKey, area: shape(object, where, fail) })
          break
        }
        case 'portal': {
          const to =
            text(property(object.properties, 'to')) ??
            fail(`${where} has no "to" — the place it leads to`)
          portals.push({
            key: text(property(object.properties, 'key')) ?? to,
            name: named || to,
            to,
            area: shape(object, where, fail),
          })
          break
        }
        case 'spawn': {
          if (spawn) fail(`${where} is a second spawn — a location has one`)
          spawn = { x: number(object.x) ?? 0, y: number(object.y) ?? 0 }
          break
        }
        default:
          fail(
            kind
              ? `${where} has an unknown class ${JSON.stringify(kind)}`
              : `${where} has no class — give it one, or delete it`,
          )
      }
    }
  }

  if (!floor) fail('no object with class "floor" — there is nowhere to walk')
  if (!spawn) fail('no object with class "spawn" — there is nowhere to arrive')

  const region: Region = { outer: floor, holes }

  // Smallest first: standing on your own stoop is standing on the East Side,
  // and the useful answer to "where am I" is the more particular of the two.
  zones.sort((a, b) => a.area.size - b.area.size)

  return {
    key,
    name,
    place,
    url,
    extent,
    painting,
    region,
    navmesh: buildNavmesh(region),
    occluders,
    zones,
    portals,
    spawn,
  }
}

/**
 * The trigger zones a point is inside, most specific first.
 *
 * The order is the sort `readMap` did once at load: smaller area is more
 * particular, so the first answer is the one worth saying out loud.
 */
export function zonesAt(level: Level, at: Point): Zone[] {
  return level.zones.filter((zone) => inArea(zone.area, at))
}

/** The portals a point is standing on. */
export function portalsAt(level: Level, at: Point): Portal[] {
  return level.portals.filter((portal) => inArea(portal.area, at))
}

/* ── reading Tiled's shapes ─────────────────────────────────────────────── */

type Fail = (why: string) => never

/** A closed outline: a polygon object, or a rectangle object's four corners. */
function outline(object: Record<string, unknown>, where: string, fail: Fail): Polygon {
  const points = polygon(object)
  if (points) {
    if (points.length < 3) fail(`${where} has fewer than three points`)
    if (area(points) <= 0) fail(`${where} encloses no area`)
    return points
  }
  return corners(box(object, where, fail))
}

/** The same, as something with an identity that can be asked what is in it. */
function shape(object: Record<string, unknown>, where: string, fail: Fail): Area {
  return toArea(outline(object, where, fail))
}

/**
 * A polygon object's points, in the map's own coordinates.
 *
 * Tiled writes them relative to the object's position, which is what lets an
 * author drag a whole zone without every vertex changing. Everything past this
 * function works in one space.
 */
function polygon(object: Record<string, unknown>): Polygon | null {
  const raw = object.polygon
  if (!Array.isArray(raw)) return null
  const ox = number(object.x) ?? 0
  const oy = number(object.y) ?? 0
  const points: Point[] = []
  for (const entry of raw) {
    const point = asRecord(entry)
    if (!point) return null
    points.push({ x: ox + (number(point.x) ?? 0), y: oy + (number(point.y) ?? 0) })
  }
  return points
}

/** A rectangle object. Tiled's x,y is its top-left corner. */
function box(object: Record<string, unknown>, where: string, fail: Fail): Rect {
  const points = polygon(object)
  if (points) return bounds(points)

  const width = number(object.width) ?? 0
  const height = number(object.height) ?? 0
  if (width <= 0 || height <= 0) {
    fail(`${where} has no size — it is a point where an area was wanted`)
  }
  return { x: number(object.x) ?? 0, y: number(object.y) ?? 0, width, height }
}

function within(inner: Rect, outer: Rect): boolean {
  return (
    inner.x >= outer.x &&
    inner.y >= outer.y &&
    inner.x + inner.width <= outer.x + outer.width &&
    inner.y + inner.height <= outer.y + outer.height
  )
}

/* ── reading Tiled's JSON ───────────────────────────────────────────────── */

/**
 * Every layer, including the ones inside group layers.
 *
 * Groups are how an author keeps a big map tidy, and a reader that only looked
 * at the top level would quietly lose a whole folder of trigger zones.
 */
function flatten(source: unknown): Record<string, unknown>[] {
  const out: Record<string, unknown>[] = []
  if (!Array.isArray(source)) return out
  for (const entry of source) {
    const layer = asRecord(entry)
    if (!layer) continue
    if (layer.type === 'group') out.push(...flatten(layer.layers))
    else out.push(layer)
  }
  return out
}

/** One of Tiled's custom properties, by name. */
function property(source: unknown, name: string): unknown {
  if (!Array.isArray(source)) return undefined
  for (const entry of source) {
    const each = asRecord(entry)
    if (each && each.name === name) return each.value
  }
  return undefined
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function text(value: unknown): string | null {
  return typeof value === 'string' && value !== '' ? value : null
}

/** A finite number, or null. Zero is null — every caller has a default for it. */
function number(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value !== 0 ? value : null
}

/**
 * A path relative to the map file, the way Tiled stores an image.
 *
 * Deliberately not `new URL`: the map's own URL is a path this app serves
 * (`/locations/…`), not an absolute one, and resolution has to work the same in
 * a test with no document to be relative to.
 */
export function resolve(base: string, relative: string): string {
  if (relative.startsWith('/') || /^[a-z][a-z0-9+.-]*:|^\/\//i.test(relative)) return relative

  const directory = base.slice(0, base.lastIndexOf('/') + 1)
  const out: string[] = []
  for (const part of (directory + relative).split('/')) {
    if (part === '.') continue
    if (part === '..') {
      out.pop()
      continue
    }
    out.push(part)
  }
  return out.join('/')
}
