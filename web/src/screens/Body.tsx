import { useState } from 'react'

import { buyInstrument, liveADay } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type Metric } from '../world/state'
import { Overlay } from './Overlay'
import { spending } from './words'

/**
 * Your body, and what you can see of it (§3.16).
 *
 * The panel is simulated in full for everyone. What instrumentation changes is
 * the player's resolution on their own body, never their health — so the screen
 * is laid out **by tier**: one card per instrument, showing its readings if you
 * own it and a hatched slot per measurement if you do not. That is the same
 * shape `preview/` gives this screen, and it is the shape the design makes
 * true — the tier is the only thing standing between you and the number.
 *
 * Biological age is the one readout visible at every tier, composed by the
 * server on read from the panel rather than stored anywhere.
 */
export function Body({ open, onClose }: { open: boolean; onClose: () => void }) {
  const health = useWorld((w) => w.health)
  const crossings = useWorld((w) => w.crossings)
  const money = useWorld((w) => w.character?.money ?? 0)
  const busy = useWorld(sending)

  const [sleep, setSleep] = useState('ordinary')
  const [training, setTraining] = useState('light')
  const [diet, setDiet] = useState('mixed')

  if (!health) return null

  const baseline = health.tiers.find((t) => !t.forSale)
  const instruments = health.tiers.filter((t) => t.forSale)
  const of = (tier: string) => health.metrics.filter((m) => m.tier === tier)

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Your body"
      sub={
        <>
          All of it is being simulated, all of the time. What an instrument buys is the ability
          to <em>see</em> — a watch does not make anybody healthier. Everything in the locked
          cards is happening to you whether or not you can read it.
        </>
      }
    >
      <div className="grid wide-left">
        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">{baseline?.name ?? 'Baseline'}</span>
              <span className="panel-note">free · from the start</span>
            </div>
            <div className="panel-body">
              <div className="readout">
                <div>
                  <div className="stat-big accent">{health.biologicalAge.toFixed(0)}</div>
                  <div className="stat-unit">biological age</div>
                </div>
                <p className="note last">
                  Composed on read from every metric below and every one you cannot see. It is
                  never stored, so it cannot go stale — and it is the one number visible at
                  every tier.
                </p>
              </div>

              {crossings.map((c) => (
                <div key={c.metric} className={`notice${c.worse ? '' : ' cool'}`}>
                  <strong>{c.metric}</strong> went from {c.from} to {c.to}.
                </div>
              ))}

              {of('baseline').map((m) => (
                <Reading key={m.key} metric={m} />
              ))}
            </div>
          </div>

          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Today</span>
              <span className="panel-note">
                {health.livedToday ? 'already lived' : 'one day, three choices'}
              </span>
            </div>
            <div className="panel-body">
              <Living
                sleep={sleep}
                training={training}
                diet={diet}
                onSleep={setSleep}
                onTraining={setTraining}
                onDiet={setDiet}
                livedToday={health.livedToday}
              />
            </div>
          </div>
        </div>

        <div className="stack">
          {instruments.map((tier) => {
            const metrics = of(tier.key)
            return (
              <div className="panel" key={tier.key}>
                <div className="panel-head">
                  <span className="panel-title">{tier.name}</span>
                  <span className={tier.owned ? 'tag ok-text' : 'tag'}>
                    {tier.owned ? 'owned' : 'locked'}
                  </span>
                </div>

                <div className="panel-body">
                  {tier.owned ? (
                    metrics.map((m) => <Reading key={m.key} metric={m} />)
                  ) : (
                    <>
                      <p className="note first">
                        {metrics.length} measurements your body is already producing. You have
                        no way to read them.
                      </p>
                      <div className="slots">
                        {metrics.map((m) => (
                          <div className="locked-slot" key={m.key}>
                            <span className="locked-name">{m.name}</span>
                            <span className="locked-why">{tier.name.toLowerCase()}</span>
                          </div>
                        ))}
                      </div>
                      <div className="btn-row">
                        <button
                          className="btn"
                          onClick={() => void buyInstrument(tier.key)}
                          disabled={busy || money < tier.price}
                          title={money < tier.price ? 'more than you have' : tier.buys}
                        >
                          {tier.name} — {spending(tier.price)}
                        </button>
                      </div>
                      <p className="note last">
                        It reads what your body is already doing — and the history with it,
                        because the samples have been recorded since your first day.
                      </p>
                    </>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </Overlay>
  )
}

/** One day of living, as three closed choices. Never a free field. */
function Living({
  sleep,
  training,
  diet,
  onSleep,
  onTraining,
  onDiet,
  livedToday,
}: {
  sleep: string
  training: string
  diet: string
  onSleep: (v: string) => void
  onTraining: (v: string) => void
  onDiet: (v: string) => void
  livedToday: boolean
}) {
  const busy = useWorld(sending)

  if (livedToday) {
    return (
      <div className="notice cool">
        Today has been lived. One game day is one real day — come back tomorrow, or wait for the
        rollover to live it for you.
      </div>
    )
  }

  const rows: [string, string, (v: string) => void, [string, string][]][] = [
    ['Slept', sleep, onSleep, [['short', '5 hours'], ['ordinary', '7 hours'], ['full', '8½ hours']]],
    ['Trained', training, onTraining, [['rest', 'not at all'], ['light', 'lightly'], ['hard', 'hard']]],
    ['Ate', diet, onDiet, [['poor', 'badly'], ['mixed', 'mixed'], ['clean', 'well']]],
  ]

  return (
    <>
      {rows.map(([label, value, set, options]) => (
        <div className="metric-row" key={label}>
          <div>
            <div className="metric-name">{label}</div>
          </div>
          <div className="btn-row">
            {options.map(([key, said]) => (
              <button
                key={key}
                className={`btn small${key === value ? ' spent' : ''}`}
                onClick={() => set(key)}
                disabled={busy}
              >
                {said}
              </button>
            ))}
          </div>
        </div>
      ))}

      <div className="btn-row">
        <button
          className="btn primary"
          onClick={() => void liveADay({ sleep, training, diet })}
          disabled={busy}
        >
          Live today
        </button>
      </div>

      <p className="faint tiny">
        Three choices, no free fields — this is what a routine will produce automatically once
        routines exist, and what the daily rollover will apply at your own midnight.
      </p>
    </>
  )
}

/** One metric with an instrument behind it, and the trend that instrument saw. */
function Reading({ metric }: { metric: Metric }) {
  const value = metric.value ?? 0

  return (
    <>
      <div className="metric-row">
        <div>
          <div className="metric-name">{metric.name}</div>
          <div className="metric-sub">{metric.means}</div>
        </div>
        <div className="metric-value">
          {value.toFixed(metric.decimals)}
          {metric.unit && <span className="metric-sub"> {metric.unit}</span>}
        </div>
      </div>
      {/* Under the row rather than inside it: a metric row's third column is
          104px of bar, and a trend wants the width of the card. */}
      {metric.history.length > 1 && <Trend history={metric.history} />}
    </>
  )
}

/**
 * A trend, drawn from the samples the server sent.
 *
 * Pure presentation: the numbers are the server's and this only decides where
 * they land in a 280×62 box.
 */
function Trend({ history, from, to }: { history: number[]; from?: string; to?: string }) {
  const low = Math.min(...history)
  const high = Math.max(...history)
  const span = high - low || 1

  const points = history.map((v, i) => {
    const x = (i / Math.max(history.length - 1, 1)) * 280
    // SVG y grows downward, so a high reading is a low number.
    const y = 56 - ((v - low) / span) * 50
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })

  return (
    <div className="spark">
      <svg viewBox="0 0 280 62" preserveAspectRatio="none">
        <polyline
          fill="none"
          stroke="var(--ink-plot)"
          strokeWidth="1.6"
          strokeLinejoin="round"
          points={points.join(' ')}
        />
      </svg>
      {/* The same drawing reads backwards or forwards — a recorded trend and a
          projected one are the same shape, and only the axis says which. */}
      <div className="spark-axis">
        <span>{from ?? `${history.length} days ago`}</span>
        <span>{to ?? 'today'}</span>
      </div>
    </div>
  )
}
