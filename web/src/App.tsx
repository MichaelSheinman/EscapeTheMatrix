import { useEffect, useState } from 'react'

import { newKey } from './commands'
import { signInURL, type Player } from './session'
import {
  buyInstrument,
  liveADay,
  liveByRoutine,
  moveTo,
  probe,
  setRoutines,
  signOut,
  work,
} from './world/actions'
import { useWorld } from './world/react'
import type { Attempt, Mark, Metric, Routine } from './world/state'

/*
 * The DOM half of the app.
 *
 * Everything here floats **above** the canvas and never contains it
 * (technical.md §10.1). There is no route that replaces the world: closing this
 * panel leaves the same scene running underneath, in the same state, because
 * nothing React does can reach the renderer.
 *
 * Every value on screen comes out of the one store through `useWorld`, which is
 * `useSyncExternalStore` — the same store the renderer reads sixty times a
 * second in its own loop (§10.2). Nothing below holds a copy of world state;
 * the only `useState` in the file is which panel is open, which is a fact about
 * this browser tab rather than about the world.
 *
 * `@etm/ui` owns every class name here (§2). The design system is consumed,
 * never reimplemented, and its shell vocabulary — place chip, stat pods, key
 * bar, overlay — is what "the UI is part of the world" looks like in markup.
 */

/**
 * Which panel is open, and the key that opens it.
 *
 * One key per destination, which is @etm/ui's shell rule: there is no rail and
 * no top bar, so what a player can reach is a key rather than a menu.
 */
type Screen = 'readout' | 'body' | 'rules' | null

const KEYS: Record<string, Exclude<Screen, null>> = { s: 'readout', h: 'body', r: 'rules' }

export function App() {
  const session = useWorld((w) => w.session)
  const renderer = useWorld((w) => w.renderer)

  // The one piece of React state in the client: which panel this tab has open.
  // Not in the store, because the world does not care.
  const [screen, setScreen] = useState<Screen>('readout')

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      // A key pressed inside a field is a letter, not a shortcut.
      if (e.target instanceof HTMLInputElement) return
      if (e.key === 'Escape') setScreen(null)
      const opens = KEYS[e.key.toLowerCase()]
      if (opens) setScreen(opens)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <>
      <Where />

      {session.status === 'in' && <Pods onOpen={setScreen} />}

      {renderer === 'unsupported' && <CannotDraw />}

      <div className="keybar">
        {screen ? (
          <KeyHint keys={['Esc']}>back to the world</KeyHint>
        ) : (
          <>
            <KeyHint keys={['S']}>the readout</KeyHint>
            <KeyHint keys={['H']}>your body</KeyHint>
            <KeyHint keys={['R']}>your routines</KeyHint>
          </>
        )}
        {session.status === 'in' && !screen && (
          <KeyHint keys={['Click']}>probe where you tapped</KeyHint>
        )}
      </div>

      {session.status === 'unknown' && <Waiting />}
      {session.status === 'out' && <Door notice={session.notice} />}
      {session.status === 'in' && (
        <>
          <Readout
            player={session.player}
            open={screen === 'readout'}
            onClose={() => setScreen(null)}
          />
          <Body open={screen === 'body'} onClose={() => setScreen(null)} />
          <Rules open={screen === 'rules'} onClose={() => setScreen(null)} />
        </>
      )}
    </>
  )
}

/**
 * Where you are, and where home is.
 *
 * @etm/ui calls this the only orientation the shell gives you — the place names
 * itself in the world through its own signage, and this is the fallback for
 * before there is any. Both places come from the character, so a district
 * renamed on the server is renamed here without a deploy.
 */
function Where() {
  const character = useWorld((w) => w.character)

  if (!character) {
    return (
      <div className="place-chip">
        <span className="place-name">The world</span>
        <span className="place-where">placeholder scene</span>
      </div>
    )
  }

  return (
    <div className="place-chip">
      <span className="place-name">{character.at.name}</span>
      <span className="place-where">
        {character.at.key === character.home.key ? 'home' : `home is ${character.home.name}`}
      </span>
    </div>
  )
}

function KeyHint({ keys, children }: { keys: string[]; children: string }) {
  return (
    <span className="keybar-item">
      {keys.map((k) => (
        <kbd className="keycap" key={k}>
          {k}
        </kbd>
      ))}
      <span className="keybar-label">{children}</span>
    </span>
  )
}

/**
 * The numbers that are true wherever you are standing.
 *
 * The count in the first pod is the same number the canvas draws under the
 * beacon, out of the same field of the same store — one read by React when it
 * changes, one read by the renderer every frame.
 */
function Pods({ onOpen }: { onOpen: (screen: Screen) => void }) {
  const count = useWorld((w) => w.probes.count)
  const link = useWorld((w) => w.link)
  const character = useWorld((w) => w.character)
  const health = useWorld((w) => w.health)

  const said = {
    connecting: { text: 'connecting', tone: '' },
    live: { text: 'live', tone: 'accent' },
    offline: { text: 'reconnecting', tone: 'warn' },
  }[link]

  return (
    <div className="pods">
      {health && (
        <div className="pod accent" onClick={() => onOpen('body')} title="your body">
          <span className="pod-glyph">◍</span>
          <span className="pod-text">
            <span className="pod-value">{health.biologicalAge.toFixed(0)}</span>
            <span className="pod-label">biological age</span>
          </span>
        </div>
      )}

      {character && (
        <div className="pod" onClick={() => onOpen('readout')} title="the readout">
          <span className="pod-glyph">$</span>
          <span className="pod-text">
            <span className="pod-value">{spending(character.money)}</span>
            <span className="pod-label">money</span>
          </span>
        </div>
      )}

      <div className="pod" onClick={() => onOpen('readout')} title="open the readout">
        <span className="pod-glyph">◆</span>
        <span className="pod-text">
          <span className="pod-value">{count}</span>
          <span className="pod-label">probes recorded</span>
        </span>
      </div>

      <div
        className={`pod ${said.tone}`}
        title="the WebSocket carrying everything you did not just ask for"
      >
        <span className="pod-glyph">↯</span>
        <span className="pod-text">
          <span className="pod-value">{said.text}</span>
          <span className="pod-label">socket</span>
        </span>
      </div>
    </div>
  )
}

/**
 * The device cannot run the renderer.
 *
 * WebGL2 is a hard floor (§10.4) — Pixi v8 has no Canvas2D fallback, so there
 * is nothing to degrade to and nothing to offer. Said once, plainly, in the
 * corner where things that arrive live.
 */
function CannotDraw() {
  return (
    <div className="corner">
      <div className="toasts">
        <div className="toast">
          <div className="toast-head">
            <span className="toast-kicker">the world is not drawing</span>
          </div>
          <p className="toast-title">This browser cannot run the renderer.</p>
          <p className="toast-body">
            It needs WebGL2, which this device or its driver is not providing. The panels below
            still work — the world behind them does not.
          </p>
        </div>
      </div>
    </div>
  )
}

/** The moment before the server has said who this is. */
function Waiting() {
  return (
    <div className="door">
      <div className="door-scrim" />
      <div className="panel">
        <div className="panel-body">
          <p className="faint tiny mt0">Checking your session…</p>
        </div>
      </div>
    </div>
  )
}

/** The door. Account required up front, OAuth only, no password to type. */
function Door({ notice }: { notice: string | null }) {
  return (
    <div className="door">
      {/* The world keeps running behind this, dimmed. It was drawn before
          anybody signed in and will still be drawing after they sign out. */}
      <div className="door-scrim" />

      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">Escape the Matrix</span>
          <span className="panel-note">sign in to play</span>
        </div>

        <div className="panel-body">
          {notice && <div className="notice">{notice}</div>}

          <p className="faint tiny mt0">
            The game needs a stable identity — leaderboards and peer endorsement are built on one
            — so an account comes first. Sign-in is Google's: this server never sees a password and
            stores none.
          </p>

          <div className="btn-row">
            {/* A navigation rather than a fetch: the browser has to leave for
                Google and come back carrying the cookie the server sets. */}
            <a className="btn primary" href={signInURL}>
              Continue with Google
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * The walking skeleton, now as an overlay over the world rather than as the
 * whole screen.
 *
 * Closing it does not unmount anything the renderer owns — press Escape and the
 * beacon is still pulsing, the marks are still where they were, and the socket
 * is still connected. That is the property §10.1 asks for, made visible.
 *
 * Laid out as cards on the page ground rather than as one long surface, which
 * is how `preview/` composes every screen: the panels are the light, the ground
 * behind them is not, and the contrast is what separates one subject from the
 * next without a heading having to.
 */
function Readout({
  player,
  open,
  onClose,
}: {
  player: Player
  open: boolean
  onClose: () => void
}) {
  const attempt = useWorld((w) => w.attempt)
  const marks = useWorld((w) => w.probes.marks)
  const count = useWorld((w) => w.probes.count)
  const server = useWorld((w) => w.server)

  const [label, setLabel] = useState('')
  const last = marks.length > 0 ? marks[marks.length - 1] : null
  const elsewhere = lastFrom(marks, 'elsewhere')
  const busy = attempt.status === 'sending'
  const retryKey = attempt.status === 'idle' ? null : attempt.key

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          close <kbd className="keycap">Esc</kbd>
        </button>

        <div className="page-head">
          <h1 className="page-title">Walking skeleton</h1>
          <p className="page-sub">
            One store, two readers. This panel is React, subscribed through
            <code> useSyncExternalStore</code>; the world behind it is a renderer with its own
            frame loop reading the same object. Neither owns it, and the canvas never unmounts —
            press Escape and watch it keep going.
          </p>
        </div>

        <div className="overlay-body">
          <div className="grid wide-left">
            <div className="stack">
              <Life />

              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">Walking skeleton</span>
                  <span className="panel-note">one command, one transaction</span>
                </div>
                <div className="panel-body">
                  <p className="faint tiny mt0">
                    The button below sends a command over REST, which writes and reads in a single
                    Postgres transaction. So does a click on the world itself — canvas input goes
                    through the same command layer this panel uses, because there is only one.
                    Every number here was sent by the server; nothing was worked out locally.
                  </p>

                  {elsewhere && <Elsewhere mark={elsewhere} />}

                  {attempt.status === 'refused' && (
                    <div className="notice">
                      <strong>{attempt.code || '—'}</strong> {attempt.message}
                    </div>
                  )}

                  <div className="metric-row">
                    <div>
                      <div className="metric-name">Probes recorded</div>
                      <div className="metric-sub">rows in skeleton_probe</div>
                    </div>
                    <div className="metric-value">{count}</div>
                  </div>

                  <div className="metric-row">
                    <div>
                      <div className="metric-name">Last delivery</div>
                      <div className="metric-sub">{deliverySaid(attempt)}</div>
                    </div>
                    <div className="metric-value">
                      <Delivery attempt={attempt} />
                    </div>
                  </div>

                  <div className="metric-row">
                    <div>
                      <div className="metric-name">Last probe here</div>
                      <div className="metric-sub">
                        {last
                          ? last.by === 'you'
                            ? 'from this tab, out of the command response'
                            : 'from somewhere else, over the socket'
                          : 'nothing yet in this session'}
                      </div>
                    </div>
                    <div className="metric-value">{last ? last.label || '—' : '—'}</div>
                  </div>

                  <div className="metric-row">
                    <div>
                      <div className="metric-name">Written at</div>
                      <div className="metric-sub">now(), from the database clock</div>
                    </div>
                    <div className="metric-value">
                      {last ? new Date(last.notedAt).toLocaleTimeString() : '—'}
                    </div>
                  </div>

                  <label className="field">
                    <span className="metric-sub">
                      Label — optional, and no two probes may share one
                    </span>
                    <input
                      value={label}
                      onChange={(e) => setLabel(e.target.value)}
                      placeholder="unlabelled"
                      disabled={busy}
                    />
                  </label>

                  <div className="btn-row">
                    <button
                      className="btn"
                      onClick={() => void probe(label, newKey())}
                      disabled={busy}
                    >
                      {busy ? 'Probing…' : 'Probe the database'}
                    </button>
                    <button
                      className="btn ghost"
                      onClick={() => retryKey && void probe(label, retryKey)}
                      disabled={busy || !retryKey}
                      title="What a network retry or a page reload does"
                    >
                      Send again, same key
                    </button>
                  </div>

                  <p className="faint tiny">
                    Send again unchanged and the answer is replayed from the record — same values,
                    same row count. Change the label first and the server refuses it 422: one key,
                    one request.
                  </p>
                </div>
              </div>
            </div>

            <div className="stack">
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">You</span>
                  <span className="panel-note">signed in</span>
                </div>
                <div className="panel-body">
                  <Whoami player={player} />
                </div>
              </div>

              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">The server</span>
                  <span className="panel-note">from the boot snapshot</span>
                </div>
                <div className="panel-body">
                  <div className="metric-row">
                    <div>
                      <div className="metric-name">Postgres</div>
                      <div className="metric-sub">server_version</div>
                    </div>
                    <div className="metric-value">{server?.database ?? '—'}</div>
                  </div>

                  <div className="metric-row">
                    <div>
                      <div className="metric-name">Schema</div>
                      <div className="metric-sub">migration applied at startup</div>
                    </div>
                    <div className="metric-value">{server?.schema ?? '—'}</div>
                  </div>

                  <p className="faint tiny">
                    Open this page in a second tab and probe there. This one is told over the
                    socket, unasked, and the mark that appears in the world is drawn cold rather
                    than warm because it came from somewhere else.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * The character: money, where they are, and where they live (§3.3).
 *
 * Everything here is the server's. The balance is cents the server sent and
 * this formats; the districts are the city's own vocabulary, sent with the
 * snapshot rather than compiled in, so settling §17's open question about which
 * city this is changes nothing in the client. How many paid actions are left
 * today is derived by the server on read (§3.4, §3.3) — the client counts
 * nothing, which is why the number is right at the player's midnight rather
 * than at this tab's.
 */
function Life() {
  const character = useWorld((w) => w.character)
  const places = useWorld((w) => w.places)
  const busy = useWorld((w) => w.attempt.status === 'sending')

  if (!character) return null

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">Your life</span>
        <span className="panel-note">{character.at.note}</span>
      </div>

      <div className="panel-body">
        <div className="readout">
          <div>
            <div className="stat-big">{spending(character.money)}</div>
            <div className="stat-unit">money · buys instrumentation, not power</div>
          </div>
        </div>

        <div className="metric-row">
          <div>
            <div className="metric-name">Standing in</div>
            <div className="metric-sub">where you are right now</div>
          </div>
          <div className="metric-value">{character.at.name}</div>
        </div>

        <div className="metric-row">
          <div>
            <div className="metric-name">Home</div>
            <div className="metric-sub">where you live, and where you walk back to</div>
          </div>
          <div className="metric-value">{character.home.name}</div>
        </div>

        <div className="metric-row">
          <div>
            <div className="metric-name">Paid actions left today</div>
            <div className="metric-sub">
              {character.earningsLeft > 0
                ? 'income is structural — this is the small part that is not'
                : 'the rest of the day is yours, but it does not pay'}
            </div>
          </div>
          <div className="metric-value">
            {character.earningsLeft}
            <span className="metric-sub"> of {character.earningsLeft + character.earningsTaken}</span>
          </div>
        </div>

        <div className="btn-row">
          <button
            className="btn primary"
            onClick={() => void work()}
            disabled={busy || character.earningsLeft === 0}
          >
            {character.earningsLeft > 0 ? 'Take a day\u2019s work' : 'Nothing left today'}
          </button>
        </div>

        <p className="faint tiny">
          Three paid actions a day, counted by the server against your own midnight rather than
          this tab's. The fourth is refused 409: the request is fine, and it is the day that is in
          the way.
        </p>

        <div className="btn-row">
          {places.map((place) => (
            <button
              key={place.key}
              className={`btn${place.key === character.at.key ? ' spent' : ''}`}
              onClick={() => void moveTo(place.key)}
              disabled={busy}
              title={place.note}
            >
              {place.key === character.at.key ? `You are in ${place.name}` : `Walk to ${place.name}`}
            </button>
          ))}
        </div>

        <p className="faint tiny">
          Walking is the encounter surface rather than a wait — which is why it is a command the
          server decides the outcome of, and not a camera move this client could make on its own.
        </p>
      </div>
    </div>
  )
}

/**
 * Your body, and what you can see of it (§3.16).
 *
 * The panel is simulated in full for everyone. What instrumentation changes is
 * the player's resolution on their own body, never their health — so the screen
 * is laid out **by tier**: one card per instrument, showing its readings if you
 * own it and a hatched slot per measurement if you do not. That is the same
 * shape `preview/` gives this screen, and it is the shape the design makes
 * true — the tier is the only thing standing between you and the number.
 *
 * Biological age is the one readout visible at every tier, composed by the
 * server on read from the panel rather than stored anywhere.
 */
function Body({ open, onClose }: { open: boolean; onClose: () => void }) {
  const health = useWorld((w) => w.health)
  const crossings = useWorld((w) => w.crossings)
  const money = useWorld((w) => w.character?.money ?? 0)
  const busy = useWorld((w) => w.attempt.status === 'sending')

  const [sleep, setSleep] = useState('ordinary')
  const [training, setTraining] = useState('light')
  const [diet, setDiet] = useState('mixed')

  if (!health) return null

  const baseline = health.tiers.find((t) => !t.forSale)
  const instruments = health.tiers.filter((t) => t.forSale)
  const of = (tier: string) => health.metrics.filter((m) => m.tier === tier)

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          close <kbd className="keycap">Esc</kbd>
        </button>

        <div className="page-head">
          <h1 className="page-title">Your body</h1>
          <p className="page-sub">
            All of it is being simulated, all of the time. What an instrument buys is the ability
            to <em>see</em> — a watch does not make anybody healthier. Everything in the locked
            cards is happening to you whether or not you can read it.
          </p>
        </div>

        <div className="overlay-body">
          <div className="grid wide-left">
            <div className="stack">
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">{baseline?.name ?? 'Baseline'}</span>
                  <span className="panel-note">free · from the start</span>
                </div>
                <div className="panel-body">
                  <div className="readout">
                    <div>
                      <div className="stat-big accent">{health.biologicalAge.toFixed(0)}</div>
                      <div className="stat-unit">biological age</div>
                    </div>
                    <p className="note last">
                      Composed on read from every metric below and every one you cannot see. It is
                      never stored, so it cannot go stale — and it is the one number visible at
                      every tier.
                    </p>
                  </div>

                  {crossings.map((c) => (
                    <div key={c.metric} className={`notice${c.worse ? '' : ' cool'}`}>
                      <strong>{c.metric}</strong> went from {c.from} to {c.to}.
                    </div>
                  ))}

                  {of('baseline').map((m) => (
                    <Reading key={m.key} metric={m} />
                  ))}
                </div>
              </div>

              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">Today</span>
                  <span className="panel-note">
                    {health.livedToday ? 'already lived' : 'one day, three choices'}
                  </span>
                </div>
                <div className="panel-body">
                  <Living
                    sleep={sleep}
                    training={training}
                    diet={diet}
                    onSleep={setSleep}
                    onTraining={setTraining}
                    onDiet={setDiet}
                    livedToday={health.livedToday}
                    busy={busy}
                  />
                </div>
              </div>
            </div>

            <div className="stack">
              {instruments.map((tier) => {
                const metrics = of(tier.key)
                return (
                  <div className="panel" key={tier.key}>
                    <div className="panel-head">
                      <span className="panel-title">{tier.name}</span>
                      <span className={tier.owned ? 'tag ok-text' : 'tag'}>
                        {tier.owned ? 'owned' : 'locked'}
                      </span>
                    </div>

                    <div className="panel-body">
                      {tier.owned ? (
                        metrics.map((m) => <Reading key={m.key} metric={m} />)
                      ) : (
                        <>
                          <p className="note first">
                            {metrics.length} measurements your body is already producing. You have
                            no way to read them.
                          </p>
                          <div className="slots">
                            {metrics.map((m) => (
                              <div className="locked-slot" key={m.key}>
                                <span className="locked-name">{m.name}</span>
                                <span className="locked-why">{tier.name.toLowerCase()}</span>
                              </div>
                            ))}
                          </div>
                          <div className="btn-row">
                            <button
                              className="btn"
                              onClick={() => void buyInstrument(tier.key)}
                              disabled={busy || money < tier.price}
                              title={money < tier.price ? 'more than you have' : tier.buys}
                            >
                              {tier.name} — {spending(tier.price)}
                            </button>
                          </div>
                          <p className="note last">
                            It reads what your body is already doing — and the history with it,
                            because the samples have been recorded since your first day.
                          </p>
                        </>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/** One day of living, as three closed choices. Never a free field. */
function Living({
  sleep,
  training,
  diet,
  onSleep,
  onTraining,
  onDiet,
  livedToday,
  busy,
}: {
  sleep: string
  training: string
  diet: string
  onSleep: (v: string) => void
  onTraining: (v: string) => void
  onDiet: (v: string) => void
  livedToday: boolean
  busy: boolean
}) {
  if (livedToday) {
    return (
      <div className="notice cool">
        Today has been lived. One game day is one real day — come back tomorrow, or wait for the
        rollover to live it for you.
      </div>
    )
  }

  const rows: [string, string, (v: string) => void, [string, string][]][] = [
    ['Slept', sleep, onSleep, [['short', '5 hours'], ['ordinary', '7 hours'], ['full', '8½ hours']]],
    ['Trained', training, onTraining, [['rest', 'not at all'], ['light', 'lightly'], ['hard', 'hard']]],
    ['Ate', diet, onDiet, [['poor', 'badly'], ['mixed', 'mixed'], ['clean', 'well']]],
  ]

  return (
    <>
      {rows.map(([label, value, set, options]) => (
        <div className="metric-row" key={label}>
          <div>
            <div className="metric-name">{label}</div>
          </div>
          <div className="btn-row">
            {options.map(([key, said]) => (
              <button
                key={key}
                className={`btn small${key === value ? ' spent' : ''}`}
                onClick={() => set(key)}
                disabled={busy}
              >
                {said}
              </button>
            ))}
          </div>
        </div>
      ))}

      <div className="btn-row">
        <button
          className="btn primary"
          onClick={() => void liveADay({ sleep, training, diet })}
          disabled={busy}
        >
          Live today
        </button>
      </div>

      <p className="faint tiny">
        Three choices, no free fields — this is what a routine will produce automatically once
        routines exist, and what the daily rollover will apply at your own midnight.
      </p>
    </>
  )
}

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
function Rules({ open, onClose }: { open: boolean; onClose: () => void }) {
  const routines = useWorld((w) => w.routines)
  const health = useWorld((w) => w.health)
  const busy = useWorld((w) => w.attempt.status === 'sending')

  // What the player has picked but not yet stated. Empty is the ordinary case:
  // the rules on screen are the server's until somebody touches one.
  const [draft, setDraft] = useState<Record<string, string>>({})

  if (!routines) return null

  const answer = (r: Routine) => draft[r.key] ?? r.chosen
  const unstated = routines.routines.some((r) => answer(r) !== r.chosen)

  async function state() {
    if (!routines) return
    const choices: Record<string, string> = {}
    for (const r of routines.routines) choices[r.key] = answer(r)
    await setRoutines(choices)
    setDraft({})
  }

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          close <kbd className="keycap">Esc</kbd>
        </button>

        <div className="page-head">
          <h1 className="page-title">Your routines</h1>
          <p className="page-sub">
            Rules for the body — the same kind of thing the playbook will hold for judgement, and
            written the same way. You answer four standing questions once, and your days are made
            out of the answers. Nothing here is a dial: what you can put in a routine is what you
            can think of arranging.
          </p>
        </div>

        <div className="overlay-body">
          <div className="grid wide-left">
            <div className="stack">
              {routines.routines.map((r) => (
                <RoutineCard
                  key={r.key}
                  routine={r}
                  answer={answer(r)}
                  busy={busy}
                  onAnswer={(option) => setDraft({ ...draft, [r.key]: option })}
                />
              ))}
            </div>

            <div className="stack">
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">In force</span>
                  <span className="panel-note">{routines.set ? 'set by you' : 'never set'}</span>
                </div>
                <div className="panel-body">
                  {unstated ? (
                    <div className="notice">
                      You have changed an answer and not stated it yet. The rules are one thing, so
                      all four go at once.
                    </div>
                  ) : (
                    <p className="faint tiny mt0">
                      {routines.set
                        ? 'These are the rules your days are made out of. Change one and state them again.'
                        : 'You have never set these, so your days are whatever happens — which is itself a set of rules, and the one below is what it looks like.'}
                    </p>
                  )}

                  <div className="btn-row">
                    <button
                      className="btn primary"
                      onClick={() => void state()}
                      disabled={busy || !unstated}
                    >
                      {unstated ? 'State these rules' : 'Nothing to state'}
                    </button>
                  </div>
                </div>
              </div>

              <Ahead />

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
        </div>
      </div>
    </div>
  )
}

/**
 * One routine: the question, the rule in force, and the answers it would take.
 *
 * A locked routine still says what it is and what would open it (§3.6 — struggle
 * before mastery, so a tool arrives after the problem it solves). Withholding
 * the existence of it would teach nothing; what is withheld is the ability to
 * arrange it.
 */
function RoutineCard({
  routine,
  answer,
  busy,
  onAnswer,
}: {
  routine: Routine
  answer: string
  busy: boolean
  onAnswer: (option: string) => void
}) {
  const chosen = routine.options.find((o) => o.key === answer)
  const pending = answer !== routine.chosen

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">{routine.name}</span>
        <span className={routine.unlocked ? 'panel-note' : 'tag'}>
          {routine.unlocked ? (pending ? 'not stated yet' : 'in force') : 'locked'}
        </span>
      </div>

      <div className="panel-body">
        <div className="metric-row plain">
          <div>
            <div className="metric-name">{routine.asks}</div>
          </div>
        </div>

        {routine.unlocked ? (
          <>
            {/* The rule as it stands, written out. `kw` is grammar rather than
                a term — it is what makes a picked option read as a sentence
                somebody said instead of as a setting somebody toggled. */}
            {chosen && (
              <div className="rule">
                <div className="rule-line">
                  <span className="kw">my rule</span>
                  <span className="token act">{chosen.says}</span>
                </div>
                <div className="rule-meta">
                  <span>{chosen.costs}</span>
                </div>
              </div>
            )}

            <div className="tokens">
              {routine.options.map((o) => (
                <span
                  key={o.key}
                  role="button"
                  tabIndex={busy ? -1 : 0}
                  title={o.costs}
                  className={`token pickable${o.key === answer ? ' on act' : ' cond'}`}
                  onClick={() => !busy && onAnswer(o.key)}
                  onKeyDown={(e) => {
                    if (!busy && (e.key === 'Enter' || e.key === ' ')) onAnswer(o.key)
                  }}
                >
                  {o.says}
                </span>
              ))}
            </div>
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

/**
 * Where the rules lead, if they are kept.
 *
 * §3.4 asks the player to *watch* a routine move their biomarkers, and the
 * honest version of that takes real days — one game day is one real day, and
 * nothing here changes it. So the screen shows both: the days as they are lived,
 * on the body panel, and the arc they are on here. Every number was composed by
 * the server on read from the panel and the rules (§3.3); this client projects
 * nothing.
 *
 * It is instrumented like the panel is, because a projection is a reading of a
 * body too — a player with no watch is not told what their HRV will be either.
 */
function Ahead() {
  const forecast = useWorld((w) => w.forecast)
  const health = useWorld((w) => w.health)

  if (!forecast || !health || forecast.biologicalAge.length === 0) return null

  const then = forecast.biologicalAge[forecast.biologicalAge.length - 1]
  const move = then - health.biologicalAge
  const visible = health.metrics.filter((m) => m.visible)

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">If you keep them</span>
        <span className="panel-note">{forecast.days} days</span>
      </div>

      <div className="panel-body">
        <div className="readout">
          <div>
            <div className={`stat-big${move < 0 ? ' accent' : ''}`}>{then.toFixed(1)}</div>
            <div className="stat-unit">
              biological age in {forecast.days} days · {move >= 0 ? '+' : ''}
              {move.toFixed(1)} from today
            </div>
          </div>
        </div>

        <Trend history={forecast.biologicalAge} from="today" to={`in ${forecast.days} days`} />

        {visible.map((m) => {
          const values = forecast.metrics.find((f) => f.key === m.key)?.values ?? []
          if (values.length === 0) return null
          return (
            <div className="metric-row" key={m.key}>
              <div>
                <div className="metric-name">{m.name}</div>
                <div className="metric-sub">{m.means}</div>
              </div>
              <div className="metric-value">
                {values[values.length - 1].toFixed(m.decimals)}
                {m.unit && <span className="metric-sub"> {m.unit}</span>}
              </div>
            </div>
          )
        })}

        <p className="faint tiny">
          Worked out by running the same rules the days are made of, forward. It is never stored —
          it is a statement about a fortnight that has not happened, and one of those kept anywhere
          would go stale the moment you changed an answer.
        </p>
      </div>
    </div>
  )
}

/** One metric with an instrument behind it, and the trend that instrument saw. */
function Reading({ metric }: { metric: Metric }) {
  const value = metric.value ?? 0

  return (
    <>
      <div className="metric-row">
        <div>
          <div className="metric-name">{metric.name}</div>
          <div className="metric-sub">{metric.means}</div>
        </div>
        <div className="metric-value">
          {value.toFixed(metric.decimals)}
          {metric.unit && <span className="metric-sub"> {metric.unit}</span>}
        </div>
      </div>
      {/* Under the row rather than inside it: a metric row's third column is
          104px of bar, and a trend wants the width of the card. */}
      {metric.history.length > 1 && <Trend history={metric.history} />}
    </>
  )
}

/**
 * A trend, drawn from the samples the server sent.
 *
 * Pure presentation: the numbers are the server's and this only decides where
 * they land in a 280×62 box.
 */
function Trend({ history, from, to }: { history: number[]; from?: string; to?: string }) {
  const low = Math.min(...history)
  const high = Math.max(...history)
  const span = high - low || 1

  const points = history.map((v, i) => {
    const x = (i / Math.max(history.length - 1, 1)) * 280
    // SVG y grows downward, so a high reading is a low number.
    const y = 56 - ((v - low) / span) * 50
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })

  return (
    <div className="spark">
      <svg viewBox="0 0 280 62" preserveAspectRatio="none">
        <polyline
          fill="none"
          stroke="var(--ink-plot)"
          strokeWidth="1.6"
          strokeLinejoin="round"
          points={points.join(' ')}
        />
      </svg>
      {/* The same drawing reads backwards or forwards — a recorded trend and a
          projected one are the same shape, and only the axis says which. */}
      <div className="spark-axis">
        <span>{from ?? `${history.length} days ago`}</span>
        <span>{to ?? 'today'}</span>
      </div>
    </div>
  )
}

/** How this delivery was treated — the idempotency layer, said out loud. */
function Delivery({ attempt }: { attempt: Attempt }) {
  switch (attempt.status) {
    case 'idle':
      return <span className="tag dim">none yet</span>
    case 'sending':
      return <span className="tag dim">in flight</span>
    case 'applied':
      return (
        <span className={attempt.replayed ? 'tag dim' : 'tag ok-text'}>
          {attempt.replayed ? 'replayed' : 'applied'}
        </span>
      )
    case 'refused':
      return <span className="tag warn-text">refused</span>
  }
}

function deliverySaid(attempt: Attempt): string {
  switch (attempt.status) {
    case 'idle':
      return 'nothing has been sent from this tab'
    case 'sending':
      return 'waiting on the server'
    case 'applied':
      return attempt.replayed
        ? 'answered from the idempotency record — nothing was written'
        : 'applied, and recorded against its key'
    case 'refused':
      return 'the server refused it, or never heard it'
  }
}

/**
 * A probe this player made somewhere else.
 *
 * Nothing on this screen asked for it, and nothing polled for it. It arrived
 * because the server decided it had — which is the whole reason the socket
 * exists (§8) — and it went into the same store the canvas reads, so the panel
 * and the world learned about it at the same instant.
 */
function Elsewhere({ mark }: { mark: Mark }) {
  return (
    <div className="notice cool">
      <strong>Meanwhile</strong> — another tab probed{' '}
      {mark.label ? <code>{mark.label}</code> : 'unlabelled'} at{' '}
      {new Date(mark.notedAt).toLocaleTimeString()}.
    </div>
  )
}

/** Who the server says you are, and the way out. */
function Whoami({ player }: { player: Player }) {
  const [leaving, setLeaving] = useState(false)

  return (
    <div className="whoami">
      {player.avatarUrl && (
        <img className="whoami-face" src={player.avatarUrl} alt="" width={32} height={32} />
      )}
      <div className="whoami-who">
        <div className="metric-name">{player.name}</div>
        <div className="metric-sub">{player.email}</div>
      </div>
      <button
        className="btn ghost small"
        onClick={() => {
          setLeaving(true)
          void signOut()
        }}
        disabled={leaving}
      >
        {leaving ? 'Signing out…' : 'Sign out'}
      </button>
    </div>
  )
}

/** The most recent mark that came from a given direction. */
function lastFrom(marks: Mark[], by: Mark['by']): Mark | null {
  for (let i = marks.length - 1; i >= 0; i--) {
    if (marks[i].by === by) return marks[i]
  }
  return null
}

/**
 * Cents, as money reads.
 *
 * Presentation, and the only arithmetic in this file: the server sends an
 * integer number of cents and this divides it by a hundred to write it down.
 */
function spending(cents: number): string {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100)
}
