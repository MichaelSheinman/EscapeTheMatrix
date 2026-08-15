import { useState } from 'react'

import { useWorld } from '../world/react'

/*
 * How a finished run went, said once.
 *
 * **The words come first, and they are the reason this card exists.** A run
 * ends by closing the screen it was played in (see App's `inChallenge`), which
 * left the reaction the content authored for the band it landed in rendered
 * into a panel that was being unmounted in the same instant — six minutes of
 * choices, a letter written for exactly this ending, and nobody ever read it.
 * An interview was the case that made it plain: you answer three questions,
 * the screen shuts, and the game never mentions that you did not get the job.
 * So `closing` is carried here, where the player is looking.
 *
 * What moved is *supporting*: a direction and a number, and nothing else. Not
 * which factor, not how much, not whether any of it was the right call —
 * §3.10's scoring is hidden and the player is never told they were right. What
 * they are told is that something moved, because a composite that changed
 * silently is a composite they cannot learn to read. The band's name still
 * never appears: the words are the whole verdict, and a label beside them
 * would grade the run.
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
  const said = run.closing.trim()
  // A run that ended with nothing to say and nothing moved has no card. That is
  // now rare and used to be the ordinary case: an ending whose only mark on the
  // world was hidden read as the game not having noticed.
  if (!said && !moved && !run.earned) return null
  if (seen === thisRun) return null

  const by = Math.round(Math.abs(moved))

  return (
    <div className="outcome">
      <div className="outcome-card">
        {said ? <p className="outcome-closing">{said}</p> : null}
        {moved ? (
          // Beside the words, what moved is a footnote. On its own — a run the
          // content wrote no ending for — it is still the whole message, and
          // keeps the size that says so.
          <p className={said ? 'outcome-moved' : 'outcome-said'}>
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
