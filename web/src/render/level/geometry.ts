/*
 * The arithmetic the invisible layer is made of.
 *
 * technical.md §10.3: a painted location is an image with a non-rendered data
 * layer over it — a walkable polygon, depth anchors, portals, and named trigger
 * zones. Every one of those is a polygon in the painting's own pixel
 * coordinates, so this file is polygons: what is inside one, what the nearest
 * point on one is, and whether a straight line between two points stays inside
 * a region with holes in it.
 *
 * It knows nothing about Pixi, about Tiled, or about the store. That is what
 * makes it the part of the world layer worth unit-testing on its own — the
 * assertions in `geometry.test.ts` are about geometry rather than about a
 * canvas nobody can see.
 *
 * All coordinates are plate pixels: the painting's own space, y increasing
 * downward, which is the space Tiled authors in. There is no separate world
 * unit, because inventing one would mean every authored number needed
 * converting before it could be checked against the picture it was drawn on.
 */

/** A point in plate pixels. */
export type Point = { x: number; y: number }

/** A closed outline. The last vertex joins the first; it is not repeated. */
export type Polygon = readonly Point[]

/** An axis-aligned box, as Tiled writes a rectangle object. */
export type Rect = { x: number; y: number; width: number; height: number }

/**
 * Where the player may stand: one outline, minus any number of holes.
 *
 * Holes are how a painted prop standing on the floor — a planter, a parked van,
 * the foot of a lamp post — stops being walked through. They are authored as
 * their own objects rather than cut out of the outline, because an author
 * moving a planter should move one rectangle rather than re-draw the street.
 */
export type Region = { outer: Polygon; holes: Polygon[] }

/**
 * An authored area with an identity: a trigger zone, a portal.
 *
 * A rectangle drawn in Tiled arrives here as its four corners, so a rectangular
 * zone and an L-shaped one are the same kind of thing downstream and there is
 * one containment test rather than two.
 */
export type Area = {
  points: Polygon
  bounds: Rect
  /** Square pixels. What sorts the zones you are standing in, smallest first. */
  size: number
}

/**
 * How close two coordinates have to be to count as the same one.
 *
 * A hundredth of a pixel. Authored geometry is written by a tool in whole or
 * half pixels, so anything below this is floating-point noise from an
 * intersection rather than a distinction anybody drew.
 */
const EPSILON = 1e-2

export function distance(a: Point, b: Point): number {
  return Math.hypot(b.x - a.x, b.y - a.y)
}

/** The four corners of a rectangle, clockwise from the top left. */
export function corners(rect: Rect): Polygon {
  const { x, y, width, height } = rect
  return [
    { x, y },
    { x: x + width, y },
    { x: x + width, y: y + height },
    { x, y: y + height },
  ]
}

/** The box a polygon fits in. */
export function bounds(points: Polygon): Rect {
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const p of points) {
    if (p.x < minX) minX = p.x
    if (p.y < minY) minY = p.y
    if (p.x > maxX) maxX = p.x
    if (p.y > maxY) maxY = p.y
  }
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY }
}

/** A polygon's area, whichever way round it was drawn. */
export function area(points: Polygon): number {
  let twice = 0
  for (let i = 0, j = points.length - 1; i < points.length; j = i++) {
    twice += points[j].x * points[i].y - points[i].x * points[j].y
  }
  return Math.abs(twice) / 2
}

/** An authored outline, ready to be asked what is inside it. */
export function toArea(points: Polygon): Area {
  return { points, bounds: bounds(points), size: area(points) }
}

/**
 * Is the point inside the outline?
 *
 * The crossing-number test: count how many times a ray cast to the left of the
 * point passes through an edge. Odd is inside. Points exactly on an edge are
 * deliberately not special-cased — nothing above asks about one, because every
 * point that could land on a boundary is pulled off it first (`nearestInside`,
 * and the inset the navmesh builds its nodes at).
 */
export function insidePolygon(points: Polygon, p: Point): boolean {
  let inside = false
  for (let i = 0, j = points.length - 1; i < points.length; j = i++) {
    const a = points[i]
    const b = points[j]
    if (a.y > p.y === b.y > p.y) continue
    const crossing = a.x + ((p.y - a.y) / (b.y - a.y)) * (b.x - a.x)
    if (p.x < crossing) inside = !inside
  }
  return inside
}

/** Is the point somewhere the player may stand? */
export function contains(region: Region, p: Point): boolean {
  if (!insidePolygon(region.outer, p)) return false
  for (const hole of region.holes) {
    if (insidePolygon(hole, p)) return false
  }
  return true
}

/** Is the point inside an authored zone? Bounds first, because most are not. */
export function inArea(zone: Area, p: Point): boolean {
  const b = zone.bounds
  if (p.x < b.x || p.y < b.y || p.x > b.x + b.width || p.y > b.y + b.height) return false
  return insidePolygon(zone.points, p)
}

/** The closest point to `p` on the segment `a`–`b`. */
export function nearestOnSegment(a: Point, b: Point, p: Point): Point {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const length = dx * dx + dy * dy
  if (length < EPSILON) return { x: a.x, y: a.y }
  let t = ((p.x - a.x) * dx + (p.y - a.y) * dy) / length
  t = Math.max(0, Math.min(1, t))
  return { x: a.x + dx * t, y: a.y + dy * t }
}

/** Every edge of the region — the outline's and every hole's. */
export function edges(region: Region): [Point, Point][] {
  const all: [Point, Point][] = []
  for (const ring of [region.outer, ...region.holes]) {
    for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
      all.push([ring[j], ring[i]])
    }
  }
  return all
}

/**
 * The nearest point the player could actually stand on.
 *
 * This is the whole of "the character cannot leave the walkable polygon" on the
 * input side: a tap on a rooftop is not refused, it is understood as the
 * nearest bit of pavement. Refusing it would mean a click that does nothing,
 * which reads as the game being broken rather than as the wall being a wall.
 *
 * `inset` pulls the answer a hair off the boundary it landed on, so that
 * everything downstream — containment, visibility, the path — is asking about a
 * point strictly inside rather than one sitting exactly on an edge, where the
 * answer depends on rounding.
 */
export function nearestInside(region: Region, p: Point, inset = 1): Point {
  if (contains(region, p)) return p

  let best: Point | null = null
  let bestDistance = Infinity
  for (const [a, b] of edges(region)) {
    const on = nearestOnSegment(a, b, p)
    const d = distance(on, p)
    if (d >= bestDistance) continue
    best = on
    bestDistance = d
  }
  if (!best) return p

  // Step from the boundary towards the point that was already inside — which,
  // since `p` is outside, is the opposite direction. Small enough that nobody
  // sees it; large enough that no test on the result is a coin toss.
  const towards = { x: best.x - p.x, y: best.y - p.y }
  const length = Math.hypot(towards.x, towards.y)
  if (length < EPSILON) return best

  const nudged = {
    x: best.x + (towards.x / length) * inset,
    y: best.y + (towards.y / length) * inset,
  }
  return contains(region, nudged) ? nudged : best
}

/**
 * Can the player walk straight from `a` to `b` without leaving the region?
 *
 * Cut the segment at every point where it meets a boundary edge, then ask
 * whether the middle of each resulting piece is inside. That is stricter than
 * "does it cross an edge": a line that passes exactly through the corner of a
 * hole crosses nothing and is still not walkable, and this catches it because
 * the piece on the far side of the corner has its midpoint in the hole.
 *
 * A segment that lies along a boundary is refused outright. Its midpoints
 * are on the edge, where inside-ness is a rounding question, and the cost of
 * refusing is that a path hugging a wall detours by the navmesh's inset instead
 * — a fraction of a pixel, and never a wrong answer.
 */
export function clearLine(region: Region, a: Point, b: Point): boolean {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const lengthSquared = dx * dx + dy * dy
  if (lengthSquared < EPSILON) return contains(region, a)

  const cuts = [0, 1]

  for (const [c, d] of edges(region)) {
    const ex = d.x - c.x
    const ey = d.y - c.y
    const denominator = dx * ey - dy * ex
    const ox = c.x - a.x
    const oy = c.y - a.y

    if (Math.abs(denominator) > EPSILON) {
      const t = (ox * ey - oy * ex) / denominator
      const u = (ox * dy - oy * dx) / denominator
      if (t < -EPSILON || t > 1 + EPSILON) continue
      if (u < -EPSILON || u > 1 + EPSILON) continue
      cuts.push(Math.max(0, Math.min(1, t)))
      continue
    }

    // Parallel. Only interesting when it is also collinear and overlapping,
    // which is the along-the-wall case refused above.
    if (Math.abs(ox * dy - oy * dx) > EPSILON) continue
    const t0 = (ox * dx + oy * dy) / lengthSquared
    const t1 = t0 + (ex * dx + ey * dy) / lengthSquared
    const low = Math.max(0, Math.min(t0, t1))
    const high = Math.min(1, Math.max(t0, t1))
    if (high - low > EPSILON) return false
  }

  cuts.sort((x, y) => x - y)

  for (let i = 1; i < cuts.length; i++) {
    const span = cuts[i] - cuts[i - 1]
    if (span < EPSILON) continue
    const t = (cuts[i - 1] + cuts[i]) / 2
    if (!contains(region, { x: a.x + dx * t, y: a.y + dy * t })) return false
  }

  return true
}
