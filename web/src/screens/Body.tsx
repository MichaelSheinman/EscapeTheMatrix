import { buyInstrument } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type Metric } from '../world/state'
import { LineChart } from '../ui/LineChart'
import { Overlay } from './Overlay'
import { spending } from './words'

/**
 * Your body, and what you can see of it (§3.16).
 *
 * The panel is simulated in full for everyone. What instrumentation changes is
 * the player's resolution on their own body, never their health — so the screen
 * is laid out by tier: one card per instrument, showing its readings if you
 * own it and a hatched slot per measurement if you do not. The tier is the only
 * thing standing between you and the number.
 *
 * The cards are one column, in resolution order — what everybody can see, the
 * day you are choosing, then each instrument. They used to run in two, which
 * put the locked tiers in a parallel column that read as a shop beside the
 * body rather than as more of the same body.
 *
 * Biological age is the one readout visible at every tier, composed by the
 * server on read from the panel rather than stored anywhere.
 */
export function Body({ open, onClose }: { open: boolean; onClose: () => void }) {
  const health = useWorld((w) => w.health)
  const crossings = useWorld((w) => w.crossings)
  const money = useWorld((w) => w.character?.money ?? 0)
  const busy = useWorld(sending)

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
    </Overlay>
  )
}

/** One day of living, as three closed choices. Never a free field. */
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
      {metric.history.length > 1 && <Trend history={metric.history} metric={metric} />}
    </>
  )
}

/**
 * A trend, drawn from the samples the server sent.
 *
 * Pure presentation: the numbers are the server's (§10.2) and the only
 * decisions here are which of LineChart's switches are on.
 *
 * Almost all of them are off. This is the small form of the chart — one per
 * metric, several per card — and a grid, an axis and a legend around each of
 * eight of them would be more chrome than body. What stays is the shape, the
 * two dates that say which way it reads, and the pointer: a trend answers
 * *what was it on Tuesday* only if you can ask it.
 *
 * `baseline="fit"` is the one switch that is not about density. A body does
 * not start at zero — a weight is eighty kilos and moves in grams, and drawn
 * against nothing it is a flat line four fifths of the way up. Karma is drawn
 * the other way for the opposite reason: it *did* start at nothing.
 */
function Trend({ history, metric }: { history: number[]; metric: Metric }) {
  const last = history.length - 1

  return (
    <LineChart
      data={history.map((value, i) => ({
        when: i === last ? 'today' : `${last - i} ${last - i === 1 ? 'day' : 'days'} ago`,
        value,
      }))}
      dataKeys={['value']}
      categoryKey="when"
      config={{ data: { value: { label: metric.name } } }}
      allowDecimals={metric.decimals > 0}
      baseline="fit"
      showGrid={false}
      showYAxis={false}
      showLegend={false}
      xAxisTickCount={2}
      strokeWidth={1.6}
      height={62}
      yAxisWidth={0}
    />
  )
}
