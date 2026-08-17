import { answerSituation, beginChallenge, leaveJob, takeJob } from '../world/actions'
import { InfoTip } from '../chrome/InfoTip'
import { KeyHint } from '../chrome/Keybar'
import { useWorld } from '../world/react'
import { sending, type Opening as OpeningView, type Role, type Route } from '../world/state'
import { Overlay } from './Overlay'
import { spending } from './words'
import { useEffect, useRef, useState } from 'react'

/**
 * The job surface (§3.14): a board on the left, one posting open on the right.
 *
 * What a player does here is compare six jobs and pick one, and comparison needs
 * alignment — the same fields at the same position, so clicking down the list
 * moves the values and nothing else.
 *
 * Three rules:
 *
 *   1. A row carries only what you compare on: the name, the shift, the wage. No
 *      description, not even truncated, because a posting's text is meant to be
 *      read closely and a preview would choose which sentence the player saw.
 *   2. What you hold renders in the same shape as what you are considering: the
 *      first row, above a rule, opening the identical detail.
 *   3. Nothing computes a rule. Whether a posting is open is `take` and `ready`,
 *      which the server decided inside internal/factor.
 *
 * Where a job leads somewhere is a fact about the job, and it is in the posting
 * — see [Experience], the one section that reads the player's own history back
 * to them.
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

  // Which posting is open. A key rather than an index, so the board arriving
  // again over the socket does not slide the selection onto a different job.
  const [chosen, setChosen] = useState<string | null>(null)
  const list = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (open) list.current?.focus({ preventScroll: true })
  }, [open])

  if (!career) return null

  const held = career.held
  const board = career.board
  // The one posting with a place attached: a vacancy they are standing in front
  // of. The same job is on the board, and standing there makes the door a door
  // rather than an application.
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
         * The list takes the focus when the screen opens, so the board can be
         * walked with the arrow keys without being clicked first. The handler is
         * on the list rather than the window, so a screen open behind another
         * one is not eating keys.
         */}
        <div
          className="split-list"
          tabIndex={0}
          ref={list}
          onKeyDown={(e) => {
            if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
            // `stopPropagation` rather than only `preventDefault`. The arrows
            // are the character, and that handler is on `window`, so preventing
            // the default alone leaves an arrow key closing the board. Stopping
            // the event at the list lets both readings coexist: inside a focused
            // list it moves the selection, anywhere else the character.
            e.preventDefault()
            e.stopPropagation()
            move(e.key === 'ArrowDown' ? 1 : -1)
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
              onOpen={(key) => {
                if (!rows.includes(key)) return
                setChosen(key)
                // The list gets the focus back, because the selection belongs
                // to it. Without this the focus stays on a crosslink the
                // re-render is about to unmount, lands on the body, and the next
                // arrow key walks the character instead of moving down.
                list.current?.focus({ preventScroll: true })
              }}
              opens={rows}
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
 * One line of the board: what a player picks between and nothing else. The wage
 * is the only number, in the same place on every row, because that column being
 * scannable is most of what this screen is for.
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

  // The selection is a key rather than a scroll position, so arrowing down
  // still has to bring the row into view.
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
 *
 * The one closed door that does say why is experience (2026-08-15), and the
 * difference is the whole argument. A factor floor cannot be explained — three
 * of the five are ones the player is never told exist (§3.5), so "not hiring you
 * yet" is the most that can honestly be said. A route is the opposite: it names
 * jobs the player has done, in days they could count themselves, and [Experience]
 * has already printed the arithmetic four lines below. Refusing to name it there
 * would not be withholding a conclusion, it would be hiding a fact the player
 * already has.
 *
 * So the two refusals are worded apart. `worked` false is "not qualified yet",
 * against a section that says exactly what is missing; `ready` false stays "not
 * hiring you yet" and stays unexplained. The old fallback said "they are not
 * hiring for this yet" for both, which was true only of the second and read, on
 * a posting whose experience was three weeks short, as a job that did not exist.
 */
function Posting({
  opening,
  here,
  onEnter,
  switching,
  onOpen,
  opens,
}: {
  opening: OpeningView
  here: OpeningView | null
  onEnter: () => void
  switching: boolean
  onOpen: (key: string) => void
  opens: string[]
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
            disabled={busy || !opening.ready || !opening.worked}
            onClick={() => void takeJob(walkIn ? undefined : role.key)}
          >
            {!opening.worked
              ? 'Not qualified yet'
              : !opening.ready
                ? 'Not hiring you yet'
                : switching
                  ? 'Switch to it'
                  : 'Take it'}
          </button>
        ) : opening.interview ? (
          <button
            className="btn primary"
            disabled={busy}
            onClick={() => {
              void beginChallenge(opening.interview)
                .then((began) => {
                  // Only on a run that exists. A refusal leaves the player on
                  // the board, where the posting they were reading still is.
                  if (began) onEnter()
                })
                // A refusal comes back as `began: false` rather than a throw,
                // so this catches the callback itself failing. Nothing catches
                // a rejected handler promise otherwise — not an error boundary
                // — and the interview would silently never open.
                .catch((err: unknown) => {
                  console.error('sitting the interview went wrong', err)
                })
            }}
          >
            Sit the interview
          </button>
        ) : !opening.worked ? (
          <span className="faint tiny">You do not meet the qualifications yet</span>
        ) : (
          <span className="faint tiny">They are not hiring for this yet</span>
        )}
      </div>

      <div className="posting-body">
        <Advert posting={role.posting} />
        {opening.experience.length > 0 && (
          <Experience routes={opening.experience} onOpen={onOpen} opens={opens} />
        )}

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
 * The experience a posting asks for.
 *
 * The section states the requirement and nothing about the reader. It is
 * the posting's own words — "45 days as a Transport dispatcher" is what the
 * employer wants, and it says the same thing to everybody who opens it. How far
 * along the player is moved into [InfoTip], because a requirement with the
 * reader's own progress spliced into it is two documents wearing one heading,
 * and the one nobody asked for is the one that changes every day.
 *
 * Both operators are on the screen, because both are the mechanic. Every
 * route is an alternative — any one of them opens the door — and the routes are
 * separated by a plain "or" rather than by layout, since a list of bullets is
 * read as a list of requirements. Inside a route the jobs are joined by "and",
 * for the same reason from the other side: two lines under one heading do not
 * say that both are needed.
 *
 * A route reads as one sentence rather than a bullet each, deliberately. "20
 * days as a Transport dispatcher and 60 days as an Overnight warehouse
 * operative" is one condition, and breaking it in two would make it look like
 * two things that could be satisfied separately.
 *
 * The job names open their own postings. A prerequisite the player cannot
 * go and read is a name they have to go hunting for on a board they are already
 * standing on. Only the ones actually on this board are links — a job that is
 * not offered here is still named, because the requirement is the requirement,
 * but there is nothing to open.
 */
function Experience({
  routes,
  onOpen,
  opens,
}: {
  routes: Route[]
  onOpen: (key: string) => void
  opens: string[]
}) {
  return (
    <>
      <h3 className="posting-section">Experience</h3>
      <ul className="posting-list">
        {routes.map((route, i) => (
          <li key={route.needs.map((n) => n.role).join('+')}>
            {i > 0 && <span className="faint">or </span>}
            {route.needs.map((need, j) => (
              <span key={need.role}>
                {j > 0 && <span className="faint"> and </span>}
                {need.days} days as {article(need.name)}{' '}
                {opens.includes(need.role) ? (
                  <button className="crosslink" onClick={() => onOpen(need.role)}>
                    {need.name}
                  </button>
                ) : (
                  need.name
                )}
              </span>
            ))}
            <InfoTip text={standing(route)} label="How far along you are" />
          </li>
        ))}
      </ul>
    </>
  )
}

/**
 * How far along the player is on one route, as a sentence.
 *
 * `short` carries only what is still owed, so an entry missing from it is one
 * already finished — which is why the days done are read off `needs` minus
 * `short` rather than sent as a third number.
 *
 * The nothing-done case is the one worth wording carefully. "0 of 45 days"
 * three times over says the player is nowhere without saying where to start,
 * and the route beside it has just named the jobs that would.
 */
function standing(route: Route): string {
  const owed = new Map(route.short.map((t) => [t.role, t.days]))
  const done = (need: { role: string; days: number }) => need.days - (owed.get(need.role) ?? 0)

  if (route.needs.every((need) => done(need) === 0)) {
    return 'You have not worked at any of these. Consider starting with one of them instead.'
  }
  return `You have worked ${route.needs
    .map((need) => `${done(need)} of ${need.days} days as ${article(need.name)} ${need.name}`)
    .join(', and ')}.`
}

/**
 * "a" or "an", for a job title.
 *
 * Every posting in the world is a noun phrase the UI has to put an article in
 * front of, and five of the thirteen begin with a vowel — "a Overnight gym
 * front desk associate" was on the screen the first time this rendered. The
 * first letter is enough here because no job title in `content/jobs` starts
 * with a sounded "u" or a silent "h"; if one ever does, this is where it goes
 * wrong and this comment is the warning.
 */
function article(name: string): string {
  return /^[aeiou]/i.test(name) ? 'an' : 'a'
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
