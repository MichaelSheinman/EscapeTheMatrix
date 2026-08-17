/*
 * Whether it is night, kept true.
 *
 * §3.1: "time of day is real and consequential", and this is the client half
 * of making it visible — the world is drawn dark between 8 PM and 6 AM on the
 * character's own clock. The zone comes from the character (the server's copy,
 * the one the rollover runs against); before hydration, the device's own zone
 * stands in, which is almost always the same answer because the device is
 * where the server learnt it (`actions.ts` sends it on every snapshot).
 *
 * A slow interval rather than anything scheduled: technical.md §3.3 rejects a
 * tick with rules in it, and this one has none — it reads a clock everybody
 * already holds and restates a boolean-shaped fact. Same cadence as the
 * phone's status bar (`screens/Phone.tsx`), for the same reason: the boundary
 * being noticed within half a minute is indistinguishable from exact.
 *
 * The tick is also what dresses the document: `<html class="night">` is the
 * hook `ui/tokens.css` re-skins the chrome by, set here so the CSS scope and
 * the store can never disagree. It goes on before the store is patched, so a
 * frame that notices the phase change re-reads tokens that have already
 * turned.
 */

import type { Phase } from './state'
import { patch, world } from './store'

/** Night begins at 20:00 on the character's clock… */
export const NIGHT_FROM = 20
/** …and ends at 06:00. */
export const DAY_FROM = 6

/** The phase a clock reading lands in. */
export function phaseAt(now: Date, zone: string | null): Phase {
  const hour = hourIn(now, zone)
  return hour >= NIGHT_FROM || hour < DAY_FROM ? 'night' : 'day'
}

/**
 * The hour (0–23) in a named zone, or on the device's clock when the name is
 * missing or not one this browser's timezone database knows.
 */
function hourIn(now: Date, zone: string | null): number {
  if (zone) {
    try {
      const said = new Intl.DateTimeFormat('en-US', {
        timeZone: zone,
        hour: 'numeric',
        hourCycle: 'h23',
      }).format(now)
      const hour = Number(said)
      if (Number.isFinite(hour)) return hour
    } catch {
      // An unknown zone name falls back to the device, exactly as the server
      // falls back to UTC — a wrong-looking sky beats no sky.
    }
  }
  return now.getHours()
}

/**
 * The developer's hand on the clock: a phase to hold, or null for the sky's
 * own.
 *
 * Nothing here reads or writes where it came from. The toolbar remembers the
 * hold per browser (`chrome/devsettings.ts`) and `main.tsx` puts it back on at
 * boot, which keeps this module what it is — the sky's clock and a hand that
 * can be laid on it — rather than a second place that knows about storage.
 */
let forced: Phase | null = null

/**
 * Hold the clock at one phase, or hand it back (null).
 *
 * The dev toolbar's override (`chrome/DevBar.tsx`). Client-side entirely,
 * because the phase is a drawing decision the server holds nothing about —
 * which is also what makes it safe to exist in any build.
 */
export function forcePhase(phase: Phase | null): void {
  forced = phase
  settle()
}

/** Read the clock (or the hand on it) and put the answer everywhere it goes. */
function settle(): void {
  const state = world.getState()
  const phase = forced ?? phaseAt(new Date(), state.character?.timezone ?? null)
  if (phase === state.phase) return
  document.documentElement.classList.toggle('night', phase === 'night')
  patch.phase(phase)
}

/**
 * Start the cycle: settle the phase now, then keep it settled.
 *
 * Called once from `main.tsx`, before the renderer starts, so the first frame
 * is already drawn in the right half of the day. Returns the stop function,
 * for the same teardown that ends everything else booted there.
 */
export function startDayCycle(): () => void {
  settle()
  const every = setInterval(settle, 20_000)
  return () => clearInterval(every)
}
