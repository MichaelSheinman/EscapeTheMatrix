/*
 * What the canvas draws.
 *
 * This is a placeholder scene, and deliberately so: the world proper is a
 * painted location with an invisible logic layer over it, authored in Tiled
 * (§10.3), and building it is its own ticket. What this one has to prove is the
 * bridge — that the same store the panels read is read here, sixty times a
 * second, by code React does not know exists.
 *
 * So it draws the only domain there is yet. Every probe recorded lands as a
 * mark around a beacon, in the colour of how this client heard about it: warm
 * for the ones this tab did, cold for the ones the socket brought from
 * somewhere else. The number under the beacon is the server's row count.
 *
 * Two constraints from the design carry over even to a placeholder:
 *
 *   - **Nothing is derived.** The count drawn is the count the server stated
 *     (§10.2). Positions and pulses are pure presentation — the same input
 *     draws the same picture, and no number on screen was computed here.
 *   - **No full-screen filters.** §10.4 ships the Low tier first, and a look
 *     designed around a lighting pass cannot have it taken away later. There
 *     are no filters in this file, and the switchable tier that adds them is
 *     its own ticket.
 *
 * `Scene` is the thin interface §10.4 asks for — the seam that keeps Pixi types
 * out of the rest of the client. `drawScene` and `moveCamera` join `playEffect`
 * on it when there is a location to draw and a camera to move.
 */

import { Container, Graphics, Text } from 'pixi.js'

import type { Mark, World } from '../world/state'
import type { Tokens } from './tokens'

/** Where something happened, in canvas pixels. */
export type Spot = { x: number; y: number }

export type Scene = {
  /** The display object the renderer puts on its stage. */
  view: Container
  /** The drawing area changed. */
  resize(width: number, height: number): void
  /** One frame: read the world, advance whatever is moving. */
  update(world: World, elapsedMS: number): void
  /** A momentary flourish with no state behind it (§10.4). */
  playEffect(effect: 'tap', at: Spot): void
  destroy(): void
}

/** How long a mark takes to arrive, and a tap ripple to die, in milliseconds. */
const ARRIVAL = 320
const RIPPLE = 620

/**
 * The angle between consecutive marks.
 *
 * The golden angle, which is what keeps a growing spiral evenly spread instead
 * of falling into spokes — the reason a sunflower looks the way it does. Marks
 * are placed by index, so the arrangement is deterministic: the same probes
 * produce the same picture on any machine, and dropping the oldest ones off the
 * end never moves the rest.
 */
const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5))

/** One mark on screen, and how far into its arrival it is. */
type Pin = { mark: Mark; view: Graphics; age: number }

/** One tap, and how far into its life it is. */
type Ripple = { at: Spot; age: number }

export function createScene(tokens: Tokens): Scene {
  const { color, font } = tokens

  const view = new Container()

  // Drawn on resize and never per frame: this is the ground, not an animation.
  const ground = new Graphics()
  // Everything that moves lives above it. Marks are children rather than
  // redrawn geometry so an arrival animates as a transform, which costs
  // nothing, instead of as a rebuilt path sixty times a second.
  const beacon = new Graphics()
  const glow = new Graphics()
  const pins = new Container()
  const ripples = new Graphics()

  const count = new Text({
    text: '',
    style: { fontFamily: font, fontSize: 40, fontWeight: '500', fill: color.text },
  })
  count.anchor.set(0.5, 0)

  const caption = new Text({
    text: 'probes recorded',
    style: { fontFamily: font, fontSize: 10, letterSpacing: 1.4, fill: color.muted },
  })
  caption.anchor.set(0.5, 0)

  const readout = new Container()
  readout.addChild(count, caption)

  view.addChild(ground, glow, ripples, beacon, pins, readout)

  let width = 0
  let height = 0
  /** Where the beacon stands. Recomputed on resize, read every frame. */
  let centre: Spot = { x: 0, y: 0 }
  /** Milliseconds since the scene started, which is what everything cyclic uses. */
  let clock = 0

  const live: Ripple[] = []
  const shown = new Map<number, Pin>()
  /**
   * The last `probes` object seen.
   *
   * The store hands back the same object when nothing changed (`state.ts`), so
   * a frame that has to do nothing costs one pointer comparison. This is what
   * makes reading the whole world every frame the cheap option rather than the
   * expensive one — and why the renderer never needs to subscribe.
   */
  let drawn: World['probes'] | null = null
  let signedIn = false

  function layout() {
    const horizon = Math.round(height * 0.42)
    centre = { x: width / 2, y: horizon + (height - horizon) * 0.34 }

    ground.clear()
    ground.rect(0, 0, width, horizon).fill(color.sky)
    ground.rect(0, horizon, width, height - horizon).fill(color.ground)
    ground
      .moveTo(0, horizon + 0.5)
      .lineTo(width, horizon + 0.5)
      .stroke({ width: 1, color: color.horizon })

    // The pool of light the beacon stands in. Flattened, because the camera is
    // a fixed three-quarter view (§10.3) and a circle on the ground reads as an
    // ellipse from one.
    ground
      .ellipse(centre.x, centre.y + 26, Math.min(width, height) * 0.38, Math.min(width, height) * 0.17)
      .fill({ color: color.accent, alpha: 0.07 })

    beacon.position.set(centre.x, centre.y)
    glow.position.set(centre.x, centre.y)
    pins.position.set(centre.x, centre.y)

    beacon.clear()
    beacon.circle(0, 0, 13).fill(color.paper)
    beacon.circle(0, 0, 13).stroke({ width: 2, color: color.accent })
    beacon.circle(0, 0, 5).fill(color.accent)

    glow.clear()
    glow.circle(0, 0, 30).fill({ color: color.accent, alpha: 0.16 })

    readout.position.set(centre.x, centre.y + 74)
    count.position.set(0, 0)
    caption.position.set(0, 46)
  }

  /** Where the nth mark stands, relative to the beacon. */
  function spotFor(n: number): Spot {
    const angle = n * GOLDEN_ANGLE
    const radius = 46 + 15 * Math.sqrt(n)
    // Squashed vertically for the same reason the pool of light is.
    return { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius * 0.46 }
  }

  function addPin(mark: Mark) {
    const at = spotFor(mark.n)
    const tint = mark.by === 'you' ? color.accent : color.info

    const g = new Graphics()
    g.circle(0, 0, 5).fill(color.paper)
    g.circle(0, 0, 5).stroke({ width: 2, color: tint })
    g.position.set(at.x, at.y)
    g.alpha = 0
    pins.addChild(g)

    shown.set(mark.n, { mark, view: g, age: 0 })
  }

  return {
    view,

    resize(w, h) {
      width = w
      height = h
      layout()
    },

    update(world, elapsedMS) {
      clock += elapsedMS

      // ── the part that reads the store ────────────────────────────────
      // Reference comparison first: on the overwhelming majority of frames
      // nothing here has changed and none of this runs.
      if (world.probes !== drawn) {
        const marks = world.probes.marks

        for (const mark of marks) {
          if (!shown.has(mark.n)) addPin(mark)
        }
        // The store remembers a bounded number of marks; the ones that fell off
        // the end go with them.
        const kept = new Set(marks.map((mark) => mark.n))
        for (const [n, pin] of shown) {
          if (kept.has(n)) continue
          pin.view.destroy()
          shown.delete(n)
        }

        count.text = String(world.probes.count)
        drawn = world.probes
      }

      // The world is drawn at the door too — it never unmounts (§10.1) — but
      // the readout is about a player, so it waits for one.
      const isIn = world.session.status === 'in'
      if (isIn !== signedIn) {
        signedIn = isIn
        readout.visible = isIn
      }

      // ── the part that just moves ─────────────────────────────────────
      const breath = Math.sin(clock / 900)
      beacon.scale.set(1 + breath * 0.04)
      glow.scale.set(1 + breath * 0.12)
      glow.alpha = 0.75 + breath * 0.25

      for (const pin of shown.values()) {
        if (pin.age >= ARRIVAL) continue
        pin.age = Math.min(pin.age + elapsedMS, ARRIVAL)
        const t = ease(pin.age / ARRIVAL)
        pin.view.alpha = t
        pin.view.scale.set(0.4 + t * 0.6)
      }

      if (live.length > 0) {
        ripples.clear()
        for (let i = live.length - 1; i >= 0; i--) {
          const ripple = live[i]
          ripple.age += elapsedMS
          if (ripple.age >= RIPPLE) {
            live.splice(i, 1)
            continue
          }
          const t = ripple.age / RIPPLE
          ripples
            .circle(ripple.at.x, ripple.at.y, 8 + ease(t) * 64)
            .stroke({ width: 2, color: color.accent, alpha: 0.5 * (1 - t) })
        }
        if (live.length === 0) ripples.clear()
      }
    },

    playEffect(effect, at) {
      if (effect === 'tap') live.push({ at: { ...at }, age: 0 })
    },

    destroy() {
      view.destroy({ children: true })
      shown.clear()
      live.length = 0
    },
  }
}

/** Fast then settling — the shape a thing arriving should have. */
function ease(t: number): number {
  return 1 - (1 - t) ** 3
}
