/*
 * What the developer's toolbar remembers about itself.
 *
 * The toolbar's controls fall into two kinds. Most of them are commands — raise
 * this conflict, reset that challenge — and a command is a thing you do once;
 * there is nothing to remember. A few are *settings*: the hand held on the
 * clock, which panel is down, whether the bar is up at all. Those describe how
 * this machine is set up to look at the game, and losing them on every reload
 * meant re-holding the sky at night a dozen times an afternoon while working on
 * anything that only happens after dark.
 *
 * So they live in `localStorage`, beside the outfit (`world/store.ts`) and for
 * the same shape of reason: the server has no opinion about them and never
 * will. Per-browser, per-origin — which is also what keeps a development
 * machine's habits out of a production build entirely.
 *
 * Everything here is best-effort. A browser refusing storage, or an entry left
 * by an older shape of this file, costs the settings and nothing else: the
 * toolbar opens shut, with the sky on its own clock, exactly as it used to.
 */

import type { Phase } from '../world/state'

/** Which panel is down, or none. */
export type Panel = 'challenges' | 'talk' | null

/** The toolbar's own state, as this browser last left it. */
export type DevSettings = {
  /** Whether the bar is up. The keystroke is still the only thing that sets it. */
  open: boolean
  /** Which panel is down under it. */
  panel: Panel
  /** The phase the sky is held at, or null for the sky's own clock. */
  clock: Phase | null
}

/**
 * The key, versioned like `LOOK_KEY` and bumped for the same reason: the reader
 * below is the only thing standing between a stale entry and a toolbar that
 * cannot render. Bump the suffix when the shape changes and the old entry
 * simply misses.
 */
const DEV_KEY = 'etm.dev:v1'

/** What a browser with nothing remembered gets — the toolbar as it always was. */
const SHUT: DevSettings = { open: false, panel: null, clock: null }

/**
 * What this browser last left the toolbar set to.
 *
 * Read once, at module load, rather than per-call: the toolbar mounts with the
 * app and the clock reads this before the first frame, so a second read would
 * only ever return what this one did.
 */
export const devSettings: DevSettings = read()

/**
 * Remember a change to the settings, merged into the rest.
 *
 * The module's own copy is updated in place, so the clock and the toolbar are
 * reading the same answer without either one subscribing to the other — this is
 * not state React renders from, it is what React *starts* from.
 */
export function rememberDev(change: Partial<DevSettings>): void {
  Object.assign(devSettings, change)
  try {
    localStorage.setItem(DEV_KEY, JSON.stringify(devSettings))
  } catch {
    // A browser refusing storage refuses the memory, not the setting.
  }
}

/** The entry, field by field — anything unrecognised falls back to `SHUT`. */
function read(): DevSettings {
  try {
    const held = localStorage.getItem(DEV_KEY)
    if (!held) return { ...SHUT }
    const said = JSON.parse(held) as Partial<DevSettings>
    return {
      open: said.open === true,
      panel: said.panel === 'challenges' || said.panel === 'talk' ? said.panel : null,
      clock: said.clock === 'day' || said.clock === 'night' ? said.clock : null,
    }
  } catch {
    return { ...SHUT }
  }
}
