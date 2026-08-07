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

const somewhere = { key: 'east-side', name: 'East Side', note: 'where you live' }

const panel = {
  biologicalAge: 32,
  livedToday: false,
  tiers: [{ key: 'wearable', name: 'A good watch', buys: 'HRV', price: 40000, owned: false, forSale: true }],
  metrics: [
    {
      key: 'hrv',
      name: 'Heart-rate variability',
      unit: 'ms',
      means: 'the autonomic balance sheet',
      tier: 'wearable',
      tierName: 'A good watch',
      lower: false,
      decimals: 0,
      visible: false,
      value: null,
      history: [],
    },
  ],
}

/**
 * The rules for the body, as the server states them (§3.4).
 *
 * A routine is a standing question and a sentence answering it, which is the
 * same shape the playbook composes in (§3.14) — the payload is the interface
 * language as much as the screen is.
 */
const rules = {
  set: false,
  routines: [
    {
      key: 'sleep',
      name: 'Sleep',
      asks: 'What happens when the night gets late?',
      learn: 'A stretch of short nights.',
      unlocked: true,
      chosen: 'whenever',
      options: [
        { key: 'whenever', says: 'I sleep when I run out of day.', costs: 'Nothing, until it does.', chosen: true },
        { key: 'protected', says: 'Eight and a half hours.', costs: 'Evenings.', chosen: false },
      ],
    },
  ],
}

const ahead = {
  days: 2,
  biologicalAge: [31.4, 31.1],
  metrics: [
    { key: 'hrv', values: [] },
    { key: 'sleep', values: [8.5, 8.5] },
  ],
}

const snapshot = {
  character: { money: 120000, at: somewhere, home: somewhere, earningsTaken: 0, earningsLeft: 3 },
  places: [somewhere],
  health: panel,
  routines: rules,
  forecast: ahead,
  probes: 7,
  database: '17.2',
  schema: 4,
}

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
  it('keeps the slices it did not change', () => {
    const world = hydrated(initialWorld, snapshot)

    const again = hydrated(world, { ...snapshot })
    expect(again.probes).toBe(world.probes)
    expect(again.server).toBe(world.server)
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

describe('the character', () => {
  const riverfront = { key: 'riverfront', name: 'Riverfront', note: 'where the money lives' }
  const hydratedWorld = hydrated(initialWorld, snapshot)

  it('is hydrated with somewhere to stand, somewhere to live, and money', () => {
    expect(hydratedWorld.character).toEqual(snapshot.character)
    expect(hydratedWorld.places).toEqual(snapshot.places)
  })

  it('walks when another tab says it walked', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 2,
      payload: { at: riverfront },
    })

    expect(world.character?.at).toEqual(riverfront)
    // Only where they are. A message about walking is not a message about money.
    expect(world.character?.money).toBe(snapshot.character.money)
  })

  // Every number the message carries was stated by the server, including how
  // many paid actions are left — the client counts nothing (§10.2, rule 2).
  it('takes both halves of the cap from the message rather than counting', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.earned',
      seq: 3,
      payload: { money: 124500, earned: 4500, earningsTaken: 1, earningsLeft: 2 },
    })

    expect(world.character?.money).toBe(124500)
    expect(world.character?.earningsTaken).toBe(1)
    expect(world.character?.earningsLeft).toBe(2)
  })

  // A message about a character this client has not been told about yet can
  // only be applied to nothing. It is dropped rather than inventing one.
  it('ignores a change that arrives before there is anybody to change', () => {
    const world = applyMessage(initialWorld, {
      type: 'character.moved',
      seq: 1,
      payload: { at: riverfront },
    })

    expect(world).toBe(initialWorld)
  })
})

/*
 * The rules for the body (§3.4), which arrive by both routes: hydrated with the
 * world at boot, and pushed when another tab states them.
 */
describe('the routines', () => {
  it('are hydrated with the fortnight they lead to', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.routines?.set).toBe(false)
    expect(world.routines?.routines[0].asks).toBe('What happens when the night gets late?')
    expect(world.forecast?.days).toBe(2)
    expect(world.forecast?.biologicalAge).toEqual([31.4, 31.1])
  })

  // A projection is a reading of a body, so it obeys the panel's rule: what is
  // withheld is the reading, not the idea that the measurement exists (§3.16).
  it('carry no projected values for a metric with no instrument behind it', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.forecast?.metrics.find((m) => m.key === 'hrv')?.values).toEqual([])
    expect(world.forecast?.metrics.find((m) => m.key === 'sleep')?.values).toEqual([8.5, 8.5])
  })

  it('are restated whole when another tab sets them', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, {
      type: 'routine.set',
      seq: 4,
      payload: {
        routines: {
          set: true,
          routines: [{ ...rules.routines[0], chosen: 'protected', options: null }],
        },
        forecast: { days: 2, biologicalAge: [30.9, 30.2], metrics: null },
      },
    })

    expect(world.routines?.set).toBe(true)
    expect(world.routines?.routines[0].chosen).toBe('protected')
    // An empty Go slice is null on the wire, and this is the one place that is
    // turned back into a list — nothing downstream should have to think about it.
    expect(world.routines?.routines[0].options).toEqual([])
    expect(world.forecast?.metrics).toEqual([])
    expect(world.forecast?.biologicalAge).toEqual([30.9, 30.2])

    // The rules changing says nothing about the body. The panel moves when a
    // day is lived, which is a different message.
    expect(world.health).toBe(before.health)
  })
})

/*
 * The daily rollover (§3.3): the world moving with nobody asking.
 *
 * It is the only message no command of this client's caused, which makes it the
 * clearest case for the socket existing at all — a tab left open overnight
 * would otherwise still be showing yesterday's body.
 */
describe('the world rolling over', () => {
  const rolled = (crossed: { metric: string; from: string; to: string; worse: boolean }[] | null) => ({
    type: 'rollover.ran' as const,
    seq: 9,
    payload: {
      panel: { ...panel, biologicalAge: 33.6, livedToday: false },
      forecast: { days: 2, biologicalAge: [33.4, 33.2], metrics: null },
      crossed,
      days: 4,
      skipped: 0,
    },
  })

  it('restates the panel and the fortnight it now leads to', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, rolled(null))

    expect(world.health?.biologicalAge).toBe(33.6)
    expect(world.forecast?.biologicalAge).toEqual([33.4, 33.2])
    // A rollover applies the rules; it does not change them.
    expect(world.routines).toBe(before.routines)
  })

  it('says what the last day applied crossed', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(
      before,
      rolled([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }]),
    )

    expect(world.crossings).toEqual([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }])
  })

  it('leaves the last crossings alone when a quiet day crossed nothing', () => {
    const before = applyMessage(
      hydrated(initialWorld, snapshot),
      rolled([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }]),
    )
    const world = applyMessage(before, rolled(null))

    expect(world.crossings).toBe(before.crossings)
  })
})
