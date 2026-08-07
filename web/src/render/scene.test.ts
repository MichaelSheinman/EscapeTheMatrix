import { describe, expect, it } from 'vitest'
import { Container, Texture, TextureSource } from 'pixi.js'

import exported from '../../public/locations/east-side-street/east-side-street.tmj?raw'
import { initialWorld, type Standing, type World } from '../world/state'
import { contains } from './level/geometry'
import { readMap } from './level/tiled'
import { createScene, type Scene } from './scene'
import type { Tokens } from './tokens'

/*
 * The painted world, driven without a GPU.
 *
 * Pixi's display objects are plain objects until something renders them, so the
 * scene can be built, mounted and stepped in a test — and what is asserted here
 * is the part that would otherwise only be visible as somebody watching a
 * character walk through a van:
 *
 *   - **Depth is the baseline.** Every prop's `zIndex` is the y at which it
 *     meets the ground, and the character's is their feet. Behind the van means
 *     a lower number than the van's; in front means a higher one.
 *   - **The character never leaves the floor.** Asserted on every frame of
 *     every walk below, not at the end of one.
 *   - **Trigger zones are watched and reported, and nothing else happens.**
 *
 * The level is the one that ships, so a street edited into a state the renderer
 * cannot walk fails here rather than in a browser.
 */

const level = readMap(
  JSON.parse(exported),
  '/locations/east-side-street/east-side-street.tmj',
)

/** A plate of the declared size, with nothing on it. Nothing here draws. */
const plate = new Texture({
  source: new TextureSource({ width: 1920, height: 1088, resolution: 1 }),
})

const tokens: Tokens = {
  color: {
    sky: 0x000000,
    ground: 0x111111,
    horizon: 0x222222,
    paper: 0x333333,
    accent: 0x444444,
    info: 0x555555,
    text: 0x666666,
    muted: 0x777777,
  },
  font: 'sans-serif',
}

const signedIn: World = {
  ...initialWorld,
  session: {
    status: 'in',
    player: { id: 'p', name: 'A player', email: 'a@b.c', avatarUrl: '' },
  },
}

/**
 * A mounted street, sized so that one canvas pixel is one plate pixel.
 *
 * At 1920×1088 the camera's scale is exactly 1 and its clamp puts it dead
 * centre, so a screen coordinate and a plate coordinate are the same number and
 * the assertions below can be written in the coordinates the map was authored
 * in.
 */
function street() {
  const said: (Standing | null)[] = []
  const scene = createScene(tokens, { onStanding: (s) => said.push(s) })
  scene.resize(1920, 1088)
  scene.drawScene({ painted: true, level, plate })
  return { scene, said, view: scene.view }
}

function find(view: Container, label: string): Container {
  const found = view.getChildByLabel(label, true)
  if (!found) throw new Error(`no ${label} in the scene`)
  return found
}

/** Step the scene, asserting the character stays on the floor the whole way. */
function run(scene: Scene, walker: Container, ms: number) {
  for (let elapsed = 0; elapsed < ms; elapsed += 16) {
    scene.update(signedIn, 16)
    expect(
      contains(level.region, { x: walker.x, y: walker.y }),
      `left the floor at ${walker.x.toFixed(0)},${walker.y.toFixed(0)}`,
    ).toBe(true)
  }
}

describe('a mounted location', () => {
  it('cuts each authored prop out of the plate and sorts it by its baseline', () => {
    const { view } = street()

    for (const occluder of level.occluders) {
      const prop = find(view, occluder.name)
      expect(prop.zIndex).toBe(occluder.baseline)
      expect(prop.x).toBe(occluder.frame.x)
      expect(prop.y).toBe(occluder.frame.y)
      expect(prop.width).toBe(occluder.frame.width)
    }
  })

  it('puts the character where the map said, and on the floor', () => {
    const { view } = street()
    const walker = find(view, 'the character')

    expect({ x: walker.x, y: walker.y }).toEqual(level.spawn)
    expect(contains(level.region, { x: walker.x, y: walker.y })).toBe(true)
    // Their feet are their depth.
    expect(walker.zIndex).toBe(walker.y)
  })

  it('draws nobody until somebody has signed in', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    scene.update(initialWorld, 16)
    expect(walker.visible).toBe(false)

    scene.update(signedIn, 16)
    expect(walker.visible).toBe(true)
  })
})

describe('walking', () => {
  it('arrives where it was sent, without leaving the floor on the way', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    scene.walkTo({ x: 1400, y: 540 })
    run(scene, walker, 12000)

    expect(walker.x).toBeCloseTo(1400, 1)
    expect(walker.y).toBeCloseTo(540, 1)
  })

  it('goes to the nearest floor when the tap was not on any', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    // Up in the shopfronts, which are painted scenery rather than pavement.
    scene.walkTo({ x: 1000, y: 90 })
    run(scene, walker, 12000)

    expect(contains(level.region, { x: walker.x, y: walker.y })).toBe(true)
    expect(walker.y).toBeGreaterThan(300)
  })

  it('does not walk into a prop that is standing on the floor', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    // The middle of the van, which is a hole in the walkable polygon.
    scene.walkTo({ x: 1380, y: 640 })
    run(scene, walker, 12000)

    expect(contains(level.region, { x: walker.x, y: walker.y })).toBe(true)
  })

  it('has nowhere to go when no location is drawn', () => {
    const { scene, view, said } = street()
    const walker = find(view, 'the character')
    const before = { x: walker.x, y: walker.y }

    scene.drawScene({ painted: false, name: 'Downtown', note: 'not painted yet' })
    scene.walkTo({ x: 400, y: 400 })
    scene.update(signedIn, 16)

    expect({ x: walker.x, y: walker.y }).toEqual(before)
    expect(walker.visible).toBe(false)
    expect(said[said.length - 1]).toBeNull()
  })
})

describe('depth under a fixed camera', () => {
  it('puts the character behind a prop they are standing further away than', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')
    const van = find(view, 'the van')

    // On the road, north of the van's wheels.
    scene.walkTo({ x: 1400, y: 540 })
    run(scene, walker, 12000)

    expect(walker.zIndex).toBeLessThan(van.zIndex)
  })

  it('puts them in front of it when they are nearer than it is', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')
    const van = find(view, 'the van')

    scene.walkTo({ x: 1400, y: 820 })
    run(scene, walker, 12000)

    expect(walker.y).toBeCloseTo(820, 1)
    expect(walker.zIndex).toBeGreaterThan(van.zIndex)
  })
})

describe('the trigger zones', () => {
  it('are reported on arrival, most specific first', () => {
    const { said } = street()

    expect(said).toHaveLength(1)
    expect(said[0]?.level).toBe('east-side-street')
    expect(said[0]?.zones.map((z) => z.key)).toEqual(['on-foot'])
  })

  it('are reported again when a walk crosses into one', () => {
    const { scene, view, said } = street()
    const walker = find(view, 'the character')

    scene.walkTo({ x: 1000, y: 800 })
    run(scene, walker, 12000)

    expect(said[said.length - 1]?.zones.map((z) => z.key)).toEqual(['transit-stop', 'on-foot'])
  })

  it('are said once, not once a frame', () => {
    const { scene, view, said } = street()
    const walker = find(view, 'the character')

    scene.walkTo({ x: 1000, y: 800 })
    run(scene, walker, 12000)
    const settled = said.length

    run(scene, walker, 2000)
    expect(said).toHaveLength(settled)
  })

  it('carry the ways out of the spot they cover', () => {
    const { scene, view, said } = street()
    const walker = find(view, 'the character')

    scene.walkTo({ x: 1760, y: 800 })
    run(scene, walker, 12000)

    expect(said[said.length - 1]?.portals.map((p) => p.to)).toEqual(['downtown'])
  })
})

describe('the layer that is never drawn', () => {
  it('is off until it is asked for, and draws nothing when it is off', () => {
    const { scene, view } = street()
    const drawn = view.getChildAt(0) as Container

    const logicLayer = drawn.children[drawn.children.length - 1]
    expect(logicLayer.visible).toBe(false)

    scene.showLogic(true)
    expect(logicLayer.visible).toBe(true)

    scene.showLogic(false)
    expect(logicLayer.visible).toBe(false)
  })
})
