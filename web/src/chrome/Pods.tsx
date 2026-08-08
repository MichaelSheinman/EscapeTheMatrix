import type { Screen } from '../App'
import { spending } from '../screens/words'
import { useWorld } from '../world/react'

/**
 * The numbers that are true wherever you are standing.
 *
 * Every one of them comes out of the same store the renderer reads sixty times
 * a second — one read by React when it changes, one read by the world in its
 * own frame loop, and neither owning it.
 */
export function Pods({ onOpen }: { onOpen: (screen: Screen) => void }) {
  const character = useWorld((w) => w.character)
  const health = useWorld((w) => w.health)
  const karma = useWorld((w) => w.karma)

  return (
    <div className="pods">
      {health && (
        <div className="pod accent" onClick={() => onOpen('body')} title="Your body (H)">
          <span className="pod-glyph">◉</span>
          <span className="pod-text">
            <span className="pod-value">{health.biologicalAge.toFixed(0)}</span>
            <span className="pod-label">Biological age</span>
          </span>
        </div>
      )}

      {character && (
        <div className="pod" onClick={() => onOpen('readout')} title="Your life (S)">
          <span className="pod-glyph">$</span>
          <span className="pod-text">
            <span className="pod-value">{spending(character.money)}</span>
            <span className="pod-label">USD</span>
          </span>
        </div>
      )}

      {/* No level, and so no level bar (§3.5). What replaces it is one number
          the server composes on read out of five factors, three of which are
          never named — which is why it is a pod like the others rather than a
          progress meter with a maximum to fill. */}
      {karma && (
        <div className="pod" onClick={() => onOpen('standing')} title="Where you stand (K)">
          <span className="pod-glyph">◈</span>
          <span className="pod-text">
            <span className="pod-value">{karma.karma.toFixed(0)}</span>
            <span className="pod-label">Karma</span>
          </span>
        </div>
      )}
    </div>
  )
}
