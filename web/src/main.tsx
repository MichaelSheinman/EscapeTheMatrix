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
 * §10.1's "canvas input dispatches through the same command layer" is the third
 * line, and it is the one the location graph turned on. A tap on the world is a
 * walk across one location, which changes nothing the server holds; walking onto
 * an authored portal is a journey, and it goes out through `moveTo` — the same
 * function the city map's buttons call.
 */

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@etm/ui/index.css'
import './skeleton.css'

import { App } from './App'
import { setRenderer } from './render/handle'
import { mapFor } from './render/level/locations'
import { readTier, shelf } from './render/quality'
import { startRenderer, type Renderer } from './render/renderer'
import { moveTo, start, stop } from './world/actions'
import { patch, world } from './world/store'
import type { Look } from './world/state'

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

/*
 * What this machine settled on last time (§10.4).
 *
 * Before the renderer starts, so the first frame is drawn at the tier the player
 * asked for rather than at the floor and then switched — building the High tier
 * a second into a session is a visible hitch for no reason. A device that has
 * never said anything stays at Low, which is where §10.4 puts everything.
 */
const remembered = readTier(shelf())
if (remembered) patch.quality(remembered, true)

// The outfit this browser remembers (cosmetic, client-side — see `Look`).
// Restored before anybody signs in, so the figure never flashes undressed.
try {
  const worn = localStorage.getItem('etm.look')
  if (worn) patch.look(JSON.parse(worn) as Look)
} catch {
  // A browser refusing storage starts the outfit over. Nothing else is lost.
}

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
  // The one thing it sends *out*: walking onto a door is asking to go through
  // it (§3.12). On foot, always — a portal is a door, and there has never been
  // a doorway you could ride a train through. The fare is a decision, and a
  // decision belongs on the city map rather than under somebody's feet.
  //
  // A door to somewhere nobody has built yet does not open. The place exists
  // — the city is allowed to be larger than the client — but a player who
  // stepped through would be standing in nothing, with nothing to walk back
  // through. The marker over the door says "not built yet" for the same rule.
  onPortal: (to) => {
    if (mapFor(to)) void moveTo(to, 'foot')
  },
  // How fast the world is drawing, and — if it turns out this machine cannot
  // carry the tier it was asked for — the world saying so and going back to the
  // floor (§10.4). Both are facts about the device, so both land in the store
  // beside the one that says whether the renderer started at all.
  onFrames: patch.frames,
  onDemoted: patch.demoted,
}).then(
  (started) => {
    renderer = started
    setRenderer(started)
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
  setRenderer(null)
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
