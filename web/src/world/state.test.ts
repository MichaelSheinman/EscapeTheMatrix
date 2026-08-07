import { describe, expect, it } from 'vitest'

import {
  applyMessage,
  hydrated,
  initialWorld,
  probed,
  REMEMBERED,
  type World,
} from './state'

/*
 * The reducers, which are the part of the client worth testing on their own.
 *
 * They are pure by construction (§10.2) — no store, no fetch, no DOM — so what
 * is asserted below is the whole behaviour: what a statement from the server
 * does to the model, and what it deliberately leaves alone.
 *
 * The exhaustiveness of the reducer table is not tested here because it cannot
 * fail at runtime: a message type the table has no entry for is a compile
 * error, which is the point of writing it as a mapped type.
 */

const snapshot = { probes: 7, database: '17.2', schema: 4 }

function recorded(probes: number, label = '', at = '2026-08-06T12:00:00Z') {
  return {
    type: 'probe.recorded' as const,
    seq: 1,
    payload: { probes, label, notedAt: at },
  }
}

function result(probes: number, label = '', at = '2026-08-06T12:00:00Z') {
  return { probes, label, notedAt: at, database: '17.2', schema: 4 }
}

describe('hydrating over REST', () => {
  it('takes the count and the server it is talking to', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.probes.count).toBe(7)
    expect(world.server).toEqual({ database: '17.2', schema: 4 })
  })

  it('leaves marks alone — they are what this client was told, not what is', () => {
    const told = probed(initialWorld, result(1, 'first'))

    const world = hydrated(told, snapshot)

    expect(world.probes.count).toBe(7)
    expect(world.probes.marks).toBe(told.probes.marks)
  })

  // The renderer reads the store every frame and skips work by comparing
  // references. A reducer that cloned unconditionally would rebuild the scene
  // sixty times a second for nothing.
  it('returns the same object when nothing changed', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(hydrated(world, snapshot)).toBe(world)
    expect(hydrated(world, { ...snapshot }).probes).toBe(world.probes)
  })
})

describe('a probe this tab sent', () => {
  it('takes the count from the response and remembers who did it', () => {
    const world = probed(initialWorld, result(3, 'mine'))

    expect(world.probes.count).toBe(3)
    expect(world.probes.marks).toHaveLength(1)
    expect(world.probes.marks[0]).toMatchObject({ n: 1, label: 'mine', by: 'you' })
  })
})

describe('a probe the socket brought', () => {
  it('is recorded as having happened somewhere else', () => {
    const world = applyMessage(initialWorld, recorded(2, 'theirs'))

    expect(world.probes.count).toBe(2)
    expect(world.probes.marks[0]).toMatchObject({ n: 1, label: 'theirs', by: 'elsewhere' })
  })

  // §8, rule 3 in the model: a command's response and a socket message are two
  // ways of being told, and both land in one place.
  it('numbers marks in the order this client learned of them', () => {
    let world: World = probed(initialWorld, result(1, 'mine'))
    world = applyMessage(world, recorded(2, 'theirs'))
    world = probed(world, result(3, 'mine again'))

    expect(world.probes.marks.map((m) => [m.n, m.by])).toEqual([
      [1, 'you'],
      [2, 'elsewhere'],
      [3, 'you'],
    ])
  })

  // The count is the whole table, so it can move by more than the marks do —
  // other players probe too, and this client is not told about theirs.
  it('takes whatever count the server stated, jump or not', () => {
    const world = applyMessage(probed(initialWorld, result(1)), recorded(40))

    expect(world.probes.count).toBe(40)
    expect(world.probes.marks).toHaveLength(2)
  })
})

describe('the bounded memory of marks', () => {
  it('keeps the most recent and drops the oldest', () => {
    let world = initialWorld
    for (let i = 1; i <= REMEMBERED + 5; i++) world = probed(world, result(i, `probe-${i}`))

    const marks = world.probes.marks
    expect(marks).toHaveLength(REMEMBERED)
    // Numbering counts every probe this client was told about, so the ones that
    // survived keep the numbers they were given — the renderer places marks by
    // that number, and a renumbering would move every mark on screen.
    expect(marks[0].n).toBe(6)
    expect(marks[marks.length - 1].n).toBe(REMEMBERED + 5)
    expect(world.probes.seen).toBe(REMEMBERED + 5)
  })
})
