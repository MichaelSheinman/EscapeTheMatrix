import { describe, expect, it } from 'vitest'

import {
  createGovernor,
  EFFECTS,
  FLOOR,
  GRACE,
  isTier,
  readTier,
  rememberTier,
  SMOOTH,
  STALL,
  TIERS,
  WINDOW,
} from './quality'

/*
 * §10.4's table, and the clock that enforces it.
 *
 * The first block is the acceptance criterion "the Low tier has no full-screen
 * filters and no particles" as an assertion rather than an argument. It looks
 * trivial and is not: it is the reason the tier is data in one place instead of
 * a series of `if` in the drawing, where a new effect could quietly join the
 * wrong row.
 */

describe('the two tiers', () => {
  it('are exactly what §10.4 says they are', () => {
    expect(EFFECTS.low).toEqual({
      filters: false,
      lighting: false,
      particles: false,
      post: false,
    })
    expect(EFFECTS.high).toEqual({
      filters: true,
      lighting: true,
      particles: true,
      post: true,
    })
  })

  it('put the plain one on the floor, because that is the one that ships first', () => {
    expect(FLOOR).toBe('low')
    expect(TIERS.map((tier) => tier.key)).toEqual(['low', 'high'])
    for (const tier of TIERS) expect(tier.says.length).toBeGreaterThan(0)
  })

  it('are the only two things a stored preference may be', () => {
    expect(isTier('low')).toBe(true)
    expect(isTier('high')).toBe(true)
    expect(isTier('ultra')).toBe(false)
    expect(isTier(null)).toBe(false)
  })
})

/** A shelf, without a browser. Enough of `Storage` for the two calls made. */
function shelf(): Storage {
  const kept = new Map<string, string>()
  return {
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => void kept.set(key, value),
    removeItem: (key) => void kept.delete(key),
    clear: () => kept.clear(),
    key: () => null,
    length: 0,
  } as Storage
}

describe('what this machine remembered', () => {
  it('is nothing, until it is something', () => {
    const store = shelf()
    expect(readTier(store)).toBeNull()
    rememberTier(store, 'high')
    expect(readTier(store)).toBe('high')
  })

  it('is nothing when there is nowhere to keep it', () => {
    // A private window, or a blocked storage context. The choice can still be
    // made; it is simply made again next time.
    expect(readTier(null)).toBeNull()
    expect(() => rememberTier(null, 'high')).not.toThrow()
  })

  it('is nothing when what was kept is not a tier', () => {
    const store = shelf()
    store.setItem('etm.quality', 'ultra')
    expect(readTier(store)).toBeNull()
  })

  it('survives a shelf that refuses to be written to', () => {
    const angry = {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
    } as unknown as Storage
    expect(readTier(angry)).toBeNull()
    expect(() => rememberTier(angry, 'low')).not.toThrow()
  })
})

/*
 * The governor, which is the half of "verified on integrated graphics" that can
 * live in the game. A machine that asks for the High tier and cannot carry it
 * says so and falls back, rather than running at fifteen frames a second while
 * its owner wonders what is wrong.
 */

/** Feed the governor `count` frames of the same length, and hand back verdicts. */
function run(
  governor: ReturnType<typeof createGovernor>,
  count: number,
  ms: number,
  tier: 'low' | 'high' = 'high',
): string[] {
  const verdicts: string[] = []
  for (let i = 0; i < count; i++) verdicts.push(governor.measured(ms, tier))
  return verdicts
}

describe('the frame governor', () => {
  it('says nothing at all while the tier is still settling', () => {
    const governor = createGovernor()
    // Slow frames, but they are the frames a tier switch and a plate upload
    // cost. Demoting on those would demote every machine that walked through a
    // door.
    expect(run(governor, GRACE, 100)).not.toContain('drop')
  })

  it('drops the tier when a window of frames is genuinely slow', () => {
    const governor = createGovernor()
    run(governor, GRACE, 8)
    const verdicts = run(governor, WINDOW, SMOOTH + 8)
    expect(verdicts.filter((v) => v === 'drop')).toHaveLength(1)
  })

  it('keeps it when the frames are fine', () => {
    const governor = createGovernor()
    run(governor, GRACE, 8)
    expect(run(governor, WINDOW * 2, 8)).not.toContain('drop')
  })

  it('is not swayed by a handful of bad frames in a good window', () => {
    const governor = createGovernor()
    run(governor, GRACE, 8)

    // A quarter of the window stalled badly and the rest was fine. The median
    // is the reason this is a shrug rather than a demotion.
    const verdicts: string[] = []
    for (let i = 0; i < WINDOW; i++) {
      verdicts.push(governor.measured(i % 4 === 0 ? 120 : 8, 'high'))
    }
    expect(verdicts).not.toContain('drop')
  })

  it('says it once, and then goes quiet again', () => {
    const governor = createGovernor()
    run(governor, GRACE, 8)
    expect(run(governor, WINDOW, SMOOTH + 8)).toContain('drop')
    // The window was emptied and the grace restarted, so a caller that ignored
    // the verdict is not shouted at every frame after it.
    expect(run(governor, GRACE, SMOOTH + 8)).not.toContain('drop')
  })

  it('never judges the plain tier, because there is nowhere below it', () => {
    const governor = createGovernor()
    expect(run(governor, WINDOW * 3, 200, 'low')).not.toContain('drop')
  })

  it('ignores a frame that was the tab not being drawn', () => {
    const governor = createGovernor()
    run(governor, GRACE, 8)
    // A backgrounded tab, a laptop lid, a breakpoint. Counting these would let
    // a machine be demoted for having been ignored.
    expect(run(governor, WINDOW * 2, STALL + 1000)).not.toContain('drop')
  })

  it('reports a rate once it has seen enough frames to have one', () => {
    const governor = createGovernor()
    expect(governor.rate()).toBe(0)
    run(governor, 40, 20, 'low')
    expect(governor.rate()).toBe(50)
  })

  it('forgets what it has measured when something slow is announced', () => {
    const governor = createGovernor()
    run(governor, GRACE, 8)
    run(governor, WINDOW - 1, SMOOTH + 8)
    // A location mounted. The window so far described the upload, not the tier.
    governor.settle()
    expect(run(governor, GRACE, 8)).not.toContain('drop')
    expect(run(governor, WINDOW, 8)).not.toContain('drop')
  })
})
