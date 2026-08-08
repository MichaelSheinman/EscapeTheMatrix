import { useWorld } from '../world/react'
import { Overlay } from './Overlay'
import { byUnit } from './words'

/**
 * Where you stand: the composite, and the board it is made of (§3.5).
 *
 * There is no level in this game, so there is no level bar on this screen. What
 * replaces it is one number — Karma — composed by the server on read out of five
 * factors, **three of which are never named**. Health and Money are shown, as
 * the readings the design surfaces them as: a biological age, and the amount
 * being held. The other three arrive as slots with nothing in them.
 *
 * That is not a rendering decision. There is no key, no name and no value in the
 * payload for a hidden factor, so there is nothing here to accidentally reveal —
 * the veil is applied on the server, on the way out, the same way instrumentation
 * is on the body screen.
 *
 * The formula is never stated, and neither is a weight, a per-factor score, nor
 * a maximum Karma could reach. §3.5: *what are the hidden factors, how are they
 * calculated, what grows them* is the question the game is asking, and a screen
 * that answered it would be a screen with nothing left to find.
 */
export function Standing({ open, onClose }: { open: boolean; onClose: () => void }) {
  const karma = useWorld((w) => w.karma)

  if (!karma) return null

  const shown = karma.slots.filter((s) => s.shown)
  const sealed = karma.slots.filter((s) => !s.shown)

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Where you stand"
      sub="Not a level, and not a bar to fill. It is a reading of several things at once, taken
            fresh every time anything about you changes — which is why it can go down."
    >
      <div className="grid wide-left">
        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Karma</span>
              <span className="panel-note">composed on read · stored nowhere</span>
            </div>
            <div className="panel-body">
              <div className="readout">
                <div>
                  <div className="stat-big accent">{karma.karma.toFixed(0)}</div>
                  <div className="stat-unit">karma</div>
                </div>
                <p className="note last">
                  Five things go into this. You are shown two of them.
                </p>
              </div>

              {shown.map((slot) => (
                <div className="metric-row" key={slot.key}>
                  <div>
                    <div className="metric-name">{slot.name}</div>
                    <div className="metric-sub">{slot.note}</div>
                  </div>
                  <div className="metric-value accent">{byUnit(slot.reads, slot.value)}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">The rest</span>
              <span className="tag dim">{sealed.length} unnamed</span>
            </div>
            <div className="panel-body">
              <p className="note first">
                Something is being measured in each of these. They move when you commit to
                something — never when you say something.
              </p>
              <div className="slots">
                {sealed.map((_, i) => (
                  // Keyed by position, because position is all a sealed
                  // slot has. There is no key on the wire to use instead.
                  <div className="locked-slot dim" key={i}>
                    <span className="locked-name">? ? ? ? ?</span>
                    <span className="locked-why">unnamed</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Here</span>
              <span className="panel-note">{karma.days === 1 ? '1 day' : `${karma.days} days`}</span>
            </div>
            <div className="panel-body">
              {/* Who, not where. §3.15 defines an environment by its
                  people, so what this names is a population — and the label
                  says "among" for the same reason the count beside it says
                  days rather than visits. */}
              <div className="env-chip">
                <span className="env-label">among</span>
                <span className="env-name">{karma.among || 'nobody in particular'}</span>
              </div>

              {/* §3.15: what a place still pays, falling as it is worked
                  out. Stated rather than explained — the number going down
                  is the whole of the lesson, and the game never says why. */}
              <div className="decay-row">
                <span className="label">returns</span>
                <span className="decay-track">
                  <span style={{ width: `${Math.round(karma.familiarity * 100)}%` }} />
                </span>
                <span className="amount">{Math.round(karma.familiarity * 100)}%</span>
              </div>

              <p className="note last">
                {karma.familiarity > 0.75
                  ? 'There is plenty here you have not run into yet.'
                  : karma.familiarity > 0.3
                    ? 'The same effort is starting to be worth less around here.'
                    : 'You have got most of what this place has. Somewhere else would pay better.'}
              </p>
            </div>
          </div>
        </div>
      </div>
    </Overlay>
  )
}
