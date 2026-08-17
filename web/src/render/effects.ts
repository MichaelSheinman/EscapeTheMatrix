/*
 * The High tier.
 *
 * technical.md §10.4 gives it four contents — "full filter stack, real-time
 * lighting, particles, post-processing" — and this file is all four and nothing
 * else. That separation is the ticket's real requirement: Low has to look
 * right on its own, because "retrofitting *works without the lighting pass*
 * into a look designed around the lighting pass means redoing the look". So the
 * Low tier is `scene.ts` with this file never constructed, and the two tiers
 * cannot drift, because there is no branch inside the drawing — there is a
 * whole object that either exists or does not.
 *
 * The four, and where each one is:
 *
 *   - Real-time lighting. The lamps a location authored (§10.3's data
 *     layer, `light` objects) become additive pools over the floor, wavering
 *     where the author said they waver — and the character is tinted by the
 *     light they are standing in, every frame. That is the part the plate
 *     cannot do: a painter can paint the pool of sodium on the pavement, but
 *     not the person walking through it.
 *   - The filter stack. A blur over the light layer, which is the bloom.
 *   - Post-processing. A warm grade and a film grain over the finished
 *     world, and a vignette over that, in screen space.
 *   - Particles. Motes in the air, drifting, seeded from the location's own
 *     key so a room's air is the same air every time you walk into it.
 *
 * Everything expensive here is drawn with two procedurally generated
 * textures — a soft dot and its inverse — built from a `Uint8Array` rather
 * than from a canvas or a file. Three reasons, and the third is the one that
 * mattered: it needs no art (#34–#39 are elsewhere), it needs no network, and
 * it works in Node, so `effects.test.ts` can build the whole High tier and
 * assert what it contains without a GPU.
 */

import {
  BlurFilter,
  BufferImageSource,
  ColorMatrixFilter,
  Container,
  NoiseFilter,
  Particle,
  ParticleContainer,
  Sprite,
  Texture,
  type Filter,
} from 'pixi.js'

import type { Point, Rect } from './level/geometry'
import type { Level, Light } from './level/tiled'
import type { Tokens } from './tokens'

/**
 * Where the scene keeps room for the High tier.
 *
 * Three empty containers the scene owns and always has, in the z-order the
 * effects need: the light layer over the floor, the air over that, and the
 * vignette in screen space above the camera. Empty containers cost nothing to
 * draw, and the alternative — inserting display objects at computed indices in
 * a list whose order is otherwise fixed — is the kind of arithmetic that breaks
 * the day somebody adds a layer.
 */
export type Slots = { glow: Container; air: Container; edge: Container }

export type Effects = {
  /**
   * The full-screen stack, for the scene to hang on its world container.
   *
   * Handed over rather than applied, because what the filters apply *to* is the
   * scene's business — filtering the camera and not the notice over it is the
   * difference between a graded world and unreadable text.
   */
  readonly stack: Filter[]
  /** Show a location's authored lights, or none at all. */
  show(level: Level | null): void
  resize(width: number, height: number): void
  /**
   * One frame.
   *
   * Returns the tint the character should be drawn with: the lighting, applied
   * to the one thing in the scene that moves through it. White when they are
   * standing in nothing.
   */
  update(elapsedMS: number, at: Point): number
  destroy(): void
}

/** How many motes are in the air of a location. */
export const MOTES = 120

/** Plate pixels a second a mote drifts, at most. */
const DRIFT = 14

/** How far into the frame the vignette starts, as a fraction of the half-width. */
const VIGNETTE = 0.55

/** The most the light may pull the character's colour away from white. */
const LIT = 0.55

/** How much grain, and how much it is allowed to look like grain. */
const GRAIN = 0.055

export function createEffects(tokens: Tokens, slots: Slots): Effects {
  const { color } = tokens

  // Two textures, generated once and shared by every light, every mote and the
  // vignette. 128px is plenty: these are soft by definition, and GPU bilinear
  // filtering does the rest however far they are stretched.
  const glow = falloffTexture(128, 'glow')
  const dark = falloffTexture(128, 'edge')

  /* ── the light layer ─────────────────────────────────────────────────── */

  const pools = new Container()
  // The bloom. Blurring the additive layer — and only the additive layer — is
  // what makes a lamp bleed into the air around it rather than smearing the
  // painting it is standing in.
  //
  // `blendMode` is on the filter, not on the container, and that is not a
  // detail: a filtered container is drawn into a texture of its own first, so
  // the additive blending of the pool sprites happens *inside* that texture —
  // which is right, two overlapping lamps are brighter — and how the finished
  // texture meets the painting is the filter's own blend. Without this, the
  // whole light layer would be painted over the world instead of added to it.
  pools.filters = [new BlurFilter({ strength: 14, quality: 2, blendMode: 'add' })]
  slots.glow.addChild(pools)

  /** The authored lights on screen, and the sprite drawn for each. */
  let lit: { light: Light; sprite: Sprite }[] = []

  /* ── the air ─────────────────────────────────────────────────────────── */

  const air = new ParticleContainer({
    texture: glow,
    dynamicProperties: { position: true, color: true },
  })
  slots.air.addChild(air)

  let motes: Mote[] = []
  let flecks: Particle[] = []
  let extent: Rect = { x: 0, y: 0, width: 0, height: 0 }

  /* ── the vignette ────────────────────────────────────────────────────── */

  const edge = new Sprite(dark)
  edge.anchor.set(0.5)
  edge.tint = color.text
  edge.alpha = 0.42
  slots.edge.addChild(edge)

  /* ── the stack ───────────────────────────────────────────────────────── */

  // A warm grade and a grain. Both are full-screen passes, which is exactly the
  // cost §10.4 says the tier exists to make optional.
  const grade = new ColorMatrixFilter()
  grade.saturate(0.06, true)
  grade.contrast(0.06, true)
  grade.tint(0xfff1de, true)

  const grain = new NoiseFilter({ noise: GRAIN, seed: 0 })

  const stack: Filter[] = [grade, grain]

  let clock = 0

  return {
    stack,

    show(level) {
      for (const gone of pools.removeChildren()) gone.destroy()
      lit = []

      air.removeParticles()
      motes = []
      flecks = []

      if (!level) return

      extent = level.extent

      for (const light of level.lights) {
        const sprite = new Sprite(glow)
        sprite.label = light.name
        sprite.anchor.set(0.5)
        sprite.position.set(light.at.x, light.at.y)
        sprite.width = light.reach.x * 2
        sprite.height = light.reach.y * 2
        // Null means the author did not say, and the world's own warm is the
        // answer — the art bible's sodium amber, read off `@etm/ui` once.
        sprite.tint = light.color ?? color.accent
        sprite.alpha = light.intensity
        sprite.blendMode = 'add'
        pools.addChild(sprite)
        lit.push({ light, sprite })
      }

      // Seeded from the location's own key, so a room's air is the same air
      // every time you walk into it. Determinism is cheap here and it is what
      // stops a reload from being visibly a reload.
      motes = seedMotes(MOTES, extent, hash(level.key))
      flecks = motes.map(
        (mote) =>
          new Particle({
            texture: glow,
            x: mote.x,
            y: mote.y,
            anchorX: 0.5,
            anchorY: 0.5,
            scaleX: mote.size / 128,
            scaleY: mote.size / 128,
            tint: color.paper,
            alpha: mote.alpha,
          }),
      )
      for (const fleck of flecks) air.addParticle(fleck)
    },

    resize(width, height) {
      edge.position.set(width / 2, height / 2)
      // Over-sized so that the soft part of the falloff is on screen and the
      // opaque rim of the texture is not.
      edge.width = width * 1.6
      edge.height = height * 1.6
    },

    update(elapsedMS, at) {
      clock += elapsedMS

      // The lamps. A waver rather than a flicker: two sine waves at unrelated
      // periods, so it never repeats on a beat anybody can hear.
      for (const { light, sprite } of lit) {
        sprite.alpha = light.intensity * (1 + light.flicker * waver(clock, light.at.x))
      }

      if (motes.length > 0) {
        driftMotes(motes, elapsedMS, extent)
        for (let i = 0; i < motes.length; i++) {
          flecks[i].x = motes[i].x
          flecks[i].y = motes[i].y
        }
        air.update()
      }

      // The grain has to move or it reads as dirt on the screen. Advanced from
      // the clock rather than from `Math.random`, so a frame is a function of
      // the time it was drawn at.
      grain.seed = (clock % 997) / 997

      const under = litBy(lit, at)
      return under.amount > 0 ? mixColour(0xffffff, under.color, Math.min(under.amount, LIT)) : 0xffffff
    },

    destroy() {
      // Everything this built, unbuilt. The Low tier holds nothing, and "holds
      // nothing" has to survive a player switching back and forth.
      for (const gone of pools.removeChildren()) gone.destroy()
      pools.filters = []
      pools.destroy({ children: true })
      air.destroy({ children: true })
      edge.destroy()
      glow.destroy(true)
      dark.destroy(true)
      lit = []
      motes = []
      flecks = []
      stack.length = 0
    },
  }
}

/* ── the lighting, as arithmetic ────────────────────────────────────────── */

/** How much light falls on a point, and what colour it is. */
export function litBy(
  lights: readonly { light: Light; sprite: { tint: number } }[],
  at: Point,
): { color: number; amount: number } {
  let best = { color: 0xffffff, amount: 0 }
  for (const { light, sprite } of lights) {
    // Elliptical, because the pool is: a light twice as wide as it is tall
    // reaches twice as far sideways as it does up the picture.
    const dx = light.reach.x > 0 ? (at.x - light.at.x) / light.reach.x : Infinity
    const dy = light.reach.y > 0 ? (at.y - light.at.y) / light.reach.y : Infinity
    const away = Math.hypot(dx, dy)
    if (away >= 1) continue
    const amount = (1 - away) * light.intensity
    if (amount > best.amount) best = { color: sprite.tint, amount }
  }
  return best
}

/** Two colours, mixed. `t` of 0 is all of the first, 1 is all of the second. */
export function mixColour(from: number, to: number, t: number): number {
  const k = Math.min(1, Math.max(0, t))
  const blend = (shift: number) => {
    const a = (from >> shift) & 0xff
    const b = (to >> shift) & 0xff
    return Math.round(a + (b - a) * k) & 0xff
  }
  return (blend(16) << 16) | (blend(8) << 8) | blend(0)
}

/** A light's waver at a moment. Between -1 and 1, and deterministic. */
export function waver(clockMS: number, phase: number): number {
  const t = clockMS / 1000
  return (Math.sin(t * 5.3 + phase) + Math.sin(t * 11.7 + phase * 0.37)) / 2
}

/* ── the air, as arithmetic ─────────────────────────────────────────────── */

/** One thing in the air. Plate pixels, like everything else in the world. */
export type Mote = {
  x: number
  y: number
  /** Plate pixels a second. */
  vx: number
  vy: number
  /** Across the texture, in plate pixels. */
  size: number
  alpha: number
}

/**
 * Fill a location's air, the same way every time.
 *
 * Seeded rather than random: the same location produces the same motes, so a
 * reload is not visibly a reload, and a test can assert where they went.
 */
export function seedMotes(count: number, extent: Rect, seed: number): Mote[] {
  const random = mulberry(seed)
  const motes: Mote[] = []
  for (let i = 0; i < count; i++) {
    motes.push({
      x: extent.x + random() * extent.width,
      y: extent.y + random() * extent.height,
      // Mostly sideways and slightly up, which is what air in a room does when
      // something in the room is warm.
      vx: (random() - 0.35) * DRIFT,
      vy: -random() * DRIFT * 0.45,
      size: 3 + random() * 7,
      alpha: 0.05 + random() * 0.14,
    })
  }
  return motes
}

/**
 * Advance the air by one frame.
 *
 * Wrapped rather than respawned: a mote that leaves the top of the painting
 * comes back in at the bottom, so the count is constant and there is no moment
 * at which a room has less air in it than it started with.
 */
export function driftMotes(motes: Mote[], elapsedMS: number, extent: Rect): void {
  if (extent.width <= 0 || extent.height <= 0) return
  const seconds = elapsedMS / 1000
  for (const mote of motes) {
    mote.x = wrap(mote.x + mote.vx * seconds, extent.x, extent.width)
    mote.y = wrap(mote.y + mote.vy * seconds, extent.y, extent.height)
  }
}

function wrap(value: number, from: number, span: number): number {
  const offset = (value - from) % span
  return from + (offset < 0 ? offset + span : offset)
}

/* ── the two textures ───────────────────────────────────────────────────── */

/**
 * A square of white with a soft alpha falloff, as raw RGBA.
 *
 * `glow` is opaque in the middle and gone at the edge — a lamp, a mote. `edge`
 * is the inverse, and is the vignette: nothing in the middle, closing in
 * towards the corners.
 *
 * Generated rather than shipped, because it is a formula and shipping a picture
 * of a formula is how a build ends up with an asset nobody can regenerate. Pure,
 * and returned as bytes, so the shape can be asserted without a renderer.
 */
export function radialFalloff(size: number, kind: 'glow' | 'edge'): Uint8Array {
  const pixels = new Uint8Array(size * size * 4)
  const half = size / 2
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const away = Math.hypot((x + 0.5 - half) / half, (y + 0.5 - half) / half)
      const alpha =
        kind === 'glow'
          ? smooth(1 - Math.min(1, away))
          : smooth(Math.min(1, Math.max(0, (away - VIGNETTE) / (1 - VIGNETTE))))
      const at = (y * size + x) * 4
      pixels[at] = 255
      pixels[at + 1] = 255
      pixels[at + 2] = 255
      pixels[at + 3] = Math.round(alpha * 255)
    }
  }
  return pixels
}

/** Fast in, fast out, soft at both ends — the shape a falloff should have. */
function smooth(t: number): number {
  return t * t * (3 - 2 * t)
}

function falloffTexture(size: number, kind: 'glow' | 'edge'): Texture {
  return new Texture({
    source: new BufferImageSource({
      resource: radialFalloff(size, kind),
      width: size,
      height: size,
      // Deliberately not the default: these are stretched to hundreds of pixels
      // across, and a nearest-neighbour lamp is a mosaic.
      scaleMode: 'linear',
    }),
  })
}

/* ── seeded randomness ──────────────────────────────────────────────────── */

/** A string, as a number a generator can be started from. */
export function hash(text: string): number {
  let h = 2166136261
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** mulberry32: small, fast, and good enough for where a speck of dust is. */
function mulberry(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}
