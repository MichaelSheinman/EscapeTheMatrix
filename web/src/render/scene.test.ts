import { describe, expect, it } from 'vitest'
import { Container, Texture, TextureSource } from 'pixi.js'

import flat from '../../public/locations/east-side-apartment/east-side-apartment.tmj?raw'
import exported from '../../public/locations/east-side-street/east-side-street.tmj?raw'
import { initialWorld, type Standing, type World } from '../world/state'
import { contains } from './level/geometry'
import { readMap, type Portal } from './level/tiled'
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

/** The other location that ships: the flat the street's stoop leads into. */
const apartment = readMap(
  JSON.parse(flat),
  '/locations/east-side-apartment/east-side-apartment.tmj',
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
  const took: Portal[] = []
  const scene = createScene(tokens, {
    onStanding: (s) => said.push(s),
    onPortal: (p) => took.push(p),
  })
  scene.resize(1920, 1088)
  scene.drawScene({ painted: true, level, plate })
  return { scene, said, took, view: scene.view }
}

function find(view: Container, label: string): Container {
  const found = view.getChildByLabel(label, true)
  if (!found) throw new Error(`no ${label} in the scene`)
  return found
}

/** Step the scene, asserting the character stays on the floor the whole way. */
function run(scene: Scene, walker: Container, ms: number, on = level) {
  for (let elapsed = 0; elapsed < ms; elapsed += 16) {
    scene.update(signedIn, 16)
    expect(
      contains(on.region, { x: walker.x, y: walker.y }),
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

/*
 * Portals, which are where a way out was drawn (§3.12, §10.3).
 *
 * The scene decides nothing about a journey — whether it is possible, what it
 * costs, and what is on the way are the city's. What it does decide is when
 * somebody walked onto a door, and that is what these assert. The rule that
 * needs the most protection is the one about arriving: coming in through a door
 * and immediately being sent back out of it is a loop rather than a transition.
 */
describe('walking onto a way out', () => {
  it('says so, once, when the walk ends on one', () => {
    const { scene, view, took } = street()
    const walker = find(view, 'the character')

    // The far end of the street, where the way to Downtown is drawn.
    scene.walkTo({ x: 1760, y: 800 })
    run(scene, walker, 12000)

    expect(took.map((p) => p.to)).toEqual(['downtown'])

    // Standing on it is not walking onto it again.
    run(scene, walker, 4000)
    expect(took).toHaveLength(1)
  })

  it('says nothing for a walk that ends anywhere else', () => {
    const { scene, view, took } = street()
    const walker = find(view, 'the character')

    scene.walkTo({ x: 1000, y: 800 })
    run(scene, walker, 12000)

    expect(took).toEqual([])
  })

  it('says so again after stepping off it and back on', () => {
    const { scene, view, took } = street()
    const walker = find(view, 'the character')

    scene.walkTo({ x: 1760, y: 800 })
    run(scene, walker, 12000)
    scene.walkTo({ x: 1000, y: 800 })
    run(scene, walker, 12000)
    scene.walkTo({ x: 1760, y: 800 })
    run(scene, walker, 12000)

    expect(took.map((p) => p.to)).toEqual(['downtown', 'downtown'])
  })

  it('puts an arrival at the door it came in by, and does not send it back out', () => {
    const { scene, view, took } = street()
    const walker = find(view, 'the character')

    // Coming into the flat from the street, the way the renderer mounts it.
    scene.drawScene({ painted: true, level: apartment, plate }, 'east-side')

    const door = apartment.portals.find((p) => p.to === 'east-side')
    if (!door) throw new Error('the flat has no way back to the street')

    expect(walker.x).toBeCloseTo(door.area.bounds.x + door.area.bounds.width / 2, 0)
    expect(contains(apartment.region, { x: walker.x, y: walker.y })).toBe(true)

    // Standing in the doorway is not walking through it.
    run(scene, walker, 3000, apartment)
    expect(took).toEqual([])

    // Walking away from it and back is.
    scene.walkTo({ x: 1000, y: 900 })
    run(scene, walker, 12000, apartment)
    scene.walkTo({ x: door.area.bounds.x + 40, y: door.area.bounds.y + 40 })
    run(scene, walker, 12000, apartment)
    expect(took.map((p) => p.to)).toEqual(['east-side'])
  })

  it('arrives at the map spawn when it came from nowhere in particular', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    scene.drawScene({ painted: true, level: apartment, plate }, 'the-strip')

    expect({ x: walker.x, y: walker.y }).toEqual(apartment.spawn)
  })
})

/*
 * §10.1: **the canvas never unmounts.** Navigating between locations swaps what
 * is drawn inside one scene rather than replacing the scene — the display object
 * the renderer put on its stage is the same object before and after, and the
 * character is the same sprite, which is what makes the WebGL context and the
 * texture cache survive going somewhere.
 */
describe('navigating between locations', () => {
  it('swaps what is drawn without replacing the scene or the character', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    scene.drawScene({ painted: true, level: apartment, plate }, 'east-side')
    expect(scene.view).toBe(view)
    expect(find(view, 'the character')).toBe(walker)

    scene.drawScene({ painted: false, name: 'Downtown', note: 'not painted yet' })
    expect(scene.view).toBe(view)
    expect(find(view, 'the character')).toBe(walker)

    scene.drawScene({ painted: true, level, plate }, 'downtown')
    expect(scene.view).toBe(view)
    expect(find(view, 'the character')).toBe(walker)
  })

  it('lets go of the props of the location it left', () => {
    const { scene, view } = street()

    for (const occluder of level.occluders) find(view, occluder.name)

    scene.drawScene({ painted: true, level: apartment, plate }, 'east-side')

    // The street's van is gone; the flat's own props are there instead.
    expect(view.getChildByLabel('the van', true)).toBeNull()
    for (const occluder of apartment.occluders) find(view, occluder.name)
  })
})

/*
 * §10.4's two tiers, switchable at runtime and correct from the first scene.
 *
 * The Low tier's promise is the one worth protecting, because it is the one a
 * refactor could quietly break: **no full-screen filters and no particles**. It
 * is asserted here as a property of the scene rather than as a claim about the
 * code — nothing is on the camera, and the two layers the High tier draws into
 * are empty — and it is asserted again *after* a round trip through High, since
 * a tier you can switch to and not properly out of is not two tiers.
 */
describe('the effects tier', () => {
  /** Where the High tier draws, in the order the scene keeps them. */
  function layers(scene: Scene) {
    const camera = scene.view.getChildAt(0) as Container
    return {
      camera,
      glow: camera.children[4] as Container,
      air: camera.children[5] as Container,
      edge: scene.view.getChildAt(1) as Container,
    }
  }

  it('starts plain, with nothing over the painting', () => {
    const { scene } = street()
    const { camera, glow, air, edge } = layers(scene)

    expect(camera.filters ?? []).toHaveLength(0)
    expect(glow.children).toHaveLength(0)
    expect(air.children).toHaveLength(0)
    expect(edge.children).toHaveLength(0)
  })

  it('adds the stack, the light and the air when it is asked for', () => {
    const { scene } = street()
    scene.setQuality('high')
    const { camera, glow, air, edge } = layers(scene)

    expect(camera.filters).toHaveLength(2)
    expect(glow.children).toHaveLength(1)
    expect(air.children).toHaveLength(1)
    expect(edge.children).toHaveLength(1)

    // The lights of the location that was already mounted, not of the next one
    // to be mounted: switching tiers mid-street has to light the street.
    const pools = glow.children[0] as Container
    expect(pools.children).toHaveLength(level.lights.length)
  })

  it('takes all of it away again, and gives the character their colour back', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')
    const body = walker.children[1]

    scene.setQuality('high')
    // Walk into the pool of sodium and take its colour.
    const pool = level.lights.find((light) => light.name === 'the lamp pool')
    if (!pool) throw new Error('the street lost its lamp')
    scene.walkTo({ x: pool.at.x, y: pool.at.y })
    run(scene, walker, 12000)
    expect(body.tint).not.toBe(0xffffff)

    scene.setQuality('low')
    const { camera, glow, air, edge } = layers(scene)
    expect(camera.filters ?? []).toHaveLength(0)
    expect(glow.children).toHaveLength(0)
    expect(air.children).toHaveLength(0)
    expect(edge.children).toHaveLength(0)
    // An amber tint left behind would be the one piece of the High tier that
    // survived being switched off.
    expect(body.tint).toBe(0xffffff)
  })

  it('lights the next location too, and unlights an unpainted one', () => {
    const { scene } = street()
    scene.setQuality('high')

    scene.drawScene({ painted: true, level: apartment, plate }, 'east-side')
    const pools = layers(scene).glow.children[0] as Container
    expect(pools.children).toHaveLength(apartment.lights.length)

    scene.drawScene({ painted: false, name: 'Downtown', note: 'not painted yet' })
    expect((layers(scene).glow.children[0] as Container).children).toHaveLength(0)
  })

  it('is not rebuilt by being asked for the tier it is already at', () => {
    const { scene } = street()
    scene.setQuality('high')
    const before = layers(scene).glow.children[0]

    scene.setQuality('high')
    expect(layers(scene).glow.children[0]).toBe(before)
  })

  it('leaves the canvas and the character alone through every switch', () => {
    const { scene, view } = street()
    const walker = find(view, 'the character')

    scene.setQuality('high')
    scene.setQuality('low')
    scene.setQuality('high')

    // §10.1: the canvas never unmounts, and neither does the person standing
    // on it. A quality switch is not a navigation.
    expect(scene.view).toBe(view)
    expect(find(view, 'the character')).toBe(walker)
  })
})

/*
 * The flat, as it ships. The same standard the street is held to: a location
 * edited into a state the renderer cannot walk fails here rather than in a
 * browser.
 */
describe('the flat, as it ships', () => {
  it('is a home with a way back to the street and somewhere to arrive', () => {
    expect(apartment.key).toBe('east-side-apartment')
    expect(apartment.place).toBe('east-side-apartment')
    expect(apartment.portals.map((p) => p.to)).toEqual(['east-side'])
    expect(contains(apartment.region, apartment.spawn)).toBe(true)
  })

  it('can be walked across without leaving the floor', () => {
    const said: (Standing | null)[] = []
    const scene = createScene(tokens, { onStanding: (s) => said.push(s), onPortal: () => {} })
    scene.resize(1920, 1088)
    scene.drawScene({ painted: true, level: apartment, plate })
    const walker = find(scene.view, 'the character')

    for (const target of [
      { x: 400, y: 900 },
      { x: 1500, y: 900 },
      { x: 900, y: 500 },
      { x: 250, y: 500 },
    ]) {
      scene.walkTo(target)
      run(scene, walker, 12000, apartment)
    }

    expect(said[said.length - 1]?.level).toBe('east-side-apartment')
  })
})
