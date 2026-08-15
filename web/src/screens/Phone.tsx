import { useEffect, useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { answerConflict, loadPhone } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type Phone, type PhoneMessage, type PhoneThread } from '../world/state'
import { clockOf, dayOf } from './words'

/*
 * The phone (§3.14, §3.15).
 *
 * # Why it is a device and not a panel
 *
 * Every other screen in this client is an overlay: a sheet of the game's own
 * paper that slides in from the right, in the game's own typography. This one is
 * an object the character owns, drawn as one — a frame, a screen inside it, a
 * status bar, threads. That is not decoration. The rest of the UI is the game
 * talking to the player; the phone is *other people* talking to the character,
 * and the two have to be told apart at a glance or the second one reads as more
 * of the first.
 *
 * # Why this screen exists
 *
 * There used to be a panel listing the people you live among, each with a line
 * saying what they can be relied on to do. It is gone (§3.2: nothing is
 * labelled), and it left a real question behind: how does anybody ever learn who
 * these people are? This is the answer. They read what happened. A thread is
 * every dealing with one person in the order it happened, and everything a
 * player concludes about somebody is concluded off that.
 *
 * # The three parts of an exchange
 *
 * Each message is one moment and renders as up to three bubbles: what they
 * said (`says`), what your character said back (`did`), and what they sent
 * after that (`came`). The middle one is the whole delegation loop — a reply that is
 * already sent when you open the thread is your playbook having answered in your
 * name while you were elsewhere, and reading it back is §3.14's step 1. That
 * used to be a table of tallies under the playbook editor, which is analytics;
 * this is the conversation.
 *
 * # What is not here
 *
 * No text field. The replies are the authored answers to the one moment that is
 * waiting, and a player writing their own words would need a model to read them
 * (§7). No unread badge: at most one message in the whole phone is ever waiting,
 * because nothing new arises until the last one is dealt with, so a count would
 * only ever count to one. And nothing anywhere marks a reply as the right one.
 */
export function PhoneScreen({ open, onClose }: { open: boolean; onClose: () => void }) {
  const waiting = useWorld((w) => w.conflicts?.waiting ?? null)
  const [phone, setPhone] = useState<Phone | null>(null)
  const [chosen, setChosen] = useState<string | null>(null)

  // Re-read when the screen opens and whenever the world says something was
  // settled — answering is the one thing that changes a thread, and the id
  // going away is how this screen hears about it without knowing what an
  // answer does.
  const standing = waiting?.id ?? 0
  useEffect(() => {
    if (!open) return
    let live = true
    void loadPhone().then((got) => {
      if (live) setPhone(got)
    })
    return () => {
      live = false
    }
  }, [open, standing])

  const threads = phone?.threads ?? []
  const thread = threads.find((t) => key(t) === chosen) ?? null

  return (
    <div className={`phone-shell${open ? ' open' : ''}`}>
      <div className="phone-scrim" onClick={onClose} />

      <div className="handset">
        <div className="phone-frame">
          <div className="phone-screen">
            <Status />
            {thread ? (
              <Conversation thread={thread} onBack={() => setChosen(null)} />
            ) : (
              <Inbox threads={threads} onOpen={(t) => setChosen(key(t))} />
            )}
          </div>
          <div className="phone-bar" />
        </div>

        <button className="phone-esc" onClick={onClose}>
          <kbd className="keycap">Esc</kbd> put it away
        </button>
      </div>
    </div>
  )
}

/**
 * A thread's identity for this tab.
 *
 * Population *and* person: two places may key somebody the same way and they
 * are not the same somebody. Getting this wrong would open one person's
 * conversation under another one's name after a move.
 */
const key = (t: PhoneThread) => `${t.environment}/${t.key}`

/**
 * The status bar: the time, and nothing else.
 *
 * A real one carries a carrier, a signal and a battery, and all three would be
 * props — this world has no network and no charge, and drawing a battery that
 * never moves is set dressing pretending to be a mechanic. What is left is the
 * clock, which is true: the server keeps this player's day in their own
 * timezone, so the time on the device is the time where the character is.
 */
function Status() {
  const [now, setNow] = useState(() => new Date())

  useEffect(() => {
    const tick = setInterval(() => setNow(new Date()), 20000)
    return () => clearInterval(tick)
  }, [])

  return (
    <div className="phone-status">
      <span>{now.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}</span>
      <span className="phone-notch" />
      <span className="phone-signal">
        <i />
        <i />
        <i />
      </span>
    </div>
  )
}

/**
 * The list of conversations.
 *
 * Ordered by when anything last passed, which is the server's order and not
 * this file's — a list that re-sorted itself in the browser would disagree with
 * the next read. The preview is the person's own last line rather than a
 * summary of the exchange, because the thread has no summaries in it anywhere
 * and the list is not the place to start.
 */
function Inbox({
  threads,
  onOpen,
}: {
  threads: PhoneThread[]
  onOpen: (t: PhoneThread) => void
}) {
  return (
    <>
      <div className="phone-head">
        <span className="phone-title">Messages</span>
      </div>

      <div className="phone-body">
        {threads.length === 0 ? (
          <p className="phone-empty">
            Nothing yet. Things come up with the people you live among, and this is where
            you find out what was said.
          </p>
        ) : (
          threads.map((t) => {
            const last = t.messages[t.messages.length - 1]
            return (
              <button className="phone-thread" key={key(t)} onClick={() => onOpen(t)}>
                <span className="phone-face" aria-hidden>
                  {initials(t.name)}
                </span>
                <span className="phone-thread-body">
                  <span className="phone-thread-top">
                    <span className="phone-who">
                      {t.name}
                      {!t.here && <span className="phone-left"> · {t.among}</span>}
                    </span>
                    <span className="phone-when">{last ? dayOf(last.at) : ''}</span>
                  </span>
                  <span className={`phone-preview${t.waiting ? ' unread' : ''}`}>
                    {last?.says ?? ''}
                  </span>
                </span>
                {t.waiting && <span className="phone-dot" aria-label="waiting on you" />}
              </button>
            )
          })
        )}
      </div>
    </>
  )
}

/**
 * One conversation, oldest first.
 *
 * The scroll sits at the bottom on open, which is where a conversation is: the
 * newest thing is the thing you came to read. A thread with somebody the player
 * has moved away from says so at the foot, because a conversation that has
 * stopped is a fact about having left rather than about them.
 */
function Conversation({ thread, onBack }: { thread: PhoneThread; onBack: () => void }) {
  const busy = useWorld(sending)
  const waiting = thread.messages.find((m) => m.waiting) ?? null

  return (
    <>
      <div className="phone-head">
        <button className="phone-back" onClick={onBack} aria-label="all messages">
          ‹
        </button>
        <span className="phone-face small" aria-hidden>
          {initials(thread.name)}
        </span>
        <span className="phone-title">{thread.name}</span>
      </div>

      <div className="phone-body chat" ref={toBottom}>
        {thread.messages.map((m, i) => (
          <Exchange
            key={m.id}
            message={m}
            day={i === 0 || thread.messages[i - 1].on !== m.on}
          />
        ))}

        {!thread.here && (
          <p className="phone-note">
            You moved away from {thread.among}. Nothing new comes here.
          </p>
        )}
      </div>

      {waiting && (
        <div className="phone-replies">
          {waiting.answers.map((a) => (
            <button
              className="phone-reply"
              key={a.key}
              disabled={busy}
              onClick={() => void answerConflict(waiting.id, a.key)}
            >
              {a.does}
            </button>
          ))}
        </div>
      )}
    </>
  )
}

/**
 * One exchange: what they said, what your character said back, and what they
 * sent after that.
 *
 * All three are bubbles, because all three were typed by somebody: the thread
 * has no narrator. What came of it is theirs, so it lands on their side, and it
 * carries no time — the authored moment holds one clock reading and their reply
 * belongs to whenever it belongs to.
 *
 * `covered` gets one quiet line, because it is the difference between a reply
 * the player chose and a reply their rules sent for them, and that difference is
 * the entire delegation loop. It says who sent it and stops there: nothing here
 * says whether the rule was right, which is the diagnosis and the diagnosis is
 * the player's (§3.6).
 */
function Exchange({ message, day }: { message: PhoneMessage; day: boolean }) {
  return (
    <>
      {day && <div className="phone-day">{dayOf(message.at)}</div>}

      <div className="phone-msg them">
        <span className="phone-bubble">{message.says}</span>
        <span className="phone-stamp">{clockOf(message.at)}</span>
      </div>

      {message.did && (
        <div className="phone-msg you">
          <span className="phone-bubble">{message.did}</span>
          <span className="phone-stamp">
            {message.covered ? 'sent by your playbook' : clockOf(message.at)}
          </span>
        </div>
      )}

      {message.came && (
        <div className="phone-msg them">
          <span className="phone-bubble">{message.came}</span>
        </div>
      )}
    </>
  )
}

/** Somebody's initials, for a face nobody has drawn. One letter for one name. */
function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}

/**
 * Open a conversation at the bottom of it.
 *
 * A callback ref rather than an effect, because the node arriving is exactly the
 * moment there is something to scroll — an effect would run once against a
 * thread that had not been laid out yet and leave the player at the top of six
 * months of their own life.
 */
function toBottom(node: HTMLDivElement | null) {
  if (node) node.scrollTop = node.scrollHeight
}

/**
 * The phone in the key bar.
 *
 * The same device every other hint uses: the label reads differently rather than
 * a badge appearing, because the shell has no notification area and the key bar
 * saying something else *is* how this world tells you something is there.
 */
export function PhoneHint() {
  const waiting = useWorld((w) => w.conflicts?.waiting ?? null)
  return (
    <KeyHint keys={['T']}>
      {waiting ? `${waiting.who.toLowerCase()} is waiting on you` : 'your phone'}
    </KeyHint>
  )
}
