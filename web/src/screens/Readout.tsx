import { useState } from 'react'

import { TIERS } from '../render/quality'
import type { Player } from '../session'
import { signOut, work } from '../world/actions'
import { useWorld } from '../world/react'
import { sending } from '../world/state'
import { patch } from '../world/store'
import { Overlay } from './Overlay'
import { spending } from './words'

/**
 * Your life, at a glance: money, where you stand, and the day's paid actions.
 *
 * Closing it does not unmount anything the renderer owns — press Escape and the
 * street is still there, the character is still standing where they walked to.
 * §10.1's property, made visible.
 */
export function Readout({
  player,
  open,
  onClose,
}: {
  player: Player
  open: boolean
  onClose: () => void
}) {
  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Your life"
      sub="What you hold and what a day can be made to produce. Everything else about you is a
            screen of its own — your body, your playbook, your standing."
    >
      <div className="grid wide-left">
        <div className="stack">
          <Life />

          <Drawing />
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
        </div>
      </div>
    </Overlay>
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
  const busy = useWorld(sending)

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
            {character.earningsLeft > 0 ? 'Take a day’s work' : 'Nothing left today'}
          </button>
        </div>

        <p className="faint tiny">
          Three paid actions a day, counted against your own midnight. When they are gone the rest
          of the day is still yours — it just does not pay.
        </p>

        <p className="faint tiny">
          Going anywhere is walking, and walking is where life happens to you. Press{' '}
          <kbd className="keycap">M</kbd> for the city, or walk onto a door.
        </p>
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
          Switching is immediate, and <kbd className="keycap">Q</kbd> does it from anywhere.
        </p>
      </div>
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
