/*
 * A location, as Tiled exports it.
 *
 * Tiled is the authoring tool (§10.3): an image layer plus object layers is a
 * visual editor for the walkable polygon, the depth anchors, the portals and the
 * trigger zones, and it exports JSON the renderer loads directly. So this is a
 * reader for someone else's format, and anything this file needs has to be
 * something an author can draw.
 *
 * Two choices:
 *
 *   1. Layer names mean nothing and object classes mean everything. An author
 *      can reorganise the layer panel or split the triggers across three groups
 *      without it reaching here. What is read is the class on each object.
 *
 *   2. An object this file does not understand is an error. A zone typed
 *      `triger` would otherwise load as a level with one fewer place for
 *      anything to happen, and nothing would say so until an event failed to
 *      fire months later.
 *
 * The result is a plain data `Level` with no Pixi in it. Building the sprites is
 * `../scene.ts`'s job, and this part is tested against a JSON fixture.
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
 * A piece of the painting that draws over the character when they are behind it
 * (§10.3's depth-sort anchors). `frame` is a rectangle of the props sheet and
 * `baseline` is the y at which the prop meets the ground. Sorting everything on
 * the floor by that one number covers depth under a fixed camera: further up the
 * image is further away.
 *
 * The rectangle is a bounding box, which is why the cut-out comes from the props
 * sheet rather than the plate. A tree's box is a hundred pixels of crown and
 * ninety of the pavement around it; cut from an opaque plate it would put a
 * rectangle of street over the character. Cut from a sheet where everything but
 * the props is transparent, only the tree occludes.
 */
export type Occluder = { name: string; frame: Rect; baseline: number }

/**
 * A named place inside the location where something can happen — the same
 * objects as event placement rules (§10.3, §4). `key` is authored rather than
 * generated, because it is the vocabulary the event system matches against and a
 * key that changed when somebody nudged a rectangle would unplace every event
 * that named it.
 */
/** One thing standing in a challenge's room, by the key its run knows it as. */
export type Thing = {
  key: string
  name: string
  at: Point
}

export type Zone = { key: string; name: string; area: Area }

/** A way out. #12 turns these into transitions; here they are authored data. */
export type Portal = { key: string; name: string; to: string; area: Area }

/**
 * Where you stand when you come in from a particular place. Ordinarily that is
 * the middle of the doorway you came through, which is what every street wants.
 * A room can want otherwise: the apartment's door is tucked under the closet, so
 * the middle of that doorway draws the character entirely behind a wardrobe.
 * Such a room authors the arrival as a point on the floor, matched to the place
 * it is an arrival from.
 */
export type Arrival = { from: string; at: Point }

/**
 * A source of light, authored as the area it reaches. The Low tier keeps its
 * lighting in the source art (§10.4): the plate is painted with the lamp already
 * lit, and a light object says where that lamp is so the High tier can let it
 * reach the things the painter could not paint — the character, the air, and
 * whatever is standing on the floor.
 *
 * Drawn as a rectangle, because an author drags rectangles: its centre is the
 * light and its half-extents are how far the light gets. A pool of sodium on a
 * pavement is wide and shallow under a fixed three-quarter camera (§10.3), so
 * the two reaches are separate numbers rather than one radius.
 *
 * `color` is null when the author did not say, and stays null: the palette
 * belongs to `@etm/ui` and is resolved once, in the renderer.
 */
export type Light = {
  name: string
  /** The centre of the drawn rectangle, in plate pixels. */
  at: Point
  /** How far the light reaches along each axis, in plate pixels. */
  reach: { x: number; y: number }
  /** The light's own colour, or null for the world's warm. */
  color: number | null
  /** How strongly it adds, 0 to 1. */
  intensity: number
  /** How much it wavers, 0 to 1. A sodium lamp does; a window does not. */
  flicker: number
}

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
  /**
   * The same picture with the ground taken out of it, or null. Everything the
   * character can walk behind, on a transparent sheet the size of the plate and
   * in the same place, so an occluder's frame indexes both pictures identically
   * and the cut-out has the prop's own silhouette rather than its bounding box.
   */
  props: Painting | null
  region: Region
  navmesh: Navmesh
  occluders: Occluder[]
  /** Sorted smallest first, so the most specific zone you are in comes first. */
  zones: Zone[]
  /**
   * The things a challenge's room has in it, if it is one.
   *
   * Empty for every location in the city — a street has furniture, not items —
   * and populated only for a room a run stands in. Drawn by the renderer rather
   * than baked into the plate, because a thing you have picked up has to stop
   * being in the room and a plate is baked once.
   */
  items: Thing[]
  portals: Portal[]
  /** The doors that say where you stand after coming through them. Usually empty. */
  arrivals: Arrival[]
  /** Where the light in the painting comes from. Only the High tier reads it. */
  lights: Light[]
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

  // ── the painting, and the props over it ───────────────────────────────
  //
  // Two image layers at most, told apart by their class like every object
  // below: `props` is the transparent sheet and anything else is the plate.
  const images = layers.filter((layer) => layer.type === 'imagelayer')
  const plates = images.filter((layer) => text(layer.class) !== 'props')
  const sheets = images.filter((layer) => text(layer.class) === 'props')
  if (plates.length !== 1) {
    fail(`expected exactly one plate image layer, found ${plates.length}`)
  }
  if (sheets.length > 1) fail(`expected at most one props image layer, found ${sheets.length}`)

  function readImage(image: Record<string, unknown>, what: string): Painting {
    return {
      url: resolve(url, text(image.image) ?? fail(`the ${what} image layer has no image`)),
      width: number(image.imagewidth) ?? fail(`the ${what} image layer has no width`),
      height: number(image.imageheight) ?? fail(`the ${what} image layer has no height`),
      x: number(image.x) ?? 0,
      y: number(image.y) ?? 0,
    }
  }

  const painting = readImage(plates[0], 'plate')
  const props = sheets.length === 1 ? readImage(sheets[0], 'props') : null
  if (props && (props.width !== painting.width || props.height !== painting.height)) {
    fail('the props sheet is not the size of the plate — an occluder frame indexes both')
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
  const items: Thing[] = []
  const portals: Portal[] = []
  const arrivals: Arrival[] = []
  const lights: Light[] = []
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
        case 'arrival': {
          const cameFrom =
            text(property(object.properties, 'from')) ??
            fail(`${where} has no "from" — the place it is an arrival from`)
          if (arrivals.some((arrival) => arrival.from === cameFrom)) {
            fail(`${where} is a second arrival from ${cameFrom} — you come in one way`)
          }
          arrivals.push({
            from: cameFrom,
            at: { x: number(object.x) ?? 0, y: number(object.y) ?? 0 },
          })
          break
        }
        case 'light': {
          const frame = box(object, where, fail)
          const intensity = fraction(property(object.properties, 'intensity'), where, fail) ?? 0.4
          const flicker = fraction(property(object.properties, 'flicker'), where, fail) ?? 0
          lights.push({
            name: named || 'a light',
            at: { x: frame.x + frame.width / 2, y: frame.y + frame.height / 2 },
            reach: { x: frame.width / 2, y: frame.height / 2 },
            color: colour(property(object.properties, 'color'), where, fail),
            intensity,
            flicker,
          })
          break
        }
        case 'item': {
          // A thing a challenge's room has in it. The key is the mechanic's own
          // — the same string the run offers as an option — so the renderer can
          // draw *this* object and the command layer can commit it, without
          // either learning what the other calls it.
          const itemKey =
            text(property(object.properties, 'key')) ??
            fail(`${where} has no "key" — the challenge item it is`)
          items.push({
            key: itemKey,
            name: named || itemKey,
            at: { x: number(object.x) ?? 0, y: number(object.y) ?? 0 },
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

  // Smallest first: standing on your own stoop is standing in Wilmington,
  // and the useful answer to "where am I" is the more particular of the two.
  zones.sort((a, b) => a.area.size - b.area.size)

  return {
    key,
    name,
    place,
    url,
    extent,
    painting,
    props,
    region,
    navmesh: buildNavmesh(region),
    occluders,
    zones,
    items,
    portals,
    arrivals,
    lights,
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

/**
 * A number an author meant as "how much", between 0 and 1.
 *
 * Absent is null and the caller supplies its own default. Out of range is an
 * error rather than a clamp: an intensity of 40 is somebody who meant per cent,
 * and quietly drawing them a 1 would leave them wondering why the slider did
 * nothing.
 */
function fraction(value: unknown, where: string, fail: Fail): number | null {
  if (value === undefined || value === null) return null
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    fail(`${where} has a value that is not a number where a fraction was wanted`)
  }
  if (value < 0 || value > 1) fail(`${where} is ${value}, and a fraction is between 0 and 1`)
  return value
}

/**
 * A colour, the way Tiled's colour property writes one.
 *
 * `#aarrggbb` is Tiled's own spelling and `#rrggbb` is what a hand-typed string
 * property looks like; both are read, and the alpha is dropped. How opaque a
 * light is belongs to its intensity, which is a number an author can see the
 * effect of, rather than to two hex digits at the front of a colour.
 */
function colour(value: unknown, where: string, fail: Fail): number | null {
  if (value === undefined || value === null) return null
  const written = text(value) ?? fail(`${where} has a colour that is not written down`)
  const hex = /^#?([0-9a-f]{6}|[0-9a-f]{8})$/i.exec(written)
  if (!hex) fail(`${where} has ${JSON.stringify(written)} where a #rrggbb colour was wanted`)
  const digits = hex[1]
  return parseInt(digits.length === 8 ? digits.slice(2) : digits, 16)
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
