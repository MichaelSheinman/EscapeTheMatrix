import { Container, Graphics, Rectangle } from 'pixi.js'

import type { Point } from './level/geometry'
import type { Level, Thing } from './level/tiled'

/*
 * The things in a challenge's room, drawn on the canvas rather than pinned over
 * it as chrome. Painted as filled shapes in the room's own palette, inside the
 * camera container, so they sit on the boards, take the firelight, move with the
 * view and can be walked up to.
 *
 * Drawn rather than baked into the plate, because a thing you have put in your
 * bag has to stop being in the room. The room's furniture is in the plate and
 * its contents are here, and which of the two something is turns out to be the
 * same question as whether you can pick it up.
 *
 * Each is a handful of filled paths, sized to read at a glance from a
 * three-quarter camera at this zoom. Detail invisible at this size costs frames
 * and buys nothing.
 *
 * Nothing here is random: the bob and the glow come off a clock and the item's
 * own position, so the same room draws the same way on every machine (§7).
 */

/** How far a thing lifts on its bob, in plate pixels. */
const BOB = 3

/**
 * The box every `paint` below draws inside, in its own units. Stated rather than
 * measured: `getLocalBounds` would also be whatever the last edit to a shape
 * happened to make it, widening what you can click without anybody deciding to.
 * `top` is how far above the foot the paint reaches, and every case is drawn to
 * fit.
 */
const DRAWN = { wide: 56, tall: 68, top: -58 }

/**
 * The smallest a thing may be to point at, in plate pixels.
 *
 * A floor under the target, never a replacement for it: anything bigger
 * answers for its own envelope and nothing more, so the things in a crowded
 * room stop answering for each other. About a fingertip at this zoom.
 */
const LEAST = 78

/*
 * What a thing is drawn in: much lighter than the room. A flat at midnight lit
 * by one fire is nearly black, so anything you are meant to find in it has to
 * sit well above the background rather than in key with it. These read as
 * objects catching the firelight.
 */
const LIT = 0xfff1da
const DEEP = 0xd9a874
const DARK = 0x6b4426

/**
 * How big a thing stands, against the plate. The plate is 2520px across and
 * shown at roughly half that, so an object at icon scale is a dozen pixels on
 * screen and invisible in a dark room. A person is about forty plate pixels tall
 * here, and `BIG` is what stands beside one.
 *
 * Not one number, because these are not one size of thing. A dog and an armchair
 * are person-sized; a wallet and a phone are a hand's width and at the same
 * scale would read as furniture. The small things go far enough below the
 * default to read as objects on a surface and no further, because under about a
 * third of `BIG` they stop being findable in a dark room.
 */
const BIG = 2.4
const SIZE: Record<string, number> = {
  phone: 1.3,
  wallet: 1.2,
  cash: 1.3,
  papers: 1.7,
  photos: 1.8,
  flowers: 1.9,
  urn: 1.9,
}

export type Things = {
  readonly view: Container
  /**
   * Put a room's things on screen, or take them off.
   *
   * `offering` is the set of item keys the run still has on the table — a thing
   * already in the bag is not in the room any more. Called every frame with
   * what the world says, so it does nothing when nothing changed.
   */
  show(level: Level | null, offering: ReadonlySet<string> | null): void
  /**
   * Which thing the pointer is over, and where the top of it is. Read by the
   * chrome at its own cadence rather than pushed at it (§10.1), so a mouse
   * crossing a room does not re-render React every frame. The area it answers
   * for is the hit area below, the same one a press uses.
   *
   * `top` is the plate point the drawing reaches up to. The chrome floats its
   * tip above that rather than above the foot, so the tip does not cover the
   * thing it names — and how tall a thing is drawn is only known here.
   */
  hovering(): { key: string; top: Point } | null
  /** One frame. Advances the bob and the halo. */
  update(elapsedMS: number): void
  destroy(): void
}

export function createThings(onPress: (key: string) => void): Things {
  const view = new Container()
  view.sortableChildren = true

  /** What is drawn, so `show` can do nothing when nothing moved. */
  let drawn = ''
  let clock = 0
  /** Which thing the pointer is over, or null. */
  let over: string | null = null
  const standing: { key: string; at: Point; top: Point; art: Container }[] = []

  function clear() {
    for (const thing of standing) thing.art.destroy({ children: true })
    standing.length = 0
    // The thing the pointer was over may be the thing that just went into the
    // bag. Nothing is under the pointer until something says otherwise.
    over = null
  }

  function place(level: Level | null, offering: ReadonlySet<string> | null) {
    const here = level?.items ?? []
    const want =
      level && offering
        ? `${level.key}|${here
            .filter((t) => offering.has(t.key))
            .map((t) => t.key)
            .join(',')}`
        : ''
    if (want === drawn) return
    drawn = want
    clear()

    if (!level || !offering) return
    for (const thing of here) {
      if (!offering.has(thing.key)) continue
      const art = paint(thing)
      // Sized against the plate rather than against the drawing (see SIZE).
      const size = SIZE[thing.key] ?? BIG
      art.scale.set(size)
      // Sorted just in front of what it is standing on. A thing in this room is
      // mostly on something — the urn on a shelf, the papers on the fridge — and
      // sorting it by its own foot puts it behind the cut-out of that
      // furniture. Small enough that the character still passes in front of
      // anything nearer the camera.
      art.zIndex = thing.at.y + 24
      art.position.set(thing.at.x, thing.at.y)
      art.eventMode = 'static'
      art.cursor = 'pointer'
      /*
       * The target is the thing you can see, grown to a size you can hit. Taken
       * from the drawing's own bounds rather than stated as a rectangle: a fixed
       * box tall enough for an armchair, hung on every item, would overlap its
       * neighbours and let the urn answer for a click on the phone in front of
       * it.
       *
       * Grown, because a wallet is a few plate pixels of paint. `LEAST` is a
       * floor on the target rather than the target, and it opens the small
       * things out around their own middle so they stay where they look.
       *
       * Local coordinates, so it is stated in the drawing's units and scales
       * with them; the arithmetic below is in plate pixels and divided back
       * through. It does not move with the bob, so the area you click does not
       * squirm away between the press and the release.
       */
      const wide = Math.max(DRAWN.wide * size, LEAST) / size
      const tall = Math.max(DRAWN.tall * size, LEAST) / size
      art.hitArea = new Rectangle(-wide / 2, DRAWN.top + DRAWN.tall / 2 - tall / 2, wide, tall)
      art.on('pointertap', (event) => {
        event.stopPropagation()
        onPress(thing.key)
      })
      // What the chrome floats its tip over. Cleared on the way out so a thing
      // taken out from under the pointer does not leave a tip behind it.
      art.on('pointerover', () => {
        over = thing.key
      })
      art.on('pointerout', () => {
        if (over === thing.key) over = null
      })
      view.addChild(art)
      // Where the paint stops, so the chrome can float a tip clear of it.
      const top = { x: thing.at.x, y: thing.at.y + DRAWN.top * size }
      standing.push({ key: thing.key, at: thing.at, top, art })
    }
  }

  return {
    view,
    show: place,
    hovering() {
      if (over === null) return null
      const thing = standing.find((s) => s.key === over)
      return thing ? { key: thing.key, top: thing.top } : null
    },
    update(elapsedMS) {
      if (standing.length === 0) return
      clock += elapsedMS
      const t = clock / 1000
      for (const thing of standing) {
        // A phase per object, off its own position, so they do not bob in
        // unison.
        const phase = (thing.at.x + thing.at.y) * 0.01
        thing.art.y = thing.at.y + Math.sin(t * 1.6 + phase) * BOB
      }
    },
    destroy() {
      clear()
      view.destroy({ children: true })
    },
  }
}

/*
 * One thing, painted. Every object is drawn standing on the origin, so the
 * container's position is where it meets the floor. The shadow goes down first
 * and the object over it.
 */
function paint(thing: Thing): Container {
  const art = new Container()

  const shadow = new Graphics()
  shadow.ellipse(0, 0, 22, 8).fill({ color: 0x000000, alpha: 0.38 })
  art.addChild(shadow)

  const g = new Graphics()
  switch (thing.key) {
    case 'dog': {
      /*
       * A dog, sitting, three-quarter on. Built as a silhouette: haunch, chest,
       * front legs, neck, head, muzzle, ear, tail. It reads as a dog at this
       * size because of the proportions between those parts rather than any
       * detail inside them, so the fills are flat and the only shading is one
       * darker mass for the far side.
       */
      const coat = 0xc98b52
      const shade = 0x9a6636
      // Tail, curling up behind the haunch.
      g.moveTo(-16, -20)
        .quadraticCurveTo(-34, -26, -30, -46)
        .quadraticCurveTo(-24, -34, -14, -28)
        .closePath()
        .fill(shade)
      // Haunch and body.
      g.ellipse(-6, -20, 17, 16).fill(shade)
      g.ellipse(4, -22, 15, 15).fill(coat)
      // Front legs, down to the boards.
      g.roundRect(8, -16, 7, 17, 3).fill(coat)
      g.roundRect(-1, -14, 7, 15, 3).fill(shade)
      // Chest into the neck.
      g.moveTo(6, -30).quadraticCurveTo(16, -34, 15, -44).lineTo(6, -40).closePath().fill(coat)
      // Head and muzzle.
      g.ellipse(17, -47, 11, 10).fill(coat)
      g.ellipse(26, -45, 7, 5).fill(coat)
      g.ellipse(30, -45, 3, 2.5).fill(0x35211a)
      // Ear, folded forward.
      g.moveTo(11, -55).quadraticCurveTo(8, -42, 16, -44).closePath().fill(shade)
      // One eye, because two at this size read as a face rather than a dog.
      g.circle(21, -50, 1.8).fill(0x2a1a14)
      break
    }

    case 'urn':
      g.moveTo(-13, -8)
        .bezierCurveTo(-18, -30, -10, -44, 0, -44)
        .bezierCurveTo(10, -44, 18, -30, 13, -8)
        .closePath()
        .fill(DEEP)
      g.ellipse(0, -44, 12, 4).fill(LIT)
      g.rect(-14, -12, 28, 6).fill(DARK)
      g.rect(-9, -30, 18, 4).fill(0xf3e2c4)
      break

    case 'papers':
      g.rect(-16, -40, 32, 40).fill(0xe8dcc0)
      g.rect(-16, -40, 32, 7).fill(LIT)
      g.rect(-10, -28, 20, 3).fill(DEEP)
      g.rect(-10, -21, 14, 3).fill(DEEP)
      break

    case 'photos':
      g.rect(-19, -22, 38, 22).fill(DEEP)
      g.rect(-21, -28, 42, 7).fill(LIT)
      g.moveTo(6, -28).lineTo(14, -44).lineTo(26, -39).lineTo(18, -24).closePath().fill(0xf1e6cf)
      break

    case 'phone':
      g.roundRect(-11, -38, 22, 38, 4).fill(DARK)
      g.roundRect(-8, -34, 16, 28, 2).fill(0x9ec4d8)
      break

    case 'wallet':
      g.roundRect(-20, -22, 40, 22, 3).fill(DEEP)
      g.roundRect(-20, -26, 40, 8, 2).fill(0x6f4526)
      g.rect(4, -30, 16, 10).fill(0xe8dcc0)
      break

    case 'cash':
      g.roundRect(-19, -20, 38, 20, 4).fill(0xb9c9a2)
      g.rect(-4, -20, 8, 20).fill(0x8fa77c)
      g.circle(-9, -10, 5).fill(0xd8e3c6)
      break

    case 'flowers':
      g.moveTo(-9, 0).lineTo(-6, -26).lineTo(6, -26).lineTo(9, 0).closePath().fill(0x9fb6c4)
      g.rect(-7, -30, 14, 5).fill(0xbccbd6)
      g.circle(-8, -40, 7).fill(0xd9756f)
      g.circle(8, -43, 7).fill(0xe0a05c)
      g.circle(0, -50, 7).fill(0xd9756f)
      g.circle(0, -44, 3).fill(0xf3e2c4)
      break

    case 'chair':
      g.rect(-20, -26, 40, 26).fill(0x7a4340)
      g.rect(-24, -34, 10, 34).fill(0x6b3a37)
      g.rect(14, -34, 10, 34).fill(0x6b3a37)
      g.rect(-20, -50, 40, 24).fill(0x8a4f4b)
      break

    default:
      // Something this build has not drawn: a parcel, plainly.
      g.rect(-16, -30, 32, 30).fill(DEEP)
      g.rect(-16, -30, 32, 6).fill(LIT)
      g.rect(-3, -30, 6, 30).fill(DARK)
  }
  art.addChild(g)

  return art
}
