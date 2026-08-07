/*
 * How much of the city the GPU is allowed to be holding.
 *
 * technical.md §10.4: "**per-location lazy loading with adjacent-location
 * preload, and a texture budget so the GPU never holds the whole city.**"
 * Painted backgrounds are the main frontend performance constraint — a plate is
 * a megapixel or two, and a district is a dozen of them — so the answer is not
 * to load less, it is to *let go*.
 *
 * All three of §10.4's requirements are one policy and it is the policy that
 * lives here, with no Pixi in it and no fetch in it:
 *
 *   - **Lazy** is the budget having nothing in it until somebody walks
 *     somewhere.
 *   - **Preload** is `nextToWarm`, which answers "what is worth fetching now"
 *     given where you are standing and what the city says is one step away
 *     (§3.12) — the graph is what makes the question answerable at all.
 *   - **The budget** is `createBudget`, a least-recently-shown map with a hard
 *     size and one exemption: the location on screen is never a candidate,
 *     however old it is.
 *
 * `renderer.ts` is what turns "you have let go of this" into `Assets.unload`.
 * The separation is the point — the holding *policy* is arithmetic over strings
 * and can be driven a hundred locations deep in a test, which the thing that
 * needs a GPU cannot.
 */

/**
 * How many locations are held at once.
 *
 * Small enough to be a budget; large enough that walking a street, going
 * indoors, and coming back out again costs one upload rather than three.
 */
export const BUDGET = 6

/** One thing let go of, so the caller can let go of what hangs off it. */
export type Dropped<T> = { key: string; value: T }

export type Budget<T> = {
  /** How many may be held at once. */
  readonly size: number
  /**
   * Which location is on screen.
   *
   * The one exemption from eviction. Null before anything has been shown, which
   * is a real state — the renderer holds nothing until the world says where the
   * player is.
   */
  showing(key: string | null): void
  has(key: string): boolean
  /** Take one and make it the most recently used, or undefined if it is gone. */
  use(key: string): T | undefined
  /** Put one in, and hand back everything that fell out of the budget. */
  hold(key: string, value: T): Dropped<T>[]
  /** Everything held, least recently used first. */
  keys(): string[]
  /** Let go of everything, and hand it all back. */
  empty(): Dropped<T>[]
}

/**
 * A least-recently-shown store with a hard ceiling.
 *
 * An insertion-ordered `Map` used as an LRU: re-showing a location moves it to
 * the end, and the front is what goes when the ceiling is exceeded. The
 * location currently on screen is skipped over rather than counted out, so a
 * budget of one still works — you cannot evict what you are looking at.
 */
export function createBudget<T>(size: number = BUDGET): Budget<T> {
  const held = new Map<string, T>()
  let shown: string | null = null

  function evict(): Dropped<T>[] {
    const dropped: Dropped<T>[] = []
    for (const [key, value] of held) {
      if (held.size <= size) break
      if (key === shown) continue
      held.delete(key)
      dropped.push({ key, value })
    }
    return dropped
  }

  return {
    size,

    showing(key) {
      shown = key
    },

    has(key) {
      return held.has(key)
    },

    use(key) {
      const value = held.get(key)
      if (value === undefined) return undefined
      // Deleting and re-setting is what moves it to the end of the insertion
      // order, which is the whole of the "recently" in least-recently-used.
      held.delete(key)
      held.set(key, value)
      return value
    },

    hold(key, value) {
      held.delete(key)
      held.set(key, value)
      return evict()
    },

    keys() {
      return [...held.keys()]
    },

    empty() {
      const dropped = [...held].map(([key, value]) => ({ key, value }))
      held.clear()
      return dropped
    },
  }
}

/**
 * The one location worth fetching before it is asked for, or null.
 *
 * §10.4's adjacent-location preload, as a decision rather than a loop. **One at
 * a time, and only when nothing else is loading**: a city block of plates
 * fetched at once is the same bytes arriving later, on a connection the arrival
 * the player is waiting for is sharing.
 *
 * `paintable` is the question only the build can answer — most of the city is
 * not painted yet (#35), and warming a place with no map is a fetch that can
 * only 404.
 */
export function nextToWarm(
  budget: Pick<Budget<unknown>, 'has'>,
  loading: ReadonlySet<string>,
  near: readonly string[],
  paintable: (key: string) => boolean,
): string | null {
  if (loading.size > 0) return null
  for (const key of near) {
    if (!budget.has(key) && paintable(key)) return key
  }
  return null
}
