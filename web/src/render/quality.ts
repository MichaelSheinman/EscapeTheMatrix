/*
 * The effects tier, as data (§10.4):
 *
 *   | Low  | No full-screen filters, no particles, lighting baked into the art |
 *   | High | Full filter stack, real-time lighting, particles, post-processing |
 *
 * The table is here rather than spread through `scene.ts` as `if (high)`, which
 * buys two things. "The Low tier has no full-screen filters and no particles" is
 * an assertion about `EFFECTS.low` that `quality.test.ts` makes, and an effect
 * that forgot to ask which tier it was in would have nothing to add itself to.
 * And Low is what a new device gets, always, with High a thing a player asks
 * for.
 *
 * There is no Pixi in this file. It is the decision; `effects.ts` is the
 * drawing.
 */

/** Which of §10.4's two tiers the world is drawn at. */
export type Tier = 'low' | 'high'

/**
 * What a tier contains. Four booleans rather than one, because §10.4 names four
 * separate costs and they are separately expensive: a machine that can carry the
 * light pools may still not carry a full-screen blur. The two rows below are the
 * whole vocabulary.
 */
export type Effects = {
  /** Full-screen filters over the finished frame. Fill-rate bound (§10.4). */
  filters: boolean
  /** Lights that respond to where the character is standing, and bloom. */
  lighting: boolean
  /** Drifting motes in the air of a location. */
  particles: boolean
  /** The grade, the grain and the vignette, after everything else. */
  post: boolean
}

/** §10.4's table. The one place a tier's contents are stated. */
export const EFFECTS: Record<Tier, Effects> = {
  low: { filters: false, lighting: false, particles: false, post: false },
  high: { filters: true, lighting: true, particles: true, post: true },
}

/** The tier a device gets before anybody has said otherwise (§10.4). */
export const FLOOR: Tier = 'low'

/** Every tier, as a screen offers them. */
export const TIERS: { key: Tier; name: string; says: string }[] = [
  {
    key: 'low',
    name: 'Plain',
    says: 'The painting, and nothing over it. The light in the world is the light that was painted into it.',
  },
  {
    key: 'high',
    name: 'Lit',
    says: 'Lamps that reach, air with something in it, and a graded frame. Costs a modern GPU rather than an old one.',
  },
]

/** Whether a string off a shelf is one of the two tiers. */
export function isTier(value: unknown): value is Tier {
  return value === 'low' || value === 'high'
}

/* ── what this device remembered ──────────────────────────────────────────── */

/** Where the chosen tier is kept. A device preference, not a fact about a player. */
const SHELF = 'etm.quality'

/**
 * Local storage, if this browser will give it up.
 *
 * Private windows and blocked third-party contexts throw on *access*, not on
 * use, so the whole thing is inside the try. A device that will not remember
 * the choice still gets to make it — it just makes it again next time.
 */
export function shelf(): Storage | null {
  try {
    return globalThis.localStorage ?? null
  } catch {
    return null
  }
}

/** The tier this device last settled on, or null if it never has. */
export function readTier(store: Storage | null): Tier | null {
  try {
    const kept = store?.getItem(SHELF)
    return isTier(kept) ? kept : null
  } catch {
    return null
  }
}

/** Remember a tier for next time. Never throws: this is a convenience. */
export function rememberTier(store: Storage | null, tier: Tier): void {
  try {
    store?.setItem(SHELF, tier)
  } catch {
    // A full or refused quota is not worth failing a frame over.
  }
}

/* ── the governor ─────────────────────────────────────────────────────────── */

/**
 * The frame time the High tier has to hold, in milliseconds. 20ms is 50 frames a
 * second rather than 16.7, because a machine that misses 60Hz by a hair should
 * not be thrown back to the plain tier.
 */
export const SMOOTH = 20

/**
 * How many frames are weighed before a verdict: about three seconds. Long enough
 * that a single stall — a texture upload, a shader compile, the browser doing
 * something else — cannot decide it, which is also why the verdict is the median
 * rather than the mean.
 */
export const WINDOW = 180

/**
 * How many frames are ignored after something changed. Switching tier compiles
 * shaders and allocates render targets, and mounting a location uploads a plate.
 * Both are slow frames that say nothing about whether the machine can carry the
 * tier once it is running.
 */
export const GRACE = 120

/**
 * A frame longer than this was not a frame: a backgrounded tab, a laptop lid, a
 * breakpoint. Counting it would demote a machine for having been ignored.
 */
export const STALL = 250

/** How many frames the reported rate is averaged over. */
const RATE = 30

export type Verdict = 'keep' | 'drop'

/**
 * Watches the frame clock and decides whether the High tier is holding. A
 * machine that cannot carry the tier says so and falls back rather than running
 * at fifteen frames a second while the player wonders what is wrong. It only
 * ever goes down: climbing back up on its own would make a machine near the line
 * oscillate, and the player already has a button.
 */
export type Governor = {
  /**
   * One frame happened, and the return says whether the tier should stand.
   * `drop` comes back once per verdict — the window is emptied and the grace
   * restarted — so a caller that ignores it is not told again immediately.
   */
  measured(elapsedMS: number, tier: Tier): Verdict
  /**
   * Frames a second over the last stretch, or 0 before there has been one.
   * Averaged over `RATE` frames and restated only when that many have passed,
   * because a React panel reads it and a value changing every frame would be a
   * render every frame.
   */
  rate(): number
  /** Something slow and one-off is about to happen: do not judge it. */
  settle(): void
}

export function createGovernor(): Governor {
  /** Frame times inside the current window. Emptied at each verdict. */
  const window: number[] = []
  /** Frames still to be ignored. */
  let grace = GRACE

  let rateSum = 0
  let rateCount = 0
  let rate = 0

  return {
    measured(elapsedMS, tier) {
      if (!Number.isFinite(elapsedMS) || elapsedMS <= 0 || elapsedMS > STALL) return 'keep'

      rateSum += elapsedMS
      rateCount++
      if (rateCount >= RATE) {
        rate = Math.round(1000 / (rateSum / rateCount))
        rateSum = 0
        rateCount = 0
      }

      // Nothing to judge: the plain tier is the floor, with nowhere below it to
      // be sent.
      if (tier === 'low') return 'keep'

      if (grace > 0) {
        grace--
        return 'keep'
      }

      window.push(elapsedMS)
      if (window.length < WINDOW) return 'keep'

      const middle = median(window)
      window.length = 0
      grace = GRACE
      return middle > SMOOTH ? 'drop' : 'keep'
    },

    rate() {
      return rate
    },

    settle() {
      window.length = 0
      grace = GRACE
    },
  }
}

/** The middle of a sample. Sorts a copy — the caller's window is a ring. */
function median(sample: readonly number[]): number {
  const sorted = [...sample].sort((a, b) => a - b)
  const half = sorted.length >> 1
  return sorted.length % 2 === 1 ? sorted[half] : (sorted[half - 1] + sorted[half]) / 2
}
