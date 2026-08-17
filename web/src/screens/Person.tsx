import type { Player } from '../session'
import { useWorld } from '../world/react'
import { briefly } from './words'

/**
 * Whose life this is: the face, the name, the population they live among and
 * how long they have been there, then the two readings that hold at every tier.
 *
 * The picture is the account's own, which is Google's (`player.avatarUrl`) —
 * the game draws no portrait and the character's body is not a face.
 *
 * Karma is deliberately absent. It has no ceiling (§3.5), and a figure in a
 * circle beside two others reads as a score however it is worded; where it
 * belongs is the trace on the left, which has no end drawn on it.
 */
export function Person({ player }: { player: Player }) {
  const karma = useWorld((w) => w.karma)
  const age = useWorld((w) => w.health?.biologicalAge ?? null)
  const money = useWorld((w) => w.character?.money ?? null)

  const among = karma?.among ?? ''
  const days = karma?.days ?? 0

  return (
    <div className="panel person">
      <div className="person-face">
        {player.avatarUrl && (
          <img src={player.avatarUrl} alt="" width={118} height={118} referrerPolicy="no-referrer" />
        )}
      </div>

      <div className="person-name">{player.name}</div>
      {among && (
        <div className="person-where">
          {among} · {days === 1 ? '1 day' : `${days} days`}
        </div>
      )}

      <div className="orbs">
        {age !== null && (
          <div className="orb">
            <div>
              <div className="orb-value">{age.toFixed(0)}</div>
              <div className="orb-name">age</div>
            </div>
          </div>
        )}
        {money !== null && (
          <div className="orb">
            <div>
              <div className="orb-value">{briefly(money)}</div>
              <div className="orb-name">held</div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
