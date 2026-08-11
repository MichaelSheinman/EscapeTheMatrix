/*
 * The one transform the bake pipeline shares.
 *
 * The bake page draws a scene onto a canvas, and the map generator writes the
 * logic layer over the same picture — so both must agree, to the pixel, on
 * where the scene sits on the plate. This module is that agreement: the iso
 * projection at its authored tile size, the margins around the drawing, and
 * the scale each location ships at.
 *
 * Pure and environment-free: imported by a Node script and by a page in a
 * browser, and it must never need to care which.
 */

export const TW = 64
export const TH = 32

/** Plate pixels per world pixel, per scene. Interiors ship at 2× for crispness. */
export const SCALES = { street: 1, apartment: 2, riverfront: 1, quayroom: 2, housefire: 2 }

/** The classic 2:1 diamond: the top corner of tile (x, y). */
export function iso(x, y) {
  return { x: (x - y) * (TW / 2), y: (x + y) * (TH / 2) }
}

/** The centre of tile (x, y) — where a thing standing on it stands. */
export function isoc(x, y) {
  const p = iso(x, y)
  return { x: p.x, y: p.y + TH / 2 }
}

/**
 * Where a scene sits on its plate, before scaling.
 *
 * `offset` maps world pixels (the iso projection of the grid) to plate pixels:
 * plate = (world + offset) * scale. The margins are generous rather than
 * exact — a roofline poking off the top of the plate is a bug the bake cannot
 * see, and forty spare pixels of sky is not.
 */
export function frameFor(scene) {
  const rows = scene.rows.length
  const cols = scene.rows[0].length

  // The grid's own extent in world pixels.
  const minX = iso(0, rows).x
  const maxX = iso(cols, 0).x
  const maxY = iso(cols, rows).y

  // The tallest thing standing on it, with room for roofs, towers and crowns.
  let above = 0
  for (const b of scene.boxes ?? []) above = Math.max(above, b.h)
  const minY = -(above + 190)

  const pad = 70
  const offset = { x: -(minX - pad), y: -(minY - 40) }
  return {
    offset,
    width: Math.ceil(maxX - minX + pad * 2),
    height: Math.ceil(maxY - minY + 40 + pad),
  }
}
