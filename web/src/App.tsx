import { useEffect, useState } from 'react'

import { newKey } from './commands'
import { TIERS } from './render/quality'
import { signInURL, type Player } from './session'
import { patch } from './world/store'
import {
  buyInstrument,
  liveADay,
  liveByRoutine,
  loadBoardPage,
  loadBoards,
  moveTo,
  probe,
  setRoutines,
  signOut,
  work,
} from './world/actions'
import { districts, inside, named, waysBy } from './world/city'
import { useWorld } from './world/react'
import type {
  Attempt,
  BoardId,
  BoardPage,
  Mark,
  Metric,
  Place,
  Routine,
  Slot,
  Trip,
  Way,
} from './world/state'

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
type Screen = 'readout' | 'body' | 'rules' | 'map' | 'standing' | 'boards' | null

const KEYS: Record<string, Exclude<Screen, null>> = {
  s: 'readout',
  h: 'body',
  r: 'rules',
  m: 'map',
  k: 'standing',
  l: 'boards',
}

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
            <KeyHint keys={['K']}>where you stand</KeyHint>
            <KeyHint keys={['L']}>the boards</KeyHint>
            <KeyHint keys={['M']}>the city</KeyHint>
          </>
        )}
        {session.status === 'in' && !screen && <KeyHint keys={['Click']}>walk there</KeyHint>}
        {session.status === 'in' && !screen && <Ways />}
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
          <Standing open={screen === 'standing'} onClose={() => setScreen(null)} />
          <BoardsScreen open={screen === 'boards'} onClose={() => setScreen(null)} />
          <Map open={screen === 'map'} onClose={() => setScreen(null)} />
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
  const standing = useWorld((w) => w.standing)

  if (!character) {
    return (
      <div className="place-chip">
        <span className="place-name">The world</span>
        <span className="place-where">nobody signed in</span>
      </div>
    )
  }

  // The zone the world layer reports is the more particular answer, and the
  // district is the one it sits inside: standing on your own stoop is standing
  // on the East Side. `zones` arrives smallest first, so the first is the spot.
  const spot = standing?.zones[0] ?? null

  return (
    <div className="place-chip">
      <span className="place-name">{spot ? spot.name : character.at.name}</span>
      <span className="place-where">
        {spot
          ? character.at.name
          : character.at.key === character.home.key
            ? 'home'
            : `home is ${character.home.name}`}
      </span>
    </div>
  )
}

/**
 * The ways out of the spot the character is standing on.
 *
 * A portal is an authored object in the map (§10.3) and it is where a way *is*:
 * walk onto it and you go through it. Which is why this says "you are standing
 * on one" rather than offering a button — the input is the world, and it goes
 * through the same command layer a button would (§10.1).
 */
function Ways() {
  const standing = useWorld((w) => w.standing)
  const places = useWorld((w) => w.places)
  if (!standing || standing.portals.length === 0) return null

  return (
    <>
      {standing.portals.map((portal) => (
        <span className="keybar-item" key={portal.key}>
          <span className="keybar-label">
            you are in the way through to {named(places, portal.to).name}
          </span>
        </span>
      ))}
    </>
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
 * Every one of them comes out of the same store the renderer reads sixty times
 * a second — one read by React when it changes, one read by the world in its
 * own frame loop, and neither owning it.
 */
function Pods({ onOpen }: { onOpen: (screen: Screen) => void }) {
  const count = useWorld((w) => w.probes.count)
  const link = useWorld((w) => w.link)
  const character = useWorld((w) => w.character)
  const health = useWorld((w) => w.health)
  const karma = useWorld((w) => w.karma)

  const said = {
    connecting: { text: 'connecting', tone: '' },
    live: { text: 'live', tone: 'accent' },
    offline: { text: 'reconnecting', tone: 'warn' },
  }[link]

  return (
    <div className="pods">
      {/* No level, and so no level bar (§3.5). What replaces it is one number
          the server composes on read out of five factors, three of which are
          never named — which is why it is a pod like the others rather than a
          progress meter with a maximum to fill. */}
      {karma && (
        <div className="pod" onClick={() => onOpen('standing')} title="where you stand">
          <span className="pod-glyph">☯</span>
          <span className="pod-text">
            <span className="pod-value">{karma.karma.toFixed(0)}</span>
            <span className="pod-label">karma</span>
          </span>
        </div>
      )}

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
 * street is still there, the character is still standing where they walked to,
 * and the socket is still connected. §10.1's property, made visible.
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
                    Postgres transaction. There is one command layer, and both trees dispatch
                    through it — a tap on the world is a walk across one painted location, which
                    changes nothing the server holds, and stepping between locations will come
                    through here rather than around it. Every number on this panel was sent by the
                    server; nothing was worked out locally.
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

              <Drawing />
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * How the world is drawn, and how well it is going (§10.4).
 *
 * Two tiers, switchable here and at any moment: **Plain** is the painting with
 * nothing over it — no full-screen filter and no particle, and the light in the
 * world is the light that was painted into the plate. **Lit** adds the filter
 * stack, lamps that reach the person walking under them, air with something in
 * it, and a graded frame.
 *
 * Plain is what every device gets until somebody says otherwise, and that is the
 * design rather than caution: a look built around a lighting pass cannot have
 * the lighting pass taken away later, so the plain one has to be the one that is
 * right. The choice is remembered in this browser, because it is an answer about
 * this machine and a phone is not a desktop.
 *
 * The world also measures itself. A machine that asks for Lit and then cannot
 * hold fifty frames a second is put back, and told why — which is the only
 * honest version of "verified on integrated graphics" that can live in the game
 * rather than in somebody's test rig.
 */
function Drawing() {
  const quality = useWorld((w) => w.quality)
  const renderer = useWorld((w) => w.renderer)

  if (renderer === 'unsupported') return null

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">The world</span>
        <span className="panel-note">{quality.fps > 0 ? `${quality.fps} fps` : 'measuring'}</span>
      </div>

      <div className="panel-body">
        {quality.demoted && (
          <div className="notice">
            This machine could not hold the lit tier at a steady frame rate, so the world went back
            to plain. Ask for it again if you want to see for yourself.
          </div>
        )}

        <div className="btn-row">
          {TIERS.map((tier) => (
            <button
              key={tier.key}
              className={`btn small${tier.key === quality.tier ? ' spent' : ''}`}
              onClick={() => patch.quality(tier.key)}
              disabled={tier.key === quality.tier}
              title={tier.says}
            >
              {tier.name}
            </button>
          ))}
        </div>

        <p className="faint tiny">
          {TIERS.find((t) => t.key === quality.tier)?.says}
        </p>

        <p className="faint tiny">
          Plain is the floor, and the tier the look is designed at — a game that looks right without
          the juice pass looks better with it, and the reverse is never true. Switching is
          immediate: the canvas does not unmount, the character does not move, and nothing is
          re-fetched.
        </p>
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

        <p className="faint tiny">
          Walking is the encounter surface rather than a wait — which is why going anywhere is a
          command the server decides the outcome of, and not a camera move this client could make
          on its own. Press <kbd className="keycap">M</kbd> for the city, or walk onto a door.
        </p>
      </div>
    </div>
  )
}

/**
 * Where you stand: the composite, and the board it is made of (§3.5).
 *
 * There is no level in this game, so there is no level bar on this screen. What
 * replaces it is one number — Karma — composed by the server on read out of five
 * factors, **three of which are never named**. Health and Money are shown, as
 * the readings the design surfaces them as: a biological age, and the amount
 * being held. The other three arrive as slots with nothing in them.
 *
 * That is not a rendering decision. There is no key, no name and no value in the
 * payload for a hidden factor, so there is nothing here to accidentally reveal —
 * the veil is applied on the server, on the way out, the same way instrumentation
 * is on the body screen.
 *
 * The formula is never stated, and neither is a weight, a per-factor score, nor
 * a maximum Karma could reach. §3.5: *what are the hidden factors, how are they
 * calculated, what grows them* is the question the game is asking, and a screen
 * that answered it would be a screen with nothing left to find.
 */
function Standing({ open, onClose }: { open: boolean; onClose: () => void }) {
  const karma = useWorld((w) => w.karma)
  const places = useWorld((w) => w.places)

  if (!karma) return null

  const shown = karma.slots.filter((s) => s.shown)
  const sealed = karma.slots.filter((s) => !s.shown)
  const here = named(places, karma.where)

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          close <kbd className="keycap">Esc</kbd>
        </button>

        <div className="page-head">
          <h1 className="page-title">Where you stand</h1>
          <p className="page-sub">
            Not a level, and not a bar to fill. It is a reading of several things at once, taken
            fresh every time anything about you changes — which is why it can go down.
          </p>
        </div>

        <div className="overlay-body">
          <div className="grid wide-left">
            <div className="stack">
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">Karma</span>
                  <span className="panel-note">composed on read · stored nowhere</span>
                </div>
                <div className="panel-body">
                  <div className="readout">
                    <div>
                      <div className="stat-big accent">{karma.karma.toFixed(0)}</div>
                      <div className="stat-unit">karma</div>
                    </div>
                    <p className="note last">
                      Five things go into this. You are shown two of them.
                    </p>
                  </div>

                  {shown.map((slot) => (
                    <div className="metric-row" key={slot.key}>
                      <div>
                        <div className="metric-name">{slot.name}</div>
                        <div className="metric-sub">{slot.note}</div>
                      </div>
                      <div className="metric-value accent">{reading(slot)}</div>
                    </div>
                  ))}
                </div>
              </div>

              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">The rest</span>
                  <span className="tag dim">{sealed.length} unnamed</span>
                </div>
                <div className="panel-body">
                  <p className="note first">
                    Something is being measured in each of these. They move when you commit to
                    something — never when you say something.
                  </p>
                  <div className="slots">
                    {sealed.map((_, i) => (
                      // Keyed by position, because position is all a sealed
                      // slot has. There is no key on the wire to use instead.
                      <div className="locked-slot dim" key={i}>
                        <span className="locked-name">? ? ? ? ?</span>
                        <span className="locked-why">unnamed</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>

            <div className="stack">
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">Here</span>
                  <span className="panel-note">{karma.days === 1 ? '1 day' : `${karma.days} days`}</span>
                </div>
                <div className="panel-body">
                  <div className="env-chip">
                    <span className="env-label">standing in</span>
                    <span className="env-name">{here.name}</span>
                  </div>

                  {/* §3.15: what a place still pays, falling as it is worked
                      out. Stated rather than explained — the number going down
                      is the whole of the lesson, and the game never says why. */}
                  <div className="decay-row">
                    <span className="label">returns</span>
                    <span className="decay-track">
                      <span style={{ width: `${Math.round(karma.familiarity * 100)}%` }} />
                    </span>
                    <span className="amount">{Math.round(karma.familiarity * 100)}%</span>
                  </div>

                  <p className="note last">
                    {karma.familiarity > 0.75
                      ? 'There is plenty here you have not run into yet.'
                      : karma.familiarity > 0.3
                        ? 'The same effort is starting to be worth less around here.'
                        : 'You have got most of what this place has. Somewhere else would pay better.'}
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

/** A revealed slot's number, in the units the server said to read it in. */
function reading(slot: Slot): string {
  if (slot.value === null) return ''
  return slot.reads === 'money' ? spending(slot.value) : slot.value.toFixed(0)
}

/**
 * The leaderboards (§3.5).
 *
 * §3.5 asks for a **distributed** board: one overall board for the composite,
 * plus a separate board for each individual factor — and the three hidden
 * factors get boards too, because a ranking is a *position* rather than a
 * number, and showing one discloses nothing about the formula.
 *
 * So three of the tabs below have no name. They are not placeholders and they
 * are not disabled: they rank real players by something real, and finding out
 * what is the question §3.5 is asking. Nothing on this screen could reveal it if
 * it wanted to — a sealed board's rows arrive with `value: null`, and the field
 * a number would be in is empty on the wire rather than filtered here.
 *
 * The list of other people is the one thing in this client that is fetched
 * rather than pushed (§10.2's read-only paginated list). Where the *player*
 * stands is in the store, because the socket moves it when somebody else plays.
 */
function BoardsScreen({ open, onClose }: { open: boolean; onClose: () => void }) {
  const boards = useWorld((w) => w.boards)
  const [key, setKey] = useState('karma')
  const [page, setPage] = useState(1)
  const [showing, setShowing] = useState<BoardPage | null>(null)

  // Opening the screen is what loads it. A leaderboard nobody is looking at is
  // a fetch nobody asked for, and the boot snapshot deliberately omits it.
  useEffect(() => {
    if (open) void loadBoards()
  }, [open])

  // The page is re-read whenever the board, the page, or the *snapshot* changes
  // — the last of those is how a rank that moved while this was open arrives:
  // the socket patches `refreshedAt`, and the list underneath it is stale.
  const refreshedAt = boards?.refreshedAt ?? ''
  useEffect(() => {
    if (!open) return
    let live = true
    void loadBoardPage(key, page).then((got) => {
      if (live) setShowing(got)
    })
    return () => {
      live = false
    }
  }, [open, key, page, refreshedAt])

  function choose(next: string) {
    setKey(next)
    setPage(1)
  }

  const standing = boards?.boards.find((p) => p.board.key === key) ?? null

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          close <kbd className="keycap">Esc</kbd>
        </button>

        <div className="page-head">
          <h1 className="page-title">The boards</h1>
          <p className="page-sub">
            One board for the whole of it, and one for each of the things it is made of. Three of
            those have no name. They rank you all the same.
          </p>
        </div>

        <div className="overlay-body">
          {!boards ? (
            <p className="note first">Reading the boards…</p>
          ) : (
            <>
              <div className="tabs">
                {boards.boards.map((placing, i) => (
                  <a
                    key={placing.board.key}
                    className={placing.board.key === key ? 'active' : ''}
                    tabIndex={0}
                    onClick={() => choose(placing.board.key)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') choose(placing.board.key)
                    }}
                  >
                    {/* A sealed board is keyed by position on the wire, and the
                        tab says so — the same "there is something here" a
                        locked slot says on the standing screen. */}
                    {placing.board.shown ? placing.board.name : `? ? ? ? ? (${i})`}
                  </a>
                ))}
              </div>

              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">
                    {standing?.board.shown ? standing.board.name : 'Something'}
                  </span>
                  <span className="panel-note">
                    {boards.players === 1 ? '1 player' : `${boards.players} players`}
                    {boards.refreshedAt ? ` · as of ${clockOf(boards.refreshedAt)}` : ''}
                  </span>
                </div>

                <div className="panel-body">
                  <p className="note first">{standing?.board.note ?? ''}</p>

                  {standing && standing.rank > 0 && (
                    <div className="env-chip">
                      <span className="env-label">you are</span>
                      <span className="env-name">
                        {ordinal(standing.rank)} of {standing.of}
                      </span>
                    </div>
                  )}

                  {!showing || showing.rows.length === 0 ? (
                    <p className="note last">
                      Nobody is on this board yet. It is rebuilt in the background rather than
                      worked out when you look, so it will fill in on its own.
                    </p>
                  ) : (
                    <>
                      {showing.rows.map((row) => (
                        <div className={`board${row.you ? ' you' : ''}`} key={row.rank}>
                          <span className="rank">{row.rank}</span>
                          <span>
                            <b>{row.name}</b>
                            <i>{row.you ? 'you' : ''}</i>
                          </span>
                          <span className="metric-value">
                            {rung(showing.board.reads, row.value)}
                          </span>
                        </div>
                      ))}

                      {showing.pages > 1 && (
                        <div className="btn-row">
                          <button
                            className="btn small"
                            onClick={() => setPage(showing.page - 1)}
                            disabled={showing.page <= 1}
                          >
                            back
                          </button>
                          <span className="faint tiny">
                            page {showing.page} of {showing.pages}
                          </span>
                          <button
                            className="btn small"
                            onClick={() => setPage(showing.page + 1)}
                            disabled={showing.page >= showing.pages}
                          >
                            next
                          </button>
                        </div>
                      )}
                    </>
                  )}

                  <p className="faint tiny">
                    Rebuilt by the server on a schedule, not when you open this. Which is why it
                    says when it was taken, and why your own place can move while you are reading
                    it — somebody else played.
                  </p>
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

/**
 * A rung's number, in whatever the server said to read it in.
 *
 * Empty for a sealed board — and there is nothing to hide, because the value
 * arrived null. The formatting is chosen by a string the server sent, so this
 * client still knows no rule about what any factor means.
 */
function rung(reads: BoardId['reads'], value: number | null): string {
  if (value === null) return ''
  return reads === 'money' ? spending(value) : value.toFixed(0)
}

/** 1 → 1st. Presentation, and English's problem rather than the server's. */
function ordinal(n: number): string {
  const tens = n % 100
  if (tens >= 11 && tens <= 13) return `${n}th`
  return `${n}${['th', 'st', 'nd', 'rd'][n % 10] ?? 'th'}`
}

/** An RFC 3339 instant, as a local time somebody can read. */
function clockOf(at: string): string {
  const when = new Date(at)
  if (Number.isNaN(when.getTime())) return at
  return when.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

/**
 * The city map (§3.12).
 *
 * §3.12's layers are home grid → interior → district → city map, and the last of
 * them is **not a place**: it is the district layer seen from above, a way of
 * choosing where to go rather than somewhere to go. Making it somewhere you
 * could stand would have made it a free ride between districts, which is
 * exactly the thing the fare below is meant to buy.
 *
 * Two kinds of offer, and the difference between them is the whole ticket:
 *
 *   - **The ways out of here** come with their own fares, because the server
 *     sent them attached to the place. A train is only ever offered where the
 *     network actually calls — you cannot ride out of your own kitchen.
 *   - **Everywhere else** is a walk, offered without a number on it. The client
 *     could add up a route itself and would be right nearly every time; the
 *     first time it was wrong the player would have been quoted a price nobody
 *     charged. So it asks, and the answer comes back with the journey in it
 *     (§10.2, rule 2).
 *
 * Nothing here unmounts the world. This is an overlay above a canvas that is
 * still running (§10.1) — walk somewhere from this panel and the scene behind it
 * changes underneath while the panel stays open.
 */
function Map({ open, onClose }: { open: boolean; onClose: () => void }) {
  const character = useWorld((w) => w.character)
  const places = useWorld((w) => w.places)
  const trip = useWorld((w) => w.trip)
  const busy = useWorld((w) => w.attempt.status === 'sending')

  if (!character) return null

  const here = character.at
  const onFoot = waysBy(here, 'foot')
  const byTransit = waysBy(here, 'transit')

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          close <kbd className="keycap">Esc</kbd>
        </button>

        <div className="page-head">
          <h1 className="page-title">The city</h1>
          <p className="page-sub">
            One city, built deep. Going anywhere costs no game time — what separates walking from
            the train is what happens on the way. Walking is free and is where things happen to
            you; the fare buys you out of that for one journey. A player who always takes the
            fastest route sees less of their own life.
          </p>
        </div>

        <div className="overlay-body">
          <div className="grid wide-left">
            <div className="stack">
              <div className="panel">
                <div className="panel-head">
                  <span className="panel-title">Standing in {here.name}</span>
                  <span className="panel-note">{here.note}</span>
                </div>
                <div className="panel-body">
                  <div className="btn-row">
                    {onFoot.map((way) => (
                      <WayButton key={`foot-${way.to}`} way={way} places={places} busy={busy} />
                    ))}
                  </div>

                  {byTransit.length > 0 ? (
                    <>
                      <div className="btn-row">
                        {byTransit.map((way) => (
                          <WayButton
                            key={`transit-${way.to}`}
                            way={way}
                            places={places}
                            busy={busy}
                          />
                        ))}
                      </div>
                      <p className="faint tiny">
                        The network calls here. A ride takes the fare and passes through nothing —
                        no street, no doorway, and nobody who might have stopped you.
                      </p>
                    </>
                  ) : (
                    <p className="faint tiny">
                      No stop here. Leaving anywhere indoors begins on foot, however much money you
                      have — which is the one thing a fare cannot buy.
                    </p>
                  )}
                </div>
              </div>

              {trip && <Journey trip={trip} places={places} />}

              {districts(places).map((street) => (
                <div className="panel" key={street.key}>
                  <div className="panel-head">
                    <span className="panel-title">{street.name}</span>
                    <span className="panel-note">{street.note}</span>
                  </div>
                  <div className="panel-body">
                    <div className="btn-row">
                      <GoButton place={street} here={here.key} busy={busy} />
                      {inside(places, street.key).map((place) => (
                        <GoButton key={place.key} place={place} here={here.key} busy={busy} />
                      ))}
                    </div>
                  </div>
                </div>
              ))}

              <p className="faint tiny">
                Every one of these is one command — <code>character.move</code> — and the route is
                worked out by the city rather than by this panel. Most of them are not painted yet:
                the world draws the ground and says so, and the paintings arrive one at a time.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/** One step out of where you are, with the price the server put on it. */
function WayButton({ way, places, busy }: { way: Way; places: Place[]; busy: boolean }) {
  return (
    <button
      className="btn"
      onClick={() => void moveTo(way.to, way.mode)}
      disabled={busy}
      title={named(places, way.to).note}
    >
      {way.says}
      {way.fare > 0 && <span className="metric-sub"> · {spending(way.fare)}</span>}
    </button>
  )
}

/**
 * Anywhere in the city, on foot.
 *
 * No fare on it, because there is none to show: every walk in the city is free,
 * and the number of streets between here and there is the city's to know.
 */
function GoButton({ place, here, busy }: { place: Place; here: string; busy: boolean }) {
  const there = place.key === here
  return (
    <button
      className={`btn${there ? ' spent' : ''}`}
      onClick={() => void moveTo(place.key, 'foot')}
      disabled={busy || there}
      title={place.note}
    >
      {there ? `You are in ${place.name}` : `Walk to ${place.name}`}
    </button>
  )
}

/**
 * The journey just made.
 *
 * `chances` is the honest half of the trade before the event system exists
 * (#21, #22): how many places the journey passed through where something could
 * have happened. Zero for a ride, always — and that difference is the only
 * thing the fare bought.
 */
function Journey({ trip, places }: { trip: Trip; places: Place[] }) {
  if (trip.steps.length === 0) return null

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">The way you came</span>
        <span className="panel-note">{trip.by === 'foot' ? 'on foot' : 'by transit'}</span>
      </div>
      <div className="panel-body">
        {trip.steps.map((step, i) => (
          <div className="metric-row" key={`${i}-${step.to}`}>
            <div>
              <div className="metric-name">{step.says}</div>
              <div className="metric-sub">{named(places, step.to).note}</div>
            </div>
            <div className="metric-value">{named(places, step.to).name}</div>
          </div>
        ))}

        <div className="metric-row">
          <div>
            <div className="metric-name">The fare</div>
            <div className="metric-sub">
              {trip.fare > 0 ? 'what it cost to skip what was on the way' : 'walking is free'}
            </div>
          </div>
          <div className="metric-value">{trip.fare > 0 ? spending(trip.fare) : 'nothing'}</div>
        </div>

        <div className="metric-row">
          <div>
            <div className="metric-name">Places passed through</div>
            <div className="metric-sub">
              {trip.chances > 0
                ? 'each one somewhere your life could have happened to you'
                : 'the fare bought the whole journey back'}
            </div>
          </div>
          <div className="metric-value">{trip.chances}</div>
        </div>
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
