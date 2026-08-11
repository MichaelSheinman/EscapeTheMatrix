/* @etm/ui — chart geometry
   ===================================================================
   The numbers behind LineChart, and nothing else: no DOM, no state, no
   copy. It is its own module because this component has two renderers —
   the string builder in index.js, the JSX in `web/` — and the contract
   they must share is arithmetic rather than markup. Two implementations
   of a monotone spline drift the first time either is touched.

   Rule 1 from index.js holds here too: a series is named by its role,
   never by its hue. Only tokens.css may name paint.
   =================================================================== */

/** The order extra series are given roles in, when the caller names none. */
const SERIES_ROLES = ['plot', 'info', 'success', 'danger', 'accent', 'muted'];

/**
 * seriesInk — a series colour, as a role rather than as a hue.
 *
 * `step` is the shade within that role: 500 is the role itself, below it
 * mixes toward the paper and above it toward the ink. That is the same
 * direction a design-system ramp moves, without this package having to
 * invent nine literals per hue — which it may not do anyway, since only
 * tokens.css is allowed to name paint.
 */
export const seriesInk = (role = 'plot', step = 500) => {
  const base = `var(--ink-series-${role}, var(--ink-series-plot))`;
  if (step === 500) return base;
  const toward = step < 500 ? 'var(--bg-surface)' : 'var(--text-default)';
  return `color-mix(in oklab, ${base}, ${toward} ${Math.round(Math.abs(step - 500) / 500 * 55)}%)`;
};

/**
 * scale — a tick step that lands on a number somebody can read, and the two
 * ends of the axis it implies.
 *
 * Both ends are ticks, so the topmost grid line is the top of the plot.
 *
 * `min` is where the axis is allowed to start, and it is the whole of the
 * argument between LineChart's two baselines. At zero, growth reads against
 * nothing — which is what a composite wants, and what stops a quiet fortnight
 * being drawn as a mountain range. Fitted to the readings, a weight that moved
 * four hundred grams is a shape rather than a flat line at eighty per cent of
 * nowhere. Neither is right for both, so neither is the only one.
 */
const scale = (max, count = 4, min = 0) => {
  const span = max - min;
  if (!(span > 0)) return { ticks: [], top: max || 1, bottom: min };
  const magnitude = Math.pow(10, Math.floor(Math.log10(span / count)));
  const step = [1, 2, 2.5, 4, 5, 10].map((m) => m * magnitude).find((s) => s >= span / count)
    ?? magnitude * 10;
  const bottom = Math.floor(min / step) * step;
  const top = Math.ceil(max / step) * step;
  const ticks = [];
  for (let v = bottom + step; v <= top + step / 1000; v += step) ticks.push(v);
  return { ticks, top, bottom };
};

/**
 * readAs — a value as the axis or the tip says it.
 * @param unit 'number' | 'pct' | 'currency' | 'time' | 'date'
 */
export const readAs = (v, unit = 'number', allowDecimals = true) => {
  if (v === null || v === undefined || Number.isNaN(v)) return '—';
  const digits = allowDecimals ? 1 : 0;
  const n = (d = digits) => Number(v).toLocaleString(undefined, { maximumFractionDigits: d });
  switch (unit) {
    case 'pct': return `${n()}%`;
    case 'currency': return `$${n(0)}`;
    case 'time': {
      const s = Math.max(0, Math.round(Number(v)));
      return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
    }
    // A day number — whole days since the epoch — as a date. The axis is
    // spaced by it, so a week nobody played is a week wide.
    case 'date': return new Date(Number(v) * 86400000)
      .toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });
    default: return n();
  }
};

/**
 * spline — a path through the points, monotone so it never overshoots.
 *
 * Fritsch–Carlson: a plain cubic through readings that plateau invents a dip
 * on either side of the flat part, and a dip nobody had is a worse lie than
 * a corner nobody drew.
 */
export const spline = (pts) => {
  const n = pts.length;
  if (n < 2) return '';
  const dx = [], m = [], t = [];
  for (let i = 0; i < n - 1; i++) {
    dx[i] = pts[i + 1][0] - pts[i][0];
    m[i] = dx[i] === 0 ? 0 : (pts[i + 1][1] - pts[i][1]) / dx[i];
  }
  t[0] = m[0];
  for (let i = 1; i < n - 1; i++) t[i] = m[i - 1] * m[i] <= 0 ? 0 : (m[i - 1] + m[i]) / 2;
  t[n - 1] = m[n - 2];
  for (let i = 0; i < n - 1; i++) {
    if (m[i] === 0) { t[i] = 0; t[i + 1] = 0; continue; }
    const a = t[i] / m[i], b = t[i + 1] / m[i], s = a * a + b * b;
    if (s > 9) { const f = 3 / Math.sqrt(s); t[i] = f * a * m[i]; t[i + 1] = f * b * m[i]; }
  }
  let d = `M${pts[0][0]},${pts[0][1]}`;
  for (let i = 0; i < n - 1; i++) {
    const h = dx[i] / 3;
    d += `C${pts[i][0] + h},${pts[i][1] + t[i] * h} ` +
         `${pts[i + 1][0] - h},${pts[i + 1][1] - t[i + 1] * h} ` +
         `${pts[i + 1][0]},${pts[i + 1][1]}`;
  }
  return d;
};

/** A path through the points, corner to corner. */
export const polypath = (pts) => pts.map((p, i) => `${i ? 'L' : 'M'}${p[0]},${p[1]}`).join('');

/**
 * runs — the points, split wherever the series has nothing to say.
 *
 * A gap is a gap by default. `connectNulls` bridges it, which is a claim
 * about days nobody measured and is therefore never the default.
 */
export const runs = (pts, connectNulls) => {
  if (connectNulls) return [pts.filter((p) => p)];
  const out = [];
  let run = [];
  for (const p of pts) {
    if (p) { run.push(p); continue; }
    if (run.length) out.push(run);
    run = [];
  }
  if (run.length) out.push(run);
  return out;
};

/**
 * chart — everything a LineChart needs to draw itself, from its arguments.
 *
 * Split out from the markup so that the two renderers of this component —
 * the string below, the component in `web/` — share one set of numbers.
 */
export const chart = (o) => {
  const data = o.data || [];
  const keys = o.dataKeys || [];
  const cfg = (o.config && o.config.data) || {};
  const stacked = !!o.stacked;

  const series = keys.map((key, i) => {
    const c = cfg[key] || {};
    const role = c.colorScheme || (keys.length > 1 ? SERIES_ROLES[i % SERIES_ROLES.length]
                                                   : (o.colorScheme || 'plot'));
    return {
      key,
      label: c.label || key,
      dash: c.strokeDasharray,
      ink: seriesInk(role, c.colorStep ?? (o.colorStep ?? 500)),
    };
  });

  // One row of readings per row of data, stacked into running totals when
  // asked. A null stays null: it is a reading nobody took, and adding zero
  // to the stack in its place would draw it as one.
  const rows = data.map((row) => {
    let acc = 0;
    return series.map((s) => {
      const raw = row[s.key];
      const v = raw === null || raw === undefined || raw === '' ? null : Number(raw);
      if (!stacked || v === null) return v;
      acc += v;
      return acc;
    });
  });

  let max = 0, min = Infinity;
  for (const r of rows) for (const v of r) {
    if (v === null) continue;
    if (v > max) max = v;
    if (v < min) min = v;
  }
  // A stack is read from its own floor whatever the caller says: the bottom
  // series starts at zero by construction, and a fitted axis would cut it off.
  const floor = o.baseline === 'fit' && !stacked && Number.isFinite(min) ? min : 0;
  const { ticks, top, bottom } = scale(max, 4, floor);

  // Where each row sits along the axis. A category axis spaces them evenly;
  // a number axis spaces them by their own value, which is what makes a
  // gap in the data look like a gap in the time.
  const cats = data.map((row) => row[o.categoryKey]);
  const numeric = o.xAxisType === 'number';
  let xs;
  if (data.length < 2) {
    xs = data.map(() => 50);
  } else if (numeric) {
    const vs = cats.map(Number);
    const lo = Math.min(...vs), span = Math.max(...vs) - lo || 1;
    xs = vs.map((v) => ((v - lo) / span) * 100);
  } else {
    xs = data.map((_, i) => (i / (data.length - 1)) * 100);
  }

  // How far up the plot a value sits, and the same thing as an SVG y — which
  // grows downward, so a high reading is a low number.
  const pct = (v) => ((v - bottom) / (top - bottom || 1)) * 100;
  const y = (v) => 100 - pct(v);
  const points = series.map((_, si) => rows.map((r, i) => r[si] === null ? null : [xs[i], y(r[si])]));

  // The bands that answer the pointer: each row owns the ground up to the
  // midpoint of its neighbours, so every pixel of the plot belongs to
  // exactly one reading and there is nowhere to hover that says nothing.
  const bands = xs.map((x, i) => {
    const left = i === 0 ? 0 : (xs[i - 1] + x) / 2;
    const right = i === xs.length - 1 ? 100 : (x + xs[i + 1]) / 2;
    return { left, width: Math.max(right - left, 0) };
  });

  // At most `xAxisTickCount` labels, always including the first and last —
  // sixty daily readings are sixty points and never sixty dates.
  const want = Math.max(2, Math.min(o.xAxisTickCount || 5, data.length));
  const marks = data.length <= want
    ? data.map((_, i) => i)
    : Array.from({ length: want }, (_, k) => Math.round((k / (want - 1)) * (data.length - 1)));

  return { series, rows, xs, pct, y, top, bottom, ticks, points, bands, cats,
           marks: [...new Set(marks)] };
};
