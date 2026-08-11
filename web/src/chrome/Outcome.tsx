import { useState } from 'react'

import { useWorld } from '../world/react'

/*
 * What a finished run moved, said once.
 *
 * A direction and a number, and nothing else. Not which factor, not how much,
 * not whether any of it was the right call — §3.10's scoring is hidden and the
 * player is never told they were right. What they are told is that something
 * moved, because a composite that changed silently is a composite they cannot
 * learn to read.
 *
 * Money is shown as a figure because it is already on their balance. Karma is
 * shown as a direction because it is not a number they have ever been given.
 */
export function Outcome() {
  const run = useWorld((w) => w.challenge?.run ?? null)
  const [seen, setSeen] = useState<string | null>(null)

  if (!run?.done) return null
  /*
   * Which *run* has been acknowledged, not which challenge.
   *
   * Keyed on the code alone, pressing OK once suppressed the card for every
   * later run of the same challenge — which is invisible in normal play, where
   * a challenge happens once ever, and immediately obvious the moment the dev
   * toolbar puts one back. `began` is the run's own instant, so a replay is a
   * different key.
   */
  const thisRun = `${run.challenge}|${run.began}`
  const moved = run.karma ?? 0
  if (!moved && !run.earned) return null
  if (seen === thisRun) return null

  const by = Math.round(Math.abs(moved))

  return (
    <div className="outcome">
      <div className="outcome-card">
        {moved ? (
          <p className="outcome-said">
            Your karma {moved < 0 ? 'decreased' : 'increased'} by {by}
          </p>
        ) : null}
        {run.earned ? <p className="outcome-got">+${(run.earned / 100).toLocaleString()}</p> : null}
        <button className="outcome-ok" onClick={() => setSeen(thisRun)}>
          OK
        </button>
      </div>
    </div>
  )
}
