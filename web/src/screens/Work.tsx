import { KeyHint } from '../chrome/Keybar'
import { answerSituation, beginChallenge, leaveJob, takeJob } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type Opening as OpeningView } from '../world/state'
import { Overlay } from './Overlay'
import { spending } from './words'
import { useState } from 'react'

/**
 * The job, and the ladder above it (§3.14).
 *
 * Three things on one screen, and the order they are in is the argument:
 *
 *   1. **What you hold**, with its day rate *and* its shift. A rung that showed
 *      its salary and not its hours would be making half of §3.14's case.
 *   2. **The way up**, if there is one from here — either a job being taken on
 *      where you are standing, or the interview you have become able to sit.
 *      The requirement arrives already veiled: two chips with names on them,
 *      and blanked ones for the factors nobody is ever told exist (§3.5).
 *   3. **The whole ladder**, including the four rungs this player cannot have.
 *      §3.14: "the ladder itself teaches the trade: more money costs more of
 *      your attention", and a ladder you can see one rung of teaches nothing.
 *
 * Nothing here computes a rule. Whether the step up is open is `ready`, which
 * the server decided inside internal/factor's veil — this file cannot work out
 * what is missing and must not be able to.
 */
export function Work({
  open,
  onClose,
  onEnter,
}: {
  open: boolean
  onClose: () => void
  onEnter: () => void
}) {
  const career = useWorld((w) => w.career)
  const character = useWorld((w) => w.character)
  const busy = useWorld(sending)

  if (!career) return null

  const held = career.held
  // At most one opening is worth acting on. What is in front of them wins over
  // what is above them, because one is a door and the other is a plan.
  const opening = career.here ?? career.next

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title={held ? held.name : 'Not working'}
      sub={
        held
          ? 'It pays whether or not you think about it, and it takes the hours out of the day before your routines get them.'
          : 'Nothing is coming in. What you earn by hand is capped at three things a day; everything else has to come from holding something.'
      }
    >
      <div className="grid wide-left">
        <div className="stack">
          <OnYourDesk />

          {held ? (
            <div className="panel">
              <div className="panel-head">
                <span className="panel-title">What you hold</span>
                <span className="panel-note">
                  {held.hours} hours{held.nights ? ' · nights' : ''}
                </span>
              </div>
              <div className="panel-body">
                <div className="readout">
                  <div>
                    <div className="stat-big accent">{spending(career.paid)}</div>
                    <div className="stat-unit">a day, for holding it</div>
                  </div>
                  <p className="note last">{held.does}</p>
                </div>
                <p className="note">{held.costs}</p>
                <div className="btn-row">
                  <button className="btn" disabled={busy} onClick={() => void leaveJob()}>
                    Give it up
                  </button>
                </div>
              </div>
            </div>
          ) : (
            <div className="panel">
              <div className="panel-body">
                <p className="note first">
                  Jobs are taken where they are. Nobody posts them and there is no list —
                  you find the loading bay by walking onto it.
                </p>
                {character && (
                  <p className="note last">
                    {character.earningsLeft} paid {character.earningsLeft === 1 ? 'action' : 'actions'} left
                    today. That is the whole of what a day can be made to produce by hand.
                  </p>
                )}
              </div>
            </div>
          )}

          {opening && <Opening opening={opening} onEnter={onEnter} />}
        </div>

        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">The ladder</span>
              <span className="panel-note">{career.ladder.length} rungs</span>
            </div>
            <div className="panel-body">
              {career.ladder.map((rung) => (
                <div
                  className={`ladder-row${
                    held?.tier === rung.tier
                      ? ' current'
                      : rung.tier > (held?.tier ?? 0)
                        ? ' future'
                        : ''
                  }`}
                  key={rung.key}
                >
                  <span className="ladder-tier">{rung.tier}</span>
                  <span>
                    <b>{rung.name}</b>
                    <i>
                      {rung.hours} hours{rung.nights ? ', at night' : ''}
                    </i>
                  </span>
                  <span className="metric-value">{spending(rung.pay)}</span>
                </div>
              ))}
              <p className="note last">
                Every step up pays more and asks for more of the day. The one at the bottom is
                the exception, and it is the one anybody can have.
              </p>
            </div>
          </div>
        </div>
      </div>
    </Overlay>
  )
}

/**
 * A moment your job produced while you were somewhere else, waiting on you
 * (§3.14).
 *
 * The whole of the delegation loop's fourth step, on one panel. It is a live
 * decision rather than a report: the role ran, the playbook could not settle
 * this one, and the answer is the player's to give — in the content's own
 * words, with nothing anywhere saying which of them is right (§3.2).
 *
 * Nothing here composes a rule. Writing the answer so that the *next* one is
 * covered is the playbook screen, which is a different key and deliberately a
 * different act: the gap between deciding this one and answering the question
 * behind it is the loop.
 */
function OnYourDesk() {
  const waiting = useWorld((w) => w.situations?.waiting ?? null)
  const busy = useWorld(sending)
  const [came, setCame] = useState<{ id: number; said: string } | null>(null)

  if (!waiting) {
    // What was just answered, held on screen until something else happens on
    // this surface. The consequence is the only account of it the player gets,
    // and taking it away the moment the panel re-rendered would be the game
    // saying something and then not letting them read it.
    if (!came) return null
    return (
      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">What came of it</span>
        </div>
        <div className="panel-body">
          <p className="note first last">{came.said}</p>
        </div>
      </div>
    )
  }

  const answer = (key: string) => {
    void answerSituation(waiting.id, key).then((said) => {
      if (said) setCame({ id: waiting.id, said })
    })
  }

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">{waiting.name}</span>
        <span className="panel-note">{waiting.role}</span>
      </div>
      <div className="panel-body">
        <p className="note first">{waiting.line}</p>
        <p className="faint tiny">{waiting.why}</p>
        <div className="stack">
          {waiting.answers.map((option) => (
            <button
              className="btn wide"
              key={option.key}
              disabled={busy}
              onClick={() => answer(option.key)}
            >
              {option.does}
            </button>
          ))}
        </div>
        <p className="faint tiny">
          It happened on {waiting.on}, while you were elsewhere. Your job will not hand you
          anything else until this is dealt with.
        </p>
      </div>
    </div>
  )
}

/**
 * A rung this player does not hold, as @etm/ui's `Posting` (§2).
 *
 * The requirement chips are the piece worth reading carefully. `met` and
 * `unmet` carry a name because those two factors are ones the game shows;
 * `hidden` carries nothing at all — not a masked number, not a name, and not
 * whether it is cleared. A blanked chip that flipped to met would say what it
 * had been measuring at the moment it stopped mattering (§3.5).
 */
function Opening({ opening, onEnter }: { opening: OpeningView; onEnter: () => void }) {
  const busy = useWorld(sending)
  const { role } = opening

  return (
    <div className="posting">
      <div className="posting-head">
        <div>
          <div className="posting-role">{role.name}</div>
          <div className="posting-where">{role.does}</div>
        </div>
        <div className="right">
          <div className="posting-pay">
            {spending(role.pay)} <span className="faint tiny">a day</span>
          </div>
          <div className="posting-where">
            {role.hours} hours{role.nights ? ' · nights' : ''}
          </div>
        </div>
      </div>

      {opening.wants.length > 0 && (
        <div className="req">
          {opening.wants.map((want, i) =>
            want.state === 'hidden' ? (
              // Keyed by position, because position is all a blanked one has.
              <span className="hidden" key={i}>
                ? ? ?
              </span>
            ) : (
              <span className={want.state} key={i}>
                {want.label}
              </span>
            ),
          )}
        </div>
      )}

      <div className="posting-foot">
        <span className="faint tiny">{role.costs}</span>
        {opening.walk ? (
          <button
            className="btn primary"
            disabled={busy || !opening.ready}
            onClick={() => void takeJob()}
          >
            {opening.ready ? 'Take it' : 'Not for you yet'}
          </button>
        ) : opening.interview ? (
          <button
            className="btn primary"
            disabled={busy}
            onClick={() => {
              void beginChallenge(opening.interview).then(onEnter)
            }}
          >
            Sit the interview
          </button>
        ) : (
          <span className="faint tiny">not from here</span>
        )}
      </div>
    </div>
  )
}

/**
 * The job in the key bar (§3.14).
 *
 * Two things are worth interrupting somebody for and neither of them is the job
 * they already have: a vacancy they are standing in front of, and a step up
 * they have become able to take. Everything else is a screen they can go to.
 */
export function WorkHint() {
  const career = useWorld((w) => w.career)
  const waiting = useWorld((w) => w.situations?.waiting ?? null)
  if (!career) return null

  // Ahead of everything else on this key. A moment the playbook could not
  // settle is the job standing still until somebody decides — §3.14's "gaps in
  // the playbook are what demand attention", said in the one place a player is
  // definitely looking.
  if (waiting) return <KeyHint keys={['J']}>{waiting.name.toLowerCase()}</KeyHint>
  if (career.here) return <KeyHint keys={['J']}>{career.here.role.name.toLowerCase()}</KeyHint>
  if (career.next?.ready && (career.next.interview || career.next.walk)) {
    return <KeyHint keys={['J']}>{`they will see you — ${career.next.role.name.toLowerCase()}`}</KeyHint>
  }
  if (career.held) return <KeyHint keys={['J']}>{career.held.name.toLowerCase()}</KeyHint>
  return <KeyHint keys={['J']}>work</KeyHint>
}
