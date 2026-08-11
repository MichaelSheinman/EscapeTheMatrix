import { useEffect, useRef, useState } from 'react'

import { devChallenges, devDisable, devEnable, devReset } from '../world/actions'
import { forcePhase } from '../world/clock'
import { useWorld } from '../world/react'
import { sending, type Phase } from '../world/state'

/*
 * The developer's toolbar (⌘⇧\).
 *
 * Not part of the game and deliberately not dressed as it. Everything else in
 * this client is written in the world's voice and uses @etm/ui's vocabulary
 * because it is a thing the player is looking at; this is a strip of controls
 * for whoever is building the thing, and it says so — its own dark bar at the
 * foot of the screen, monospace, no place chips and no prose.
 *
 * Looking like the game is the specific failure to avoid. A panel that could be
 * mistaken for a screen is a panel somebody eventually screenshots into a
 * design review, or worse, wires a real feature into because it was already
 * there and looked the part.
 *
 * # What makes it safe
 *
 * Nothing on this side. The four commands behind it are not registered by a
 * production server (internal/api/dev.go), so in production every button here
 * gets a 404 and the strip says the toolbar is not available. There is no build
 * flag, no `import.meta.env.DEV` check and no role — a client that decided for
 * itself whether it was allowed would be a client somebody could talk into
 * deciding yes.
 *
 * The keystroke is the only way in. There is no key hint for it, because the
 * key bar is what the *player* can reach.
 */
export function DevBar() {
  const [open, setOpen] = useState(false)
  const [menu, setMenu] = useState(false)
  /*
   * Whether the bar is up, as the key listener sees it.
   *
   * A ref beside the state, because the listener is registered once with an
   * empty dependency list and would otherwise close over `open` as it was on
   * the first render — permanently false. Re-registering the listener on every
   * toggle would fix the staleness and add a window listener churn for a value
   * that is read exactly once per keypress; a ref is the smaller thing.
   */
  const up = useRef(false)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      // ⌘⇧\ — and Ctrl⇧\ for whoever is not on a Mac. `code` rather than `key`,
      // because with shift held the key is '|' and with a non-US layout it is
      // anybody's guess; the physical backslash is the same key everywhere.
      if (e.code !== 'Backslash' || !e.shiftKey) return
      if (!e.metaKey && !e.ctrlKey) return
      e.preventDefault()

      const next = !up.current
      up.current = next
      setOpen(next)

      // The side effects sit here rather than inside a `setOpen` updater, and
      // that is not style. StrictMode invokes an updater twice on purpose to
      // catch impure ones, so a command sent from in there goes twice on every
      // open — harmless for this read, and not harmless at all for the three
      // beside it. An updater returns the next state and does nothing else.
      if (!next) {
        setMenu(false)
        return
      }
      // Read the catalogue on the way open rather than on mount: a toolbar
      // nobody has asked for should not be sending commands, least of all in a
      // production build where they would 404 on every load.
      void devChallenges()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  if (!open) return null
  return (
    <div className="devbar">
      <div className="devbar-row">
        <span className="devbar-badge">dev</span>
        <button className="devbar-btn" onClick={() => setMenu((was) => !was)}>
          Challenges {menu ? '▾' : '▸'}
        </button>
        <Clock />
        <Notice />
        <button className="devbar-btn devbar-shut" onClick={() => setOpen(false)}>
          ⌘⇧\ to close
        </button>
      </div>
      {menu && <Catalogue />}
    </div>
  )
}

/**
 * The hand on the sky's clock: hold it at day, hold it at night, or let go.
 *
 * Unlike everything else on this strip it sends no command — the phase is a
 * drawing decision the server holds nothing about (`world/clock.ts`) — so it
 * works in any build, and closing the tab is what puts the sun back.
 */
function Clock() {
  const phase = useWorld((w) => w.phase)
  const [held, setHeld] = useState<Phase | null>(null)

  function hold(next: Phase | null) {
    setHeld(next)
    forcePhase(next)
  }

  return (
    <span className="devbar-actions">
      <span className="devbar-meta">clock: {phase}</span>
      <button className="devbar-btn" disabled={held === null} onClick={() => hold(null)}>
        auto
      </button>
      <button className="devbar-btn" disabled={held === 'day'} onClick={() => hold('day')}>
        day
      </button>
      <button className="devbar-btn" disabled={held === 'night'} onClick={() => hold('night')}>
        night
      </button>
    </span>
  )
}

/** What the last toolbar command actually did, in plain words. */
function Notice() {
  const dev = useWorld((w) => w.dev)
  const attempt = useWorld((w) => w.attempt)

  // A refused command is the toolbar's own report: in production these four do
  // not exist, and saying so is more useful than a strip that does nothing.
  if (attempt.status === 'refused') {
    return (
      <span className="devbar-notice devbar-bad">
        {attempt.code === 404
          ? 'the dev commands are not registered — this is a production build'
          : attempt.message}
      </span>
    )
  }
  if (!dev?.notice) return <span className="devbar-notice" />
  return <span className="devbar-notice">{dev.notice}</span>
}

/**
 * Every authored challenge, and the one or two things you can do to each.
 *
 * The whole catalogue including what is played and what is shut, which is the
 * exact opposite of what the player-facing surface is allowed to show (§4 keeps
 * the world a sandbox rather than a quest list). That asymmetry is the reason
 * this reads from `w.dev` rather than from `w.challenge`.
 */
function Catalogue() {
  const dev = useWorld((w) => w.dev)
  const busy = useWorld(sending)

  if (!dev) return <div className="devbar-menu devbar-notice">reading the catalogue…</div>
  // Optional chaining because this crossed a JSON boundary: an empty catalogue
  // is a nil slice in Go and arrives as `null` rather than as `[]`, and the
  // branch below it would then be unreachable behind a TypeError.
  if (!dev.challenges?.length) {
    return <div className="devbar-menu devbar-notice">nothing is authored</div>
  }

  return (
    <div className="devbar-menu">
      {dev.challenges.map((c) => (
        <div className="devbar-item" key={c.challenge}>
          <span className="devbar-code">{c.challenge}</span>
          <span className="devbar-name">{c.name}</span>
          <span className="devbar-meta">{c.mechanic}</span>

          {/* What state this player is in for it, said as flatly as possible. */}
          <span className="devbar-state">
            {c.running ? 'running' : c.played ? 'played' : c.gate === 'flag' ? (c.on ? 'open' : 'shut') : 'standing'}
          </span>

          <span className="devbar-actions">
            {/* An open challenge has no gate to turn; what it has is a place to
                walk to, which the notice says when you press it. */}
            <button
              className="devbar-btn"
              disabled={busy || c.running}
              onClick={() => void devEnable(c.challenge)}
            >
              {c.opens ? 'enable + raise' : 'enable'}
            </button>
            {c.gate === 'flag' && c.on && (
              <button
                className="devbar-btn"
                disabled={busy}
                onClick={() => void devDisable(c.challenge)}
              >
                shut
              </button>
            )}
            {/* Only offered where there is something to undo. Resetting is the
                one control here that deletes a player's history, so it does not
                sit there looking like a refresh button. */}
            {(c.played || c.running) && (
              <button
                className="devbar-btn devbar-danger"
                disabled={busy}
                onClick={() => void devReset(c.challenge)}
              >
                reset
              </button>
            )}
          </span>
        </div>
      ))}
    </div>
  )
}
