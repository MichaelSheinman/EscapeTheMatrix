import { answerSituation, beginChallenge, leaveJob, takeJob } from '../world/actions'
import { KeyHint } from '../chrome/Keybar'
import { useWorld } from '../world/react'
import { sending, type Opening as OpeningView, type Role } from '../world/state'
import { Overlay } from './Overlay'
import { spending } from './words'
import { useEffect, useRef, useState } from 'react'

/**
 * The job surface (§3.14): a board on the left, one posting open on the right.
 *
 * The shape is the argument, and it is a different argument from the one this
 * screen made until 2026-08-08. It used to be three panels stacked by
 * importance — what you hold, the board, the whole ladder — which is a screen
 * built to state a thesis. What a player actually does here is compare six jobs
 * and pick one, and comparison needs alignment: the same fields at the same
 * position, so that clicking down the list moves the values and nothing else.
 *
 * Three rules hold the rest of it together:
 *
 *   1. **The row carries only what you compare on** — the name, the shift, the
 *      wage. No description, not even truncated: a posting's text is a thing
 *      the player is meant to read closely (see [Posting]) and a preview would
 *      decide for them which sentence they saw.
 *   2. **What you hold renders in the same shape as what you are considering.**
 *      It is the first row, above a rule, and it opens the identical detail.
 *      Then the difference between the job you have and the job on offer is
 *      the only thing that moves on the screen.
 *   3. **Nothing computes a rule.** Whether a posting is open is `take` and
 *      `ready`, which the server decided inside internal/factor's veil.
 *
 * The ladder is gone from here entirely. It was five rungs of a universal
 * staircase the game has never claimed exists, four of which this player could
 * not have — and where a role does lead somewhere, that is a fact about the
 * role and belongs in its own posting.
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
  const waiting = useWorld((w) => w.situations?.waiting ?? null)

  // Which posting is open. A key rather than an index, so that the board
  // arriving again over the socket — a wage paid, a rung reached — does not
  // slide the selection onto a different job under the player's cursor.
  const [chosen, setChosen] = useState<string | null>(null)
  const list = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (open) list.current?.focus({ preventScroll: true })
  }, [open])

  if (!career) return null

  const held = career.held
  const board = career.board
  // The one posting with a place attached: a vacancy they are standing in
  // front of. The same job is on the board; what standing there changes is
  // that the door is a door rather than an application.
  const here = career.here

  const rows: string[] = [...(held ? [held.key] : []), ...board.map((o) => o.role.key)]
  const selected = chosen && rows.includes(chosen) ? chosen : (rows[0] ?? null)
  const opening = board.find((o) => o.role.key === selected) ?? null

  const move = (by: number) => {
    if (!selected) return
    const at = rows.indexOf(selected)
    setChosen(rows[Math.min(rows.length - 1, Math.max(0, at + by))])
  }

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Work"
      sub={
        held
          ? `${held.name}. It pays whether or not you think about it, and it takes the hours out of the day before your routines get them.`
          : 'Nothing is coming in. Everything except what a day can be made to produce by hand has to come from a job.'
      }
    >
      {waiting && <OnYourDesk />}

      <div className="split fill">
        {/*
         * The list takes the focus when the screen opens, so that a board can
         * be walked with the arrow keys without first being clicked. The
         * handler is here rather than on the window because it is this list's
         * behaviour, and a screen that is open behind another one must not be
         * quietly eating keys.
         */}
        <div
          className="split-list"
          tabIndex={0}
          ref={list}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              move(1)
            }
            if (e.key === 'ArrowUp') {
              e.preventDefault()
              move(-1)
            }
          }}
        >
          {held && (
            <Row
              role={held}
              pay={career.paid}
              held
              on={selected === held.key}
              onPick={() => setChosen(held.key)}
            />
          )}
          {board.map((o) => (
            <Row
              key={o.role.key}
              role={o.role}
              pay={o.role.pay}
              on={selected === o.role.key}
              onPick={() => setChosen(o.role.key)}
            />
          ))}
        </div>

        <div className="split-detail">
          {opening ? (
            <Posting
              opening={opening}
              here={here?.role.key === opening.role.key ? here : null}
              onEnter={onEnter}
              switching={held !== null}
            />
          ) : held ? (
            <Yours role={held} pay={career.paid} since={career.since} />
          ) : null}
        </div>
      </div>
    </Overlay>
  )
}

/**
 * One line of the board.
 *
 * Everything a player picks *between* and nothing else. The wage is the only
 * number, and it is the same number in the same place on every row, because
 * that column being scannable is most of what this screen is for.
 */
function Row({
  role,
  pay,
  held = false,
  on,
  onPick,
}: {
  role: Role
  pay: number
  held?: boolean
  on: boolean
  onPick: () => void
}) {
  const mine = useRef<HTMLButtonElement>(null)

  // Arrowing down a list whose selection is a key, not a scroll position, still
  // has to bring the row into view — otherwise the keyboard walks off the
  // bottom of the pane and the player is reading a posting they cannot see the
  // row for.
  useEffect(() => {
    if (on) mine.current?.scrollIntoView({ block: 'nearest' })
  }, [on])

  return (
    <button
      ref={mine}
      className={`split-row job-row${on ? ' on' : ''}${held ? ' held' : ''}`}
      onClick={onPick}
      aria-current={on}
    >
      <span className="job-row-name">{role.name}</span>
      <span className="job-row-pay">{spending(pay)}</span>
      <span className="job-row-shift">
        {role.hours} hours{role.nights ? ' · nights' : ''}
      </span>
      <span className="job-row-unit">a day</span>
    </button>
  )
}

/**
 * A posting, open.
 *
 * The text is the whole of it, and the typography is not decoration: some of
 * these jobs quietly cost their holder something and the only account the game
 * ever gives of that is a clause in a paragraph, written flat, marked in no
 * way at all. So it is set at reading size on a reading measure — and nothing
 * in this component is allowed to know which sentence matters. No emphasis, no
 * chip, no ordering that correlates with the answer. What the player has to go
 * on is what somebody reading a real advert has to go on.
 *
 * That rule finished off the requirement chips too. `wants` still arrives on the
 * wire, and this screen no longer draws it: a red "Biological age" under a
 * posting reads as the game naming the factor a job moves — §3.2's conclusion,
 * handed over before the player has worked anything out, and in the same breath
 * colour-coded good or bad. What the requirements do instead is the door: it
 * does not open, and nothing says why.
 */
function Posting({
  opening,
  here,
  onEnter,
  switching,
}: {
  opening: OpeningView
  here: OpeningView | null
  onEnter: () => void
  switching: boolean
}) {
  const busy = useWorld(sending)
  const { role } = opening

  // Two doors, and the difference is only ever which one the server is being
  // asked to use: standing at the workplace lets placement be the server's,
  // and the board names the role. An interview is sat from either — interviews
  // happen online.
  const walkIn = here !== null && here.walk !== null

  return (
    <>
      <div className="posting-bar">
        <div>
          <h2 className="posting-name">{role.name}</h2>
          <p className="posting-shift">
            {role.shift}
            {role.nights ? ' (overnight)' : ''} · {spending(role.pay)} a day
          </p>
        </div>
        {opening.take ? (
          <button
            className="btn primary"
            disabled={busy || !opening.ready}
            onClick={() => void takeJob(walkIn ? undefined : role.key)}
          >
            {!opening.ready ? 'Not hiring you yet' : switching ? 'Switch to it' : 'Take it'}
          </button>
        ) : opening.interview ? (
          <button
            className="btn primary"
            disabled={busy}
            onClick={() => void beginChallenge(opening.interview).then(onEnter)}
          >
            Sit the interview
          </button>
        ) : (
          <span className="faint tiny">They are not hiring for this yet</span>
        )}
      </div>

      <div className="posting-body">
        <Advert posting={role.posting} />

        {walkIn && (
          <p className="faint tiny">
            You are standing in it. They will take you on at the {here?.walk?.zone}.
          </p>
        )}
      </div>
    </>
  )
}

/**
 * The advert itself: intro, duties, qualifications — the sections a real
 * posting has, in the same order on every posting. The section headers carry
 * no judgement; which duty line is the one a careful reader would rather not
 * do is still the player's to work out.
 */
function Advert({ posting }: { posting: Role['posting'] }) {
  return (
    <>
      <p className="posting-text">{posting.intro}</p>
      <h3 className="posting-section">Duties</h3>
      <ul className="posting-list">
        {posting.duties.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
      <h3 className="posting-section">Qualifications</h3>
      <ul className="posting-list">
        {posting.qualifications.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </>
  )
}

/**
 * The job you are in, in the shape of a posting.
 *
 * Same fields, same order, same positions — which is the point. A player
 * clicking between this and the board is running a diff, and a different layout
 * here would mean doing that diff by memory instead of by eye.
 *
 * It carries no advert text: the posting is what a job says to somebody outside
 * it, and this player is inside. What it has instead is the two facts only the
 * holder has — what it has paid so far, and since when.
 */
function Yours({ role, pay, since }: { role: Role; pay: number; since: string }) {
  const busy = useWorld(sending)
  const started = since ? new Date(since) : null
  const day =
    started && !Number.isNaN(started.valueOf())
      ? started.toLocaleDateString(undefined, { day: 'numeric', month: 'long' })
      : null

  return (
    <>
      <div className="posting-bar">
        <div>
          <h2 className="posting-name">{role.name}</h2>
          <p className="posting-shift">
            {role.shift}
            {role.nights ? ' (overnight)' : ''} · {spending(pay)} a day
          </p>
        </div>
        <button className="btn" disabled={busy} onClick={() => void leaveJob()}>
          Leave it
        </button>
      </div>

      <div className="posting-body">
        <Advert posting={role.posting} />
        {day && <p className="faint tiny">Yours since {day}.</p>}
      </div>
    </>
  )
}

/**
 * A moment your job produced while you were somewhere else, waiting on you
 * (§3.14).
 *
 * The whole of the delegation loop's fourth step. It is a live decision rather
 * than a report: the role ran, the playbook could not settle this one, and the
 * answer is the player's to give — in the content's own words, with nothing
 * anywhere saying which of them is right (§3.2).
 *
 * It sits above the board because it is the one thing on this screen that is
 * not a choice between jobs, and because a job standing still until somebody
 * decides is §3.14's "gaps in the playbook are what demand attention".
 */
function OnYourDesk() {
  const waiting = useWorld((w) => w.situations?.waiting ?? null)
  const busy = useWorld(sending)
  const [came, setCame] = useState<{ id: number; said: string } | null>(null)

  if (!waiting) {
    if (!came) return null
    return (
      <div className="panel desk">
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
    <div className="panel desk">
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
 * The job in the key bar (§3.14).
 *
 * Two things are worth interrupting somebody for and neither of them is the job
 * they already have: a moment the playbook could not settle, and a vacancy they
 * are standing in front of.
 */
export function WorkHint() {
  const career = useWorld((w) => w.career)
  const waiting = useWorld((w) => w.situations?.waiting ?? null)
  if (!career) return null

  if (waiting) return <KeyHint keys={['J']}>{waiting.name.toLowerCase()}</KeyHint>
  if (career.here) return <KeyHint keys={['J']}>{career.here.role.name.toLowerCase()}</KeyHint>
  if (career.held) return <KeyHint keys={['J']}>{career.held.name.toLowerCase()}</KeyHint>
  return <KeyHint keys={['J']}>work</KeyHint>
}
