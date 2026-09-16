import { useEffect, useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { answerConflict, loadPhone } from '../world/actions'
import { useWorld } from '../world/react'
import {
  sending,
  type Neighbour,
  type Phone,
  type PhoneMessage,
  type PhoneThread,
} from '../world/state'
import { clockOf, dayOf } from './words'

/*
 * The phone (§3.14, §3.15).
 *
 * A device rather than a panel. Every other screen is an overlay in the game's
 * own typography; this is an object the character owns, drawn as one — a frame,
 * a screen inside it, a status bar, threads. The rest of the UI is the game
 * talking to the player, and the phone is other people talking to the character.
 *
 * It is how a player learns who these people are: they read what happened. A
 * thread is every dealing with one person in the order it happened, and
 * everything a player concludes about somebody is concluded off that. Nothing is
 * labelled (§3.2).
 *
 * Two apps, answering different questions. A thread says how it has gone with
 * somebody. A card says who they are before anything has gone any way: where
 * they live, and how your character knows them. Both are facts: "two doors up
 * on your landing" is an address, and "turns up with the thing you needed before
 * you asked" is a conclusion the player reaches by living there.
 *
 * Each message is one moment and renders as up to three bubbles: what they said
 * (`says`), what your character said back (`did`), and what they sent after
 * (`came`). The middle one is the delegation loop — a reply already sent when you
 * open the thread is your playbook having answered in your name, and reading it
 * back is §3.14's step 1.
 *
 * No text field: the replies are the authored answers to the one waiting moment,
 * and a player writing their own words would need a model to read them (§7). No
 * unread badge, since at most one message in the phone is ever waiting. And
 * nothing marks a reply as the right one.
 */
export function PhoneScreen({
  open,
  onClose,
  at = null,
}: {
  open: boolean
  onClose: () => void
  /**
   * Whose conversation to open, when the player was sent here from somewhere
   * that was talking about it. Keys rather than an index, because the thread
   * list is fetched and the answer to "which one" has to survive it arriving.
   */
  at?: { environment: string; person: string } | null
}) {
  const waiting = useWorld((w) => w.conflicts?.waiting ?? null)
  const among = useWorld((w) => w.conflicts?.among ?? null)
  const [phone, setPhone] = useState<Phone | null>(null)
  const [chosen, setChosen] = useState<string | null>(null)
  const [app, setApp] = useState<'messages' | 'contacts'>('messages')
  const [card, setCard] = useState<string | null>(null)

  // Opened at somebody. On the aim changing rather than on the screen opening:
  // an aim honoured once is spent, and re-honouring it every time the phone is
  // opened afterwards would take the player somewhere they did not ask to go.
  useEffect(() => {
    if (!at) return
    setChosen(`${at.environment}/${at.person}`)
    setApp('messages')
  }, [at])

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
  const people = among?.people ?? []
  const person = people.find((p) => p.key === card) ?? null

  // Whether there is anything to read with somebody yet. A card offers to open
  // a conversation only when one exists: this phone cannot start one, so a
  // button that led to an empty thread would be a control the player does not
  // have.
  const talked = (p: Neighbour) =>
    threads.some((t) => t.key === p.key && t.environment === among?.environment)

  return (
    <div className={`phone-shell${open ? ' open' : ''}`}>
      <div className="phone-scrim" onClick={onClose} />

      <div className="handset">
        <div className="phone-frame">
          <div className="phone-screen">
            <Status />

            {app === 'messages' ? (
              thread ? (
                <Conversation thread={thread} onBack={() => setChosen(null)} />
              ) : (
                <Inbox threads={threads} onOpen={(t) => setChosen(key(t))} />
              )
            ) : person ? (
              <Card
                person={person}
                district={among?.environment ?? ''}
                onBack={() => setCard(null)}
                onMessage={
                  talked(person)
                    ? () => {
                        setChosen(`${among?.environment}/${person.key}`)
                        setApp('messages')
                      }
                    : null
                }
              />
            ) : (
              <Contacts people={people} onOpen={(p) => setCard(p.key)} />
            )}

            <Apps
              app={app}
              waiting={threads.some((t) => t.waiting)}
              onSwitch={(next) => {
                setApp(next)
                if (next === 'messages') setCard(null)
                else setChosen(null)
              }}
            />
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
            // Somebody nothing has happened with yet still has the exchange you
            // met over, so the row shows that rather than an empty line.
            const preview =
              last?.says.join(' ') ?? t.intro?.[t.intro.length - 1]?.says ?? ''
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
                    {preview}
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
        {/* How the two of you met, ahead of everything. It carries no clock and
            no day: it is the same text for every player and is not a thing that
            happened on a date the world knows about. */}
        {thread.intro?.map((turn, i) => (
          <div
            className={`phone-msg ${turn.them ? 'them' : 'you'}${
              i > 0 && thread.intro[i - 1].them === turn.them ? ' run' : ''
            }`}
            key={`intro-${i}`}
          >
            <span className="phone-bubble">{turn.says}</span>
          </div>
        ))}

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

      {message.says.map((bubble, i) => (
        <div className={`phone-msg them${i > 0 ? ' run' : ''}`} key={`says-${i}`}>
          <span className="phone-bubble">{bubble}</span>
          {i === message.says.length - 1 && (
            <span className="phone-stamp">{clockOf(message.at)}</span>
          )}
        </div>
      ))}

      {message.did && (
        <div className="phone-msg you">
          <span className="phone-bubble">{message.did}</span>
          <span className="phone-stamp">
            {message.covered ? 'sent by your playbook' : clockOf(message.at)}
          </span>
        </div>
      )}

      {message.came.map((bubble, i) => (
        <div className={`phone-msg them${i > 0 ? ' run' : ''}`} key={`came-${i}`}>
          <span className="phone-bubble">{bubble}</span>
        </div>
      ))}
    </>
  )
}

/**
 * The two apps, at the foot of the device.
 *
 * A phone this small has one row of them and no home screen, because a home
 * screen full of apps that do not open would be a shelf of controls the player
 * does not have. Two, both of which work.
 *
 * The dot on Messages is the one exception to the screen's no-badge rule, and it
 * is not a badge: there is no number on it. It says *something is waiting* while
 * the player is looking at somebody's address, which is the only moment on this
 * device where that could otherwise go unnoticed.
 */
function Apps({
  app,
  waiting,
  onSwitch,
}: {
  app: 'messages' | 'contacts'
  waiting: boolean
  onSwitch: (app: 'messages' | 'contacts') => void
}) {
  return (
    <div className="phone-apps">
      <button
        className={`phone-app${app === 'messages' ? ' on' : ''}`}
        onClick={() => onSwitch('messages')}
      >
        Messages
        {waiting && app !== 'messages' && <span className="phone-dot" aria-hidden />}
      </button>
      <button
        className={`phone-app${app === 'contacts' ? ' on' : ''}`}
        onClick={() => onSwitch('contacts')}
      >
        Contacts
      </button>
    </div>
  )
}

/**
 * Everybody the player lives among.
 *
 * Held by the world model rather than fetched: who you are among is a fact about
 * where you live and it is already on the boot snapshot, so opening this reads
 * nothing. It is the people of the *current* population and only those — a
 * contacts list is who you could go and find, and somebody you left behind two
 * districts ago is a thread, which is where they are.
 *
 * By name, the way a contacts list on any phone is, rather than in the order the
 * population was authored in. Nothing here is ranked, and an order that came
 * from anywhere else would look like one.
 */
function Contacts({
  people,
  onOpen,
}: {
  people: Neighbour[]
  onOpen: (p: Neighbour) => void
}) {
  const sorted = [...people].sort((a, b) => a.name.localeCompare(b.name))

  return (
    <>
      <div className="phone-head">
        <span className="phone-title">Contacts</span>
      </div>

      <div className="phone-body">
        {sorted.map((p) => (
          <button className="phone-card-row" key={p.key} onClick={() => onOpen(p)}>
            <span className="phone-face" aria-hidden>
              {initials(p.name)}
            </span>
            <span className="phone-who">{p.name}</span>
          </button>
        ))}
      </div>
    </>
  )
}

/**
 * One person's card: where they live, how you know them, and where they turn up.
 *
 * Facts, and deliberately nothing else. What is here that the content did not
 * write is the place, which is looked up rather than
 * authored so that a person's card and the map never disagree about what the
 * corner shop is called, and the standing, which is the server's own sentences
 * about what is outstanding between the two of you.
 *
 * The labels are the ones a real contacts card carries. They are the only words
 * this component supplies, and each names a row rather than explaining it.
 */
function Card({
  person,
  district,
  onBack,
  onMessage,
}: {
  person: Neighbour
  /** The population's own district, which is not a place to go and find anybody. */
  district: string
  onBack: () => void
  /** Null while there is nothing to open — this phone cannot start a thread. */
  onMessage: (() => void) | null
}) {
  // Somewhere inside the district, or nowhere in particular. The people who turn
  // up on the street rather than behind a counter have the district itself as
  // their place, and a row saying they can be found in Wilmington is one that
  // repeats their address back at somebody who lives there too.
  const inside = person.where !== district
  const at = useWorld((w) =>
    inside ? (w.places.find((p) => p.key === person.where)?.name ?? '') : '',
  )

  return (
    <>
      <div className="phone-head">
        <button className="phone-back" onClick={onBack} aria-label="all contacts">
          ‹
        </button>
        <span className="phone-title">Contacts</span>
      </div>

      <div className="phone-body">
        <div className="phone-card-head">
          <span className="phone-face big" aria-hidden>
            {initials(person.name)}
          </span>
          <span className="phone-card-name">{person.name}</span>
          <span className="phone-card-known">{person.known}</span>
        </div>

        <dl className="phone-card-facts">
          <dt>home</dt>
          <dd>{person.lives}</dd>
          {at && (
            <>
              <dt>find them</dt>
              <dd>{at}</dd>
            </>
          )}
          {person.standing.length > 0 && (
            <>
              <dt>between you</dt>
              <dd>
                {person.standing.map((s) => (
                  <span key={s}>{s}</span>
                ))}
              </dd>
            </>
          )}
        </dl>

        {onMessage && (
          <button className="phone-card-open" onClick={onMessage}>
            Messages
          </button>
        )}
      </div>
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
