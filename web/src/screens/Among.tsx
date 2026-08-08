import { useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { answerConflict } from '../world/actions'
import { useWorld } from '../world/react'
import { Overlay } from './Overlay'

/**
 * The people you live among, and what living beside them has come to (§3.15).
 *
 * This is the screen for the mechanic the design calls the engine of the whole
 * loop. Three things are on it and they are one argument:
 *
 *  - **The friction your rules did not settle**, as a live decision. Answering
 *    it is the only command this screen has.
 *  - **Who these people are**, described entirely by what they can be relied on
 *    to do. Nothing here says what kind of place this is, because there is no
 *    such sentence in the game (§3.2) — a player works out that the same answers
 *    stop working by watching them stop working.
 *  - **How the fortnight went**, as counts. `covered` against `came to you` is
 *    the whole of "good rules buy time, and the holes in your playbook are
 *    exactly what demands your attention", and it is deliberately a pair of
 *    numbers rather than a verdict: pointing at your own evidence is the
 *    diagnosis, and the diagnosis is the player's (§3.6).
 *
 * Nothing here composes a rule. Writing the answer so the *next* one is covered
 * is the playbook screen, which is a different key and deliberately a different
 * act.
 */
export function Among({ open, onClose }: { open: boolean; onClose: () => void }) {
  const conflicts = useWorld((w) => w.conflicts)
  const [came, setCame] = useState<{ id: number; said: string } | null>(null)

  if (!conflicts?.among) return null
  const among = conflicts.among
  const waiting = conflicts.waiting

  const answer = (key: string) => {
    if (!waiting) return
    void answerConflict(waiting.id, key).then((said) => {
      if (said) setCame({ id: waiting.id, said })
    })
  }

  return (
    <Overlay open={open} onClose={onClose} title={among.name} sub={among.note}>
      <div className="grid wide-left">
        <div className="stack">
          {waiting ? (
            <div className="panel">
              <div className="panel-head">
                <span className="panel-title">{waiting.name}</span>
                <span className="panel-note">{waiting.who}</span>
              </div>
              <div className="panel-body">
                <p className="note first">{waiting.line}</p>
                {waiting.standing.length > 0 && (
                  <p className="faint tiny">{waiting.standing.join(' · ')}</p>
                )}
                <p className="faint tiny">{waiting.why}</p>
                <div className="stack">
                  {waiting.answers.map((option) => (
                    <button className="btn wide" key={option.key} onClick={() => answer(option.key)}>
                      {option.does}
                    </button>
                  ))}
                </div>
                <p className="faint tiny">
                  It happened on {waiting.on}. Nothing else with these people comes up until
                  this is dealt with.
                </p>
              </div>
            </div>
          ) : (
            came && (
              // What was just settled, held on screen until something else
              // happens here. The consequence is the only account of it the
              // player gets, and taking it away the moment the panel
              // re-rendered would be the game saying something and then not
              // letting them read it.
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">What came of it</span>
                </div>
                <div className="panel-body">
                  <p className="note first last">{came.said}</p>
                </div>
              </div>
            )
          )}

          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">What they have in common</span>
              <span className="panel-note">read off how they behave</span>
            </div>
            <div className="panel-body">
              {among.holds.map((held, i) => (
                <p className={`note${i === 0 ? ' first' : ''}`} key={held}>
                  {held}
                </p>
              ))}
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">The people</span>
              <span className="panel-note">since {among.since}</span>
            </div>
            <div className="panel-body">
              {among.people.map((person) => (
                <div className="metric-row" key={person.key}>
                  <div>
                    <div className="metric-name">{person.name}</div>
                    <div className="metric-sub">{person.does}</div>
                    {person.standing.length > 0 && (
                      <div className="metric-sub">{person.standing.join(' · ')}</div>
                    )}
                  </div>
                  <div className="metric-value">
                    {person.met === 0 ? '—' : person.met}
                  </div>
                </div>
              ))}
              <p className="note last">
                The number is how many times something came up with them in the last fortnight.
              </p>
            </div>
          </div>

          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">What keeps coming up</span>
              <span className="panel-note">counted, not graded</span>
            </div>
            <div className="panel-body">
              {among.questions.length === 0 ? (
                <p className="note first last">
                  Nothing has come up yet. It will, and this is where a fortnight of it adds up.
                </p>
              ) : (
                <>
                  {among.questions.map((q) => (
                    <div className="metric-row" key={q.question}>
                      <div>
                        <div className="metric-name">{q.ask}</div>
                        <div className="metric-sub">
                          {q.covered} of {q.met} settled by your rules
                          {q.poor > 0 ? ` · ${q.poor} went badly` : ''}
                        </div>
                      </div>
                      <div className="metric-value">{q.met}</div>
                    </div>
                  ))}
                  <p className="note last">
                    Anything your rules did not cover came to you instead. One bad morning is
                    nothing; the same one every morning for a week is something you can act on.
                  </p>
                </>
              )}
            </div>
          </div>
        </div>
      </div>
    </Overlay>
  )
}

/**
 * The people you live among, in the key bar (§3.15).
 *
 * The one thing worth interrupting somebody for here is friction they have not
 * dealt with, because nothing else with these people happens until they do. The
 * label reads differently rather than a badge appearing — the shell has no
 * notification area, and the key bar saying something else *is* how this world
 * tells you something is there.
 */
export function AmongHint() {
  const conflicts = useWorld((w) => w.conflicts)
  if (!conflicts?.among) return null
  if (conflicts.waiting) {
    return <KeyHint keys={['A']}>{conflicts.waiting.name.toLowerCase()}</KeyHint>
  }
  return <KeyHint keys={['A']}>{conflicts.among.name.toLowerCase()}</KeyHint>
}
