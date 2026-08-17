/*
 * A person, drawn once.
 *
 * At this size a figure needs a silhouette that reads: proportions, a gesture,
 * and enough light on one side to say which way the sun is. A face is four
 * pixels of nothing from across the street.
 *
 * One drawing, two surfaces. The street draws with Pixi and the closet's mirror
 * with Canvas2D, and a coat that looks right in the mirror and wrong from across
 * the road is the closet lying to you. Both go through `draw` below, over a
 * `Pen` thin enough that each surface is thirty lines of adapter.
 *
 * The figure stands at the origin on its feet, in plate pixels — the same units
 * the location is painted in, so a figure on the street is the size the painter
 * drew everybody else at. Nothing here knows where it is, what it is standing
 * on, or whether it is the player.
 */

/**
 * The drawing surface, as much of one as a figure needs.
 *
 * Deliberately smaller than either API underneath it. Anything a figure cannot
 * be drawn with — a clip region, a gradient, a shadow — is a thing that would
 * not survive the trip to the other surface.
 */
export type Pen = {
  /** Start a fresh path. */
  begin(): void
  moveTo(x: number, y: number): void
  lineTo(x: number, y: number): void
  quadTo(cx: number, cy: number, x: number, y: number): void
  /** Angles in radians, clockwise, y down — canvas's own convention. */
  arc(x: number, y: number, radius: number, from: number, to: number): void
  ellipse(x: number, y: number, rx: number, ry: number): void
  close(): void
  fill(color: number, alpha?: number): void
  stroke(color: number, width: number, alpha?: number): void
}

/** What a figure is made of. Colours, and nothing about where it is. */
export type Figure = {
  /** The coat, which is the whole of the torso at this size. */
  coat: number
  skin: number
  hair: number | null
  hat: number | null
  bag: number | null
  /** The warm edge the sun leaves down the lit side. */
  rim: number
}

/** What a figure is doing. */
export type Pose = {
  /** The walk, between -1 and 1. Zero is standing still. */
  step: number
  /** Which way they are going: 1 right, -1 left. Only the bag can tell. */
  facing: number
  /**
   * Mid-argument, as a swing between -1 and 1, or null for arms down.
   *
   * The diner argument is the reason this exists: §10.3's cast is authored
   * where it stands, and the only thing that says two people are arguing rather
   * than queueing is what their arms are doing.
   */
  gesture: number | null
}

/** Feet to crown, in plate pixels. What the caller scales against. */
export const FIGURE_HEIGHT = 64

/**
 * The skin every figure is drawn in.
 *
 * Nobody has picked one — there is no drawer for it, and #36 is where the
 * cast's own come from. Until then everybody is the tone the preview's figures
 * are, which is the one the plates were painted beside.
 */
export const SKIN = 0xc39468

/**
 * The build every figure is drawn on.
 *
 * One build for now, because the player is the only figure the client draws.
 * The cast (#36) arrives with the rest of `E.actor`'s — a labourer's shoulders,
 * a gown with its hem on the floor — and they belong here beside this one.
 */
const BUILD = { hip: 8, shoulder: 9, arm: 5, leg: 6 }

/**
 * Which side the sun is on: 1 is lit from the right. A constant rather than a
 * parameter, because the plate was painted with this sun and a figure carrying
 * its own would be a second light in the same street. It is also why a figure
 * turning round does not flip — mirroring would swing the rim light across.
 */
const SUN = 1

export function draw(pen: Pen, figure: Figure, pose: Pose): void {
  const B = BUILD
  const { step } = pose
  const coat = figure.coat
  const shadeSide = shade(coat, 0.72)

  // Legs. Round caps, because a leg at six pixels wide is a line and a line
  // with a square end reads as a plank.
  pen.begin()
  pen.moveTo(0, -22)
  pen.lineTo(step * 6, -1)
  pen.stroke(coat, B.leg)
  pen.begin()
  pen.moveTo(0, -22)
  pen.lineTo(-step * 6, -1)
  pen.stroke(coat, B.leg)

  // Torso: hips out to shoulders and over the top, in two curves.
  pen.begin()
  pen.moveTo(-B.hip, -22)
  pen.quadTo(-B.shoulder, -46, 0, -48)
  pen.quadTo(B.shoulder, -46, B.hip, -22)
  pen.close()
  pen.fill(coat)

  // Arms, swinging against the legs — or up, if this is an argument.
  const shoulder = B.hip - 1
  if (pose.gesture !== null) {
    const g = pose.gesture
    pen.begin()
    pen.moveTo(shoulder, -43)
    pen.lineTo(shoulder + 8 + g * 5, -50 - g * 7)
    pen.stroke(coat, B.arm)
    pen.begin()
    pen.moveTo(-shoulder, -43)
    pen.lineTo(-shoulder - 6, -28)
    pen.stroke(coat, B.arm)
  } else {
    pen.begin()
    pen.moveTo(shoulder, -43)
    pen.lineTo(shoulder + 1 - step * 4, -25)
    pen.stroke(coat, B.arm)
    pen.begin()
    pen.moveTo(-shoulder, -43)
    pen.lineTo(-shoulder - 1 + step * 4, -25)
    pen.stroke(coat, B.arm)
  }

  // The bag, worn across. A strap and a brim read from across the street where a
  // face never will, which is why the closet offers silhouette rather than
  // detail (§12).
  if (figure.bag !== null) {
    const side = pose.facing < 0 ? -1 : 1
    pen.begin()
    pen.moveTo(-6 * side, -46)
    pen.lineTo(7 * side, -28)
    pen.stroke(shade(figure.bag, 0.72), 2.6)
    pen.begin()
    pen.ellipse(10 * side, -25, 6, 5.4)
    pen.fill(figure.bag)
    pen.begin()
    pen.ellipse(11 * side, -27, 3.4, 2.4)
    pen.fill(0xffffff, 0.12)
  }

  // The head, and the hair over the top of it. Canvas angles run clockwise
  // with y down, so π → 2π is the upper half.
  pen.begin()
  pen.arc(0, -56, 8, 0, Math.PI * 2)
  pen.fill(figure.skin)
  pen.begin()
  pen.arc(0, -58, 8, Math.PI * 0.95, Math.PI * 2.15)
  pen.close()
  pen.fill(figure.hair ?? shade(figure.skin, 0.42))

  if (figure.hat !== null) {
    pen.begin()
    pen.arc(0, -59, 8.4, Math.PI * 0.94, Math.PI * 2.06)
    pen.close()
    pen.fill(figure.hat)
    pen.begin()
    pen.ellipse(2, -58.5, 11.5, 2.4)
    pen.fill(figure.hat)
    pen.begin()
    pen.ellipse(3, -60, 5.5, 1.6)
    pen.fill(figure.rim, 0.22)
  }

  // The shaded half, away from the sun. Half the torso redrawn darker rather
  // than a clip region, because a clip is the one thing the two surfaces spell
  // differently.
  pen.begin()
  pen.moveTo(-SUN * B.hip, -22)
  pen.quadTo(-SUN * B.shoulder, -46, 0, -48)
  pen.lineTo(0, -22)
  pen.close()
  pen.fill(shadeSide, 0.55)

  // The rim down the lit side, and around the top of the head. It is what makes
  // a flat shape a person standing outdoors.
  pen.begin()
  pen.moveTo(SUN * B.hip, -23)
  pen.quadTo(SUN * B.shoulder, -46, 0, -48)
  pen.stroke(figure.rim, 1.8, 0.8)
  pen.begin()
  pen.arc(0, -56, 8, SUN > 0 ? -1.5 : 1.7, SUN > 0 ? 0.5 : 3.6)
  pen.stroke(figure.rim, 1.8, 0.8)
}

/** The shadow a figure stands in. Drawn by the caller, because it never bobs. */
export function shadow(pen: Pen, ink: number): void {
  pen.begin()
  pen.ellipse(0, 0, 15, 6)
  pen.fill(ink, 0.22)
}

/** A colour, darkened. `f` of 1 is unchanged; 0 is black. */
export function shade(color: number, f: number): number {
  const r = Math.min(255, Math.round(((color >> 16) & 0xff) * f))
  const g = Math.min(255, Math.round(((color >> 8) & 0xff) * f))
  const b = Math.min(255, Math.round((color & 0xff) * f))
  return (r << 16) | (g << 8) | b
}

/** A `#rrggbb` the closet stores, as the number everything here draws with. */
export function paint(css: string | null, fallback: number): number {
  if (!css) return fallback
  const hex = /^#?([0-9a-f]{6})$/i.exec(css.trim())
  return hex ? parseInt(hex[1], 16) : fallback
}

/* ── the surfaces ───────────────────────────────────────────────────────── */

/**
 * A pen over a 2D canvas. Here rather than beside the closet, because a surface
 * that spelled `quadTo` differently would be a second figure. Pixi's adapter is
 * in `scene.ts`, where Pixi is allowed to be (§10.4).
 */
export function canvasPen(ctx: CanvasRenderingContext2D): Pen {
  const css = (color: number, alpha = 1) => {
    const r = (color >> 16) & 0xff
    const g = (color >> 8) & 0xff
    const b = color & 0xff
    return `rgba(${r},${g},${b},${alpha})`
  }
  return {
    begin: () => ctx.beginPath(),
    moveTo: (x, y) => ctx.moveTo(x, y),
    lineTo: (x, y) => ctx.lineTo(x, y),
    quadTo: (cx, cy, x, y) => ctx.quadraticCurveTo(cx, cy, x, y),
    arc: (x, y, r, from, to) => ctx.arc(x, y, r, from, to),
    ellipse: (x, y, rx, ry) => ctx.ellipse(x, y, rx, ry, 0, 0, Math.PI * 2),
    close: () => ctx.closePath(),
    fill: (color, alpha) => {
      ctx.fillStyle = css(color, alpha)
      ctx.fill()
    },
    stroke: (color, width, alpha) => {
      ctx.strokeStyle = css(color, alpha)
      ctx.lineWidth = width
      ctx.lineCap = 'round'
      ctx.lineJoin = 'round'
      ctx.stroke()
    },
  }
}
