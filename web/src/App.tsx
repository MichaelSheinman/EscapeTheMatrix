import { useCallback, useEffect, useState } from 'react'
import {
  command,
  CommandError,
  newKey,
  type ProbeResult,
  type Sent,
} from './commands'
import {
  currentPlayer,
  signInURL,
  signOut,
  takeSignInFailure,
  type Player,
} from './session'
import { connect, type Status } from './realtime/client'
import type { ProbeRecorded } from './realtime/messages.gen'

/*
 * The walking skeleton's one screen, now behind a door.
 *
 * It proves the path — a click here becomes a REST command, which becomes one
 * Postgres transaction, which becomes a number on screen that could not have
 * been produced client-side — plus the two guarantees on top of it: the same
 * command delivered twice applies once, and no command runs for anyone who is
 * not signed in.
 *
 * There is no anonymous mode to fall back to (technical.md §9). Signing in is
 * the first thing that happens, the identity that comes back survives a reload
 * because the credential is a cookie the server issued and this code cannot
 * read, and signing out ends the session in Postgres rather than only in this
 * tab.
 *
 * The socket adds the other direction (§8). Open this page in two tabs and
 * probe in one: the other one is told, without asking and without polling, and
 * the tab that acted is not — it already has the command's response. Kill the
 * network in between and the message is still there when the socket comes
 * back, because it was written to an outbox rather than to a connection.
 *
 * Styling comes from @etm/ui — the design system is consumed, never
 * reimplemented (§2). The class names below are its component vocabulary;
 * `ui/index.js` holds the same components as HTML strings for `preview/`, and
 * those are the translation target when real screens arrive.
 */

type Auth =
  | { status: 'loading' }
  | { status: 'out'; notice: string | null }
  | { status: 'in'; player: Player }

export function App() {
  const [auth, setAuth] = useState<Auth>({ status: 'loading' })

  // Asked once, on load. This is the whole of "the identity survives a reload":
  // the client stores nothing, and the answer comes back from the session the
  // cookie stands for.
  useEffect(() => {
    // Read before the fetch, so a failed attempt is still reported even though
    // the answer that follows it is simply "signed out".
    const notice = takeSignInFailure()

    currentPlayer()
      .then((player) =>
        setAuth(player ? { status: 'in', player } : { status: 'out', notice }),
      )
      .catch(() =>
        setAuth({
          status: 'out',
          notice: notice ?? 'The server could not be reached.',
        }),
      )
  }, [])

  // Every command goes through one middleware, so any of them can be the one
  // that discovers the session ended — expired, or signed out on another
  // device. Whichever it is, the app belongs back at the door.
  const sessionEnded = useCallback(
    () => setAuth({ status: 'out', notice: 'That session ended. Sign in again.' }),
    [],
  )

  return (
    <main className="skeleton">
      {auth.status === 'loading' && <Waiting />}
      {auth.status === 'out' && <SignIn notice={auth.notice} />}
      {auth.status === 'in' && (
        <Skeleton player={auth.player} onSessionEnded={sessionEnded} />
      )}
    </main>
  )
}

/** The moment before the server has said who this is. */
function Waiting() {
  return (
    <div className="panel">
      <div className="panel-body">
        <p className="faint tiny mt0">Checking your session…</p>
      </div>
    </div>
  )
}

/** The door. Account required up front, OAuth only, no password to type. */
function SignIn({ notice }: { notice: string | null }) {
  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">Escape the Matrix</span>
        <span className="panel-note">sign in to play</span>
      </div>

      <div className="panel-body">
        {notice && <div className="notice">{notice}</div>}

        <p className="faint tiny mt0">
          The game needs a stable identity — leaderboards and peer endorsement
          are built on one — so an account comes first. Sign-in is Google's:
          this server never sees a password and stores none.
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
  )
}

type State =
  | { status: 'idle' }
  | { status: 'sending' }
  | { status: 'ok'; sent: Sent<ProbeResult> }
  | { status: 'failed'; code: number; message: string }

function Skeleton({
  player,
  onSessionEnded,
}: {
  player: Player
  onSessionEnded: () => void
}) {
  const [state, setState] = useState<State>({ status: 'idle' })
  const [label, setLabel] = useState('')
  // The key of the last command sent, whatever became of it. Holding it is the
  // whole of what a client has to do to make its retries safe.
  const [lastKey, setLastKey] = useState<string | null>(null)

  const [link, setLink] = useState<Status>('connecting')
  // The last thing this player did somewhere else. Nothing on this screen asked
  // for it — that is the point of the socket.
  const [elsewhere, setElsewhere] = useState<ProbeRecorded | null>(null)

  useEffect(() => {
    const stream = connect({
      onStatus: setLink,
      onMessage: (message) => {
        // Exhaustive over the generated union: adding a message type on the
        // server turns an unhandled case here into a type error rather than
        // into a message quietly ignored.
        switch (message.type) {
          case 'probe.recorded':
            setElsewhere(message.payload)
            break
        }
      },
      // The replay this tab needed had already been swept, so the patches that
      // follow cannot be trusted to land on the right state. A reload *is* the
      // rehydrate — the page loads its state over REST — which is why the
      // outbox only has to be long-lived enough to cover a drop.
      onGap: () => window.location.reload(),
    })
    return () => stream.close()
  }, [])

  async function send(key: string) {
    setState({ status: 'sending' })
    setLastKey(key)
    try {
      setState({
        status: 'ok',
        sent: await command<ProbeResult>('probe', { label }, key),
      })
    } catch (err) {
      if (err instanceof CommandError && err.isSignedOut) {
        onSessionEnded()
      } else if (err instanceof CommandError) {
        setState({ status: 'failed', code: err.status, message: err.message })
      } else {
        setState({ status: 'failed', code: 0, message: 'the server could not be reached' })
      }
    }
  }

  const busy = state.status === 'sending'

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">Walking skeleton</span>
        <Link status={link} />
      </div>

      <div className="panel-body">
        <Whoami player={player} onSignedOut={onSessionEnded} />

        <p className="faint tiny">
          This page is served from inside the Go binary. The button sends a
          command over REST, which writes and reads in a single Postgres
          transaction. Every command carries an idempotency key, so sending one
          twice applies it once — and carries your session, so it runs as you.
        </p>

        {elsewhere && <Elsewhere probe={elsewhere} />}

        {state.status === 'ok' && <Result sent={state.sent} />}

        {state.status === 'failed' && (
          <div className="notice">
            <strong>{state.code || '—'}</strong> {state.message}
          </div>
        )}

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
          <button className="btn" onClick={() => send(newKey())} disabled={busy}>
            {busy ? 'Probing…' : 'Probe the database'}
          </button>
          <button
            className="btn ghost"
            onClick={() => lastKey && send(lastKey)}
            disabled={busy || !lastKey}
            title="What a network retry or a page reload does"
          >
            Send again, same key
          </button>
        </div>

        <p className="faint tiny">
          Send again unchanged and the answer is replayed from the record —
          same values, same row count. Change the label first and the server
          refuses it 422: one key, one request.
        </p>

        <p className="faint tiny">
          Open this page in a second tab and probe there. This one is told over
          the socket, unasked. Take the network down first and it is still told
          when the socket returns: the message went to a per-player outbox, and
          the reconnect says which sequence number it had reached.
        </p>
      </div>
    </div>
  )
}

/** Whether the push channel is up, said plainly rather than hidden. */
function Link({ status }: { status: Status }) {
  const said = {
    connecting: { text: 'connecting', tone: 'tag dim' },
    live: { text: 'live', tone: 'tag ok-text' },
    offline: { text: 'reconnecting', tone: 'tag amber' },
  }[status]

  return (
    <span className={said.tone} title="the WebSocket carrying everything you did not just ask for">
      {said.text}
    </span>
  )
}

/**
 * A probe this player made in another tab.
 *
 * Nothing on this screen asked for it, and nothing polled for it. It arrived
 * because the server decided it had — which is the whole reason the socket
 * exists (§8).
 */
function Elsewhere({ probe }: { probe: ProbeRecorded }) {
  return (
    <div className="notice cool">
      <strong>Meanwhile</strong> — another tab probed{' '}
      {probe.label ? <code>{probe.label}</code> : 'unlabelled'} at{' '}
      {new Date(probe.notedAt).toLocaleTimeString()}. That makes {probe.probes}.
    </div>
  )
}

/** Who the server says you are, and the way out. */
function Whoami({ player, onSignedOut }: { player: Player; onSignedOut: () => void }) {
  const [leaving, setLeaving] = useState(false)

  async function leave() {
    setLeaving(true)
    try {
      await signOut()
    } finally {
      // Either the session is revoked or the request never landed. Neither
      // leaves this app anything it can trust, so both end at the sign-in
      // screen — and the server, not this button, is what decides whether the
      // cookie still works.
      onSignedOut()
    }
  }

  return (
    <div className="whoami">
      {player.avatarUrl && (
        <img className="whoami-face" src={player.avatarUrl} alt="" width={32} height={32} />
      )}
      <div className="whoami-who">
        <div className="metric-name">{player.name}</div>
        <div className="metric-sub">{player.email}</div>
      </div>
      <button className="btn ghost small" onClick={leave} disabled={leaving}>
        {leaving ? 'Signing out…' : 'Sign out'}
      </button>
    </div>
  )
}

/** The values Postgres just returned, and how this delivery was treated. */
function Result({ sent }: { sent: Sent<ProbeResult> }) {
  const { result, replayed } = sent

  return (
    <>
      <div className="metric-row">
        <div>
          <div className="metric-name">Probes recorded</div>
          <div className="metric-sub">rows in skeleton_probe</div>
        </div>
        <div className="metric-value">{result.probes}</div>
      </div>

      <div className="metric-row">
        <div>
          <div className="metric-name">This delivery</div>
          <div className="metric-sub">
            {replayed
              ? 'answered from the idempotency record — nothing was written'
              : 'applied, and recorded against its key'}
          </div>
        </div>
        <div className="metric-value">
          <span className={replayed ? 'tag dim' : 'tag ok-text'}>
            {replayed ? 'replayed' : 'applied'}
          </span>
        </div>
      </div>

      <div className="metric-row">
        <div>
          <div className="metric-name">Label</div>
          <div className="metric-sub">as it was stored on the first delivery</div>
        </div>
        <div className="metric-value">{result.label || '—'}</div>
      </div>

      <div className="metric-row">
        <div>
          <div className="metric-name">Written at</div>
          <div className="metric-sub">now(), from the database clock</div>
        </div>
        <div className="metric-value">
          {new Date(result.notedAt).toLocaleTimeString()}
        </div>
      </div>

      <div className="metric-row">
        <div>
          <div className="metric-name">Postgres</div>
          <div className="metric-sub">server_version</div>
        </div>
        <div className="metric-value">{result.database}</div>
      </div>

      <div className="metric-row">
        <div>
          <div className="metric-name">Schema</div>
          <div className="metric-sub">migration applied at startup</div>
        </div>
        <div className="metric-value">{result.schema}</div>
      </div>
    </>
  )
}
