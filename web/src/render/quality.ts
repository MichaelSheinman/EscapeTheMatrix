/*
 * The effects tier, as data.
 *
 * technical.md §10.4 states it as a table, and this file is that table:
 *
 *   | Low  | No full-screen filters, no particles, lighting baked into the art |
 *   | High | Full filter stack, real-time lighting, particles, post-processing |
 *
 * Two things follow from the table being *here* rather than distributed through
 * `scene.ts` as a series of `if (high)`.
 *
 *   1. **Low is checkable.** "The Low tier has no full-screen filters and no
 *      particles" is an assertion about `EFFECTS.low`, not an argument about
 *      code paths, and `quality.test.ts` makes it. A future effect that forgot
 *      to ask which tier it was in would have nothing to add itself to.
 *   2. **Low ships first, and stays shippable.** §10.4: "Retrofitting *works
 *      without the lighting pass* into a look designed around the lighting pass
 *      means redoing the look." So Low is what a new device gets, always, and
 *      High is a thing a player asks for.
 *
 * There is no Pixi in this file and there must not be. It is the *decision*;
 * `effects.ts` is the drawing.
 */

/** Which of §10.4's two tiers the world is drawn at. */
export type Tier = 'low' | 'high'

/**
 * What a tier contains.
 *
 * Four booleans rather than one, because §10.4 names four separate costs and
 * they are separately expensive — a machine that can carry the light pools may
 * still not carry a full-screen blur. Nothing in V1 mixes them: the two rows
 * below are the whole vocabulary, and a third row would be a design decision
 * rather than a configuration.
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
 * The frame time the High tier has to hold, in milliseconds.
 *
 * 20ms is 50 frames a second. Not 16.7: a machine that misses 60Hz by a hair is
 * not a machine that should be thrown back to the plain tier, and §10.4's worry
 * is "three stacked full-screen shaders at 1440p on weak integrated graphics",
 * which does not miss by a hair.
 */
export const SMOOTH = 20

/**
 * How many frames are weighed before a verdict.
 *
 * About three seconds. Long enough that a single stall — a texture upload, a
 * shader compile, the browser doing something else — cannot decide it, and the
 * verdict is the *median* of the window rather than its mean for the same
 * reason.
 */
export const WINDOW = 180

/**
 * How many frames are ignored after something changed.
 *
 * Switching tier compiles shaders and allocates render targets; mounting a
 * location uploads a plate. Both are slow frames that say nothing about whether
 * the machine can carry the tier once it is running, and judging them would
 * demote every device that ever walked through a door.
 */
export const GRACE = 120

/**
 * A frame longer than this was not a frame.
 *
 * A backgrounded tab, a laptop lid, a breakpoint. Counting it would let a
 * machine be demoted for having been ignored.
 */
export const STALL = 250

/** How many frames the reported rate is averaged over. */
const RATE = 30

export type Verdict = 'keep' | 'drop'

/**
 * Watches the frame clock and decides whether the High tier is holding.
 *
 * §10.4's last acceptance line asks for the Low tier to be verified on
 * integrated graphics. This is the half of that which can live in the game: the
 * world measures itself, and a machine that cannot carry the tier says so and
 * falls back rather than being asked to run at fifteen frames a second while
 * the player wonders what is wrong. It only ever goes **down** — climbing back
 * up on its own would make a machine near the line oscillate, and the player
 * already has a button.
 */
export type Governor = {
  /**
   * One frame happened. Returns whether the tier should stand.
   *
   * `drop` is returned exactly once per verdict: the window is emptied and the
   * grace restarted, so a caller that ignores it is not told again immediately.
   */
  measured(elapsedMS: number, tier: Tier): Verdict
  /**
   * Frames a second over the last stretch, or 0 before there has been one.
   *
   * Averaged over `RATE` frames and restated only when that many have passed,
   * because this number is read by a React panel and a value that changed every
   * frame would be a render every frame.
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

      // Nothing to judge. The plain tier is the floor: there is nowhere below it
      // to be sent, so measuring it would only be a way of being wrong.
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
