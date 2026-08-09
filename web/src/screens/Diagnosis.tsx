import { useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { nameCause } from '../world/actions'
import { useWorld } from '../world/react'
import { sending } from '../world/state'
import { Overlay } from './Overlay'

/**
 * Saying what you think happened (§3.6) — struggle-gated unlocking, and the
 * only thing in this game that hands the player a tool.
 *
 * It is a conversation and not a form, and the difference is structural rather
 * than cosmetic. Four things hold it to that, and each one is a rule from §3.6:
 *
 *  - **The evidence is theirs.** Every line under "what you have" is a reading
 *    off the player's own history with their own numbers in it. Nothing on this
 *    screen was authored for somebody to choose between, which is why the
 *    evidence is checkboxes over their logs and not a list of four answers.
 *  - **The accounts come out of what they picked.** An account is only offered
 *    once something the player has bears on it, and it is only *nameable* once
 *    they have actually pointed at that something — so the order of the screen
 *    is evidence first, conclusion second, and it cannot be worked backwards.
 *  - **Nothing is ever confirmed.** There is no right answer here, no tick, no
 *    buzzer and no retry. What comes back is the person's closing line, which
 *    is the same line whichever account was named because they do not know
 *    either.
 *  - **What it costs is stated.** Each account says what naming it hands over,
 *    because gating a tool behind an insight only works if the player can see
 *    what they are reaching for. Hiding it would make this a lottery.
 *
 * The screen exists only when something has gone badly. There is no empty
 * state to design because there is no empty state: `open` is false most days
 * and the key does nothing, which is the mechanic rather than a gap in it.
 */
export function Diagnosis({ open, onClose }: { open: boolean; onClose: () => void }) {
  const diagnosis = useWorld((w) => w.diagnosis)
  const busy = useWorld(sending)
  const [pointed, setPointed] = useState<string[]>([])
  const [closed, setClosed] = useState<{ said: string; closes: string } | null>(null)

  if (!diagnosis) return null

  const point = (key: string) =>
    setPointed((held) => (held.includes(key) ? held.filter((k) => k !== key) : [...held, key]))

  const name = (cause: string) => {
    void nameCause(diagnosis.occasion, cause, pointed).then((said) => {
      if (said) setClosed({ said, closes: diagnosis.scene.closes })
      setPointed([])
    })
  }

  // An account is nameable once the player has pointed at something it bears
  // on. Computed here rather than sent, because it is a fact about what they
  // have selected in this tab and the server has no business holding it.
  const bears = (speaks: string[]) => speaks.some((k) => pointed.includes(k))

  if (closed) {
    return (
      <Overlay open={open} onClose={onClose} title={diagnosis.scene.who} sub="What you said">
        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">What you said</span>
            </div>
            <div className="panel-body">
              <p className="note first">{closed.said}</p>
              <p className="faint">{closed.closes}</p>
            </div>
          </div>
        </div>
      </Overlay>
    )
  }

  if (!diagnosis.open) {
    // Nothing is open, so there is nothing to sit down about. The record is
    // still worth showing: a conclusion the player now doubts is the thing
    // §3.6's fifth rule is argued against, and it has to be somewhere.
    return (
      <Overlay open={open} onClose={onClose} title="What you have worked out" sub="">
        <Record named={diagnosis.named} />
      </Overlay>
    )
  }

  const scene = diagnosis.scene

  return (
    <Overlay open={open} onClose={onClose} title={scene.who} sub={scene.note}>
      <div className="stack">
        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">What happened</span>
            {diagnosis.again && <span className="panel-note">you have been over this before</span>}
          </div>
          <div className="panel-body">
            <p className="note first">{diagnosis.went}</p>
            <p className="faint">{scene.opens}</p>
            <p className="note">{scene.asks}</p>
            {diagnosis.again && (
              <p className="faint tiny">Last time you said: {diagnosis.said}</p>
            )}
          </div>
        </div>

        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">What you have</span>
            <span className="panel-note">your own logs</span>
          </div>
          <div className="panel-body">
            {diagnosis.evidence.length === 0 ? (
              <p className="faint">
                Nothing you keep says anything about the last fortnight. You have no more to
                go on than he does.
              </p>
            ) : (
              <div className="stack">
                {diagnosis.evidence.map((o) => (
                  <button
                    className={pointed.includes(o.key) ? 'btn wide on' : 'btn wide'}
                    key={o.key}
                    onClick={() => point(o.key)}
                  >
                    <span>{o.says}</span>
                    <span className="faint tiny"> {o.from}</span>
                  </button>
                ))}
              </div>
            )}
            {pointed.length > 0 && <p className="faint tiny">{scene.weighs}</p>}
          </div>
        </div>

        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">What you think happened</span>
            <span className="panel-note">nothing here is checked</span>
          </div>
          <div className="panel-body">
            {diagnosis.causes.length === 0 ? (
              <p className="faint">
                There is nothing you could say about this that anything you keep would bear
                on.
              </p>
            ) : (
              <div className="stack">
                {diagnosis.causes.map((c) => (
                  <div className="rule" key={c.key}>
                    <p className="note first">{c.says}</p>
                    <p className="faint tiny">{c.because}</p>
                    {c.unlocks && <p className="faint tiny">This would give you {c.unlocks}.</p>}
                    <button
                      className="btn"
                      disabled={!bears(c.speaks) || busy}
                      onClick={() => name(c.key)}
                    >
                      {bears(c.speaks) ? 'Say it' : 'Point at something about this first'}
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <Record named={diagnosis.named} />
      </div>
    </Overlay>
  )
}

/**
 * What the player has concluded before.
 *
 * Nothing on it says how any of them turned out, because the game does not
 * know and would not say if it did. What it does say is what each one handed
 * over — including, when it comes to that, a routine they have never once
 * used.
 */
function Record({ named }: { named: { says: string; at: string; unlocks: string }[] }) {
  if (named.length === 0) return null
  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">What you have said before</span>
        <span className="panel-note">no account of how it went</span>
      </div>
      <div className="panel-body">
        {named.map((n, i) => (
          <div className="rule-line" key={`${n.at}-${i}`}>
            <p className="note first">{n.says}</p>
            <p className="faint tiny">
              {n.at}
              {n.unlocks && ` · gave you ${n.unlocks}`}
            </p>
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * The key hint, and it is only there when there is something to sit down about.
 *
 * A permanent hint would advertise a screen that does nothing on most days,
 * which is the same failure as making the scene always available: it would turn
 * a moment of consequence into a menu item.
 */
export function DiagnosisHint() {
  const diagnosis = useWorld((w) => w.diagnosis)
  if (!diagnosis?.open) return null
  return <KeyHint keys={['D']}>what you think happened</KeyHint>
}
