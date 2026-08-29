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
  street: 'wilmington',
  apartment: 'wilmington-apartment',
  washroom: 'wilmington-apartment-washroom',
  riverfront: 'riverfront',
  quayroom: 'riverfront-rooms',
  // Not a city place — a room a challenge stands you in. It bakes exactly like
  // the other four, because a plate does not care whether you can walk there.
  housefire: 'house-fire',
}

/**
 * A canvas at plate size, and the painter state that draws the scene onto it.
 *
 * With `night`, the scene is the same geometry under the scene's authored
 * `night` block — sky, sun and grade swapped in, `day` off — which is the
 * whole difference between the two plates: nothing moves, the light changes.
 */
function easel(sceneKey, night = false) {
  const base = E.SCENES[sceneKey]
  const scene = night ? Object.assign({}, base, base.night, { night: true, day: false }) : base
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

/**
 * The dark, multiplied over a finished sheet.
 *
 * A vertical gradient from the scene's `night.grade`: colder up where the sky
 * ends, warmer down where the lamps work. Multiply paints its own colour
 * wherever the sheet is transparent, which is why both callers mask with a
 * silhouette afterwards — the same trick paintProps plays with sunWash.
 */
function grade(ctx, S) {
  const g = ctx.createLinearGradient(0, 0, 0, S.H)
  g.addColorStop(0, S.scene.grade[0])
  g.addColorStop(1, S.scene.grade[1])
  ctx.save()
  ctx.globalCompositeOperation = 'multiply'
  ctx.fillStyle = g
  ctx.fillRect(0, 0, S.W, S.H)
  ctx.restore()
}

/**
 * The night plate: the authored night sky untouched, the world graded dark
 * over its own silhouette, and the lights drawn last so they are what
 * survives the dark — lit windows, lamp heads, the pools under them.
 */
function paintNightPlate({ canvas, ctx, scale, S, P }) {
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  S.emissive.length = 0
  P.drawSky()

  // The world on its own sheet, so the grade cannot touch the sky behind it.
  const sheet = document.createElement('canvas')
  sheet.width = canvas.width
  sheet.height = canvas.height
  const world = sheet.getContext('2d')
  S.ctx = world
  world.setTransform(scale, 0, 0, scale, 0, 0)
  world.translate(S.W / 2, S.H / 2)
  world.translate(-S.cam.x, -S.cam.y)
  P.drawGround()
  P.drawShadows()
  P.drawObjects()

  const silhouette = document.createElement('canvas')
  silhouette.width = sheet.width
  silhouette.height = sheet.height
  silhouette.getContext('2d').drawImage(sheet, 0, 0)

  world.setTransform(scale, 0, 0, scale, 0, 0)
  grade(world, S)
  world.setTransform(1, 0, 0, 1, 0, 0)
  world.globalCompositeOperation = 'destination-in'
  world.drawImage(silhouette, 0, 0)
  world.globalCompositeOperation = 'source-over'

  S.ctx = ctx
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.drawImage(sheet, 0, 0)

  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  ctx.translate(S.W / 2, S.H / 2)
  ctx.translate(-S.cam.x, -S.cam.y)
  P.drawEmissive()
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
}

/**
 * The night props: the same passes the night plate ran over its world sheet —
 * objects, grade, lights — masked back to the object silhouette, so a cut-out
 * lands pixel-exact over the plate underneath it, dark for dark.
 */
function paintNightProps({ canvas, ctx, scale, S, P }) {
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  ctx.clearRect(0, 0, canvas.width / scale, canvas.height / scale)
  S.emissive.length = 0
  ctx.translate(S.W / 2, S.H / 2)
  ctx.translate(-S.cam.x, -S.cam.y)
  P.drawObjects()

  const silhouette = document.createElement('canvas')
  silhouette.width = canvas.width
  silhouette.height = canvas.height
  silhouette.getContext('2d').drawImage(canvas, 0, 0)

  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  grade(ctx, S)
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  ctx.translate(S.W / 2, S.H / 2)
  ctx.translate(-S.cam.x, -S.cam.y)
  P.drawEmissive()

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

  // The same place after dark, when the scene has authored one. The client
  // asks for the `-night` pair between 8 PM and 6 AM (renderer.ts) and falls
  // back to the day pair where no scene authored a night.
  if (E.SCENES[sceneKey].night) {
    const nightPlate = easel(sceneKey, true)
    paintNightPlate(nightPlate)
    said.push(await put(nightPlate.canvas, 'plate-night', key))

    const nightProps = easel(sceneKey, true)
    paintNightProps(nightProps)
    said.push(await put(nightProps.canvas, 'props-night', key))
  }

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
