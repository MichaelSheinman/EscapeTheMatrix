import { useState } from 'react'
import { command, CommandError, type ProbeResult } from './commands'

/*
 * The walking skeleton's one screen.
 *
 * Its entire job is to prove the path: a click here becomes a REST command,
 * which becomes one Postgres transaction, which becomes a number on screen
 * that could not have been produced client-side.
 *
 * Styling comes from @etm/ui — the design system is consumed, never
 * reimplemented (technical.md §2). The class names below are its component
 * vocabulary; `ui/index.js` holds the same components as HTML strings for
 * `preview/`, and those are the translation target when real screens arrive.
 */

type State =
  | { status: 'idle' }
  | { status: 'sending' }
  | { status: 'ok'; result: ProbeResult }
  | { status: 'failed'; message: string }

export function App() {
  const [state, setState] = useState<State>({ status: 'idle' })

  async function probe() {
    setState({ status: 'sending' })
    try {
      setState({ status: 'ok', result: await command<ProbeResult>('probe') })
    } catch (err) {
      const message =
        err instanceof CommandError
          ? `${err.message} (${err.status})`
          : 'the server could not be reached'
      setState({ status: 'failed', message })
    }
  }

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
            transaction.
          </p>

          {state.status === 'ok' && <Result result={state.result} />}

          {state.status === 'failed' && (
            <div className="notice">{state.message}</div>
          )}

          <div className="btn-row">
            <button
              className="btn"
              onClick={probe}
              disabled={state.status === 'sending'}
            >
              {state.status === 'sending' ? 'Probing…' : 'Probe the database'}
            </button>
          </div>
        </div>
      </div>
    </main>
  )
}

/** The values Postgres just returned. */
function Result({ result }: { result: ProbeResult }) {
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
