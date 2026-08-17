import { useEffect, useRef, useState } from 'react'

import type { Aim, Screen } from '../App'
import { KeyHint } from '../chrome/Keybar'
import { acceptEvent, declineEvent, loadEventLog } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type EventAbout, type EventLog, type EventRecord } from '../world/state'
import { world } from '../world/store'
import { Overlay } from './Overlay'
import { clockOf, dayOf } from './words'

/**
 * What the world has put in front of you, and what it has put there before
 * (§4).
 *
 * Two halves, and the gap between them is the ticket.
 *
 * Above: the one thing standing now, or nothing. Nothing is the ordinary
 * state. §4's sandbox "depends on not everything arriving as a push", and a
 * screen that apologised for an empty moment would be teaching the player to
 * come here and wait. So the empty state says where things happen instead —
 * which is true, and is the only advice this screen ever gives.
 *
 * Below: what has happened, a page at a time. It is fetched when the screen
 * opens rather than carried in the world snapshot, because history is unbounded
 * (§10.2) — and because the *backlog* is not fetchable at all. A client that
 * could read what the world was holding could walk towards it, and there would
 * be no sandbox left to have.
 *
 * Note what neither half offers: a way to ask for something. There is no button
 * that summons an event, because release is a server decision (§4) and a button
 * would make it a timer with a lever on it.
 */
export function EventsScreen({
  open,
  onClose,
  onEnter,
  onGo,
}: {
  open: boolean
  onClose: () => void
  onEnter: () => void
  /**
   * Open another panel at a spot in it. An escalation is about a conversation
   * and a rule, and both of those are somewhere else in this client — see
   * `Aim`.
   */
  onGo: (to: Screen, at: Aim) => void
}) {
  const events = useWorld((w) => w.events)
  const busy = useWorld(sending)
  const [page, setPage] = useState(1)
  const [log, setLog] = useState<EventLog | null>(null)
  const [after, setAfter] = useState('')
  // Which record the pane is reading. An id rather than an index, so that the
  // log arriving again — a page turned, something answered — does not slide the
  // selection onto a different moment under the player's cursor.
  const [looking, setLooking] = useState<number | null>(null)

  const offer = events?.offer ?? null
  const standing = offer?.id ?? 0

  // Re-read whenever the screen opens, the page moves, or the offer changes —
  // the last of those is how answering something puts it into the record
  // underneath without this screen knowing what an answer does.
  useEffect(() => {
    if (!open) return
    let live = true
    void loadEventLog(page).then((got) => {
      if (live) setLog(got)
    })
    return () => {
      live = false
    }
  }, [open, page, standing])

  // What the last answer came to, cleared the moment something else arrives.
  // It is a sentence the server wrote, held here because there is nowhere in
  // the world model for "what you were told a minute ago" to live.
  //
  // The guard reads `standing` rather than `offer` because it is the same
  // question — `standing` is the offer's id, and 0 is how this screen already
  // spells "nothing is being offered". Asking it that way is asking about the
  // render the effect actually belongs to, instead of a captured `offer` that
  // a later render has moved on from.
  useEffect(() => {
    if (standing) setAfter('')
  }, [standing])

  // The first row when nothing has been picked, and the first row again when
  // the page turns out from under what was: a pane reading a record that is no
  // longer in the list would be showing last page's moment beside this one's.
  const records = log?.records ?? []
  const reading = records.some((r) => r.id === looking) ? looking : (records[0]?.id ?? null)
  const showing = records.find((r) => r.id === reading) ?? null

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="What is happening"
      sub="The world has things ready for you and decides when to put one in front of you. Walking
            is where most of them find you — and walking away from one does not use it up."
    >
      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">{offer ? offer.name : 'Nothing right now'}</span>
          {offer && <span className="panel-note">until {clockOf(offer.until)}</span>}
        </div>

        <div className="panel-body">
          {offer ? (
            <>
              <div className="ask">
                <span className="ask-n">●</span>
                <span>
                  <p className="ask-text">{offer.line}</p>
                  {offer.about ? (
                    <About about={offer.about} onGo={onGo} />
                  ) : (
                    offer.because && <p className="ask-because">{offer.because}</p>
                  )}
                  <p className="ask-note">
                    {offer.kind} · {offer.zone.replace(/-/g, ' ')}. Leave it and it goes back to
                    the world — it is not used up, and it may find you again.
                  </p>
                </span>
              </div>

              <div className="btn-row">
                <button
                  className="btn primary"
                  disabled={busy}
                  onClick={() => {
                    void acceptEvent(offer.id)
                      .then((said) => {
                        setAfter(said)
                        // §4's doorway: some events open a challenge, and the
                        // response has already put the run in the store. The
                        // player said yes to a thing, so they are taken to
                        // it rather than told it is now available somewhere.
                        if (world.getState().challenge?.run) onEnter()
                      })
                      // `acceptEvent` records a refusal rather than throwing,
                      // so what this can catch is this callback breaking. An
                      // error boundary never sees a rejected handler promise:
                      // without this the screen would sit there looking fine.
                      .catch((err: unknown) => {
                        console.error('accepting the event went wrong', err)
                      })
                  }}
                >
                  {offer.accept}
                </button>
                <button
                  className="btn small"
                  disabled={busy}
                  onClick={() => void declineEvent(offer.id)}
                >
                  {offer.decline}
                </button>
              </div>
            </>
          ) : (
            <p className="note first">
              {after || 'Things happen where you are, and mostly while you are walking.'}
            </p>
          )}
        </div>
      </div>

      {log && log.total > 0 && (
        <>
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">What has happened</span>
              <span className="panel-note">{log.total} in all</span>
            </div>

            {/*
             * The list picks and the pane reads, which is the shape the job
             * board takes and for the same reason: these rows are things you
             * compare and then look into, and a list that sat under what it
             * opened would move out from under the cursor on every pick.
             *
             * It replaced a flat log of one line each — a title, a place and a
             * verb. Everything a person would actually want to know about
             * their own week (who it was, what they wanted, how it turned out)
             * was authored, shown once, and then thrown away.
             */}
            <div className="split">
              <div className="split-list">
                {log.records.map((record) => (
                  <Row
                    key={record.id}
                    record={record}
                    on={record.id === reading}
                    onPick={() => setLooking(record.id)}
                  />
                ))}
              </div>
              <div className="split-detail">
              {showing && <Happened record={showing} onGo={onGo} />}
            </div>
            </div>
          </div>

          {log.pages > 1 && (
            <div className="btn-row">
              <button
                className="btn small"
                onClick={() => setPage(log.page - 1)}
                disabled={log.page <= 1}
              >
                back
              </button>
              <span className="faint tiny">
                page {log.page} of {log.pages}
              </span>
              <button
                className="btn small"
                onClick={() => setPage(log.page + 1)}
                disabled={log.page >= log.pages}
              >
                next
              </button>
            </div>
          )}
        </>
      )}
    </Overlay>
  )
}

/**
 * What became of one event, in words.
 *
 * Five answers rather than "yes" and "no", because the differences are the
 * point: `declined` is a decision, `left` is walking away, `lapsed` is not
 * looking, and `dropped` is the world giving up on something you were never
 * near. Only the first two were ever a choice.
 */
const SAID: Record<string, string> = {
  accepted: 'you took it up',
  declined: 'you let it go',
  left: 'you walked on',
  lapsed: 'it stood there and you did not answer',
  dropped: 'it never found you',
}

/** One line of the record: when, what, and what became of it. */
function Row({
  record,
  on,
  onPick,
}: {
  record: EventRecord
  on: boolean
  onPick: () => void
}) {
  return (
    <button
      className={`split-row happened${on ? ' on' : ''}`}
      onClick={onPick}
      aria-current={on}
    >
      <span className="happened-when">
        {dayOf(record.at)}
        <span className="happened-clock">{clockOf(record.at)}</span>
      </span>
      <span className="happened-body">
        <span className="happened-what">{record.name}</span>
        <span className="happened-said">{SAID[record.answer] ?? record.answer}</span>
      </span>
      {record.answer === 'accepted' && !record.resolved && (
        <span className="tag amber">still going</span>
      )}
    </button>
  )
}

/**
 * One moment, opened: what it was, what you did, and what came of it.
 *
 * The pane is the writing, and the writing is the whole point of it — the row
 * beside it is a title and a verb, and a title and a verb is not an account of
 * anything. The order is the order it happened in, which is also the order the
 * questions arrive in: what was this, what did I do, how did it turn out.
 *
 * Set at reading size on a reading measure, and marked in no way at all. §3.2's
 * rule holds here exactly as it does on a job posting: nothing in this component
 * knows which of these moments mattered, so nothing here emphasises one. What
 * an accepted event came to is stated and never graded.
 */
function Happened({
  record,
  onGo,
}: {
  record: EventRecord
  onGo: (to: Screen, at: Aim) => void
}) {
  return (
    <>
      <div className="posting-bar">
        <div>
          <h2 className="posting-name">{record.name}</h2>
          <p className="posting-shift">
            {dayOf(record.at)}, {clockOf(record.at)} · {record.place.replace(/-/g, ' ')}
            {record.zone ? ` · ${record.zone.replace(/-/g, ' ')}` : ''}
          </p>
        </div>
        <span className={`tag${record.answer === 'accepted' ? '' : ' dim'}`}>{record.kind}</span>
      </div>

      <div className="posting-body">
        {/* The moment itself when it survives, and the sentence composed when
            it was raised when it does not — a record whose moment has been
            tidied out of the tables still says what it said at the time. Never
            both: they are the same facts, and one of them is shorter. */}
        {!record.about && record.because && (
          <div className="happened-turn">
            <span className="happened-lead">Why this reached you</span>
            <p className="note first last">{record.because}</p>
          </div>
        )}

        {record.line ? (
          <p className="posting-text">{record.line}</p>
        ) : (
          <p className="faint tiny">
            The world does not have this one in it any more, so there is nothing left to
            read. It still happened.
          </p>
        )}

        {record.about && <About about={record.about} onGo={onGo} />}

        {/* Only when they did something. The three unanswered states are not
            choices, and the sentence the row beside this uses for them —
            "it stood there and you did not answer" — is false the moment the
            event was an escalation: a rule of theirs answered it, which is the
            whole reason the thing reached them. What became of an event nobody
            took up is said once, at the foot. */}
        {record.did && (
          <div className="happened-turn">
            <span className="happened-lead">What you did</span>
            <p className="note first last">{record.did}</p>
          </div>
        )}

        {record.came && (
          <div className="happened-turn">
            <span className="happened-lead">What came of it</span>
            <p className="note first last">{record.came}</p>
          </div>
        )}

        {record.answer === 'accepted' && !record.came && !record.resolved && (
          <p className="faint tiny">
            You took it up and it is not finished. What it comes to is somewhere else — a
            challenge, a job, or the people you live among.
          </p>
        )}

        {(record.answer === 'declined' ||
          record.answer === 'left' ||
          record.answer === 'lapsed') && (
          <p className="faint tiny">
            Walking away does not use it up. It went back to the world, and it may find you
            again.
          </p>
        )}
      </div>
    </>
  )
}

/**
 * The moment an escalation is about: the exchange it happened in, and the
 * answers of yours that were weighed in it.
 *
 * Three lines and no fourth. What they said, what your character said back, and
 * what they sent afterwards — the last of those is the whole account of how it
 * went, in the words somebody authored for it, and this component adds nothing
 * to it. An escalation is the one moment in the game that exists to send
 * somebody back to their own answer, and a sentence here explaining *why* it
 * came out the way it did would be the game doing the reading (§3.2).
 *
 * The two buttons are the point of the whole thing. The conversation is on the
 * phone and the rule is in the playbook; before them, an account of something
 * you could not go and look at was the entire experience of this event.
 */
function About({
  about,
  onGo,
}: {
  about: EventAbout
  onGo: (to: Screen, at: Aim) => void
}) {
  const who = about.who || 'They'
  return (
    <>
      {about.says && (
        <div className="happened-turn">
          <span className="happened-lead">{about.name || 'What was said'}</span>
          <div className="said">
            <p className="said-line">
              <span className="said-who">{who}</span>
              {about.says}
            </p>
            {about.did && (
              <p className="said-line yours">
                <span className="said-who">You</span>
                {about.did}
              </p>
            )}
            {about.came && (
              <p className="said-line">
                <span className="said-who">{who}</span>
                {about.came}
              </p>
            )}
          </div>
          {about.person && (
            <button
              className="btn ghost small"
              onClick={() =>
                onGo('phone', {
                  thread: { environment: about.environment, person: about.person },
                })
              }
            >
              Open the conversation
            </button>
          )}
        </div>
      )}

      {about.rules.length > 0 && (
        <div className="happened-turn">
          <span className="happened-lead">
            {about.rules.length > 1 ? 'Your answers' : 'Your answer'}
          </span>
          {about.rules.map((rule) => (
            <div className="said-rule" key={`${rule.environment}/${rule.question}`}>
              <p className="note first last">{rule.says}</p>
              <button
                className="btn ghost small"
                onClick={() =>
                  onGo('playbook', {
                    rule: { environment: rule.environment, question: rule.question },
                  })
                }
              >
                Open the rule
              </button>
            </div>
          ))}
        </div>
      )}
    </>
  )
}

/**
 * Something arrived while you were walking.
 *
 * Dismissible and ignorable by construction: it never blocks the world, and
 * letting it expire loses nothing — the offer stands on the phone (E) until
 * its own window closes.
 */
export function Toasts({ onOpen }: { onOpen: (screen: Screen) => void }) {
  const offer = useWorld((w) => w.events?.offer ?? null)
  const [shown, setShown] = useState<{ id: number; name: string; line: string } | null>(null)
  const [leaving, setLeaving] = useState(false)
  const seen = useRef(0)

  useEffect(() => {
    if (!offer || offer.id === seen.current) return
    seen.current = offer.id
    setShown({ id: offer.id, name: offer.name, line: offer.line })
    setLeaving(false)
    const going = setTimeout(() => setLeaving(true), 13000)
    const gone = setTimeout(() => setShown(null), 13300)
    return () => {
      clearTimeout(going)
      clearTimeout(gone)
    }
  }, [offer])

  return (
    <div className="toasts">
      {shown && (
        <div className={`toast${leaving ? ' leaving' : ''}`}>
          <div className="toast-head">
            <span className="toast-kicker">{shown.name}</span>
            <span
              className="toast-x"
              role="button"
              tabIndex={0}
              onClick={() => setShown(null)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') setShown(null)
              }}
            >
              ✕
            </span>
          </div>
          <p className="toast-title">{shown.line}</p>
          <button
            className="btn small"
            onClick={() => {
              setShown(null)
              onOpen('events')
            }}
          >
            Open
          </button>
        </div>
      )}
    </div>
  )
}

/**
 * The key that opens what the world has put in front of you (§4).
 *
 * An offer stands for well under an hour and then lapses back into the backlog,
 * so this is the one hint in the bar that is genuinely time-critical. The same
 * device the other two use — the label reads differently rather than a badge
 * appearing, because the shell has no notification area and the key bar saying
 * something else *is* how this world tells you something is there.
 */
export function EventsHint() {
  const offer = useWorld((w) => w.events?.offer ?? null)
  return (
    <KeyHint keys={['E']}>
      {offer ? `something is happening — ${offer.name.toLowerCase()}` : 'what is happening'}
    </KeyHint>
  )
}
