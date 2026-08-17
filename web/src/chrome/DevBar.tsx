import { useEffect, useRef, useState } from 'react'

import {
  devChallenges,
  devConflictDay,
  devConversations,
  devDisable,
  devEnable,
  devForgetPerson,
  devRaiseConflict,
  devReset,
} from '../world/actions'
import { forcePhase } from '../world/clock'
import { useWorld } from '../world/react'
import { sending, type Phase } from '../world/state'
import { devSettings, rememberDev, type Panel } from './devsettings'

/*
 * The developer's toolbar (⌘⇧\).
 *
 * Not part of the game and deliberately not dressed as it. Everything else in
 * this client is written in the world's voice and uses @etm/ui's vocabulary
 * because it is a thing the player is looking at; this is a strip of controls
 * for whoever is building the thing, and it says so — its own dark bar at the
 * foot of the screen, monospace, no place chips and no prose.
 *
 * Looking like the game is the failure to avoid: a panel that could be mistaken
 * for a screen gets screenshotted into a design review, or has a real feature
 * wired into it because it was already there.
 *
 * Nothing on this side makes it safe. The four commands behind it are not
 * registered by a production server (internal/api/dev.go), so in production
 * every button gets a 404 and the strip says the toolbar is not available. There
 * is no build flag, no `import.meta.env.DEV` check and no role: a client that
 * decided for itself whether it was allowed could be talked into deciding yes.
 *
 * The keystroke is the only way in, and there is no key hint for it, because the
 * key bar is what the player can reach. How the bar was left is remembered per
 * browser (`devsettings.ts`), which reopens a setting rather than a door.
 */
export function DevBar() {
  /*
   * Where this browser left the toolbar (`devsettings.ts`). The keystroke is
   * still the only thing that *opens* it — remembering that it was open is not
   * a second way in, it is the same way in, yesterday.
   */
  const [open, setOpen] = useState(devSettings.open)
  /*
   * Which panel is down, or none. One value rather than a boolean per panel:
   * they are alternatives on a strip at the foot of the screen, and two open at
   * once would push the game off the top of it.
   */
  const [panel, setPanel] = useState<Panel>(devSettings.panel)
  /*
   * Whether the bar is up, as the key listener sees it.
   *
   * A ref beside the state, because the listener is registered once with an
   * empty dependency list and would otherwise close over `open` as it was on
   * the first render — permanently false. Re-registering the listener on every
   * toggle would fix the staleness and add a window listener churn for a value
   * that is read exactly once per keypress; a ref is the smaller thing.
   */
  const up = useRef(devSettings.open)

  /** Put the bar up or down, and remember which for next time. */
  function raise(next: boolean) {
    up.current = next
    setOpen(next)
    rememberDev({ open: next })
    if (!next) {
      setPanel(null)
      rememberDev({ panel: null })
    }
  }

  /** Drop a panel, or the one that is down (toggling it shut), and remember. */
  function drop(next: Panel) {
    const settled = panel === next ? null : next
    setPanel(settled)
    rememberDev({ panel: settled })
    return settled
  }

  useEffect(() => {
    // Whatever this browser reopened into needs its data, and the two places
    // that normally fetch it are presses that already happened. StrictMode runs
    // this twice in development, which costs a second read of a catalogue and
    // nothing else — these are the only two commands here without consequences.
    if (devSettings.open) {
      void devChallenges()
      if (devSettings.panel === 'talk') void devConversations()
    }

    function onKey(e: KeyboardEvent) {
      // ⌘⇧\ — and Ctrl⇧\ for whoever is not on a Mac. `code` rather than `key`,
      // because with shift held the key is '|' and with a non-US layout it is
      // anybody's guess; the physical backslash is the same key everywhere.
      if (e.code !== 'Backslash' || !e.shiftKey) return
      if (!e.metaKey && !e.ctrlKey) return
      e.preventDefault()

      const next = !up.current
      // The side effects sit here rather than inside a `setOpen` updater, and
      // that is not style. StrictMode invokes an updater twice on purpose to
      // catch impure ones, so a command sent from in there goes twice on every
      // open — harmless for this read, and not harmless at all for the three
      // beside it. An updater returns the next state and does nothing else.
      raise(next)
      if (!next) return
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
        <button className="devbar-btn" onClick={() => drop('challenges')}>
          Challenges {panel === 'challenges' ? '▾' : '▸'}
        </button>
        <button
          className="devbar-btn"
          onClick={() => {
            // Read on the way open, for the same reason the catalogue is: a
            // panel nobody has asked for should not be sending commands.
            if (drop('talk')) void devConversations()
          }}
        >
          Conversations {panel === 'talk' ? '▾' : '▸'}
        </button>
        <Clock />
        <Notice />
        <button className="devbar-btn devbar-shut" onClick={() => raise(false)}>
          ⌘⇧\ to close
        </button>
      </div>
      {panel === 'challenges' && <Catalogue />}
      {panel === 'talk' && <Conversations />}
    </div>
  )
}

/**
 * The hand on the sky's clock: hold it at day, hold it at night, or let go.
 *
 * Unlike everything else on this strip it sends no command — the phase is a
 * drawing decision the server holds nothing about (`world/clock.ts`) — so it
 * works in any build. The hold is remembered per browser (`devsettings.ts`) and
 * `main.tsx` puts it back on before the first frame, so a machine set to night
 * stays at night across a reload; `auto` is what puts the sun back.
 */
function Clock() {
  const phase = useWorld((w) => w.phase)
  const [held, setHeld] = useState<Phase | null>(devSettings.clock)

  function hold(next: Phase | null) {
    setHeld(next)
    rememberDev({ clock: next })
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
  const talk = useWorld((w) => w.devTalk)
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
  // Whichever half spoke last. Both write their own notice and neither blanks
  // the other's, so the toolbar prefers the conversation one when it has
  // something to say — it is the half whose commands have consequences worth
  // reading back.
  const notice = talk?.notice || dev?.notice
  if (!notice) return <span className="devbar-notice" />
  return <span className="devbar-notice">{notice}</span>
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

/**
 * The conversation panel: who is around, and what is authored for them.
 *
 * Two columns because there are two questions somebody writing
 * `content/conflicts` is actually asking here, and they are not the same
 * question:
 *
 *   - Who would turn up? The people, with the pressure the server's own
 *     selection sorted on. Not a bar and not a score — it is the integer
 *     `Standing.Pressure` returned, so what is on screen is what `Turn` read.
 *   - What is written? The pathways this population has that the selected
 *     person could be put *right now* — theirs to send, with whatever they wait
 *     on already behind them, and under a pressure they are actually under. A
 *     moment out of turn is not drawn rather than drawn and refused: the server
 *     would turn it down (`ErrOutOfTurn`), and a button whose only outcome is a
 *     notice is a button that teaches the rule one wasted press at a time.
 *
 * The thread itself is not drawn here. The phone renders the same messages in
 * the same order and renders them as the player reads them, which is the state
 * being proof-read; a second rendering in toolbar type could only agree with it
 * or be wrong. Press raise, then press T.
 *
 * Only pathways authored for the population the player lives among are offered.
 * The rest of the catalogue is listed but not puttable, because a pathway is
 * authored *for* a population (§3.15) and putting a Riverfront moment to an
 * East Side neighbour would be proof-reading something the game cannot produce.
 */
function Conversations() {
  const talk = useWorld((w) => w.devTalk)
  const busy = useWorld(sending)
  /*
   * Who the next raise goes to, or nobody — in which case the server picks,
   * which is the world's own answer and usually the one wanted. Held here
   * rather than in the store because it is a thing about this panel and not a
   * thing about the world.
   */
  const [who, setWho] = useState('')

  if (!talk) return <div className="devbar-menu devbar-notice">reading the conversations…</div>
  if (!talk.where) {
    return <div className="devbar-menu devbar-notice">nobody lives around here</div>
  }

  // Optional chaining throughout: an empty slice is nil in Go and arrives as
  // `null` rather than `[]`, which is the same crossing `Catalogue` guards.
  const here = talk.pathways?.filter((p) => p.here) ?? []
  /*
   * What could be put right now, straight off the server's own answer
   * (`devPersonView.raise`) rather than worked out here. It has to be the
   * server's: whether a sequel is unlocked depends on what has already passed
   * with that person, which is in the database and not in this client.
   *
   * With nobody selected that is the union over everybody — the raise goes to
   * whoever the world picks among the people it could go to, so a pathway is
   * offered exactly when somebody out there is ready for it. A union rather
   * than no filter, because an unfiltered empty selection would draw a live
   * button beside every locked sequel and make the ordering rule look absent.
   */
  const raisable = new Set(
    who
      ? (talk.people?.find((p) => p.person === who)?.raise ?? [])
      : (talk.people?.flatMap((p) => p.raise ?? []) ?? []),
  )
  const sends = (p: { conflict: string }) => raisable.has(p.conflict)

  return (
    <div className="devbar-menu">
      <div className="devbar-item">
        <span className="devbar-code">{talk.where}</span>
        <span className="devbar-actions">
          <button className="devbar-btn" disabled={busy} onClick={() => void devConflictDay()}>
            run today
          </button>
          <span className="devbar-meta">the real selection, now</span>
        </span>
      </div>

      {/* Who is around, and how overdue each of them is. Clicking a name aims
          the next raise at them; clicking again lets the server choose. */}
      {talk.people?.map((p) => (
        <div className="devbar-item" key={p.person}>
          <button
            className="devbar-btn"
            onClick={() => setWho((was) => (was === p.person ? '' : p.person))}
          >
            {who === p.person ? '● ' : '○ '}
            {p.name}
          </button>
          <span className="devbar-meta">pressure {p.pressure}</span>
          <span className="devbar-meta">{p.under?.join(', ') || 'nothing'}</span>
          <span className="devbar-state">{p.today ? 'turn taken' : `met ${p.met}`}</span>
          <span className="devbar-actions">
            <button
              className="devbar-btn devbar-danger"
              disabled={busy || p.met === 0}
              onClick={() => void devForgetPerson(p.person)}
            >
              forget
            </button>
          </span>
        </div>
      ))}

      <div className="devbar-item">
        <span className="devbar-meta">
          raising to {who || 'whoever is under the most pressure'}
        </span>
      </div>

      {/* What is written for this population. */}
      {here.filter(sends).map((c) => (
        <div className="devbar-item" key={c.conflict}>
          <span className="devbar-code">{c.conflict}</span>
          <span className="devbar-name">{c.name}</span>
          <span className="devbar-meta">{c.follows}</span>
          <span className="devbar-meta">{c.puts}</span>
          {/* What it waits on, and how often it comes back — the two things
              that decide whether pressing raise can work today. */}
          {!!c.after?.length && <span className="devbar-meta">after {c.after.join(', ')}</span>}
          {!c.after?.length && !!c.every && (
            <span className="devbar-meta">every {c.every}d</span>
          )}
          <span className="devbar-actions">
            <button
              className="devbar-btn"
              disabled={busy}
              onClick={() => void devRaiseConflict(c.conflict, who)}
            >
              raise
            </button>
          </span>
        </div>
      ))}
      {!here.length && <div className="devbar-notice">nothing is authored for {talk.where}</div>}
      {!!here.length && !here.filter(sends).length && (
        <div className="devbar-notice">
          nothing {who || 'anybody here'} sends is in turn yet — everything of theirs waits on a
          conversation that has not happened, or on a pressure nobody is under
        </div>
      )}
    </div>
  )
}
