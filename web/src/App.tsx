import { useEffect, useState } from 'react'

import { newKey } from './commands'
import { signInURL, type Player } from './session'
import { moveTo, probe, signOut, work } from './world/actions'
import { useWorld } from './world/react'
import type { Attempt, Mark } from './world/state'

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

/** The key that opens the readout. One key per destination — see `ui/README`. */
const READOUT_KEY = 's'

export function App() {
  const session = useWorld((w) => w.session)
  const renderer = useWorld((w) => w.renderer)

  // The one piece of React state in the client: which panel this tab has open.
  // Not in the store, because the world does not care.
  const [open, setOpen] = useState(true)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      // A key pressed inside a field is a letter, not a shortcut.
      if (e.target instanceof HTMLInputElement) return
      if (e.key === 'Escape') setOpen(false)
      if (e.key.toLowerCase() === READOUT_KEY) setOpen(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <>
      <Where />

      {session.status === 'in' && <Pods onOpen={() => setOpen(true)} />}

      {renderer === 'unsupported' && <CannotDraw />}

      <div className="keybar">
        <KeyHint keys={[open ? 'Esc' : READOUT_KEY.toUpperCase()]}>
          {open ? 'back to the world' : 'the readout'}
        </KeyHint>
        {session.status === 'in' && <KeyHint keys={['Click']}>probe where you tapped</KeyHint>}
      </div>

      {session.status === 'unknown' && <Waiting />}
      {session.status === 'out' && <Door notice={session.notice} />}
      {session.status === 'in' && (
        <Readout player={session.player} open={open} onClose={() => setOpen(false)} />
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
function Pods({ onOpen }: { onOpen: () => void }) {
  const count = useWorld((w) => w.probes.count)
  const link = useWorld((w) => w.link)
  const character = useWorld((w) => w.character)

  const said = {
    connecting: { text: 'connecting', tone: '' },
    live: { text: 'live', tone: 'accent' },
    offline: { text: 'reconnecting', tone: 'warn' },
  }[link]

  return (
    <div className="pods">
      {character && (
        <div className="pod accent" onClick={onOpen} title="open the readout">
          <span className="pod-glyph">$</span>
          <span className="pod-text">
            <span className="pod-value">{spending(character.money)}</span>
            <span className="pod-label">money</span>
          </span>
        </div>
      )}

      <div className="pod" onClick={onOpen} title="open the readout">
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
          <Whoami player={player} />

          <Life />

          <p className="faint tiny">
            The button below sends a command over REST, which writes and reads in a single Postgres
            transaction. So does a click on the world itself — canvas input goes through the same
            command layer this panel uses, because there is only one. Every number here was sent by
            the server; nothing on this screen was worked out locally.
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

          <div className="metric-row">
            <div>
              <div className="metric-name">Postgres</div>
              <div className="metric-sub">server_version, from the boot snapshot</div>
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

          <label className="field">
            <span className="metric-sub">Label — optional, and no two probes may share one</span>
            <input
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="unlabelled"
              disabled={busy}
            />
          </label>

          <div className="btn-row">
            <button className="btn" onClick={() => void probe(label, newKey())} disabled={busy}>
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
            Send again unchanged and the answer is replayed from the record — same values, same row
            count. Change the label first and the server refuses it 422: one key, one request.
          </p>

          <p className="faint tiny">
            Open this page in a second tab and probe there. This one is told over the socket,
            unasked, and the mark that appears in the world is drawn cold rather than warm because
            it came from somewhere else. Take the network down first and it still arrives when the
            socket returns.
          </p>
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
    <>
      <div className="metric-row">
        <div>
          <div className="metric-name">Money</div>
          <div className="metric-sub">what it buys is instrumentation, not power</div>
        </div>
        <div className="metric-value">{spending(character.money)}</div>
      </div>

      <div className="metric-row">
        <div>
          <div className="metric-name">Standing in</div>
          <div className="metric-sub">{character.at.note || 'somewhere in the city'}</div>
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
        Three paid actions a day, counted by the server against your own midnight rather than this
        tab's. The fourth is refused 409: the request is fine, and it is the day that is in the way.
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
    </>
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
