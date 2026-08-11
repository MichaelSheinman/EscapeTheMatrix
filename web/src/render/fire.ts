import { Container, Graphics, Rectangle } from 'pixi.js'

import type { Level } from './level/tiled'
import type { Tier } from './quality'

/*
 * A building on fire, drawn over the plate rather than baked into it.
 *
 * The plates are static (#39: scene data in, `.tmj` and two WebPs out, byte for
 * byte repeatable), which is right for a street — a street is the same street
 * every time you walk down it. A fire is the opposite kind of thing. It is
 * happening *now*, it is over in five minutes, and the whole reason it is worth
 * drawing is that it moves.
 *
 * So this is a runtime layer, and it is the only thing in the renderer that
 * draws something the painting does not contain.
 *
 * # Where it goes, and why the server chooses
 *
 * The offer says `glyph: "fire"` and names a `zone`, and both come from
 * `content/events/a-fire-two-doors-down.yaml`. The client looks that zone up in
 * whatever location is on screen and puts the fire over it. Nothing here knows
 * which building is burning or why — §10.3's rule that trigger zones are the
 * shared vocabulary between the world and the paintings, used for the one thing
 * it is obviously for.
 *
 * That means the same fire draws correctly in the street and in the flat above
 * it without this file knowing there are two, and a location that has never
 * heard of the zone simply does not draw one. A painting missing a spot the
 * world says is there is a ticket, not a crash.
 *
 * # Why it is Graphics and not a particle system
 *
 * Because it has to look like the baked flames in the burning flat, which are
 * Canvas2D quadratics drawn by `preview/src/paint.js`. Two fires in one game
 * that are visibly different techniques would read as a bug. The animation is a
 * phase per tongue rather than a simulation: nothing here integrates, nothing
 * accumulates, and a tab that was backgrounded for a minute resumes looking
 * exactly like a tab that was not.
 */

/** How many tongues, per tier. High is not twice Low because they overlap. */
const TONGUES: Record<Tier, number> = { low: 5, high: 9 }

/**
 * Plate pixels the flames stand, before the zone's own size scales them.
 *
 * Shorter than it looks like it should be. Flames out of a window are wider
 * than they are tall at this distance — a tall narrow flame is a candle, and
 * the first pass drew nine of them and read as a row of spikes.
 */
const HEIGHT = 130

/** How far up the facade the fire sits, from the middle of the ground zone. */
const LIFT = 150

/** How wide the fire is, whatever size rectangle the zone happens to be. */
const WIDTH = { least: 110, most: 240 }

/** How fast a tongue wavers, in radians a second. */
const WAVER = 3.4

export type Fire = {
  readonly view: Container
  /**
   * Put the fire on a zone of the level on screen, or take it off.
   *
   * Called every frame with what the world currently says, so it has to be
   * cheap when nothing has changed — and it is: the same (level, zone) pair
   * twice in a row does nothing at all.
   */
  show(level: Level | null, zone: string | null): void
  setQuality(tier: Tier): void
  /** One frame. Advances the flicker; draws nothing when there is no fire. */
  update(elapsedMS: number): void
  destroy(): void
}

export function createFire(onPress: () => void): Fire {
  const view = new Container()
  view.visible = false

  /*
   * The fire can be pressed, and it is the only part of the painting that can.
   *
   * `static` rather than `dynamic` because it never moves under a still cursor.
   * The hit area is set with the flames each frame — a Graphics object's own
   * bounds would work and would also grow and shrink with the flicker, which is
   * a target that squirms away from the pointer.
   *
   * The tap is stopped here rather than allowed to bubble. The stage's own
   * handler walks the character to wherever you clicked (§10.1), and clicking a
   * burning building to answer it is not also asking to walk into the middle of
   * the road — so this consumes it.
   */
  view.eventMode = 'static'
  view.cursor = 'pointer'
  view.on('pointertap', (event) => {
    event.stopPropagation()
    onPress()
  })

  /*
   * Blend modes per layer, and this is the whole difference between a fire and
   * a smudge of light.
   *
   * The obvious build is one additive container — that is how the lamps work,
   * and it is wrong here. These streets are painted at full midday sun, and
   * additive orange over a bright plate saturates to pale yellow: the first
   * version of this drew what read unmistakably as shafts of sunlight coming
   * out of a doorway.
   *
   * A fire in daylight is legible because of *contrast*, not brightness. So the
   * flame bodies are drawn normally, in saturated colour that is darker than
   * the plate behind them, and only the small hot core is additive. The thing
   * that actually sells it is the smoke: dark, opaque, and the one element that
   * cannot be confused with sunshine.
   */
  const glow = new Graphics()
  glow.blendMode = 'add'

  const tongues = new Graphics()
  const core = new Graphics()
  core.blendMode = 'add'
  const smoke = new Graphics()

  // Smoke last, over the flames — it is coming off them and rising in front of
  // the building above.
  view.addChild(glow, tongues, core, smoke)

  let tier: Tier = 'low'
  let clock = 0
  let at: { x: number; y: number; w: number; h: number } | null = null
  // What is currently drawn, so `show` can do nothing when nothing moved.
  let drawn = ''

  function place(level: Level | null, zone: string | null) {
    const want = level && zone ? `${level.key}|${zone}` : ''
    if (want === drawn) return
    drawn = want

    if (!level || !zone) {
      at = null
      view.visible = false
      return
    }
    const found = level.zones.find((z) => z.key === zone)
    if (!found) {
      // The world says something is happening at a spot this painting has not
      // drawn. Not an error and not a guess: no fire.
      at = null
      view.visible = false
      return
    }

    const box = found.area.bounds
    at = { x: box.x + box.width / 2, y: box.y + box.height / 2, w: box.width, h: box.height }
    // Generous, and steady. The flames flicker and the plume drifts; a hit area
    // that tracked either would be a target that moves out from under the
    // cursor between the press and the release.
    const reach = Math.min(WIDTH.most, Math.max(WIDTH.least, box.width))
    view.hitArea = new Rectangle(-reach, -HEIGHT * 1.4, reach * 2, HEIGHT * 1.9)
    /*
     * Lifted up the face of the building rather than left on the pavement.
     *
     * A trigger zone is a patch of *ground* — `home-door` is where you stand to
     * go in — and a fire drawn there is a bonfire on the doorstep. What is
     * burning is a flat above it. Buildings rise straight up the screen in this
     * projection, so a fixed offset up from the zone puts the fire on the
     * facade, which is what "two doors down" looks like from the street.
     */
    view.position.set(at.x, at.y - LIFT)
    view.visible = true
  }

  return {
    view,
    show: place,
    setQuality(next) {
      tier = next
    },
    update(elapsedMS) {
      if (!at || !view.visible) return
      clock += elapsedMS

      const t = clock / 1000
      const n = TONGUES[tier]
      const width = Math.min(WIDTH.most, Math.max(WIDTH.least, at.w))
      const tall = HEIGHT * (0.7 + Math.min(1, width / 260) * 0.6)

      tongues.clear()
      core.clear()
      for (let i = 0; i < n; i++) {
        // Each tongue has its own phase, so they lean out of step. Derived from
        // the index rather than from a random seed, so the fire looks the same
        // on every machine — §7's determinism, which the renderer keeps too.
        const phase = i * 1.9
        const sway = Math.sin(t * WAVER + phase)
        const pulse = 0.62 + 0.38 * Math.abs(Math.sin(t * (WAVER * 0.7) + phase * 1.3))

        const foot = ((i + 0.5) / n - 0.5) * width
        // Each tongue is twice its share of the width, so they overlap heavily
        // and the cluster reads as one body of fire rather than as n separate
        // flames standing in a row.
        const wide = (width / n) * 2.1
        const height = tall * pulse
        const lean = sway * wide * 0.5

        // Deep, saturated, and nearly opaque. Against a sunlit plate the flame
        // has to be *darker and redder* than what is behind it or it reads as
        // glare — the orange is doing the work here, not the brightness.
        tongues
          .moveTo(foot - wide / 2, 0)
          .quadraticCurveTo(foot - wide * 0.28, -height * 0.55, foot + lean, -height)
          .quadraticCurveTo(foot + wide * 0.28, -height * 0.55, foot + wide / 2, 0)
          .fill({ color: i % 2 ? 0xe2551c : 0xc63b12, alpha: 0.92 })

        // A brighter tongue inside the dark one, still not additive.
        tongues
          .moveTo(foot - wide * 0.3, 0)
          .quadraticCurveTo(foot - wide * 0.1, -height * 0.44, foot + lean * 0.6, -height * 0.66)
          .quadraticCurveTo(foot + wide * 0.1, -height * 0.44, foot + wide * 0.3, 0)
          .fill({ color: 0xff9c33, alpha: 0.9 })

        // The hot core, low and small, and the only additive thing in the
        // flame — what stops the whole shape reading as cut orange paper.
        core
          .moveTo(foot - wide * 0.18, 0)
          .quadraticCurveTo(foot, -height * 0.3, foot + wide * 0.18, 0)
          .fill({ color: 0xfff0c8, alpha: 0.55 })
      }

      // A slow breath in the glow, out of phase with the tongues so the wall
      // does not pulse in time with them. Tight to the flames rather than a
      // pool over the block: the first version lit half the street, which in
      // daylight looked like an overexposed photograph rather than a fire.
      const breath = 0.85 + 0.15 * Math.sin(t * 1.7)
      glow.clear()
      for (let i = 3; i >= 1; i--) {
        const spread = (i / 3) * breath
        glow
          .ellipse(0, -tall * 0.3, width * 0.85 * spread, tall * 0.6 * spread)
          .fill({ color: 0xff7a28, alpha: 0.13 })
      }

      /*
       * Smoke, at both tiers, and the most important thing on screen.
       *
       * A fire against a bright sky is read almost entirely by its smoke —
       * flames at this scale are a few dozen pixels and the plume is the whole
       * height of the building. It is also the only part that cannot possibly
       * be mistaken for sunlight, which is what the first version got wrong.
       *
       * Each puff has its own life running 0→1, offset by its index, so the
       * column is always full and nothing pops into existence. Deterministic:
       * the same second of the same fire draws identically everywhere (§7).
       */
      smoke.clear()
      const puffs = tier === 'high' ? 9 : 5
      for (let i = 0; i < puffs; i++) {
        const life = (t * 0.16 + i / puffs) % 1
        const rise = tall * (0.55 + life * 3.2)
        const drift = Math.sin(t * 0.35 + i * 1.7) * width * (0.15 + life * 0.55)
        const size = width * (0.34 + life * 0.85)
        smoke
          .ellipse(drift, -rise, size, size * 0.62)
          // Thins as it climbs, and never fully opaque at the bottom either —
          // you have to be able to see the flames through the base of it.
          .fill({ color: 0x3a2b24, alpha: 0.34 * (1 - life) * (1 - life) })
      }
    },
    destroy() {
      view.destroy({ children: true })
    },
  }
}
