import { useWorld } from '../world/react'

/**
 * The last week of one metric, one bar a day, with last night lit.
 *
 * Bars rather than a line: the samples are whole days and there are seven of
 * them, and a curve through seven points draws a trend the body did not make.
 * The bars are scaled against the highest of the week rather than against zero,
 * so a week that ran between six and eight hours reads as the difference it was.
 *
 * Nothing is drawn when the metric belongs to a tier the player has not bought
 * — a shape with no numbers behind it is a chart of a reading they do not have.
 * On a first day there is one sample and the bars are left off; the figure
 * under them is the whole of what there is to say.
 */
export function Nights({ metric, label }: { metric: string; label: string }) {
  const found = useWorld((w) => w.health?.metrics.find((m) => m.key === metric) ?? null)

  if (!found || found.value === null) return null

  const week = found.history.slice(-7)
  const top = Math.max(...week)
  const last = week.length - 1
  const mean = week.reduce((sum, n) => sum + n, 0) / week.length

  return (
    <div className="panel reading">
      <div className="reading-label">{label}</div>

      {week.length > 1 && (
        <div className="nights">
          {week.map((n, i) => (
            <span
              key={i}
              className={i === last ? 'now' : ''}
              style={{ height: `${top > 0 ? Math.round((n / top) * 100) : 0}%` }}
            />
          ))}
        </div>
      )}

      <div className="reading-now">
        <span className="value">{found.value.toFixed(found.decimals)}</span>
        <span className="unit">{found.unit}</span>
        {week.length > 1 && (
          <span className="over">
            {mean.toFixed(found.decimals)} {found.unit} average
          </span>
        )}
      </div>
    </div>
  )
}
