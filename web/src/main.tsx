/*
 * Boot.
 *
 * Three things start here, in this order, and none of them is inside a
 * component (technical.md §10.1):
 *
 *   1. **React**, immediately, so the door is on screen while the rest of boot
 *      is still in flight.
 *   2. **The world**: who this is, what is true, and the socket that keeps it
 *      true. Outside React because a connection belongs to the app rather than
 *      to whatever is currently rendered.
 *   3. **The renderer**, once, for the life of the page. Never created by a
 *      mount and never destroyed by an unmount.
 *
 * This file is also the only place the two trees are wired to each other, and
 * the wiring is two lines: the renderer reads `world.getState()` every frame,
 * and it writes back exactly one thing — where in the painted location the
 * character ended up standing (§10.3). There is no store the canvas can see
 * that React cannot, and no second path to the server.
 *
 * §10.1's "canvas input dispatches through the same command layer" still holds
 * and is still the rule; it is simply not exercised yet. A tap on the world is
 * a walk across one location, which changes nothing the server holds. Stepping
 * *between* locations does, and that is the command #12 wires the authored
 * portals to.
 */

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@etm/ui/index.css'
import './skeleton.css'

import { App } from './App'
import { startRenderer, type Renderer } from './render/renderer'
import { start, stop } from './world/actions'
import { patch, world } from './world/store'

const root = document.getElementById('root')
const canvas = document.getElementById('world')
if (!root) throw new Error('#root is missing from index.html')
if (!(canvas instanceof HTMLCanvasElement)) {
  throw new Error('#world is missing from index.html')
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

void start()

/** The one renderer, held only so that teardown can end it. */
let renderer: Renderer | null = null

void startRenderer({
  canvas,
  // Read, not subscribed to. The renderer runs at its own cadence and takes
  // whatever the world says at the moment each frame is drawn.
  read: () => world.getState(),
  // The one thing the world layer puts into the store: which authored trigger
  // zones the character is inside (§10.3). Reported only when it changes.
  onStanding: patch.standing,
}).then(
  (started) => {
    renderer = started
    patch.renderer('running')
    window.addEventListener('keydown', logicKey)
  },
  (err) => {
    // WebGL2 is a hard floor (§10.4). This is not a degradation to paper over —
    // the store carries it and the UI says so plainly.
    console.error('the renderer could not start', err)
    patch.renderer('unsupported')
  },
)

/**
 * Backquote shows the layer that is never drawn.
 *
 * A development view of the walkable polygon, the trigger zones, the portals,
 * the depth baselines and the graph routes are found over (§10.3). It is bound
 * here rather than in `App.tsx` because it belongs to the renderer, not to a
 * screen — and because the world stays visible when every panel is closed.
 */
let logic = false

function logicKey(e: KeyboardEvent) {
  if (e.key !== '`' || e.target instanceof HTMLInputElement) return
  logic = !logic
  renderer?.showLogic(logic)
}

/** The far end of §10.1's lifecycle: created at boot, destroyed here. */
function teardown() {
  stop()
  window.removeEventListener('keydown', logicKey)
  renderer?.destroy()
  renderer = null
}

// `pagehide` rather than `beforeunload`, which is the one browsers are removing
// — and only when the page is really going away. A page put into the back /
// forward cache is coming back with all of this still alive, and the socket
// client already reconnects by itself when the tab is shown again.
window.addEventListener('pagehide', (e) => {
  if (!e.persisted) teardown()
})

// The dev-server equivalent: a module replaced under a running page would
// otherwise leave the old socket and the old WebGL context behind the new ones.
import.meta.hot?.dispose(() => teardown())
