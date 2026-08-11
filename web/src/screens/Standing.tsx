import { useWorld } from '../world/react'
import type { Point } from '../world/state'
import { Overlay } from './Overlay'
import { LineChart } from '../ui/LineChart'
import { byUnit } from './words'

/**
 * Where you stand: the composite, and how it has moved since the day you
 * arrived (§3.5).
 *
 * There is no level in this game, so there is no level bar on this screen. What
 * replaces it is one number — Karma — composed by the server on read, and the
 * curve it has traced since the player's first day. Health and Money are shown
 * beneath it, as the readings the design surfaces them as: a biological age,
 * and the amount being held.
 *
 * Everything the player cannot act on is absent rather than drawn shut. The
 * hidden factors are hidden, which is not the same as displayed and sealed, and
 * a slot with a question mark in it is a feature somebody is being shown and
 * cannot use. The veil is applied on the server, on the way out — there is no
 * key, no name and no value in the payload for a hidden factor — so what this
 * screen has to do about them is nothing.
 *
 * The formula is never stated, and neither is a weight, a per-factor score, nor
 * a maximum Karma could reach. The curve is composites and only composites: it
 * says a thing happened without saying which of five things moved, which leaves
 * §3.5's question — *what are the hidden factors, what grows them* — intact.
 */
export function Standing({ open, onClose }: { open: boolean; onClose: () => void }) {
  const karma = useWorld((w) => w.karma)

  if (!karma) return null

  const shown = karma.slots.filter((s) => s.shown)

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Where you stand"
      sub="Not a level, and not a bar to fill. It is a reading of several things at once, taken
            fresh every time anything about you changes — which is why it can go down."
    >
      <div className="stack">
        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">Karma</span>
          </div>
          <div className="panel-body">
            <div className="readout">
              <div>
                <div className="stat-big accent">{karma.karma.toFixed(0)}</div>
                <div className="stat-unit">karma</div>
              </div>
            </div>

            <Curve points={karma.curve} />

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
            <span className="panel-title">Here</span>
            <span className="panel-note">{karma.days === 1 ? '1 day' : `${karma.days} days`}</span>
          </div>
          <div className="panel-body">
            {/* Who, not where. §3.15 defines an environment by its
                people, so what this names is a population — and the label
                says "among" for the same reason the count beside it says
                days rather than visits.

                A row rather than an env-chip: that chip is world chrome and
                is positioned absolutely, so inside a panel it left the panel
                entirely and landed on the page title. */}
            <div className="metric-row">
              <div>
                <div className="metric-name">Among</div>
                <div className="metric-sub">who you live beside, not where you are standing</div>
              </div>
              <div className="metric-value">{karma.among || 'nobody in particular'}</div>
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
    </Overlay>
  )
}

/**
 * The composite since the day the player arrived.
 *
 * Pure presentation: the numbers and the dates are the server's (§10.2), and
 * the only decisions here are which of LineChart's switches are on.
 *
 * Two of them carry the meaning. `xAxisType="number"` puts each reading at its
 * own date rather than at its position in the list, so days the rollover
 * skipped read as the fortnight they were instead of as one step. And nothing
 * turns the axis into a target: the top is a tick above the highest reading,
 * never a maximum Karma could reach, because a line with a ceiling above it is
 * a progress bar however it is styled (§3.5).
 *
 * Hovering a day states that day's Karma. That is the whole reason a curve
 * beats a number — the number is already printed above it.
 */
function Curve({ points }: { points: Point[] }) {
  // A player on their first day has one reading, and one reading is the number
  // printed above — an empty frame under it would be a chart they cannot read
  // yet, drawn to tell them so. It arrives when there is something in it.
  if (points.length < 2) return null

  return (
    <LineChart
      data={points.map((p) => ({ at: stamp(p.day), karma: p.karma }))}
      dataKeys={['karma']}
      categoryKey="at"
      config={{ data: { karma: { label: 'Karma' } } }}
      xAxisType="number"
      xAxisUnit="date"
      allowDecimals={false}
      showLegend={false}
      height={160}
      yAxisWidth={22}
    />
  )
}

/**
 * A plain calendar date as a number of days, which is what the axis is spaced
 * by.
 *
 * Parsed at midnight local, for the same reason [dateOf] does it: the server
 * sent the player's own day, and a UTC reading of it is the day before west of
 * Greenwich.
 */
function stamp(day: string): number {
  return Math.round(new Date(`${day}T00:00:00`).getTime() / 86_400_000)
}
