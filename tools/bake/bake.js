/*
 * Draw one scene, whole, onto one canvas — the preview's own painter pointed
 * at a plate instead of a viewport — and hand the result over as WebP.
 *
 * The state object S is the same shape preview/src/app.js feeds E.Painter
 * every frame. Here the "viewport" is the entire plate, the camera sits at its
 * centre, the clock is stopped at t=0, and nobody is standing in the street:
 * people are live, so people are never baked.
 *
 * Each scene bakes **two** pictures, and the second one is what gives the
 * painting depth:
 *
 *   - **plate.webp** — the location, whole and opaque. What is drawn under
 *     everybody.
 *   - **props.webp** — the same picture with the sky, the ground and the
 *     shadows left out, so everything a character can walk behind stands on
 *     transparency. `scene.ts` cuts its occluders out of this one, which is why
 *     a tree occludes as a tree and not as the rectangle it is authored in.
 *
 * The two are drawn by the same painter in the same transform, so a cut-out
 * lands pixel-exact over the plate underneath it.
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
  riverfront: 'riverfront',
  quayroom: 'riverfront-rooms',
  // Not a city place — a room a challenge stands you in. It bakes exactly like
  // the other four, because a plate does not care whether you can walk there.
  housefire: 'house-fire',
}

/** A canvas at plate size, and the painter state that draws the scene onto it. */
function easel(sceneKey) {
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
  return { canvas, ctx, scale, S, P: E.Painter(S) }
}

/** The whole location, opaque — the picture everything is drawn on top of. */
function paintPlate({ ctx, scale, S, P }) {
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
}

/**
 * Everything standing on the ground, on transparency.
 *
 * The plate's passes minus the two that are the ground itself — the sky and
 * the ground — and minus the shadows, which lie *on* the floor and must never
 * occlude somebody standing in front of them.
 *
 * `sunWash` is a full-screen add, so on a sheet that is mostly nothing it would
 * tint the nothing. It is drawn anyway, because the plate has it and a cut-out
 * a shade cooler than the picture it sits on shows as a rectangle — and then
 * the silhouette taken before the wash is composited back as the alpha, which
 * puts the wash inside the props and takes it off everything else.
 */
function paintProps({ canvas, ctx, scale, S, P }) {
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  ctx.clearRect(0, 0, canvas.width / scale, canvas.height / scale)
  S.emissive.length = 0
  ctx.translate(S.W / 2, S.H / 2)
  ctx.translate(-S.cam.x, -S.cam.y)
  P.drawObjects()
  P.drawEmissive()
  ctx.setTransform(scale, 0, 0, scale, 0, 0)

  const silhouette = document.createElement('canvas')
  silhouette.width = canvas.width
  silhouette.height = canvas.height
  silhouette.getContext('2d').drawImage(canvas, 0, 0)

  P.sunWash()

  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.globalCompositeOperation = 'destination-in'
  ctx.drawImage(silhouette, 0, 0)
  ctx.globalCompositeOperation = 'source-over'
}

async function put(canvas, picture, key) {
  const blob = await new Promise((ready) => canvas.toBlob(ready, 'image/webp', 0.9))
  if (!blob) throw new Error('the canvas would not encode WebP')
  const saved = await fetch(`/${picture}/${key}`, { method: 'PUT', body: blob })
  if (!saved.ok) throw new Error(`the server refused ${picture} (${saved.status}) — is serve.mjs running?`)

  const shown = document.createElement('img')
  shown.src = URL.createObjectURL(blob)
  document.body.appendChild(shown)
  return `${picture}.webp ${canvas.width}×${canvas.height}, ${(blob.size / 1024 / 1024).toFixed(2)} MB`
}

async function bake(sceneKey) {
  const key = FILES[sceneKey]
  const said = []

  const plate = easel(sceneKey)
  paintPlate(plate)
  said.push(await put(plate.canvas, 'plate', key))

  // A fresh easel rather than the same one cleared: the painter accumulates
  // emissive geometry per pass, and a props sheet is a different pass.
  const props = easel(sceneKey)
  paintProps(props)
  said.push(await put(props.canvas, 'props', key))

  status.textContent = `${key}: ${said.join(' · ')} — saved to web/public/locations/${key}/`
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
