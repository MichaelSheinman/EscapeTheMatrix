import { describe, expect, it } from 'vitest'

import { clearLine, contains, corners, distance, type Point, type Region } from './geometry'
import { buildNavmesh, findPath } from './navmesh'

/*
 * The waypoint graph, and what it is for.
 *
 * Two properties are worth more than any particular route, and both are
 * asserted on every path this file produces:
 *
 *   1. **Every leg stays on the floor.** That is the whole of "the character
 *      cannot leave the walkable polygon" — the scene never places them
 *      anywhere except on a leg of a route this returned.
 *   2. **The route is the shortest one.** A visibility graph is exact, so a
 *      route that bends is only allowed to bend as far as it has to.
 */

/** Assert a route is walkable end to end, and say how long it is. */
function walk(region: Region, route: Point[]): number {
  expect(route.length).toBeGreaterThan(1)
  let length = 0
  for (let i = 1; i < route.length; i++) {
    expect(contains(region, route[i - 1])).toBe(true)
    expect(contains(region, route[i])).toBe(true)
    expect(clearLine(region, route[i - 1], route[i])).toBe(true)
    length += distance(route[i - 1], route[i])
  }
  return length
}

/** A hundred-pixel room with a pillar in the middle of it. */
const room: Region = {
  outer: corners({ x: 0, y: 0, width: 100, height: 100 }),
  holes: [corners({ x: 40, y: 40, width: 20, height: 20 })],
}

/** An L-shaped floor. The only way from one arm to the other is round a corner. */
const el: Region = {
  outer: [
    { x: 0, y: 0 },
    { x: 100, y: 0 },
    { x: 100, y: 40 },
    { x: 40, y: 40 },
    { x: 40, y: 100 },
    { x: 0, y: 100 },
  ],
  holes: [],
}

describe('a route across open floor', () => {
  it('is the straight line, and has no waypoints in it', () => {
    const mesh = buildNavmesh(room)
    const route = findPath(mesh, { x: 5, y: 5 }, { x: 5, y: 95 })
    expect(route).toEqual([
      { x: 5, y: 5 },
      { x: 5, y: 95 },
    ])
  })
})

describe('a route round something', () => {
  it('bends around a hole rather than through it', () => {
    const mesh = buildNavmesh(room)
    const from = { x: 50, y: 5 }
    const to = { x: 50, y: 95 }
    const route = findPath(mesh, from, to)

    const length = walk(room, route)
    expect(route[0]).toEqual(from)
    expect(route[route.length - 1]).toEqual(to)
    // Longer than straight through, because straight through is the pillar.
    expect(length).toBeGreaterThan(distance(from, to))
    // And not much longer: past two corners of a twenty-pixel pillar.
    expect(length).toBeLessThan(distance(from, to) * 1.2)
  })

  it('bends around a concave corner', () => {
    const mesh = buildNavmesh(el)
    const from = { x: 10, y: 90 }
    const to = { x: 90, y: 10 }
    const route = findPath(mesh, from, to)

    walk(el, route)
    // One bend, and it is at the inside corner — just inside it, by the
    // waypoint inset, which is what keeps the route off the wall it turns on.
    expect(route).toHaveLength(3)
    expect(distance(route[1], { x: 40, y: 40 })).toBeLessThan(3)
  })

  it('takes the shorter way round, not merely a way round', () => {
    const mesh = buildNavmesh(room)
    // Starting just left of centre, the way round is to the left.
    const route = findPath(mesh, { x: 45, y: 5 }, { x: 45, y: 95 })
    walk(room, route)
    for (const point of route) expect(point.x).toBeLessThanOrEqual(50)
  })
})

describe('a route that does not exist', () => {
  it('is empty rather than a straight line through a wall', () => {
    const mesh = buildNavmesh(room)
    expect(findPath(mesh, { x: 5, y: 5 }, { x: 50, y: 50 })).toEqual([])
    expect(findPath(mesh, { x: 5, y: 5 }, { x: 500, y: 500 })).toEqual([])
  })
})

describe('the graph itself', () => {
  it('puts a waypoint just inside each corner, never on one', () => {
    const mesh = buildNavmesh(el)
    expect(mesh.nodes).toHaveLength(el.outer.length)
    for (const node of mesh.nodes) expect(contains(el, node)).toBe(true)
    for (const corner of el.outer) {
      const nearest = Math.min(...mesh.nodes.map((node) => distance(node, corner)))
      expect(nearest).toBeGreaterThan(0)
      expect(nearest).toBeLessThan(4)
    }
  })

  it('links only pairs that can see each other', () => {
    const mesh = buildNavmesh(room)
    for (let i = 0; i < mesh.nodes.length; i++) {
      for (const link of mesh.links[i]) {
        expect(clearLine(room, mesh.nodes[i], mesh.nodes[link.to])).toBe(true)
        expect(link.cost).toBeCloseTo(distance(mesh.nodes[i], mesh.nodes[link.to]), 6)
      }
    }
  })
})
