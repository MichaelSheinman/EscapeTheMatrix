import { useEffect, useRef, useState } from 'react'

import { Closet } from './chrome/Closet'
import { Deadline } from './chrome/Carrying'
import { ChallengeBar } from './chrome/ChallengeBar'
import { Outcome } from './chrome/Outcome'
import { Corner } from './chrome/Corner'
import { DevBar } from './chrome/DevBar'
import { Keybar } from './chrome/Keybar'
import { markersFor, nearestTo } from './chrome/markers'
import { Pods } from './chrome/Pods'
import { WorldOverlay } from './chrome/WorldOverlay'
import { getRenderer } from './render/handle'
import { BoardsScreen } from './screens/Boards'
import { Body } from './screens/Body'
import { ChallengeScreen } from './screens/Challenge'
import { EventsScreen } from './screens/Events'
import { Map, Where } from './screens/Map'
import { PhoneScreen } from './screens/Phone'
import { PlaybookScreen, RuleSet } from './screens/Playbook'
import { Readout } from './screens/Readout'
import { Rules } from './screens/Routines'
import { Standing } from './screens/Standing'
import { Work } from './screens/Work'
import { signInURL } from './session'
import { chooseInChallenge, moveTo } from './world/actions'
import { useWorld } from './world/react'
import { patch, world } from './world/store'

/*
 * The DOM half of the app — the shell of it.
 *
 * Everything here floats **above** the canvas and never contains it
 * (technical.md §10.1). There is no route that replaces the world: closing a
 * panel leaves the same scene running underneath, in the same state, because
 * nothing React does can reach the renderer.
 *
 * The screens themselves live in `screens/`, one file per domain, and each of
 * them subscribes to the store itself — the `Closet` idiom: props are `open`,
 * `onClose`, and the occasional callback that moves this shell's own state.
 * What stays in this file is the shell — who is signed in, which panel is
 * open, and what the keyboard does.
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
export type Screen =
  | 'readout'
  | 'body'
  | 'rules'
  | 'playbook'
  | 'events'
  | 'challenge'
  | 'work'
  | 'phone'
  | 'map'
  | 'standing'
  | 'boards'
  | 'closet'
  | null

const KEYS: Record<string, Exclude<Screen, null>> = {
  s: 'readout',
  h: 'body',
  r: 'rules',
  p: 'playbook',
  e: 'events',
  c: 'challenge',
  j: 'work',
  t: 'phone',
  m: 'map',
  k: 'standing',
  l: 'boards',
}

export function App() {
  const session = useWorld((w) => w.session)
  const renderer = useWorld((w) => w.renderer)
  /*
   * Challenge mode: a run is going, so this is not the shell any more.
   *
   * §5's challenges are somewhere you *are* rather than a panel you consult, and
   * a player who can press M for the map while standing in a burning flat has
   * been told by the interface that none of it is happening. So while a run is
   * in progress the screens, the shortcuts, the key card and the notifications
   * all go, and what is left is the world and two controls (`ChallengeBar`).
   *
   * It is also what makes the way out findable. With the whole shell live and
   * no visible exit, leaving was a keyboard shortcut nobody had been told about.
   */
  const inChallenge = useWorld((w) => {
    const run = w.challenge?.run ?? null
    return Boolean(run && !run.done)
  })

  // The two pieces of React state in the client: which panel this tab has
  // open, and whether the key card is. Not in the store, because the world
  // does not care.
  const [screen, setScreen] = useState<Screen>(null)
  const [card, setCard] = useState(false)

  /*
   * Whether a run is going, as the key listener sees it.
   *
   * A ref beside the state for the same reason the dev toolbar keeps one: the
   * listener is registered once with an empty dependency list, and would
   * otherwise close over the value it had on the first render forever.
   */
  const challenging = useRef(inChallenge)
  challenging.current = inChallenge

  /*
   * A finished run closes the panel it was played in.
   *
   * The last choice is committed from inside the challenge screen, and without
   * this the screen stays open on a run that is over — an empty panel with a
   * back button, which is somewhere to be dismissed from rather than an ending.
   * What says how it went is the card over the world (`Outcome`), so the honest
   * thing is to put the player back in the world and let it say so.
   */
  useEffect(() => {
    if (!inChallenge) setScreen((open) => (open === 'challenge' ? null : open))
  }, [inChallenge])

  useEffect(() => {
    // Which arrows are held right now. A fact about this keyboard, read only
    // to compose the heading the renderer steers by.
    const held = { l: false, r: false, u: false, d: false }
    const arrows: Record<string, keyof typeof held> = {
      ArrowLeft: 'l',
      ArrowRight: 'r',
      ArrowUp: 'u',
      ArrowDown: 'd',
    }

    function steer() {
      const x = (held.r ? 1 : 0) - (held.l ? 1 : 0)
      const y = (held.d ? 1 : 0) - (held.u ? 1 : 0)
      if (x === 0 && y === 0) {
        patch.heading(null)
        return
      }
      const length = Math.hypot(x, y)
      patch.heading({ x: x / length, y: y / length })
    }

    /**
     * The O key: act on the nearest thing.
     *
     * Three things a marker can be, and they are checked in the order they are
     * irreversible. A door is walked through, a panel is opened, and an item is
     * *taken* — which commits a choice in a run and cannot be undone, so it is
     * last and it is the one that reads the run rather than the marker alone.
     */
    function interact() {
      const view = getRenderer()?.view()
      if (!view) return
      const state = world.getState()
      const run = state.challenge?.run ?? null
      const offering =
        run && !run.done ? new Set(run.choices.map((choice) => choice.key)) : undefined
      const near = nearestTo(markersFor(view, state.places, offering), view.player)
      if (!near) return
      if (near.go) void moveTo(near.go, 'foot')
      else if (near.screen) setScreen(near.screen)
      else if (near.item && run && !run.done) {
        void chooseInChallenge(run.challenge, run.scene, near.item)
      }
    }

    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      // A key pressed inside a field is a letter, not a shortcut.
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return

      // The arrows are the character. Taking the wheel closes whatever panel
      // was open, because walking is being back in the world.
      const walk = arrows[e.key]
      if (walk) {
        e.preventDefault()
        if (!held[walk]) {
          held[walk] = true
          steer()
        }
        setScreen(null)
        setCard(false)
        return
      }

      if (e.key === 'Escape') {
        setCard(false)
        setScreen(null)
        return
      }

      const key = e.key.toLowerCase()

      /*
       * In a challenge, the only keys are the ones that *are* the challenge.
       *
       * Walking and taking survive because they are what a player does in the
       * room. Everything else — every screen, the key card, the effects tier —
       * is the shell, and the shell is not here. Checked after the arrows and
       * Escape above so that steering and closing a panel always work.
       */
      if (challenging.current && key !== 'o') return
      if (key === '?' || key === '/') {
        setCard((open) => !open)
        return
      }
      if (key === 'o') {
        interact()
        return
      }
      if (key === 'q') {
        const tier = world.getState().quality.tier
        patch.quality(tier === 'low' ? 'high' : 'low')
        return
      }

      const opens = KEYS[key]
      if (opens) {
        setCard(false)
        setScreen(opens)
      }
    }

    function onUp(e: KeyboardEvent) {
      const walk = arrows[e.key]
      if (walk && held[walk]) {
        held[walk] = false
        steer()
      }
    }

    // A key held while the tab loses focus would otherwise stay held.
    function onBlur() {
      held.l = held.r = held.u = held.d = false
      patch.heading(null)
    }

    window.addEventListener('keydown', onKey)
    window.addEventListener('keyup', onUp)
    window.addEventListener('blur', onBlur)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('keyup', onUp)
      window.removeEventListener('blur', onBlur)
      patch.heading(null)
    }
  }, [])

  return (
    <>
      {session.status === 'in' && <WorldOverlay onOpen={setScreen} />}

      {/* The chrome that says where you are and what you have. All of it is the
          shell, and in a challenge there is no shell — see `inChallenge`. */}
      {!inChallenge && <Where />}
      {session.status === 'in' && !inChallenge && <RuleSet />}

      {session.status === 'in' && !inChallenge && <Pods onOpen={setScreen} />}

      {/* Over the world rather than in a panel: the taking happens out here,
          and the countdown is on a moment that will be gone before you have
          finished deciding. Both draw nothing at all when there is no bag and
          nothing timed, which is nearly always. */}
      {/* The countdown is on an *offer*, which is a thing in the world rather
          than in a run — so it goes with the rest of the shell. The bag is the
          challenge's own and the challenge bar draws it. */}
      {session.status === 'in' && !inChallenge && <Deadline />}

      {renderer === 'unsupported' && <CannotDraw />}
      {session.status === 'in' && renderer !== 'unsupported' && !inChallenge && (
        <Corner onOpen={setScreen} card={card} onCard={() => setCard((open) => !open)} />
      )}

      {/* Two bars, and never both. Out of a challenge the key bar says what the
          keys do; in one, the only things there are to do are on the challenge
          bar, and a list of shortcuts that no longer work would be a lie. */}
      {session.status === 'in' && <Outcome />}

      {inChallenge ? (
        <ChallengeBar onOpen={() => setScreen('challenge')} />
      ) : (
        <Keybar open={screen !== null} card={card} />
      )}

      {/* The developer's toolbar (⌘⇧\). Outside the signed-in branch below
          because it is not a screen and does not belong to a panel; it draws
          nothing until somebody presses the key, and the commands behind it do
          not exist on a production server. There is no key hint for it — the
          key bar is what the *player* can reach. */}
      {session.status === 'in' && <DevBar />}

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
          <PlaybookScreen open={screen === 'playbook'} onClose={() => setScreen(null)} />
          <EventsScreen
            open={screen === 'events'}
            onClose={() => setScreen(null)}
            onEnter={() => setScreen('challenge')}
          />
          <ChallengeScreen open={screen === 'challenge'} onClose={() => setScreen(null)} />
          <Work
            open={screen === 'work'}
            onClose={() => setScreen(null)}
            onEnter={() => setScreen('challenge')}
          />
          <PhoneScreen open={screen === 'phone'} onClose={() => setScreen(null)} />
          <Standing open={screen === 'standing'} onClose={() => setScreen(null)} />
          <BoardsScreen open={screen === 'boards'} onClose={() => setScreen(null)} />
          <Map open={screen === 'map'} onClose={() => setScreen(null)} />
          <Closet open={screen === 'closet'} onClose={() => setScreen(null)} />
        </>
      )}
    </>
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
          <p className="toast-title">This browser cannot draw the world.</p>
          <p className="toast-body">
            It needs WebGL2, which this device or its driver is not providing. The panels still
            work — the world behind them does not.
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
