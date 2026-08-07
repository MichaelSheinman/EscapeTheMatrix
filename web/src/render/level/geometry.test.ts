import { describe, expect, it } from 'vitest'

import {
  area,
  bounds,
  clearLine,
  contains,
  corners,
  inArea,
  insidePolygon,
  nearestInside,
  nearestOnSegment,
  toArea,
  type Region,
} from './geometry'

/*
 * The arithmetic under the invisible layer.
 *
 * Pure by construction — no canvas, no fetch, no Tiled — so what is asserted is
 * the whole behaviour, and the things that would otherwise only be visible as
 * "the character got stuck" or "the character walked through the van" are
 * visible here as a false where a true was wanted.
 */

/** A hundred-pixel room with a twenty-pixel pillar in the middle of it. */
const room: Region = {
  outer: corners({ x: 0, y: 0, width: 100, height: 100 }),
  holes: [corners({ x: 40, y: 40, width: 20, height: 20 })],
}

describe('what is inside an outline', () => {
  it('is decided by the crossing count', () => {
    const square = corners({ x: 0, y: 0, width: 10, height: 10 })
    expect(insidePolygon(square, { x: 5, y: 5 })).toBe(true)
    expect(insidePolygon(square, { x: 15, y: 5 })).toBe(false)
    expect(insidePolygon(square, { x: 5, y: -1 })).toBe(false)
  })

  it('reads a concave outline the same way', () => {
    // An L. The point below is inside the outline's bounding box and outside
    // the outline, which is the case a bounds check alone would get wrong.
    const el = [
      { x: 0, y: 0 },
      { x: 100, y: 0 },
      { x: 100, y: 40 },
      { x: 40, y: 40 },
      { x: 40, y: 100 },
      { x: 0, y: 100 },
    ]
    expect(insidePolygon(el, { x: 20, y: 80 })).toBe(true)
    expect(insidePolygon(el, { x: 80, y: 80 })).toBe(false)
  })
})

describe('the walkable region', () => {
  it('is the outline minus the holes', () => {
    expect(contains(room, { x: 10, y: 10 })).toBe(true)
    expect(contains(room, { x: 50, y: 50 })).toBe(false)
    expect(contains(room, { x: 200, y: 50 })).toBe(false)
  })

  it('leaves a point that is already inside exactly where it is', () => {
    const inside = { x: 12, y: 34 }
    expect(nearestInside(room, inside)).toBe(inside)
  })

  it('pulls a point outside the outline back onto the floor', () => {
    const landed = nearestInside(room, { x: 130, y: 50 })
    expect(contains(room, landed)).toBe(true)
    expect(landed.x).toBeCloseTo(99, 0)
  })

  it('pushes a point inside a hole back out of it', () => {
    const landed = nearestInside(room, { x: 50, y: 44 })
    expect(contains(room, landed)).toBe(true)
    // Out through the near face of the pillar, not through the far one.
    expect(landed.y).toBeLessThan(40)
  })

  it('has no point it cannot answer for', () => {
    for (let x = -20; x <= 120; x += 7) {
      for (let y = -20; y <= 120; y += 7) {
        expect(contains(room, nearestInside(room, { x, y }))).toBe(true)
      }
    }
  })
})

describe('a straight line across the floor', () => {
  it('is clear when nothing is in the way', () => {
    expect(clearLine(room, { x: 5, y: 5 }, { x: 5, y: 95 })).toBe(true)
  })

  it('is not clear when it crosses a hole', () => {
    expect(clearLine(room, { x: 50, y: 5 }, { x: 50, y: 95 })).toBe(false)
    expect(clearLine(room, { x: 5, y: 50 }, { x: 95, y: 50 })).toBe(false)
  })

  it('is not clear when it only passes through a hole’s corners', () => {
    // Corner to corner of the pillar: this segment crosses no edge at an
    // interior point, so a plain crossing test would call it clear. The piece
    // between the two corners is inside the pillar.
    expect(clearLine(room, { x: 0, y: 0 }, { x: 100, y: 100 })).toBe(false)
  })

  it('is not clear when it leaves the outline', () => {
    expect(clearLine(room, { x: 5, y: 5 }, { x: 150, y: 5 })).toBe(false)
  })

  it('is refused when it runs along a wall', () => {
    // Where "inside" is a rounding question, the answer is no — the navmesh
    // insets its corners so nothing ever has to ask.
    expect(clearLine(room, { x: 0, y: 0 }, { x: 100, y: 0 })).toBe(false)
  })
})

describe('authored areas', () => {
  it('carry their own bounds and size', () => {
    const zone = toArea(corners({ x: 10, y: 20, width: 30, height: 40 }))
    expect(zone.size).toBe(1200)
    expect(zone.bounds).toEqual({ x: 10, y: 20, width: 30, height: 40 })
    expect(inArea(zone, { x: 15, y: 25 })).toBe(true)
    expect(inArea(zone, { x: 5, y: 25 })).toBe(false)
  })

  it('measure the same area whichever way round they were drawn', () => {
    const square = corners({ x: 0, y: 0, width: 10, height: 10 })
    expect(area(square)).toBe(100)
    expect(area([...square].reverse())).toBe(100)
  })

  it('know the box they fit in', () => {
    expect(bounds([{ x: 5, y: 9 }, { x: -1, y: 2 }, { x: 3, y: 20 }])).toEqual({
      x: -1,
      y: 2,
      width: 6,
      height: 18,
    })
  })
})

describe('the nearest point on a segment', () => {
  it('is the projection when the projection lands on it', () => {
    expect(nearestOnSegment({ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 4, y: 7 })).toEqual({
      x: 4,
      y: 0,
    })
  })

  it('is an end when the projection does not', () => {
    expect(nearestOnSegment({ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 40, y: 7 })).toEqual({
      x: 10,
      y: 0,
    })
  })
})
