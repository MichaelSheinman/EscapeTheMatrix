/*
 * Getting from where you are standing to where you tapped.
 *
 * technical.md §10.3 asks the walkable polygon to be a "navmesh / waypoint
 * graph for pathfinding". This is the waypoint graph, and the waypoints are the
 * corners of the region itself: on a floor made of straight edges, the shortest
 * route between two points either goes straight there or bends around a corner,
 * and there is nowhere else it can bend. So the graph is every corner that can
 * see every other corner, and the answer A* finds over it is not an
 * approximation of the shortest path — it is the shortest path.
 *
 * That is why there is no triangulation here and no funnel step. A navmesh of
 * triangles needs one, because A* over triangles finds a *sequence of
 * triangles* and the route through them still has to be pulled taut. A
 * visibility graph skips both halves. It costs O(n²) visibility tests to build,
 * with n the number of corners — a few hundred tests for a hand-authored street,
 * paid once when the location loads.
 *
 * The graph is built at load and never rebuilt: authored geometry does not
 * move. A door that opens is a second region, not an edited one.
 */

import { clearLine, contains, distance, type Point, type Region } from './geometry'

/** One corner, and everything it can see. */
type Link = { to: number; cost: number }

export type Navmesh = {
  region: Region
  /** The corners, pulled a little inside the region. See `INSET`. */
  nodes: Point[]
  /** `links[i]` is every node `nodes[i]` has a clear line to. */
  links: Link[][]
}

/**
 * How far inside the region a corner's waypoint sits.
 *
 * A waypoint exactly on a corner is on the boundary, where "is this inside?"
 * depends on rounding, and where every line drawn to it runs along a wall — the
 * case `clearLine` deliberately refuses. Two pixels is under the width of the
 * character's foot and well above the noise.
 */
const INSET = 2

/**
 * Build the graph for a region.
 *
 * Corners whose inset falls outside the region are dropped rather than fudged.
 * That happens at a spike — an outline doubling back on itself — which is an
 * authoring mistake rather than a shape anybody walks round, and a waypoint
 * outside the floor would let a path leave it.
 */
export function buildNavmesh(region: Region): Navmesh {
  const nodes: Point[] = []

  for (const ring of [region.outer, ...region.holes]) {
    for (let i = 0; i < ring.length; i++) {
      const inside = insetCorner(region, ring, i)
      if (inside) nodes.push(inside)
    }
  }

  const links: Link[][] = nodes.map(() => [])
  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      if (!clearLine(region, nodes[i], nodes[j])) continue
      const cost = distance(nodes[i], nodes[j])
      links[i].push({ to: j, cost })
      links[j].push({ to: i, cost })
    }
  }

  return { region, nodes, links }
}

/**
 * The route from `from` to `to`, as the points to walk through.
 *
 * Both ends are assumed to be inside the region — `nearestInside` is what makes
 * that true of a tap, and the character never leaves it. The answer always
 * begins at `from` and ends at `to`.
 *
 * An empty list means there is no route: two floors that do not touch, which a
 * single authored location should not have. The caller stands still rather than
 * inventing a way across.
 */
export function findPath(mesh: Navmesh, from: Point, to: Point): Point[] {
  if (!contains(mesh.region, to) || !contains(mesh.region, from)) return []
  if (clearLine(mesh.region, from, to)) return [from, to]

  const { nodes, links, region } = mesh
  const count = nodes.length
  const START = count
  const GOAL = count + 1

  // The two ends join the graph for the length of this query and leave again.
  // Building them into it would mean rebuilding the whole thing per tap.
  const fromStart: Link[] = []
  const toGoal: boolean[] = new Array(count).fill(false)
  for (let i = 0; i < count; i++) {
    if (clearLine(region, from, nodes[i])) {
      fromStart.push({ to: i, cost: distance(from, nodes[i]) })
    }
    if (clearLine(region, nodes[i], to)) toGoal[i] = true
  }
  if (fromStart.length === 0) return []

  const at = (i: number): Point => (i === START ? from : i === GOAL ? to : nodes[i])
  const near = (i: number): Link[] => {
    if (i === GOAL) return []
    if (i === START) return fromStart
    const out = links[i].slice()
    if (toGoal[i]) out.push({ to: GOAL, cost: distance(nodes[i], to) })
    return out
  }

  const best = new Array<number>(count + 2).fill(Infinity)
  const guess = new Array<number>(count + 2).fill(Infinity)
  const cameFrom = new Array<number>(count + 2).fill(-1)
  const settled = new Array<boolean>(count + 2).fill(false)

  best[START] = 0
  guess[START] = distance(from, to)

  for (;;) {
    // A linear scan for the cheapest open node. The graph is the corners of one
    // hand-authored floor — tens of nodes, not thousands — so a heap would be
    // more code than the scan it replaces and no faster at this size.
    let current = -1
    let cheapest = Infinity
    for (let i = 0; i < guess.length; i++) {
      if (settled[i] || guess[i] >= cheapest) continue
      current = i
      cheapest = guess[i]
    }
    if (current === -1) return []
    if (current === GOAL) break

    settled[current] = true
    for (const link of near(current)) {
      const through = best[current] + link.cost
      if (through >= best[link.to]) continue
      best[link.to] = through
      guess[link.to] = through + distance(at(link.to), to)
      cameFrom[link.to] = current
    }
  }

  const route: Point[] = []
  for (let i: number = GOAL; i !== -1; i = cameFrom[i]) route.unshift(at(i))
  return route
}

/**
 * A ring's `i`th corner, moved just inside the region.
 *
 * The direction is the bisector of the two edges meeting there. Which side of
 * it is the inside depends on the corner and on whether the ring is an outline
 * or a hole, so rather than track winding — which an author can reverse by
 * drawing a polygon the other way round — both sides are tried and the one that
 * lands inside the region wins.
 */
function insetCorner(region: Region, ring: readonly Point[], i: number): Point | null {
  const here = ring[i]
  const before = ring[(i - 1 + ring.length) % ring.length]
  const after = ring[(i + 1) % ring.length]

  const a = unit(before.x - here.x, before.y - here.y)
  const b = unit(after.x - here.x, after.y - here.y)
  let dx = a.x + b.x
  let dy = a.y + b.y

  // A straight-through vertex has no bisector; step perpendicular instead.
  if (Math.hypot(dx, dy) < 1e-6) {
    dx = -b.y
    dy = b.x
  }

  const step = unit(dx, dy)
  for (const sign of [1, -1]) {
    const candidate = {
      x: here.x + step.x * INSET * sign,
      y: here.y + step.y * INSET * sign,
    }
    if (contains(region, candidate)) return candidate
  }
  return null
}

function unit(x: number, y: number): Point {
  const length = Math.hypot(x, y)
  return length < 1e-9 ? { x: 0, y: 0 } : { x: x / length, y: y / length }
}
