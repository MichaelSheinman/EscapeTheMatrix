/*
 * Draw one scene, whole, onto one canvas — the preview's own painter pointed
 * at a plate instead of a viewport — and hand the result over as WebP.
 *
 * The state object S is the same shape preview/src/app.js feeds E.Painter
 * every frame. Here the "viewport" is the entire plate, the camera sits at its
 * centre, the clock is stopped at t=0, and nobody is standing in the street:
 * people are live, so people are never baked.
 */

import { SCALES, frameFor } from './frame.mjs'

import '../../preview/src/render.js'
import '../../preview/src/paint.js'
import '../../preview/src/people.js'
import '../../preview/src/scenes.js'

const E = window.ETM
const status = document.getElementById('status')

/** The shipped file name per scene, matching gen-tmj.mjs's output. */
const FILES = {
  street: 'east-side',
  apartment: 'east-side-apartment',
}

async function bake(sceneKey) {
  const scene = E.SCENES[sceneKey]
  const frame = frameFor(scene)
  const scale = SCALES[sceneKey]
  const W = frame.width
  const H = frame.height

  const canvas = document.createElement('canvas')
  canvas.width = Math.ceil(W * scale)
  canvas.height = Math.ceil(H * scale)
  const ctx = canvas.getContext('2d')

  // The camera at which "screen" equals "plate": with zoom 1 and the eye at
  // the plate's centre-minus-offset, the painter's world→screen transform is
  // exactly world + offset.
  const S = {
    ctx,
    scene,
    t: 0,
    W,
    H,
    cam: { x: W / 2 - frame.offset.x, y: H / 2 - frame.offset.y },
    zoom: 1,
    quality: 'high',
    emissive: [],
    actors: [],
    // Far off the plate: drawObjects always paints the player, so the bake
    // stands them somewhere no plate will ever show.
    player: { x: -9999, y: -9999, color: '#000' },
  }
  const P = E.Painter(S)

  // The same order drawPlate runs, minus the live-only layers (motes, people).
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  S.emissive.length = 0
  P.drawSky()
  ctx.translate(S.W / 2, S.H / 2)
  ctx.translate(-S.cam.x, -S.cam.y)
  P.drawGround()
  P.drawShadows()
  P.drawObjects()
  P.drawEmissive()
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  P.sunWash()

  const blob = await new Promise((ready) => canvas.toBlob(ready, 'image/webp', 0.9))
  if (!blob) throw new Error('the canvas would not encode WebP')

  // Hand the plate straight back to serve.mjs, which writes it into
  // web/public/locations/<key>/plate.webp — the whole bake, no hands.
  const put = await fetch(`/plate/${FILES[sceneKey]}`, { method: 'PUT', body: blob })

  status.textContent =
    `${FILES[sceneKey]}: ${canvas.width}×${canvas.height}, ${(blob.size / 1024 / 1024).toFixed(2)} MB — ` +
    (put.ok
      ? `saved to web/public/locations/${FILES[sceneKey]}/plate.webp`
      : `the server refused it (${put.status}) — is serve.mjs running?`)

  const shown = document.createElement('img')
  shown.src = URL.createObjectURL(blob)
  document.body.appendChild(shown)
}

document.body.addEventListener('click', (e) => {
  const button = e.target.closest('[data-bake]')
  if (!button) return
  status.textContent = 'Baking…'
  // Let the status paint before the long draw.
  setTimeout(() => {
    bake(button.getAttribute('data-bake')).catch((err) => {
      status.textContent = String(err)
    })
  }, 30)
})

// ?bake=street bakes on load, so the whole run can be scripted.
const asked = new URLSearchParams(location.search).get('bake')
if (asked && FILES[asked]) {
  setTimeout(() => {
    bake(asked).catch((err) => {
      status.textContent = String(err)
    })
  }, 60)
}
