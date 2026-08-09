/*
 * The logic layer, derived rather than drawn.
 *
 * technical.md §10.3 authors each location's invisible layer in Tiled. The
 * procedurally authored locations (#35) already carry all of that in their
 * scene data — where you can walk, what stands where, which spots are named —
 * so their maps are *generated* from it, by this script, in exactly the shape
 * `web/src/render/level/tiled.ts` reads. The repo stores the recipe (#39):
 * scene data in, `.tmj` out, byte-for-byte repeatable.
 *
 *   node tools/bake/gen-tmj.mjs
 *
 * writes web/public/locations/<key>/<key>.tmj for the street and the
 * apartment, and web/src/chrome/markers.gen.ts for the labels the chrome pins
 * over them. Plates come from bake.html — see README.md here.
 */

import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { SCALES, TW, TH, iso, isoc, frameFor } from './frame.mjs'

// The preview's modules publish onto window.ETM; give them one.
globalThis.window = globalThis
await import('../../preview/src/render.js')
await import('../../preview/src/people.js')
await import('../../preview/src/scenes.js')
const E = globalThis.ETM

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..')

/** Which ground codes can be walked, per scene — preview/src/app.js's table. */
const WALKABLE = { street: 'srkpgd', apartment: 'f' }

/** Which city place each scene is, and the map key it ships under. */
const PLACES = {
  street: { place: 'east-side', name: 'The East Side' },
  apartment: { place: 'east-side-apartment', name: 'Your apartment' },
}

/* ── the walkable region, traced off the grid ───────────────────────────── */

/**
 * Every boundary loop of the walkable mask: one outer outline and its holes,
 * as lattice-point polygons. Directed edges are emitted with the interior on
 * the left and chained end to start; collinear runs are merged after.
 */
function traceLoops(rows, codes) {
  const R = rows.length
  const C = rows[0].length
  const walk = (x, y) => x >= 0 && x < C && y >= 0 && y < R && codes.includes(rows[y][x])

  /** Directed border edges, keyed by their start lattice point. */
  const edges = new Map()
  const put = (sx, sy, ex, ey) => {
    const key = `${sx},${sy}`
    const list = edges.get(key) ?? []
    list.push({ sx, sy, ex, ey, used: false })
    edges.set(key, list)
  }

  for (let y = 0; y < R; y++) {
    for (let x = 0; x < C; x++) {
      if (!walk(x, y)) continue
      if (!walk(x, y - 1)) put(x, y, x + 1, y)
      if (!walk(x + 1, y)) put(x + 1, y, x + 1, y + 1)
      if (!walk(x, y + 1)) put(x + 1, y + 1, x, y + 1)
      if (!walk(x - 1, y)) put(x, y + 1, x, y)
    }
  }

  const loops = []
  for (const list of edges.values()) {
    for (const first of list) {
      if (first.used) continue
      const loop = []
      let edge = first
      while (!edge.used) {
        edge.used = true
        loop.push([edge.sx, edge.sy])
        const nexts = (edges.get(`${edge.ex},${edge.ey}`) ?? []).filter((e) => !e.used)
        if (nexts.length === 0) break
        // At a pinch point two loops touch; preferring the sharpest left turn
        // keeps each loop to its own side of the touch.
        const dx = edge.ex - edge.sx
        const dy = edge.ey - edge.sy
        nexts.sort((a, b) => turn(dx, dy, a) - turn(dx, dy, b))
        edge = nexts[0]
      }
      if (loop.length >= 4) loops.push(mergeCollinear(loop))
    }
  }

  // The outer outline is the loop enclosing the most area; the rest are holes.
  loops.sort((a, b) => Math.abs(shoelace(b)) - Math.abs(shoelace(a)))
  return { outer: loops[0], holes: loops.slice(1) }
}

/** Left turns first: 0 sharp left, 1 straight, 2 right, 3 back the way we came. */
function turn(dx, dy, edge) {
  const ndx = edge.ex - edge.sx
  const ndy = edge.ey - edge.sy
  const cross = dx * ndy - dy * ndx
  const dot = dx * ndx + dy * ndy
  if (cross < 0) return 0
  if (cross === 0) return dot > 0 ? 1 : 3
  return 2
}

function mergeCollinear(points) {
  const out = []
  for (let i = 0; i < points.length; i++) {
    const a = points[(i + points.length - 1) % points.length]
    const b = points[i]
    const c = points[(i + 1) % points.length]
    const cross = (b[0] - a[0]) * (c[1] - b[1]) - (b[1] - a[1]) * (c[0] - b[0])
    if (cross !== 0) out.push(b)
  }
  return out
}

function shoelace(points) {
  let twice = 0
  for (let i = 0, j = points.length - 1; i < points.length; j = i++) {
    twice += points[j][0] * points[i][1] - points[i][0] * points[j][1]
  }
  return twice / 2
}

/* ── projection onto the plate ──────────────────────────────────────────── */

function projector(scene, sceneKey) {
  const frame = frameFor(scene)
  const scale = SCALES[sceneKey]
  const world = (p) => ({
    x: Math.round((p.x + frame.offset.x) * scale * 10) / 10,
    y: Math.round((p.y + frame.offset.y) * scale * 10) / 10,
  })
  return {
    width: Math.ceil(frame.width * scale),
    height: Math.ceil(frame.height * scale),
    scale,
    /** A lattice/grid point, to plate pixels. */
    grid: (gx, gy) => world(iso(gx, gy)),
    /** The centre of a tile, to plate pixels. */
    at: (gx, gy) => world(isoc(gx, gy)),
    world,
  }
}

/** A grid-space rectangle as a plate-space polygon object. */
function gridPoly(P, x0, y0, x1, y1) {
  return [P.grid(x0, y0), P.grid(x1, y0), P.grid(x1, y1), P.grid(x0, y1)]
}

/* ── tmj assembly ───────────────────────────────────────────────────────── */

function makeMap(sceneKey, build) {
  const scene = E.SCENES[sceneKey]
  const P = projector(scene, sceneKey)
  let id = 0
  const objects = []

  const object = (cls, name, shape, properties = []) => {
    objects.push({
      id: ++id,
      name,
      class: cls,
      visible: true,
      rotation: 0,
      ...shape,
      ...(properties.length > 0 ? { properties } : {}),
    })
  }
  const poly = (points) => ({ x: 0, y: 0, width: 0, height: 0, polygon: points })
  const rect = (a, b) => ({
    x: Math.min(a.x, b.x),
    y: Math.min(a.y, b.y),
    width: Math.abs(b.x - a.x),
    height: Math.abs(b.y - a.y),
  })
  const prop = (name, type, value) => ({ name, type, value })

  // The region, off the grid.
  const { outer, holes } = traceLoops(scene.rows, WALKABLE[sceneKey])
  object('floor', 'where you can walk', poly(outer.map(([x, y]) => P.grid(x, y))))
  for (const hole of holes) {
    object('hole', 'not through here', poly(hole.map(([x, y]) => P.grid(x, y))))
  }

  build({ scene, P, object, poly, rect, prop, id: () => id })

  const { place, name } = PLACES[sceneKey]
  return {
    map: {
      type: 'map',
      version: '1.10',
      tiledversion: 'generated — tools/bake/gen-tmj.mjs',
      orientation: 'orthogonal',
      renderorder: 'right-down',
      infinite: false,
      width: Math.ceil(P.width / TW),
      height: Math.ceil(P.height / TH),
      tilewidth: TW,
      tileheight: TH,
      nextlayerid: 4,
      nextobjectid: id + 1,
      properties: [
        prop('key', 'string', place),
        prop('name', 'string', name),
        prop('place', 'string', place),
      ],
      layers: [
        {
          id: 1,
          type: 'imagelayer',
          class: 'plate',
          name: 'plate',
          image: 'plate.webp',
          imagewidth: P.width,
          imageheight: P.height,
          x: 0,
          y: 0,
          visible: true,
          opacity: 1,
        },
        {
          id: 2,
          type: 'imagelayer',
          class: 'props',
          name: 'props',
          image: 'props.webp',
          imagewidth: P.width,
          imageheight: P.height,
          x: 0,
          y: 0,
          visible: true,
          opacity: 1,
        },
        {
          id: 3,
          type: 'objectgroup',
          name: 'logic',
          visible: true,
          opacity: 1,
          objects,
        },
      ],
    },
    P,
  }
}

/**
 * An occluder for a box standing on the grid: the projected bounding box of
 * its footprint and height, clipped to the plate, with its baseline at the
 * front corner's ground line.
 */
function boxOccluder(P, b, extraTop, plateW, plateH) {
  const left = P.grid(b.x, b.y + b.d)
  const right = P.grid(b.x + b.w, b.y)
  const back = P.grid(b.x, b.y)
  const front = P.grid(b.x + b.w, b.y + b.d)
  const top = back.y - (b.h + extraTop) * P.scale
  const bottom = front.y + (TH / 2 + 10) * P.scale
  const x0 = Math.max(0, left.x - 10 * P.scale)
  const x1 = Math.min(plateW, right.x + 10 * P.scale)
  const y0 = Math.max(0, top)
  const y1 = Math.min(plateH, bottom)
  if (x1 <= x0 || y1 <= y0) return null
  return {
    frame: { x: x0, y: y0, width: Math.round(x1 - x0), height: Math.round(y1 - y0) },
    baseline: Math.round(front.y + (TH / 4) * P.scale),
  }
}

/** How much taller than its box a building's roof furniture makes it. */
function roofExtra(b) {
  if (b.tower && b.type === 'civic') return 140
  if (b.type === 'terminal') return 150
  if (b.type === 'works' || b.type === 'gym') return 30
  return 12
}

/** Rough frames for the free-standing fixtures a character can walk behind. */
const FIXTURE_FRAMES = {
  tree: { half: 50, up: 100, down: 10 },
  lamp: { half: 20, up: 100, down: 8 },
  stall: { half: 70, up: 86, down: 26 },
  newsstand: { half: 60, up: 86, down: 22 },
  busStop: { half: 96, up: 84, down: 20 },
  fountain: { half: 56, up: 74, down: 28 },
  plane: { half: 250, up: 70, down: 60 },
  boat: { half: 50, up: 70, down: 24 },
  car: { half: 78, up: 56, down: 26 },
  bus: { half: 130, up: 94, down: 30 },
  floorLamp: { half: 22, up: 100, down: 8 },
  plant: { half: 24, up: 56, down: 8 },
}

function fixtureOccluders(P, fixtures, plateW, plateH, object, prop) {
  for (const f of fixtures ?? []) {
    const shape = FIXTURE_FRAMES[f.kind]
    if (!shape) continue
    const c = P.at(f.x, f.y)
    const s = P.scale
    const x0 = Math.max(0, c.x - shape.half * s)
    const x1 = Math.min(plateW, c.x + shape.half * s)
    const y0 = Math.max(0, c.y - shape.up * s)
    const y1 = Math.min(plateH, c.y + shape.down * s)
    if (x1 <= x0 || y1 <= y0) continue
    object(
      'occluder',
      f.kind,
      { x: Math.round(x0), y: Math.round(y0), width: Math.round(x1 - x0), height: Math.round(y1 - y0) },
      [prop('baseline', 'float', Math.round(c.y))],
    )
  }
}

/** The axis-aligned bounds of a polygon, as a rect shape. */
function boundsOf(points) {
  const xs = points.map((p) => p.x)
  const ys = points.map((p) => p.y)
  const x = Math.min(...xs)
  const y = Math.min(...ys)
  return { x, y, width: Math.max(...xs) - x, height: Math.max(...ys) - y }
}

/** A rect centred on a point, with the given half-extents in plate pixels. */
function around(c, halfX, halfY) {
  return { x: Math.round(c.x - halfX), y: Math.round(c.y - halfY), width: halfX * 2, height: halfY * 2 }
}

/* ── the chrome's markers, generated beside the maps ────────────────────── */

/** Preview panel names → the web client's screens. */
const SCREENS = {
  work: 'work',
  jobs: 'work',
  health: 'body',
  routines: 'rules',
  travel: 'map',
  conflicts: 'among',
  playbook: 'playbook',
  closet: 'closet',
}

function clientMarkers(P, scene, skip = new Set()) {
  const out = []
  for (const m of scene.markers ?? []) {
    if (m.kind === 'zone' || m.go || skip.has(m.label)) continue
    const at = P.at(m.x, m.y)
    const screen = m.panel ? SCREENS[m.panel] : undefined
    out.push({
      kind: m.kind === 'use' && !screen ? 'sign' : m.kind,
      label: m.label,
      tip: m.tip ?? '',
      ...(m.hint ? { hint: m.hint } : {}),
      at: { x: Math.round(at.x), y: Math.round(at.y) },
      ...(screen ? { screen } : {}),
    })
  }
  return out
}

/* ── the street ─────────────────────────────────────────────────────────── */

function street() {
  const generated = makeMap('street', ({ scene, P, object, poly, prop }) => {
    const W = P.width
    const H = P.height

    // Buildings, then the taller street furniture, all as cut-outs.
    for (const b of scene.boxes) {
      const cut = boxOccluder(P, b, roofExtra(b), W, H)
      if (cut) object('occluder', b.type ?? 'a building', cut.frame, [prop('baseline', 'float', cut.baseline)])
    }
    fixtureOccluders(P, scene.fixtures, W, H, object, prop)

    // The named spots, in the city's own zone vocabulary (internal/city).
    // HOME sits in the middle block's first run: pavement band y 39–41.
    const home = scene.markers.find((m) => m.go === 'apartment')
    object('trigger', 'On foot, the East Side', boundsOf(gridPoly(P, 0, 9, 120, 63)), [
      prop('key', 'string', 'on-foot'),
    ])
    object('trigger', 'Your stoop', boundsOf(gridPoly(P, home.x - 1.6, home.y - 0.2, home.x + 1.6, home.y + 1.9)), [
      prop('key', 'string', 'home-door'),
    ])
    const shop = scene.boxes.find((b) => b.blade === '#3f6b57') // the bodega
    object(
      'trigger',
      'The corner shop',
      boundsOf(gridPoly(P, shop.x - 0.5, shop.y + shop.d, shop.x + shop.w + 0.5, shop.y + shop.d + 2)),
      [prop('key', 'string', 'corner-shop')],
    )
    const stop = scene.fixtures.find((f) => f.kind === 'busStop')
    object(
      'trigger',
      'The bus stop',
      boundsOf(gridPoly(P, stop.x - 2, stop.y - 0.3, stop.x + 4.6, stop.y + 2.1)),
      [prop('key', 'string', 'transit-stop')],
    )

    // The ways out: your own front door, and the road off the west edge.
    object(
      'portal',
      'Your apartment',
      boundsOf(gridPoly(P, home.x - 1.4, home.y - 0.2, home.x + 1.4, home.y + 1.2)),
      [prop('to', 'string', 'east-side-apartment')],
    )
    object('portal', 'Downtown', boundsOf(gridPoly(P, 0, 26, 1.2, 31)), [
      prop('to', 'string', 'downtown'),
    ])

    // You start on the pavement outside your own door.
    const spawn = P.at(home.x, home.y + 2.6)
    object('spawn', 'spawn', { x: Math.round(spawn.x), y: Math.round(spawn.y), point: true })
  })

  const scene = E.SCENES.street
  const markers = clientMarkers(projector(scene, 'street'), scene, new Set(['Apartment 3B']))
  return { generated, markers }
}

/* ── the apartment ──────────────────────────────────────────────────────── */

function apartment() {
  const generated = makeMap('apartment', ({ scene, P, object, poly, prop }) => {
    const W = P.width
    const H = P.height

    // The furniture is walked around as well as behind: each solid box is a
    // hole in the floor and a cut-out over the character.
    for (const b of scene.boxes) {
      if (b.wall) continue
      object('hole', b.label ?? 'furniture', poly([
        P.grid(b.x, b.y),
        P.grid(b.x + b.w, b.y),
        P.grid(b.x + b.w, b.y + b.d),
        P.grid(b.x, b.y + b.d),
      ]))
      const cut = boxOccluder(P, b, 10, W, H)
      if (cut) object('occluder', b.label ?? 'furniture', cut.frame, [prop('baseline', 'float', cut.baseline)])
    }
    fixtureOccluders(P, scene.fixtures, W, H, object, prop)

    // The named spots, in the city's own zone vocabulary.
    object('trigger', 'At home', boundsOf(gridPoly(P, 1, 2, 14, 10)), [prop('key', 'string', 'at-home')])
    const bed = scene.boxes.find((b) => b.label === 'bed')
    object(
      'trigger',
      'The bed',
      boundsOf(gridPoly(P, bed.x - 0.6, bed.y, bed.x + bed.w + 0.4, bed.y + bed.d + 1.2)),
      [prop('key', 'string', 'bed')],
    )
    object('trigger', 'The window', boundsOf(gridPoly(P, 4, 2, 11, 4.4)), [
      prop('key', 'string', 'window'),
    ])

    // The way out: the door in the side wall — kept south of the closet, so
    // arriving through it never lands inside a furniture hole.
    const door = scene.markers.find((m) => m.go === 'street')
    object(
      'portal',
      'Out to the street',
      boundsOf(gridPoly(P, door.x - 0.3, door.y + 0.3, door.x + 1.3, door.y + 1.5)),
      [prop('to', 'string', 'east-side')],
    )

    // The three authored lights the High tier reaches with (§10.4): the wall
    // window is the art bible's one cold contrast; the lamps are warm.
    const windowBounds = boundsOf(gridPoly(P, 4, 0.6, 7, 4.2))
    object('light', 'the window', windowBounds, [
      prop('color', 'string', '#3a6ba6'),
      prop('intensity', 'float', 0.2),
    ])
    const shade = P.at(3.0, 2.4)
    object('light', 'the lamp shade', around(shade, 150, 130), [
      prop('color', 'string', '#ffdca8'),
      prop('intensity', 'float', 0.5),
      prop('flicker', 'float', 0.04),
    ])
    const pool = P.at(12.6, 6.3)
    object('light', 'the lamp pool', around(pool, 360, 150), [
      prop('intensity', 'float', 0.34),
      prop('flicker', 'float', 0.04),
    ])

    const spawn = P.at(scene.player.start[0], scene.player.start[1])
    object('spawn', 'spawn', { x: Math.round(spawn.x), y: Math.round(spawn.y), point: true })
  })

  const scene = E.SCENES.apartment
  const markers = clientMarkers(projector(scene, 'apartment'), scene, new Set(['Out to the street']))
  return { generated, markers }
}

/* ── write everything ───────────────────────────────────────────────────── */

const out = {
  'east-side': street(),
  'east-side-apartment': apartment(),
}

for (const [key, { generated }] of Object.entries(out)) {
  const dir = join(root, 'web', 'public', 'locations', key)
  mkdirSync(dir, { recursive: true })
  const file = join(dir, `${key}.tmj`)
  writeFileSync(file, JSON.stringify(generated.map, null, 1) + '\n')
  console.log(
    `${file}: ${generated.P.width}×${generated.P.height}, ` +
      `${generated.map.layers.find((l) => l.type === 'objectgroup').objects.length} objects`,
  )
}

const markers = Object.fromEntries(Object.entries(out).map(([key, o]) => [key, o.markers]))
const markersFile = join(root, 'web', 'src', 'chrome', 'markers.gen.ts')
writeFileSync(
  markersFile,
  `/* Generated by tools/bake/gen-tmj.mjs — the labels the chrome pins over the
 * generated plates, derived from the same scene data. Do not edit by hand. */

export type GeneratedMarker = {
  kind: 'portal' | 'use' | 'sign' | 'soon'
  label: string
  tip: string
  hint?: string
  at: { x: number; y: number }
  screen?: string
}

export const GENERATED: Record<string, GeneratedMarker[]> = ${JSON.stringify(markers, null, 2)}
`,
)
console.log(`${markersFile}: written`)
