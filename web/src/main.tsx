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
// The chrome a challenge with a bag needs, in the design system's own tokens.
import './chrome/carrying.css'
// Last, and outside the design system on purpose — see the file's own note.
import './chrome/devbar.css'

import { App } from './App'
import { setRenderer } from './render/handle'
import { mapFor } from './render/level/locations'
import { startRenderer, type Renderer } from './render/renderer'
import { acceptEvent, chooseInChallenge, moveTo, start, stop } from './world/actions'
import { startDayCycle } from './world/clock'
import { LOOK_KEY, patch, world } from './world/store'
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
 * What this machine settled on last time is deliberately not read (§10.4).
 *
 * There is no way to choose a tier any more — see `Readout.tsx`'s `Drawing` for
 * why — so restoring one would strand anybody who had already asked for Lit on
 * a tier with no button to leave it by. Every session starts at §10.4's floor,
 * which is where every device was anyway. `readTier` stays in `quality.ts`
 * against the switch coming back.
 */

// The outfit this browser remembers (cosmetic, client-side — see `Look`).
// Restored before anybody signs in, so the figure never flashes undressed.
// The outfit an older build left under the unversioned key is worth one
// migration — `patch.look` writes it back under `LOOK_KEY`, so reading it here
// is also what carries it forward. Then the old key goes, and this is the last
// build that has to know it existed.
try {
  const worn = localStorage.getItem(LOOK_KEY) ?? localStorage.getItem('etm.look')
  if (worn) patch.look(JSON.parse(worn) as Look)
  localStorage.removeItem('etm.look')
} catch {
  // A browser refusing storage starts the outfit over. Nothing else is lost.
}

// The sky's own clock (§3.1): night from 8 PM, day from 6 AM, on the
// character's clock once there is one. Before the renderer, so the first
// frame is already drawn in the right half of the day.
const stopDayCycle = startDayCycle()

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
  // The fire, pressed. Accepting *is* entering: §4's doorway means the event
  // and the challenge behind it are settled in one transaction, so there is no
  // second button between saying yes and being in the flat.
  //
  // The offer is read out of the store rather than remembered by the renderer,
  // because by the time somebody clicks it may have lapsed — five minutes is
  // the point of it — and the honest thing to send is what the world says is
  // standing now. Nothing at all if the answer is nothing.
  onFire: () => {
    const offer = world.getState().events?.offer
    if (offer) void acceptEvent(offer.id)
  },
  // A thing in a challenge's room, taken. The run is read at the moment of the
  // press rather than remembered, because what is still on offer is the
  // server's and the drawing is only what was true a frame ago.
  onThing: (key) => {
    const run = world.getState().challenge?.run
    if (run && !run.done) void chooseInChallenge(run.challenge, run.scene, key)
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
  stopDayCycle()
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
