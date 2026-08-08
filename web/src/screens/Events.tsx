import { useEffect, useRef, useState } from 'react'

import type { Screen } from '../App'
import { KeyHint } from '../chrome/Keybar'
import { acceptEvent, declineEvent, loadEventLog } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type EventLog, type EventRecord } from '../world/state'
import { world } from '../world/store'
import { Overlay } from './Overlay'
import { clockOf, dayOf } from './words'

/**
 * What the world has put in front of you, and what it has put there before
 * (§4).
 *
 * Two halves, and the gap between them is the ticket.
 *
 * Above: the one thing standing now, or nothing. **Nothing is the ordinary
 * state.** §4's sandbox "depends on not everything arriving as a push", and a
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
}: {
  open: boolean
  onClose: () => void
  onEnter: () => void
}) {
  const events = useWorld((w) => w.events)
  const busy = useWorld(sending)
  const [page, setPage] = useState(1)
  const [log, setLog] = useState<EventLog | null>(null)
  const [after, setAfter] = useState('')

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
  useEffect(() => {
    if (offer) setAfter('')
  }, [standing])

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
                  onClick={() =>
                    void acceptEvent(offer.id).then((said) => {
                      setAfter(said)
                      // §4's doorway: some events open a challenge, and the
                      // response has already put the run in the store. The
                      // player said yes to a thing, so they are taken to
                      // it rather than told it is now available somewhere.
                      if (world.getState().challenge?.run) onEnter()
                    })
                  }
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
            <div className="panel-body">
              {log.records.map((record) => (
                <Happened key={record.id} record={record} />
              ))}
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
 * One thing that happened, and what you did about it.
 *
 * The five answers are five words rather than "yes" and "no", because the
 * differences are the point: `declined` is a decision, `left` is walking away,
 * `lapsed` is not looking, and `dropped` is the world giving up on something
 * you were never near. Only the first two were ever a choice.
 */
function Happened({ record }: { record: EventRecord }) {
  const said: Record<string, string> = {
    accepted: 'you took it up',
    declined: 'you let it go',
    left: 'you walked on',
    lapsed: 'you were not looking',
    dropped: 'it never found you',
  }

  return (
    <div className="log-row">
      <span className="log-time">{clockOf(record.at)}</span>
      <span>
        <span className="log-what">{record.name}</span>
        <span className="log-detail">
          {dayOf(record.at)} · {record.place.replace(/-/g, ' ')} ·{' '}
          {said[record.answer] ?? record.answer}
        </span>
      </span>
      <span>
        <span className={`tag${record.answer === 'accepted' ? '' : ' dim'}`}>{record.kind}</span>
        {record.answer === 'accepted' && !record.resolved && (
          <span className="tag amber">still going</span>
        )}
      </span>
    </div>
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
