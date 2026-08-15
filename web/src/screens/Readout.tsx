import { useState } from 'react'

import type { Player } from '../session'
import { signOut } from '../world/actions'
import { useWorld } from '../world/react'
import { Overlay } from './Overlay'
import { spending } from './words'

/**
 * Your life, at a glance: money and where you stand.
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
      sub="What you hold. Everything else about you is a screen of its own — your body, your
            playbook, your standing."
    >
      <div className="stack">
        <Life />

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
    </Overlay>
  )
}

/**
 * The character: money, where they are, and where they live (§3.3).
 *
 * Everything here is the server's. The balance is cents the server sent and
 * this formats; the districts are the city's own vocabulary, sent with the
 * snapshot rather than compiled in, so settling §17's open question about which
 * city this is changes nothing in the client.
 */
function Life() {
  const character = useWorld((w) => w.character)

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
            <div className="stat-unit">money</div>
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

        <p className="faint tiny">
          Going anywhere is walking, and walking is where life happens to you. Press{' '}
          <kbd className="keycap">M</kbd> for the city, or walk onto a door.
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
