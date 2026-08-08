import { useState } from 'react'

import { liveByRoutine, setRoutines } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type Routine } from '../world/state'
import { Overlay } from './Overlay'
import { Choices } from './Playbook'

/**
 * Your routines: the rules you have set for your own body (§3.4).
 *
 * The screen is built out of `@etm/ui`'s **rule** vocabulary rather than out of
 * form controls, and that is the design rather than a styling choice. §3.14 says
 * routines and the playbook are one system with one interface language — a
 * routine already *is* a rule — so each one reads here as a standing question
 * and a sentence answering it, which is exactly the shape the playbook screen
 * will compose in. Nothing on this surface takes typing and nothing on it is a
 * number.
 *
 * The whole configuration is one statement. Answers are drafted locally and sent
 * together, because a set of rules for the body is a single decision — deciding
 * to sleep more is deciding what that costs the evening — and the server writes
 * it as one unit for the same reason.
 */
export function Rules({ open, onClose }: { open: boolean; onClose: () => void }) {
  const routines = useWorld((w) => w.routines)
  const health = useWorld((w) => w.health)
  const busy = useWorld(sending)

  // Which routine has its list open — the same one-menu-at-a-time state the
  // playbook's sentence keeps, and a fact about this tab.
  const [menu, setMenu] = useState<string | null>(null)

  if (!routines) return null

  /**
   * Pick an answer. The whole set goes at once — the server writes the rules
   * as one unit — with this one answer different, so a pick *is* the change:
   * no draft copy, and nothing to press afterwards.
   */
  function pick(key: string, option: string) {
    if (!routines || busy) return
    setMenu(null)
    const choices: Record<string, string> = {}
    for (const r of routines.routines) choices[r.key] = r.key === key ? option : r.chosen
    void setRoutines(choices)
  }

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Your routines"
      sub="Rules for the body — the same object as the rest of the playbook, with the same
            interface: the rule is a sentence, and changing a word of it is the change. Nothing
            here is a dial: what you can put in a routine is what you can think of arranging."
      // A click anywhere that is not a menu closes the one that is open,
      // exactly as the playbook's sentence behaves.
      onBodyClick={() => setMenu(null)}
    >
      <div className="grid wide-left">
        <div className="stack">
          {routines.routines.map((r, i) => (
            <RoutineCard
              key={r.key}
              n={i + 1}
              routine={r}
              open={menu === r.key}
              onOpen={() => setMenu(menu === r.key ? null : r.key)}
              onPick={(option) => pick(r.key, option)}
            />
          ))}

          {!routines.set && (
            <p className="faint tiny">
              You have never stated these, so your days are whatever happens — which is itself
              a set of rules, and the sentences above are what it looks like. Change any word
              and it is in force.
            </p>
          )}
        </div>

        <div className="stack">
          {/* No forecast here, deliberately. What the rules will do to the
              body is the player's to find out in retrospect — the body
              screen shows what a kept routine *has* done, never what one
              would. */}
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Today</span>
              <span className="panel-note">
                {health?.livedToday ? 'already lived' : 'by the rules above'}
              </span>
            </div>
            <div className="panel-body">
              {health?.livedToday ? (
                <div className="notice cool">
                  Today has been lived. One game day is one real day.
                </div>
              ) : (
                <>
                  <p className="faint tiny mt0">
                    The three choices your body answers to — how you slept, whether you trained,
                    how you ate — are produced by the rules rather than asked for. That is the
                    whole difference a routine makes, and the arc is toward never being asked
                    again.
                  </p>
                  <div className="btn-row">
                    <button
                      className="btn primary"
                      onClick={() => void liveByRoutine()}
                      disabled={busy}
                    >
                      Live today by your routines
                    </button>
                  </div>
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
 * One routine: the standing question, and the sentence answering it.
 *
 * The playbook's own editor shape (§3.14 — routines and the playbook are one
 * system with one interface language): the rule is written out, the editable
 * part opens a short list in place under the word, and picking is the change.
 *
 * A locked routine still says what it is and what would open it (§3.6 — struggle
 * before mastery, so a tool arrives after the problem it solves). Withholding
 * the existence of it would teach nothing; what is withheld is the ability to
 * arrange it.
 */
function RoutineCard({
  n,
  routine,
  open,
  onOpen,
  onPick,
}: {
  n: number
  routine: Routine
  open: boolean
  onOpen: () => void
  onPick: (option: string) => void
}) {
  const busy = useWorld(sending)
  const chosen = routine.options.find((o) => o.key === routine.chosen)

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">{routine.name}</span>
        <span className={routine.unlocked ? 'panel-note' : 'tag'}>
          {routine.unlocked ? 'in force' : 'locked'}
        </span>
      </div>

      <div className="panel-body">
        <div className="ask">
          <span className="ask-n">{n}</span>
          <span>
            <p className="ask-text">{routine.asks}</p>
          </span>
        </div>

        {routine.unlocked ? (
          <>
            <div className="sentence">
              <span className="clause">
                <span className="kw">my rule</span>
                <span className="edit-anchor">
                  <span
                    className={`editable act${chosen ? '' : ' empty'}${open ? ' open' : ''}`}
                    role="button"
                    tabIndex={busy ? -1 : 0}
                    onClick={(e) => {
                      e.stopPropagation()
                      onOpen()
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') onOpen()
                    }}
                  >
                    {chosen ? chosen.says : <span className="blank" />}
                  </span>
                  {open && (
                    <Choices
                      items={routine.options.map((o) => ({
                        label: o.says,
                        on: o.key === routine.chosen,
                        act: () => onPick(o.key),
                      }))}
                    />
                  )}
                </span>
              </span>
            </div>

            {/* What the rule as it stands costs — the sentence's own meta
                line, said by the server beside the option. */}
            {chosen && <p className="faint tiny mb0">{chosen.costs}</p>}
          </>
        ) : (
          <div className="slots">
            <div className="locked-slot">
              <span className="locked-name">{routine.name}</span>
              <span className="locked-why">{routine.learn}</span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
