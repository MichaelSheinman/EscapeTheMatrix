/*
 * The painted world.
 *
 * technical.md §10.3: a location is a hand-corrected painting with a
 * **non-rendered data layer** over it — where you can walk, what occludes what,
 * and where things can happen. This file is the half that draws; `level/` is
 * the half that reads what was authored, and it has no Pixi in it.
 *
 * Four things happen here, and each one is a line of §10.3 made literal:
 *
 *   - **The painting is one sprite.** Not a tilemap. Tiling a painterly city
 *     produces visible repetition, which is the reason §10.3 rejects tilemaps
 *     outright.
 *   - **The walkable polygon is the only place the character can be.** A tap
 *     outside it is understood as the nearest bit of floor rather than refused,
 *     and the route there is pathfound around whatever is in the way. Nothing
 *     moves the character except walking a route, so there is no second code
 *     path that could put them through a wall.
 *   - **Depth is one number.** Everything standing on the floor is sorted by
 *     the y at which it meets the ground: the character by their feet, a prop
 *     by its authored baseline. Under a fixed three-quarter camera that is the
 *     whole of occlusion, and the props are cut out of the plate itself, so a
 *     painted location gets depth without a second set of art.
 *   - **Trigger zones are watched, not fired.** Which authored rectangle the
 *     character is standing in is reported outwards every time it changes.
 *     What *happens* there is the event system's (#21, #22) — §3.9's placement
 *     conditions and these zones are the same objects, which is exactly why
 *     this file must not start deciding between them.
 *
 * `Scene` is the thin interface §10.4 asks for — `drawScene`, `moveCamera`,
 * `playEffect` — and it is the only reason Pixi types stop here.
 *
 * **This file is the Low tier, whole.** §10.4 ships Low first, because "a look
 * designed around a lighting pass cannot have it taken away later" — so what is
 * drawn below has no full-screen filter and no particle in it, and the lighting
 * it shows is the lighting the plate was painted with. `setQuality('high')`
 * builds `effects.ts` *over* this and changes nothing in it; `setQuality('low')`
 * destroys that object again. There is no third state and no branch inside the
 * drawing, which is what stops the two tiers from drifting apart.
 */

import { Container, Graphics, Rectangle, Sprite, Text, Texture } from 'pixi.js'

import type { Standing, World } from '../world/state'
import { createEffects, type Effects } from './effects'
import { contains, distance, nearestInside, type Area, type Point, type Rect } from './level/geometry'
import { findPath } from './level/navmesh'
import { portalsAt, zonesAt, type Level, type Portal } from './level/tiled'
import { EFFECTS, type Tier } from './quality'
import type { Tokens } from './tokens'

/** Where something happened, in canvas pixels. */
export type Spot = { x: number; y: number }

/**
 * What the world is showing.
 *
 * A district with no location yet is a case rather than an absence: the ground
 * is drawn, the place is named, and the screen says it has not been painted.
 * Drawing nothing would read as the renderer having failed.
 */
export type Showing =
  | { painted: true; level: Level; plate: Texture }
  | { painted: false; name: string; note: string }

/** How the scene tells the app what the character did. */
export type Watch = {
  /** Which authored rectangles they are inside, whenever that changes. */
  onStanding: (standing: Standing | null) => void
  /**
   * They walked onto a way out, and stopped there (§3.12).
   *
   * A portal is where a way *is*; walking onto one takes it. The scene says so
   * and decides nothing — whether the journey is possible, what it costs, and
   * what happens on the way are the server's, and the app turns this into the
   * same `character.move` command a button sends (§10.1).
   *
   * Fired once per arrival, and never for the portal you arrived *on*: coming
   * in through a door and immediately being sent back out of it is a loop, not
   * a transition.
   */
  onPortal: (portal: Portal) => void
}

/**
 * What is on screen, for the chrome floating over it.
 *
 * The DOM half of the app pins markers, tips and the action prompt over
 * authored points in the painting (§10.1's chrome, the preview's markers made
 * real). It needs to know where those points land on the glass, and nothing
 * else — so this is a read, taken at the chrome's own cadence, and it carries
 * no way to change anything.
 */
export type SceneView = {
  /** Which place is being shown. */
  key: string
  /** Where the character is standing, in plate pixels — or null before sign-in. */
  player: Point | null
  /** The authored ways out, each at the middle of its doorway. */
  portals: { key: string; name: string; to: string; at: Point }[]
  /** The authored trigger zones, each at the middle of its area. */
  zones: { key: string; name: string; at: Point }[]
  /** Where a point in the painting lands on the canvas, in CSS pixels. */
  toScreen(p: Point): Spot
}

export type Scene = {
  /** The display object the renderer puts on its stage. */
  view: Container
  /** The drawing area changed. */
  resize(width: number, height: number): void
  /**
   * Put a location on screen, or take one off (§10.4's `drawScene`).
   *
   * `from` is the place the player has just come from, if they came from
   * anywhere. Arriving puts them at the portal that leads back to it — you come
   * in through the door you went through — rather than at the map's spawn,
   * which is where you start rather than where you arrive.
   */
  drawScene(showing: Showing | null, from?: string | null): void
  /** Point the camera somewhere in the painting (§10.4's `moveCamera`). */
  moveCamera(to: Point, options?: { snap?: boolean }): void
  /** Pull the view in or out, about its own centre. Clamped both ways. */
  zoomBy(factor: number): void
  /**
   * Draw at one of §10.4's two tiers, from now on.
   *
   * Switchable at runtime and switchable at any moment: the High tier is built
   * the first time it is asked for and destroyed the moment it is not, so a
   * player who tries it and goes back is left with a scene holding no filter,
   * no light layer and no particle — which is the Low tier's whole promise.
   * Called with the tier it is already at, it does nothing.
   */
  setQuality(tier: Tier): void
  /** Somebody tapped the canvas. Walk there, if there is a way. */
  walkTo(screen: Spot): void
  /**
   * Draw the layer that is never drawn.
   *
   * A development view, off by default and reachable from a key in `main.tsx`.
   * "Non-rendered" is a property of the shipped frame, not a reason an author
   * should have to guess where they put a rectangle.
   */
  showLogic(on: boolean): void
  /** One frame: read the world, advance whatever is moving. */
  update(world: World, elapsedMS: number): void
  /** What is on screen, for the chrome floating over it — null when nothing is. */
  observe(): SceneView | null
  /** A momentary flourish with no state behind it (§10.4). */
  playEffect(effect: 'tap' | 'blocked', at: Spot): void
  destroy(): void
}

/** Plate pixels a second. A walk across this street takes about seven. */
const SPEED = 260

/** How long a ripple lives, in milliseconds. */
const RIPPLE = 620

/** How quickly the camera catches up, as a time constant in milliseconds. */
const CHASE = 130

/** What the world falls back to when no location is on screen. */
const EMPTY: Rect = { x: 0, y: 0, width: 1920, height: 1088 }

/** One tap, and how far into its life it is. */
type Ripple = { at: Point; age: number; blocked: boolean }

export function createScene(tokens: Tokens, watch: Watch): Scene {
  const { color, font } = tokens

  const view = new Container()

  /**
   * The camera.
   *
   * There is no camera object in Pixi — a camera is this container's inverse
   * transform, and `frame()` below is the whole of it. The fixed three-quarter
   * angle §10.3 locks is painted into the plate; what moves is a pan and a
   * zoom, and it is clamped so the edge of the painting is never on screen.
   */
  const camera = new Container()

  const plate = new Sprite()
  const bare = new Graphics()
  const marks = new Graphics()

  /**
   * Everything standing on the floor.
   *
   * Sorted by `zIndex`, and every child sets its own to the y at which it meets
   * the ground. That single rule is what makes the character walk behind the
   * van and in front of the planter without either of them knowing about the
   * other.
   */
  const floor = new Container()
  floor.sortableChildren = true

  /*
   * The three places the High tier is allowed to draw (§10.4).
   *
   * Empty containers, always present, in the z-order the effects need: light
   * over the floor, air over the light, and the vignette in screen space above
   * the camera rather than inside it — a vignette that panned with the world
   * would be a smudge on the painting instead of the edge of a frame.
   *
   * Empty, they cost nothing. Keeping them means `effects.ts` never has to
   * compute an index into a display list whose order is otherwise fixed.
   */
  const glow = new Container()
  const air = new Container()
  const edge = new Container()

  const logic = new Container()
  logic.visible = false
  const logicLines = new Graphics()
  const logicLabels = new Container()
  logic.addChild(logicLines, logicLabels)

  const { walker, body } = makeWalker(color)
  walker.label = 'the character'

  const notice = new Container()
  const noticeName = new Text({
    text: '',
    style: { fontFamily: font, fontSize: 34, fontWeight: '500', fill: color.text },
  })
  noticeName.anchor.set(0.5, 1)
  const noticeNote = new Text({
    text: '',
    style: { fontFamily: font, fontSize: 11, letterSpacing: 1.4, fill: color.muted },
  })
  noticeNote.anchor.set(0.5, 0)
  noticeNote.position.set(0, 12)
  notice.addChild(noticeName, noticeNote)
  notice.visible = false

  camera.addChild(bare, plate, marks, floor, glow, air, logic)
  view.addChild(camera, edge, notice)

  let width = 0
  let height = 0

  let level: Level | null = null
  let extent: Rect = EMPTY

  /**
   * Which of §10.4's tiers is being drawn, and the tier's own machinery.
   *
   * `effects` is null at Low and that is the whole implementation of "no
   * full-screen filters and no particles" — there is nothing to draw them with.
   */
  let quality: Tier = 'low'
  let effects: Effects | null = null

  /** Where the character is, in plate pixels. Their feet, not their middle. */
  let at: Point = { x: EMPTY.width / 2, y: EMPTY.height / 2 }
  /** The route being walked, including where it started. Empty when still. */
  let route: Point[] = []
  /** Which leg of the route is being walked. */
  let leg = 0
  /** Which way they are facing: 1 right, -1 left. */
  let facing = 1

  let scale = 1
  /** The player's own zoom, applied over the base. 1 is the preview's plate. */
  let wheel = 1
  /** Where the camera is looking, and where it is heading. */
  let eye: Point = { ...at }
  let aim: Point = { ...at }

  const ripples: Ripple[] = []
  let clock = 0

  /** The last thing said outwards, so nothing is said twice. */
  let said = ''

  /** The outfit as last painted onto the walker, so nothing is repainted idly. */
  let dressed = '|'

  /**
   * The portal the character is standing on that has already been dealt with.
   *
   * Set when a walk ends on one, and set on arrival to the door they came in
   * by. Cleared the moment they are standing on no portal at all, which is what
   * makes stepping off a door and back onto it a second transition rather than
   * a silence.
   */
  let onPortal: string | null = null

  /* ── the camera ─────────────────────────────────────────────────────── */

  /**
   * Fit the painting to the viewport, and put the camera where it is aiming.
   *
   * The base zoom is the preview's: far enough out that a street reads as a
   * street, clamped so it never sinks into map-like abstraction. The wheel
   * moves it. The one hard floor is the cover scale — the edge of the painting
   * is never on screen, so a plate smaller than the viewport is shown at
   * whatever fills it.
   */
  function frame() {
    if (width === 0 || height === 0) return
    const cover = Math.max(width / extent.width, height / extent.height)
    const base = clamp(Math.min(width / 1900, height / 1020), 0.48, 0.86)
    scale = Math.max(cover, base * wheel)
    eye = held()
    apply()
  }

  /**
   * Where the camera may look, given where it wants to.
   *
   * Clamped half a viewport in from every edge of the painting, so the edge of
   * the world is never on screen. Since the scale is at least the fill scale,
   * there is always room for the clamp to have an answer.
   */
  function held(): Point {
    const halfW = width / (2 * scale)
    const halfH = height / (2 * scale)
    return {
      x: clamp(aim.x, extent.x + halfW, extent.x + extent.width - halfW),
      y: clamp(aim.y, extent.y + halfH, extent.y + extent.height - halfH),
    }
  }

  function apply() {
    camera.scale.set(scale)
    camera.position.set(width / 2 - eye.x * scale, height / 2 - eye.y * scale)
  }

  /** Catch up with where the camera is aiming, by a fixed fraction of the gap. */
  function chase(elapsedMS: number) {
    if (width === 0 || height === 0) return
    const to = held()
    const t = Math.min(1, elapsedMS / CHASE)
    eye = { x: eye.x + (to.x - eye.x) * t, y: eye.y + (to.y - eye.y) * t }
    apply()
  }

  /* ── the effects tier ───────────────────────────────────────────────── */

  function setQuality(tier: Tier) {
    if (tier === quality) return
    quality = tier

    if (EFFECTS[tier].filters) {
      effects = createEffects(tokens, { glow, air, edge })
      // The stack goes on the **camera**, not on the view: the world is graded
      // and grained, and the notice floating over an unpainted district is
      // typography, which wants to stay exactly the colour it was set in.
      camera.filters = effects.stack
      effects.show(level)
      effects.resize(width, height)
      return
    }

    camera.filters = []
    effects?.destroy()
    effects = null
    // The lighting let go of the character too. Left behind, an amber tint from
    // a lamp somebody walked past would be the one piece of the High tier that
    // survived being switched off.
    body.tint = 0xffffff
  }

  /* ── mounting a location ────────────────────────────────────────────── */

  function drawScene(next: Showing | null, from?: string | null) {
    // The cut-outs belong to the location that is leaving. Their textures are
    // frames of a plate the asset cache owns, so the frame goes and the plate
    // stays — §10.4's texture budget starts with not accumulating these.
    for (const gone of floor.removeChildren()) {
      if (gone !== walker) gone.destroy({ children: true, texture: true, textureSource: false })
    }
    // The character survives every location. Put back straight away rather than
    // in the painted branch below, so there is no moment — not even an unpainted
    // district — at which the scene has nobody in it (§10.1: the canvas never
    // unmounts, and neither does the person standing on it).
    floor.addChild(walker)
    route = []
    leg = 0

    onPortal = null

    // The lights and the air belong to the location, like the cut-outs above:
    // both are authored data, and a room's air does not follow you outdoors.
    effects?.show(next?.painted ? next.level : null)

    if (!next?.painted) {
      level = null
      extent = EMPTY
      plate.visible = false
      plate.texture = Texture.EMPTY
      bare.visible = true
      drawBare()
      noticeName.text = next?.name ?? ''
      noticeNote.text = next?.note ?? ''
      notice.visible = next !== null
      say(null)
      aim = { x: extent.width / 2, y: extent.height / 2 }
      frame()
      if (logic.visible) drawLogic()
      return
    }

    level = next.level
    extent = level.extent
    notice.visible = false
    bare.visible = false
    plate.visible = true

    plate.texture = next.plate
    plate.position.set(level.painting.x, level.painting.y)
    plate.width = level.painting.width
    plate.height = level.painting.height

    // How many world pixels one texture pixel is worth. Normally exactly one;
    // this is what keeps the cut-outs right if a plate is ever exported at a
    // different size than the map declares.
    const perPixel = next.plate.width > 0 ? level.painting.width / next.plate.width : 1

    for (const occluder of level.occluders) {
      const cut = new Texture({
        source: next.plate.source,
        frame: new Rectangle(
          (occluder.frame.x - level.painting.x) / perPixel,
          (occluder.frame.y - level.painting.y) / perPixel,
          occluder.frame.width / perPixel,
          occluder.frame.height / perPixel,
        ),
      })
      const prop = new Sprite(cut)
      prop.label = occluder.name
      prop.position.set(occluder.frame.x, occluder.frame.y)
      prop.width = occluder.frame.width
      prop.height = occluder.frame.height
      // The whole of depth: where it meets the ground.
      prop.zIndex = occluder.baseline
      floor.addChild(prop)
    }

    // Where they come in. The door back to wherever they were, if this
    // location has one, and otherwise the map's own spawn — which is where a
    // player *starts* rather than where they arrive.
    const back = from ? level.portals.find((portal) => portal.to === from) : undefined
    at = back ? middle(back) : { ...level.spawn }
    // Authored badly, this is the one point that could put the character
    // outside the floor for as long as it took them to walk. It cannot, because
    // arriving is the same clamp a tap gets.
    at = nearestInside(level.region, at)
    // Standing in the door they came in by is not walking onto it. Without
    // this, arriving would immediately be a departure.
    onPortal = portalsAt(level, at)[0]?.key ?? null
    walker.position.set(at.x, at.y)
    walker.zIndex = at.y

    aim = { ...at }
    frame()
    look()
    if (logic.visible) drawLogic()
  }

  /** The ground under a place nobody has painted yet. */
  function drawBare() {
    const horizon = Math.round(extent.height * 0.42)
    bare.clear()
    bare.rect(extent.x, extent.y, extent.width, horizon).fill(color.sky)
    bare
      .rect(extent.x, extent.y + horizon, extent.width, extent.height - horizon)
      .fill(color.ground)
    bare
      .moveTo(extent.x, extent.y + horizon + 0.5)
      .lineTo(extent.x + extent.width, extent.y + horizon + 0.5)
      .stroke({ width: 1, color: color.horizon })
  }

  /* ── walking ────────────────────────────────────────────────────────── */

  /** A momentary flourish with no state behind it (§10.4), at a canvas point. */
  function playEffect(effect: 'tap' | 'blocked', screen: Spot) {
    const where = camera.toLocal(screen)
    ripples.push({ at: { x: where.x, y: where.y }, age: 0, blocked: effect === 'blocked' })
  }

  function walkTo(screen: Spot) {
    if (!level) return
    const tapped = camera.toLocal(screen)
    const target = { x: tapped.x, y: tapped.y }
    const reachable = nearestInside(level.region, target)

    // A tap that had to be pulled back onto the floor is a tap at a wall, and
    // the ripple says so where it landed. Nothing is refused: the character
    // still walks to the nearest place they could have meant, because a click
    // that does nothing reads as the game being broken rather than as the wall
    // being a wall.
    const blocked = distance(reachable, target) > 1.5
    playEffect(blocked ? 'blocked' : 'tap', screen)

    // Tapping your own feet is asking to stop, which is a thing worth being
    // able to ask for.
    if (distance(reachable, at) < 1) {
      route = []
      leg = 0
      return
    }

    const found = findPath(level.navmesh, at, reachable)
    if (found.length === 0) return
    route = found
    leg = 1
  }

  /**
   * Advance along the route.
   *
   * The character is never placed anywhere except on a leg of a route the
   * navmesh returned, and every leg of such a route is inside the walkable
   * region by construction (`clearLine` is what put it there). That is the
   * whole enforcement of "the character cannot leave it" — not a check after
   * the fact, but the absence of any other way to move.
   */
  function advance(elapsedMS: number) {
    if (route.length === 0 || !level) return
    let budget = (SPEED * elapsedMS) / 1000

    while (budget > 0 && leg < route.length) {
      const next = route[leg]
      const gap = distance(at, next)
      if (gap <= budget) {
        at = { ...next }
        budget -= gap
        leg++
        continue
      }
      const t = budget / gap
      at = { x: at.x + (next.x - at.x) * t, y: at.y + (next.y - at.y) * t }
      budget = 0
    }

    if (leg >= route.length) {
      route = []
      leg = 0
    }
  }

  /* ── the zones ──────────────────────────────────────────────────────── */

  /** Say where the character is standing, if it is not what was said last. */
  function look() {
    if (level === null) {
      say(null)
      return
    }
    const under = portalsAt(level, at)
    // Off every door: the next one they stop on is a new transition, including
    // the one they are about to step back onto.
    if (under.length === 0) onPortal = null

    say({
      level: level.key,
      zones: zonesAt(level, at).map((zone) => ({ key: zone.key, name: zone.name })),
      portals: under.map((portal) => ({
        key: portal.key,
        name: portal.name,
        to: portal.to,
      })),
    })
  }

  /**
   * A walk ended. If it ended on a way out, take it.
   *
   * The one place a tap on the canvas becomes something the server holds
   * (§10.1): the scene reports the portal and the app sends the command. Which
   * is why nothing here checks whether the journey is possible — that answer
   * belongs to the city, and asking for it is the command.
   */
  function stopped() {
    if (!level) return
    const portal = portalsAt(level, at)[0]
    if (!portal || portal.key === onPortal) return
    onPortal = portal.key
    watch.onPortal(portal)
  }

  function say(standing: Standing | null) {
    const now = standing
      ? `${standing.level}|${standing.zones.map((z) => z.key).join(',')}|${standing.portals
          .map((p) => p.key)
          .join(',')}`
      : ''
    if (now === said) return
    said = now
    watch.onStanding(standing)
  }

  /* ── the development view ───────────────────────────────────────────── */

  function drawLogic() {
    logicLines.clear()
    for (const gone of logicLabels.removeChildren()) gone.destroy()
    if (!level) return

    const { region, navmesh } = level

    // What can see what — the graph the route is found over.
    for (let i = 0; i < navmesh.links.length; i++) {
      for (const link of navmesh.links[i]) {
        if (link.to < i) continue
        logicLines
          .moveTo(navmesh.nodes[i].x, navmesh.nodes[i].y)
          .lineTo(navmesh.nodes[link.to].x, navmesh.nodes[link.to].y)
          .stroke({ width: 1, color: color.muted, alpha: 0.25 })
      }
    }

    outlineOn(logicLines, region.outer, color.accent, 3)
    for (const hole of region.holes) outlineOn(logicLines, hole, color.text, 2)

    for (const zone of level.zones) {
      outlineOn(logicLines, zone.area.points, color.info, 2)
      label(zone.key, zone.area.bounds, color.info)
    }
    for (const portal of level.portals) {
      outlineOn(logicLines, portal.area.points, color.paper, 2)
      label(`→ ${portal.to}`, portal.area.bounds, color.paper)
    }

    for (const occluder of level.occluders) {
      logicLines
        .rect(occluder.frame.x, occluder.frame.y, occluder.frame.width, occluder.frame.height)
        .stroke({ width: 1, color: color.muted, alpha: 0.6 })
      logicLines
        .moveTo(occluder.frame.x, occluder.baseline)
        .lineTo(occluder.frame.x + occluder.frame.width, occluder.baseline)
        .stroke({ width: 2, color: color.accent })
    }
  }

  function label(text: string, box: Rect, tint: number) {
    const shown = new Text({
      text,
      style: { fontFamily: font, fontSize: 13, fill: tint },
    })
    shown.position.set(box.x + 4, box.y + 3)
    logicLabels.addChild(shown)
  }

  /* ── the frame ──────────────────────────────────────────────────────── */

  return {
    view,

    resize(w, h) {
      width = w
      height = h
      notice.position.set(w / 2, h / 2)
      effects?.resize(w, h)
      frame()
    },

    drawScene,
    walkTo,
    setQuality,

    moveCamera(to, options) {
      aim = { x: to.x, y: to.y }
      if (options?.snap) frame()
    },

    zoomBy(factor) {
      wheel = clamp(wheel * factor, 0.55, 3)
      frame()
    },

    showLogic(on) {
      logic.visible = on
      if (on) drawLogic()
      else {
        logicLines.clear()
        for (const gone of logicLabels.removeChildren()) gone.destroy()
      }
    },

    update(world, elapsedMS) {
      clock += elapsedMS

      // The world is drawn at the door too — the canvas never unmounts (§10.1)
      // — but there is nobody to walk until somebody has signed in.
      const isIn = world.session.status === 'in'
      walker.visible = isIn && level !== null

      // What they are wearing, reapplied only when it changes. The mirror in
      // the closet and this figure read the same slice, which is what makes a
      // pick instant out in the world.
      const worn = `${world.look.coat ?? ''}|${world.look.hair ?? ''}`
      if (worn !== dressed) {
        dressed = worn
        paintBody(body, color, world.look)
      }

      // The held arrows steer directly; a click-route is walked. Taking the
      // wheel cancels the route, which is the preview's rule: steering is you,
      // and you outrank where you clicked a moment ago.
      let walking = false
      const heading = walker.visible ? world.heading : null
      if (heading && level) {
        route = []
        leg = 0
        const step = (SPEED * elapsedMS) / 1000
        const before = { ...at }
        // Axis-separated, so a walk into a wall slides along it instead of
        // stopping dead.
        const acrossX = { x: at.x + heading.x * step, y: at.y }
        if (heading.x !== 0 && contains(level.region, acrossX)) at = acrossX
        const acrossY = { x: at.x, y: at.y + heading.y * step }
        if (heading.y !== 0 && contains(level.region, acrossY)) at = acrossY
        walking = at.x !== before.x || at.y !== before.y
        if (walking) {
          if (at.x !== before.x) facing = at.x > before.x ? 1 : -1
          walker.position.set(at.x, at.y)
          walker.zIndex = at.y
          aim = { ...at }
          look()
          // Stepping into a doorway takes it, exactly as a walk ending on one
          // does — the same guard keeps arriving through it from bouncing back.
          stopped()
        }
      } else if (route.length > 0) {
        walking = true
        const before = at
        advance(elapsedMS)
        if (at.x !== before.x) facing = at.x > before.x ? 1 : -1
        walker.position.set(at.x, at.y)
        // Their feet are their depth, restated every frame because it changes
        // every frame.
        walker.zIndex = at.y
        aim = { ...at }
        look()
        // `advance` empties the route when the last leg is walked, so this is
        // the frame the walk ended on.
        if (route.length === 0) stopped()
      }

      chase(elapsedMS)

      // A short bob while walking, and nothing at all while still. The only
      // motion in the scene that is not somebody going somewhere.
      body.y = walking ? -Math.abs(Math.sin(clock / 90)) * 3 : 0
      walker.scale.x = facing

      // The High tier's one reach into the Low tier's drawing, and the reason
      // the lighting is worth calling real-time: the lamps waver, the air
      // drifts, and the character takes the colour of whatever they are
      // standing under. At Low there is nothing here to ask.
      if (effects) body.tint = effects.update(elapsedMS, at)

      if (ripples.length > 0) {
        marks.clear()
        for (let i = ripples.length - 1; i >= 0; i--) {
          const ripple = ripples[i]
          ripple.age += elapsedMS
          if (ripple.age >= RIPPLE) {
            ripples.splice(i, 1)
            continue
          }
          const t = ripple.age / RIPPLE
          // Flattened, because a circle on the ground reads as an ellipse from
          // a fixed three-quarter camera (§10.3).
          marks
            .ellipse(ripple.at.x, ripple.at.y, 10 + ease(t) * 60, (10 + ease(t) * 60) * 0.42)
            .stroke({
              width: 2,
              color: ripple.blocked ? color.text : color.accent,
              alpha: 0.55 * (1 - t),
            })
        }
        if (ripples.length === 0) marks.clear()
      }
    },

    playEffect,

    observe() {
      if (!level) return null
      return {
        key: level.key,
        player: walker.visible ? { ...at } : null,
        portals: level.portals.map((portal) => ({
          key: portal.key,
          name: portal.name,
          to: portal.to,
          at: middle(portal),
        })),
        zones: level.zones.map((zone) => ({
          key: zone.key,
          name: zone.name,
          at: center(zone.area),
        })),
        toScreen(p: Point): Spot {
          const global = camera.toGlobal(p)
          return { x: global.x, y: global.y }
        },
      }
    },

    destroy() {
      camera.filters = []
      effects?.destroy()
      effects = null
      view.destroy({ children: true })
      ripples.length = 0
      route = []
      level = null
    },
  }
}

/**
 * The character, as a placeholder.
 *
 * Drawn rather than painted, because #36 is where figures come from and a
 * stand-in that looks like art would be mistaken for a decision. What matters
 * is the shape of the object: the container's origin is the character's
 * **feet**, so its position is where they are standing and its `zIndex` is that
 * y — the same number every prop on the floor sorts by.
 */
function makeWalker(color: Tokens['color']): { walker: Container; body: Graphics } {
  const walker = new Container()

  // The shadow stays put while the body bobs, which is what makes the bob read
  // as a step rather than as the whole character floating.
  const shade = new Graphics()
  shade.ellipse(0, 0, 15, 6).fill({ color: color.text, alpha: 0.22 })

  const body = new Graphics()
  paintBody(body, color, { coat: null, hair: null })

  walker.addChild(shade, body)
  return { walker, body }
}

/**
 * The figure, in whatever they are wearing.
 *
 * The closet's swatches land here: a null slot falls back to the token the
 * placeholder always wore. Redrawn only when the outfit changes — the walk
 * bob and the lighting tint move the Graphics, never its contents.
 */
function paintBody(
  body: Graphics,
  color: Tokens['color'],
  look: { coat: string | null; hair: string | null },
) {
  body.clear()
  body.rect(-9, -18, 7, 18).fill(color.text)
  body.rect(2, -18, 7, 18).fill(color.text)
  body.roundRect(-10, -46, 20, 32, 8).fill(look.coat ?? color.accent)
  body.circle(0, -54, 9).fill(color.paper)
  // The hair is the top of the head. Canvas angles run clockwise with y down,
  // so π → 2π is the upper half.
  if (look.hair) {
    body.moveTo(-9, -54)
    body.arc(0, -54, 9, Math.PI, 0)
    body.closePath()
    body.fill(look.hair)
  }
  body.circle(0, -54, 9).stroke({ width: 2, color: color.text })
}

function outlineOn(g: Graphics, points: readonly Point[], tint: number, width: number) {
  if (points.length === 0) return
  g.moveTo(points[0].x, points[0].y)
  for (let i = 1; i < points.length; i++) g.lineTo(points[i].x, points[i].y)
  g.lineTo(points[0].x, points[0].y)
  g.stroke({ width, color: tint, alpha: 0.9 })
}

/** The centre of a portal, which is where coming in through it puts you. */
function middle(portal: Portal): Point {
  return center(portal.area)
}

/** The centre of an authored area. */
function center(area: Area): Point {
  const { x, y, width, height } = area.bounds
  return { x: x + width / 2, y: y + height / 2 }
}

function clamp(value: number, low: number, high: number): number {
  return low > high ? (low + high) / 2 : Math.min(high, Math.max(low, value))
}

/** Fast then settling — the shape a thing arriving should have. */
function ease(t: number): number {
  return 1 - (1 - t) ** 3
}
