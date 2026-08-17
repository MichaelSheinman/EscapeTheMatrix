import { chart, polypath, readAs, runs, spline } from '@etm/ui/chart'
import type { SeriesConfig, SeriesRole, Unit } from '@etm/ui/chart'
import type { CSSProperties } from 'react'

/**
 * LineChart — a series over an axis.
 *
 * The `web/` half of @etm/ui's LineChart, and the only chart in this client:
 * a trend, a projection and a career of days are the same drawing at three
 * sizes, and they were three hand-rolled SVGs until they were one component.
 *
 * The arithmetic is not here. It is in `@etm/ui/chart`, which the string
 * renderer in `ui/index.js` also calls, because the contract those two share
 * is the numbers rather than the markup — two implementations of a monotone
 * spline drift the first time either is touched. What this file owns is JSX.
 *
 * Three things about it are decisions rather than defaults:
 *
 * The axis ends on a tick and, by default, starts at zero: a line drawn
 * between its own lowest and highest reading turns a quiet fortnight into a
 * mountain range. `baseline="fit"` is the exception a biomarker needs, where
 * the whole reading is the wobble. Neither end is ever a *target* — a line
 * with a ceiling above it is a progress bar however it is styled (§3.5).
 *
 * `colorScheme` takes a role — 'plot', 'danger' — and never a hue. That is
 * @etm/ui's first rule and there is no exception for charts: a value named
 * after the paint is wrong the first time the paint changes.
 *
 * Hover is CSS, not state. Each reading owns a band of the plot and states
 * itself when the pointer is over it, so nothing here re-renders and the
 * same markup behaves the same way coming out of the string renderer.
 */
export function LineChart({
  data = [],
  dataKeys = [],
  categoryKey,
  config,
  colorScheme = 'plot',
  colorStep = 500,
  showGrid = true,
  showXAxis = true,
  showYAxis = true,
  showLegend = true,
  showTooltip = true,
  unit = 'number',
  xAxisUnit,
  curved = true,
  filled = true,
  stacked = false,
  connectNulls = false,
  allowDecimals = true,
  xAxisType = 'category',
  xAxisTickCount,
  baseline = 'zero',
  strokeWidth = 2,
  yAxisWidth = 10,
  legendMaxItems,
  height = 148,
  containerClassName,
}: LineChartProps) {
  const g = chart({
    data, dataKeys, categoryKey, config, colorScheme, colorStep, stacked,
    xAxisType, xAxisTickCount, baseline,
  })
  if (!g.series.length || !g.xs.length) return null

  // A category axis reads its labels off the data as written. A number axis
  // has to be told how to say them, and says them the way the values are —
  // which is what puts dates under a chart whose x is a day.
  const xUnit = xAxisUnit ?? (xAxisType === 'number' ? unit : null)
  const label = (i: number) =>
    xUnit ? readAs(g.cats[i] as number, xUnit, allowDecimals) : String(g.cats[i] ?? '')
  const path = curved ? spline : polypath
  const pct = (v: number) => `${g.pct(v)}%`

  const legend = legendMaxItems ? g.series.slice(0, legendMaxItems) : g.series

  return (
    <div
      className={containerClassName ? `chart ${containerClassName}` : 'chart'}
      style={{ '--chart-h': `${height}px`, '--chart-y-w': `${yAxisWidth}px` } as CSSProperties}
    >
      <div className="chart-frame">
        {/* Always in the grid, empty or not: it is what puts the plot in the
            second column, and a chart with its labels off must not sit
            half a gutter left of one with them on. */}
        <div className="chart-y">
          {showYAxis &&
            g.ticks.map((t) => (
              <span key={t} style={{ bottom: pct(t) }}>
                {readAs(t, unit, allowDecimals)}
              </span>
            ))}
        </div>

        <div className="chart-plot">
          {/* Borders rather than strokes — see .chart-grid. A dash inside the
              stretched box is a different length depending on which way it
              points, and `vector-effect` has nothing to say about dashes. */}
          {showGrid && (
            <div className="chart-grid">
              {g.ticks.map((t) => (
                <i className="h" key={`h${t}`} style={{ bottom: pct(t) }} />
              ))}
              {g.marks.map((i) => (
                <i className="v" key={`v${i}`} style={{ left: `${g.xs[i]}%` }} />
              ))}
            </div>
          )}

          {/* One stretched box, so every coordinate in it is a percentage and
              the HTML layers above line up with it for nothing. Strokes opt
              out of the stretch or a vertical line comes out heavier than a
              horizontal one. */}
          <svg className="chart-svg" viewBox="0 0 100 100" preserveAspectRatio="none">
            {g.series.map((s, si) =>
              runs(g.points[si], connectNulls).map((run, ri) => {
                const key = `${s.key}-${ri}`
                // A run of one has no line in it. Drawn as a mark rather than
                // dropped: a reading with nothing either side of it happened.
                if (run.length < 2) {
                  return (
                    <line
                      key={key}
                      x1={run[0][0]}
                      y1={run[0][1]}
                      x2={run[0][0]}
                      y2={run[0][1]}
                      stroke={s.ink}
                      strokeWidth={strokeWidth * 2.6}
                      strokeLinecap="round"
                      vectorEffect="non-scaling-stroke"
                    />
                  )
                }
                const d = path(run)
                return (
                  <g key={key}>
                    {filled && (
                      <path
                        d={`${d}L${run[run.length - 1][0]},100L${run[0][0]},100Z`}
                        fill={s.ink}
                        fillOpacity="0.13"
                        stroke="none"
                      />
                    )}
                    <path
                      d={d}
                      fill="none"
                      stroke={s.ink}
                      strokeWidth={strokeWidth}
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeDasharray={s.dash}
                      vectorEffect="non-scaling-stroke"
                    />
                  </g>
                )
              }),
            )}
          </svg>

          {/* Every pixel of the plot belongs to exactly one reading, so there
              is nowhere to point that answers nothing. */}
          {showTooltip &&
            g.bands.map((b, i) => (
              <div
                key={i}
                className={
                  'chart-hit' + (i === 0 ? ' start' : i === g.bands.length - 1 ? ' end' : '')
                }
                style={{ left: `${b.left}%`, width: `${b.width}%` }}
              >
                <i className="chart-cursor" />
                {g.series.map((s, si) => {
                  const v = g.rows[i][si]
                  if (v === null) return null
                  return (
                    <i
                      key={s.key}
                      className="chart-dot"
                      style={{ bottom: pct(v), '--chart-key': s.ink } as CSSProperties}
                    />
                  )
                })}
                <div className="chart-tip">
                  <div className="chart-tip-head">{label(i)}</div>
                  {g.series.map((s, si) => (
                    <div className="chart-tip-row" key={s.key}>
                      <span
                        className="chart-key"
                        style={{ '--chart-key': s.ink } as CSSProperties}
                      />
                      <span>{s.label}</span>
                      <span className="chart-tip-value">
                        {readAs(g.rows[i][si], unit, allowDecimals)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
        </div>

        {showXAxis && (
          <div className="chart-x">
            {g.marks.map((i) => (
              <span
                key={i}
                className={i === 0 ? 'start' : i === g.xs.length - 1 ? 'end' : undefined}
                style={{ left: `${g.xs[i]}%` }}
              >
                {label(i)}
              </span>
            ))}
          </div>
        )}
      </div>

      {showLegend && (
        <div className="chart-legend">
          {legend.map((s) => (
            <span className="chart-legend-item" key={s.key}>
              <span className="chart-key" style={{ '--chart-key': s.ink } as CSSProperties} />
              {s.label}
            </span>
          ))}
          {legend.length < g.series.length && (
            <span className="chart-legend-item">+{g.series.length - legend.length} more</span>
          )}
        </div>
      )}
    </div>
  )
}

export type LineChartProps = {
  /** Rows; one object per point. */
  data?: Record<string, unknown>[]
  /** Which fields of a row are series. */
  dataKeys?: string[]
  /** Which field is the axis value. */
  categoryKey?: string
  /** Per-series overrides: what it is called, what it is drawn in. */
  config?: { data?: Record<string, SeriesConfig> }
  /** A series *role*, never a hue. */
  colorScheme?: SeriesRole
  /** Shade within that role; 500 is the role itself. */
  colorStep?: number
  showGrid?: boolean
  showXAxis?: boolean
  showYAxis?: boolean
  showLegend?: boolean
  showTooltip?: boolean
  /** How a value reads, on the axis and in the tip. */
  unit?: Unit
  /** How the axis value reads. Defaults to `unit` on a number axis. */
  xAxisUnit?: Unit
  /** A monotone curve between points instead of straight segments. */
  curved?: boolean
  /** Fill under the line. */
  filled?: boolean
  stacked?: boolean
  /** Bridge readings nobody took. Never the default — a gap is a gap. */
  connectNulls?: boolean
  allowDecimals?: boolean
  /** 'category' spaces rows evenly; 'number' spaces them by their value. */
  xAxisType?: 'category' | 'number'
  /** At most this many labels on the axis. */
  xAxisTickCount?: number
  /**
   * Where the value axis starts. 'zero' reads growth against nothing, which
   * is what a composite wants; 'fit' starts at the readings, which is the
   * only way four hundred grams on an eighty-kilo body is a shape.
   */
  baseline?: 'zero' | 'fit'
  strokeWidth?: number
  /** Pixels reserved for the value labels; content may widen it. */
  yAxisWidth?: number
  /** Legend entries before the rest collapse into "+N more". */
  legendMaxItems?: number
  /** The plot, in pixels. Labels and legend sit outside it. */
  height?: number
  containerClassName?: string
}
