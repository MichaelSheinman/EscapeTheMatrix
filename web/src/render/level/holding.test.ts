import { describe, expect, it } from 'vitest'

import { BUDGET, createBudget, nextToWarm } from './holding'

/*
 * §10.4's asset pipeline, driven a hundred locations deep without a GPU.
 *
 * This is the reason the policy was pulled out of `renderer.ts`: "the GPU never
 * holds the whole city" is a claim about what happens after somebody has walked
 * across twenty streets, and there is no way to walk across twenty streets in a
 * test that needs `Application.init`. Here it is twenty function calls.
 */

describe('the texture budget', () => {
  it('holds what it is given, until it is full', () => {
    const budget = createBudget<string>(3)

    expect(budget.hold('a', 'A')).toEqual([])
    expect(budget.hold('b', 'B')).toEqual([])
    expect(budget.hold('c', 'C')).toEqual([])
    expect(budget.keys()).toEqual(['a', 'b', 'c'])
  })

  it('lets go of the least recently used, and says what went', () => {
    const budget = createBudget<string>(3)
    budget.hold('a', 'A')
    budget.hold('b', 'B')
    budget.hold('c', 'C')

    expect(budget.hold('d', 'D')).toEqual([{ key: 'a', value: 'A' }])
    expect(budget.keys()).toEqual(['b', 'c', 'd'])
    expect(budget.has('a')).toBe(false)
  })

  it('counts using one as having used it', () => {
    const budget = createBudget<string>(3)
    budget.hold('a', 'A')
    budget.hold('b', 'B')
    budget.hold('c', 'C')

    // Going back to the oldest makes it the newest, so the next eviction takes
    // somebody else.
    expect(budget.use('a')).toBe('A')
    expect(budget.hold('d', 'D')).toEqual([{ key: 'b', value: 'B' }])
    expect(budget.keys()).toEqual(['c', 'a', 'd'])
  })

  it('never lets go of the location on screen, however old it is', () => {
    const budget = createBudget<string>(2)
    budget.hold('street', 'S')
    budget.showing('street')

    budget.hold('a', 'A')
    budget.hold('b', 'B')
    budget.hold('c', 'C')

    // The street was held first and has not been shown since, and it is still
    // here — you cannot evict what you are looking at.
    expect(budget.has('street')).toBe(true)
    expect(budget.keys()).toContain('street')
  })

  it('holds the city down to a handful, whatever route is walked', () => {
    const budget = createBudget<string>()
    for (let i = 0; i < 100; i++) {
      const key = `place-${i}`
      budget.showing(key)
      budget.hold(key, `plate-${i}`)
    }
    expect(budget.keys()).toHaveLength(BUDGET)
    expect(budget.has('place-99')).toBe(true)
    expect(budget.has('place-0')).toBe(false)
  })

  it('hands everything back when it is emptied', () => {
    const budget = createBudget<string>(3)
    budget.hold('a', 'A')
    budget.hold('b', 'B')

    expect(budget.empty()).toEqual([
      { key: 'a', value: 'A' },
      { key: 'b', value: 'B' },
    ])
    expect(budget.keys()).toEqual([])
  })
})

describe('what is worth warming', () => {
  const painted = (key: string) => key !== 'unpainted'

  it('is the first place one step away that is not already held', () => {
    const budget = createBudget<string>()
    budget.hold('here', 'H')

    expect(nextToWarm(budget, new Set(), ['here', 'next', 'after'], painted)).toBe('next')
  })

  it('is nothing while something is already in flight', () => {
    const budget = createBudget<string>()
    // One at a time: a city block of plates fetched at once is the same bytes
    // arriving later, on a connection the arrival is sharing.
    expect(nextToWarm(budget, new Set(['next']), ['after'], painted)).toBeNull()
  })

  it('is nothing when everywhere nearby is already held', () => {
    const budget = createBudget<string>()
    budget.hold('next', 'N')
    expect(nextToWarm(budget, new Set(), ['next'], painted)).toBeNull()
  })

  it('skips a place nobody has painted, because there is nothing to fetch', () => {
    const budget = createBudget<string>()
    expect(nextToWarm(budget, new Set(), ['unpainted', 'next'], painted)).toBe('next')
  })

  it('is nothing at all when there is nowhere to go', () => {
    expect(nextToWarm(createBudget<string>(), new Set(), [], painted)).toBeNull()
  })
})
