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
 * the wiring is one line each: the renderer reads `world.getState()`, and a tap
 * on the canvas calls the same `probe` the panel's button calls. That is the
 * whole of §10.1's "canvas input dispatches through the same command layer" —
 * there is no second path to the server, and no store the canvas can see that
 * React cannot.
 */

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@etm/ui/index.css'
import './skeleton.css'

import { App } from './App'
import { startRenderer, type Renderer } from './render/renderer'
import { probe, start, stop } from './world/actions'
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
  onTap: () => {
    // Nothing to probe for somebody who is not signed in, and the command layer
    // would refuse it anyway (§9). The scrim over the door already swallows the
    // click; this is the second half of that, said where it is a rule rather
    // than a side effect of the layout.
    if (world.getState().session.status !== 'in') return
    // Unlabelled: a label has to be unique, and the world is not a place to
    // discover that by being refused. The panel is where labels are typed.
    void probe('')
  },
}).then(
  (started) => {
    renderer = started
    patch.renderer('running')
  },
  (err) => {
    // WebGL2 is a hard floor (§10.4). This is not a degradation to paper over —
    // the store carries it and the UI says so plainly.
    console.error('the renderer could not start', err)
    patch.renderer('unsupported')
  },
)

/** The far end of §10.1's lifecycle: created at boot, destroyed here. */
function teardown() {
  stop()
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
