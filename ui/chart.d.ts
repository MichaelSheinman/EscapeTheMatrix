/* @etm/ui — chart geometry, typed.
   ===================================================================
   `web/` builds under `strict` and cannot import untyped JavaScript, so
   the shared half of LineChart carries declarations. Only the geometry
   is declared: index.js stays a string builder that nothing in `web/`
   imports, because the markup is the part that does not survive the
   port (see README, "Porting to web/").
   =================================================================== */

/** A series colour role. Never a hue — see tokens.css. */
export type SeriesRole = 'plot' | 'accent' | 'info' | 'success' | 'danger' | 'muted';

/** How a value reads. */
export type Unit = 'number' | 'pct' | 'currency' | 'time' | 'date';

/** Per-series overrides, keyed by the field name in a row. */
export type SeriesConfig = {
  label?: string
  colorScheme?: SeriesRole
  colorStep?: number
  strokeDasharray?: string
}

export type ChartOptions = {
  data?: Record<string, unknown>[]
  dataKeys?: string[]
  categoryKey?: string
  config?: { data?: Record<string, SeriesConfig> }
  colorScheme?: SeriesRole
  colorStep?: number
  stacked?: boolean
  xAxisType?: 'category' | 'number'
  xAxisTickCount?: number
  /** Where the value axis starts: at nothing, or at the readings. */
  baseline?: 'zero' | 'fit'
}

/** One series, resolved: what it is called and what it is drawn in. */
export type Series = {
  key: string
  label: string
  dash?: string
  ink: string
}

/** A point in the 0–100 box, or null where the series has nothing to say. */
export type Point = [number, number] | null

/** The band of plot one reading owns, as percentages of the width. */
export type Band = { left: number; width: number }

/** Everything a LineChart needs to draw itself, from its arguments. */
export type Geometry = {
  series: Series[]
  /** One row of readings per row of data, stacked if asked. */
  rows: (number | null)[][]
  /** Where each row sits along the axis, 0–100. */
  xs: number[]
  /** How far up the plot a value sits, 0–100. */
  pct: (value: number) => number
  /** The same thing as an SVG y, which grows downward. */
  y: (value: number) => number
  /** The two ends of the axis. Both are ticks. */
  top: number
  bottom: number
  ticks: number[]
  /** Per series, the points of that series. */
  points: Point[][]
  bands: Band[]
  /** The raw category value of each row. */
  cats: unknown[]
  /** Which rows get a label on the axis. */
  marks: number[]
}

export declare const SERIES_ROLES: SeriesRole[];
export declare function seriesInk(role?: SeriesRole, step?: number): string;
export declare function scale(
  max: number, count?: number, min?: number,
): { ticks: number[]; top: number; bottom: number };
export declare function readAs(
  value: number | null | undefined, unit?: Unit, allowDecimals?: boolean,
): string;
export declare function spline(points: [number, number][]): string;
export declare function polypath(points: [number, number][]): string;
export declare function runs(points: Point[], connectNulls?: boolean): [number, number][][];
export declare function chart(options: ChartOptions): Geometry;
