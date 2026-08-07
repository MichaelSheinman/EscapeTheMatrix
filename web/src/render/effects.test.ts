import { describe, expect, it } from 'vitest'
import { Container } from 'pixi.js'

import flat from '../../public/locations/east-side-apartment/east-side-apartment.tmj?raw'
import exported from '../../public/locations/east-side-street/east-side-street.tmj?raw'
import { readMap } from './level/tiled'
import {
  createEffects,
  driftMotes,
  hash,
  litBy,
  mixColour,
  MOTES,
  radialFalloff,
  seedMotes,
  waver,
  type Mote,
} from './effects'
import type { Tokens } from './tokens'

/*
 * The High tier, built and taken apart without a GPU.
 *
 * Pixi's display objects are plain objects until something renders them, and
 * the two textures this tier draws with are generated from a `Uint8Array` rather
 * than loaded from a file — so the whole of §10.4's "filter stack, real-time
 * lighting, particles, post-processing" can be constructed here and asked what
 * it contains.
 */

const street = readMap(JSON.parse(exported), '/locations/east-side-street/east-side-street.tmj')
const apartment = readMap(
  JSON.parse(flat),
  '/locations/east-side-apartment/east-side-apartment.tmj',
)

const tokens: Tokens = {
  color: {
    sky: 0x000000,
    ground: 0x111111,
    horizon: 0x222222,
    paper: 0x333333,
    accent: 0xb26a22,
    info: 0x3a6ba6,
    text: 0x2f2820,
    muted: 0x777777,
  },
  font: 'sans-serif',
}

function built() {
  const slots = { glow: new Container(), air: new Container(), edge: new Container() }
  return { slots, effects: createEffects(tokens, slots) }
}

describe('the high tier, as an object that either exists or does not', () => {
  it('is a full-screen stack, a light layer, an air layer and a vignette', () => {
    const { slots, effects } = built()

    // Post-processing: a grade and a grain, both full-screen passes — which is
    // the cost §10.4 says the tier exists to make optional.
    expect(effects.stack).toHaveLength(2)
    // The filter stack over the lights, which is the bloom.
    expect(slots.glow.children[0].filters).toHaveLength(1)
    expect(slots.air.children).toHaveLength(1)
    expect(slots.edge.children).toHaveLength(1)

    effects.destroy()
  })

  it('draws a sprite for every light the location authored', () => {
    const { slots, effects } = built()
    effects.show(street)

    const pools = slots.glow.children[0] as Container
    expect(pools.children.map((child) => child.label)).toEqual(
      street.lights.map((light) => light.name),
    )
    // Additive, because light adds. Anything else would be paint.
    for (const pool of pools.children) expect(pool.blendMode).toBe('add')

    effects.destroy()
  })

  it('gives a light with no colour of its own the world’s warm', () => {
    const { slots, effects } = built()
    effects.show(street)

    const pools = slots.glow.children[0] as Container
    const pool = pools.children.find((child) => child.label === 'the lamp pool')
    // The palette lives in `@etm/ui` and is resolved once, here, rather than
    // being written into a map file by hand.
    expect(pool?.tint).toBe(tokens.color.accent)

    effects.destroy()
  })

  it('fills the location’s air, and empties it again when it leaves', () => {
    const { effects } = built()
    effects.show(street)
    effects.update(16, street.spawn)

    effects.show(null)
    // A room's air does not follow you outdoors.
    expect(effects.update(16, street.spawn)).toBe(0xffffff)

    effects.destroy()
  })

  it('tints the character with whatever they are standing under', () => {
    const { effects } = built()
    effects.show(street)

    const pool = street.lights.find((light) => light.name === 'the lamp pool')
    if (!pool) throw new Error('the street lost its lamp')

    // Standing in the middle of the pool of sodium.
    const lit = effects.update(16, pool.at)
    expect(lit).not.toBe(0xffffff)
    // Warmer than it is cold, because the lamp is.
    expect((lit >> 16) & 0xff).toBeGreaterThan(lit & 0xff)

    // Well out of it, and back to being drawn as they were painted.
    expect(effects.update(16, { x: 20, y: 1000 })).toBe(0xffffff)

    effects.destroy()
  })

  it('is a different room’s air in a different room', () => {
    const { effects } = built()
    effects.show(street)
    const outdoors = seedMotes(MOTES, street.extent, hash(street.key))
    const indoors = seedMotes(MOTES, apartment.extent, hash(apartment.key))
    expect(outdoors[0]).not.toEqual(indoors[0])
    effects.destroy()
  })

  it('takes itself apart, so that going back to plain leaves nothing behind', () => {
    const { slots, effects } = built()
    effects.show(street)
    effects.destroy()

    expect(slots.glow.children).toHaveLength(0)
    expect(slots.air.children).toHaveLength(0)
    expect(slots.edge.children).toHaveLength(0)
    expect(effects.stack).toHaveLength(0)
  })
})

/* ── the arithmetic underneath it ───────────────────────────────────────── */

describe('the falloff the whole tier is drawn with', () => {
  it('is opaque in the middle and gone at the edge', () => {
    const size = 16
    const glow = radialFalloff(size, 'glow')
    const middle = ((size / 2) * size + size / 2) * 4
    expect(glow[middle + 3]).toBeGreaterThan(240)
    // The corner is the furthest point from the centre.
    expect(glow[3]).toBe(0)
    // White throughout: the colour is the sprite's tint, not the texture's.
    expect(glow[0]).toBe(255)
  })

  it('is the other way round for the vignette', () => {
    const size = 16
    const edge = radialFalloff(size, 'edge')
    const middle = ((size / 2) * size + size / 2) * 4
    expect(edge[middle + 3]).toBe(0)
    expect(edge[3]).toBeGreaterThan(240)
  })
})

describe('the air', () => {
  it('is the same air every time you walk into the same room', () => {
    expect(seedMotes(20, street.extent, hash('a-room'))).toEqual(
      seedMotes(20, street.extent, hash('a-room')),
    )
  })

  it('drifts, and comes back in the other side rather than running out', () => {
    const extent = { x: 0, y: 0, width: 100, height: 100 }
    const motes: Mote[] = [{ x: 99, y: 1, vx: 20, vy: -20, size: 4, alpha: 0.1 }]

    driftMotes(motes, 1000, extent)

    expect(motes[0].x).toBeCloseTo(19, 5)
    expect(motes[0].y).toBeCloseTo(81, 5)
    expect(motes).toHaveLength(1)
  })

  it('stays put when there is no location to drift across', () => {
    const motes: Mote[] = [{ x: 5, y: 5, vx: 20, vy: 20, size: 4, alpha: 0.1 }]
    driftMotes(motes, 1000, { x: 0, y: 0, width: 0, height: 0 })
    expect(motes[0]).toEqual({ x: 5, y: 5, vx: 20, vy: 20, size: 4, alpha: 0.1 })
  })
})

describe('how much light falls on a spot', () => {
  const lamp = {
    light: {
      name: 'a lamp',
      at: { x: 100, y: 100 },
      reach: { x: 100, y: 40 },
      color: null,
      intensity: 0.5,
      flicker: 0,
    },
    sprite: { tint: 0xb26a22 },
  }

  it('is most in the middle and none at the edge', () => {
    expect(litBy([lamp], { x: 100, y: 100 }).amount).toBeCloseTo(0.5, 5)
    expect(litBy([lamp], { x: 200, y: 100 }).amount).toBe(0)
    expect(litBy([lamp], { x: 150, y: 100 }).amount).toBeCloseTo(0.25, 5)
  })

  it('is elliptical, because a pool on the ground is', () => {
    // Ninety pixels sideways is still inside; ninety up the picture is not.
    expect(litBy([lamp], { x: 190, y: 100 }).amount).toBeGreaterThan(0)
    expect(litBy([lamp], { x: 100, y: 190 }).amount).toBe(0)
  })

  it('is the brightest of them where two lights overlap', () => {
    const dim = {
      light: { ...lamp.light, name: 'a window', intensity: 0.1 },
      sprite: { tint: 0x3a6ba6 },
    }
    expect(litBy([dim, lamp], { x: 100, y: 100 }).color).toBe(0xb26a22)
  })
})

describe('the small arithmetic', () => {
  it('mixes two colours, and clamps rather than overshooting', () => {
    expect(mixColour(0x000000, 0xffffff, 0.5)).toBe(0x808080)
    expect(mixColour(0x000000, 0xffffff, 0)).toBe(0x000000)
    expect(mixColour(0x000000, 0xffffff, 4)).toBe(0xffffff)
    expect(mixColour(0x000000, 0xffffff, -1)).toBe(0x000000)
  })

  it('wavers between −1 and 1, and is the same waver at the same moment', () => {
    for (let ms = 0; ms < 4000; ms += 37) {
      expect(Math.abs(waver(ms, 3))).toBeLessThanOrEqual(1)
    }
    expect(waver(1234, 5)).toBe(waver(1234, 5))
    expect(waver(1234, 5)).not.toBe(waver(1234, 6))
  })
})
