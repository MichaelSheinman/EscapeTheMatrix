import { useState } from 'react'
import {
  command,
  CommandError,
  newKey,
  type ProbeResult,
  type Sent,
} from './commands'

/*
 * The walking skeleton's one screen.
 *
 * It proves the path — a click here becomes a REST command, which becomes one
 * Postgres transaction, which becomes a number on screen that could not have
 * been produced client-side — and now the guarantee on top of it: the same
 * command delivered twice applies once.
 *
 * The two buttons are the demonstration. "Probe" is a new intent and gets a new
 * key. "Send again" re-delivers under the key that was already used, which is
 * what a network retry or a reload does, and the row count does not move.
 *
 * Styling comes from @etm/ui — the design system is consumed, never
 * reimplemented (technical.md §2). The class names below are its component
 * vocabulary; `ui/index.js` holds the same components as HTML strings for
 * `preview/`, and those are the translation target when real screens arrive.
 */

type State =
  | { status: 'idle' }
  | { status: 'sending' }
  | { status: 'ok'; sent: Sent<ProbeResult> }
  | { status: 'failed'; code: number; message: string }

export function App() {
  const [state, setState] = useState<State>({ status: 'idle' })
  const [label, setLabel] = useState('')
  // The key of the last command sent, whatever became of it. Holding it is the
  // whole of what a client has to do to make its retries safe.
  const [lastKey, setLastKey] = useState<string | null>(null)

  async function send(key: string) {
    setState({ status: 'sending' })
    setLastKey(key)
    try {
      setState({
        status: 'ok',
        sent: await command<ProbeResult>('probe', { label }, key),
      })
    } catch (err) {
      if (err instanceof CommandError) {
        setState({ status: 'failed', code: err.status, message: err.message })
      } else {
        setState({ status: 'failed', code: 0, message: 'the server could not be reached' })
      }
    }
  }

  const busy = state.status === 'sending'

  return (
    <main className="skeleton">
      <div className="panel">
        <div className="panel-head">
          <span className="panel-title">Walking skeleton</span>
          <span className="panel-note">one binary · one datastore</span>
        </div>

        <div className="panel-body">
          <p className="faint tiny mt0">
            This page is served from inside the Go binary. The button sends a
            command over REST, which writes and reads in a single Postgres
            transaction. Every command carries an idempotency key, so sending
            one twice applies it once.
          </p>

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
        </div>
      </div>
    </main>
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
